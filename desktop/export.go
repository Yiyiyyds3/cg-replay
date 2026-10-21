// GUI 导出（Phase 2）：把当前事件按固定帧率重演成 GIF / MP4。
//
// 与回放页共用同一套内核（player + replay.GameLoader + win32 GDI 文字），
// 但用独立 Player 实例与固定节奏，保证输出可复现：
//   - 文本点击点固定停留 exportTextWaitMs（CharMs=0，Min=Max），不依赖真人阅读速度；
//   - 逐帧按 step 推进，Done 后再多录 exportTailMs 收尾转场；
//   - 缺素材不中止（1×1 占位），但清单写 sidecar 并在 Note 里交代（FR-12）。
//
// 红线（见 docs/06、UI 文稿 §红线）：
//   - 只写 exe 同目录 exports/ 派生物（DR-30）；
//   - 不改 internal/player / internal/replay 语义（逐帧回归 diff=0 口径走 CLI）；
//   - GIF 复用 replay.WriteGIF（8 位调色板，仅预览）；MP4 走本机 ffmpeg，
//     没有 ffmpeg 时 auto 降级 GIF 并在结果里说明，format=mp4 则显式报错。
package main

import (
	"bytes"
	"fmt"
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"ucgr/internal/ir"
	"ucgr/internal/platform/win32"
	"ucgr/internal/player"
	"ucgr/internal/replay"
)

const (
	exportTextWaitMs = 1200 // 每个文本点击点的固定停留（确定性重演的节奏）
	exportTailMs     = 400  // 台本走完后再录一段，等末段转场走完
)

// ExportResult 导出结果（前端 toast 展示；路径一定可见，不静默）。
type ExportResult struct {
	Path       string   `json:"path"`
	Format     string   `json:"format"` // gif | mp4
	Frames     int      `json:"frames"`
	FPS        int      `json:"fps"`
	Width      int      `json:"width"`
	Height     int      `json:"height"`
	DurationMs int64    `json:"duration_ms"`
	Missing    []string `json:"missing"`
	Note       string   `json:"note"`
}

// ExportProgress 导出进度事件（前端监听 export:progress）。
type ExportProgress struct {
	Frame   int    `json:"frame"`
	Total   int    `json:"total"`
	Percent int    `json:"percent"`
	Format  string `json:"format"`
}

// exportPlan 输出规格。上限是显式约束（超出会截断并写进 Note，不静默）。
type exportPlan struct {
	format    string
	fps       int
	width     int
	height    int
	step      int64 // 每帧推进的演出毫秒
	capFrames int
}

// ExportEvent 把当前会话的事件导出成 GIF 或 MP4。
//
// format：gif | mp4 | auto（空等同 auto）。auto 有 ffmpeg 走 MP4，否则 GIF。
// 返回落盘路径与摘要；同一时刻只允许一个导出任务（TryLock 直接拒绝）。
func (a *App) ExportEvent(format string) (ExportResult, error) {
	if !a.exportMu.TryLock() {
		return ExportResult{}, fmt.Errorf("已有导出任务在进行，请等它结束")
	}
	defer a.exportMu.Unlock()

	a.sessMu.Lock()
	s := a.sess
	a.sessMu.Unlock()
	if s == nil {
		return ExportResult{}, fmt.Errorf("没有活动会话，请先打开一个事件")
	}
	s.mu.Lock()
	ev, root, kind, id := s.ev, s.root, s.kind, s.id
	s.mu.Unlock()
	if ev == nil || len(ev.Show) == 0 {
		return ExportResult{}, fmt.Errorf("当前事件没有可导出的台本")
	}

	want := strings.ToLower(strings.TrimSpace(format))
	if want == "" {
		want = "auto"
	}
	ffmpeg := findFFmpeg()
	plan := exportPlan{format: "gif", fps: 10, width: 480, height: 360, capFrames: 150}
	switch want {
	case "gif":
		// 默认规格：10fps / 480×360 / 15s 上限（8 位调色板，体积可控）。
	case "mp4":
		if ffmpeg == "" {
			return ExportResult{}, fmt.Errorf("未检测到 ffmpeg，无法导出 MP4；可改用 GIF，或把 ffmpeg 加入 PATH")
		}
		plan = exportPlan{format: "mp4", fps: 25, width: 960, height: 720, capFrames: 1500}
	case "auto":
		if ffmpeg != "" {
			plan = exportPlan{format: "mp4", fps: 25, width: 960, height: 720, capFrames: 1500}
		}
	default:
		return ExportResult{}, fmt.Errorf("未知导出格式 %q（支持 gif / mp4 / auto）", format)
	}
	plan.step = int64(1000 / plan.fps)

	opt := player.DefaultOptions()
	opt.FontSize = 48
	opt.Speed = 1
	opt.SkipWaits = false // 跳过等待会把所有点击点一起跳掉，导出会瞬间演完
	opt.CharMs = 0
	opt.BaseMs = exportTextWaitMs
	opt.MinWaitMs = exportTextWaitMs
	opt.MaxWaitMs = exportTextWaitMs

	total, fullMs, truncated := planExport(ev, opt, plan.step, plan.capFrames)
	if total <= 0 {
		return ExportResult{}, fmt.Errorf("台本为空，没有可导出的帧")
	}

	dir := filepath.Join(exeDir(), "exports", safeName(kind)+"_"+safeName(id))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ExportResult{}, err
	}
	outPath := filepath.Join(dir, "export_"+time.Now().Format("20060102-150405.000")+"."+plan.format)

	face := "SimSun"
	if n, ok := win32.RegisterFontFile(filepath.Join(root, "raw", "data", "SourceHanSerifSC-Regular.otf")); ok {
		face = n
	}
	td := win32.NewTextDrawer(win32.DefaultTextStyle(face, opt.FontSize))
	defer td.Free()
	cv, err := win32.NewCanvas(stageW, stageH)
	if err != nil {
		return ExportResult{}, fmt.Errorf("创建导出画布失败：%w", err)
	}
	defer cv.Free()

	miss := newExportMissing()
	load := miss.wrap(player.Cached(player.Loader(replay.GameLoader(root))))

	// MP4 边渲染边写 stdin（不攒帧，内存 O(1)）；GIF 需要整段帧表。
	var (
		gifFrames []*image.RGBA
		sink      *mp4Sink
	)
	if plan.format == "mp4" {
		sink, err = newMP4Sink(ffmpeg, outPath, plan.width, plan.height, plan.fps)
		if err != nil {
			return ExportResult{}, err
		}
	}
	cleanup := func() {
		if sink != nil {
			sink.abort()
		}
		if plan.format == "gif" {
			os.Remove(outPath)
		}
	}

	p := player.New(ev, opt)
	p.Advance(0, player.Input{})
	emitEvery := total / 50
	if emitEvery < 1 {
		emitEvery = 1
	}
	for i := 0; i < total; i++ {
		rgba, _, rerr := player.RenderFrame(p, load, stageW, stageH)
		if rerr != nil {
			cleanup()
			return ExportResult{}, fmt.Errorf("第 %d 帧合成失败：%w", i, rerr)
		}
		cv.BlitRGBA(rgba)
		td.DrawBoxes(cv.DC(), p.Frame().Texts, 1) // 导出必须有字（截图不含文字是前端叠的）
		cv.ForceOpaque()
		frame := resizeBilinear(cv.Image(), plan.width, plan.height)
		if plan.format == "gif" {
			gifFrames = append(gifFrames, frame)
		} else if werr := sink.write(frame); werr != nil {
			cleanup()
			return ExportResult{}, fmt.Errorf("写入第 %d 帧失败：%w", i, werr)
		}
		if i%emitEvery == 0 || i == total-1 {
			a.emitExportProgress(ExportProgress{
				Frame: i + 1, Total: total, Percent: (i + 1) * 100 / total, Format: plan.format,
			})
		}
		p.Advance(plan.step, player.Input{})
	}

	if plan.format == "gif" {
		if err := replay.WriteGIF(gifFrames, outPath, 100/plan.fps); err != nil {
			cleanup()
			return ExportResult{}, fmt.Errorf("写 GIF 失败：%w", err)
		}
	} else if err := sink.close(); err != nil {
		os.Remove(outPath)
		return ExportResult{}, err
	}

	missing := miss.list()
	if missing == nil {
		missing = []string{} // JSON 里给 []，别让前端拿到 null
	}
	var notes []string
	if plan.format == "gif" {
		notes = append(notes, "GIF 为 8 位调色板预览（Floyd–Steinberg），像素级效果以 MP4 为准")
	}
	if truncated {
		notes = append(notes, fmt.Sprintf("已按上限截断到 %d 帧（完整时长约 %.1fs）；当前上限 %s",
			total, float64(fullMs)/1000, humanSec(int64(total)*plan.step)))
	}
	if want == "auto" && plan.format == "gif" && ffmpeg == "" {
		notes = append(notes, "本机未检测到 ffmpeg，auto 已降级为 GIF")
	}
	if len(missing) > 0 {
		side := outPath + ".missing.txt"
		if werr := writeMissing(side, miss); werr != nil {
			return ExportResult{}, fmt.Errorf("写缺素材清单失败：%w", werr)
		}
		notes = append(notes, fmt.Sprintf("缺素材 %d 个，已用 1×1 透明占位继续合成；清单：%s", len(missing), side))
	}

	return ExportResult{
		Path:       outPath,
		Format:     plan.format,
		Frames:     total,
		FPS:        plan.fps,
		Width:      plan.width,
		Height:     plan.height,
		DurationMs: int64(total-1) * plan.step,
		Missing:    missing,
		Note:       strings.Join(notes, "；"),
	}, nil
}

// planExport 用独立 Player 按固定节奏走一遍，数出帧数（不渲染，代价很小）。
//
// 返回 total=实际导出帧数、fullMs=完整演出毫秒、truncated=是否被上限截断。
// 截断时继续快进到 Done，把「完整时长」也算出来写进 Note（不静默）。
func planExport(ev *ir.Event, opt player.Options, step int64, capFrames int) (total int, fullMs int64, truncated bool) {
	p := player.New(ev, opt)
	p.Advance(0, player.Input{})
	tail := int64(0)
	tailNeed := int64(exportTailMs)/step + 1
	done := false
	for {
		if total >= capFrames {
			truncated = !done
			break
		}
		total++
		if p.Done() && p.Frame().TransProgress == 0 {
			done = true
			tail++
			if tail > tailNeed {
				break
			}
		} else {
			done = false
			tail = 0
		}
		p.Advance(step, player.Input{})
	}
	if truncated {
		for i := 0; i < 1_000_000; i++ {
			if p.Done() && p.Frame().TransProgress == 0 {
				break
			}
			before := p.Now()
			p.Advance(500, player.Input{})
			if p.Now() == before {
				break
			}
		}
	}
	fullMs = p.Now()
	return total, fullMs, truncated
}

// emitExportProgress 进度事件（ctx 可能尚未就绪：测试/非 Wails 场景直接跳过）。
func (a *App) emitExportProgress(p ExportProgress) {
	if a.ctx == nil {
		return
	}
	wruntime.EventsEmit(a.ctx, "export:progress", p)
}

// ---------------------------------------------------------------
// 缺素材记账
// ---------------------------------------------------------------

type exportMissing struct {
	errs  map[string]string
	order []string
}

func newExportMissing() *exportMissing {
	return &exportMissing{errs: map[string]string{}}
}

// wrap 与回放页同口径：缺素材回 1×1 透明占位，画面继续，清单另行交代。
func (m *exportMissing) wrap(base player.Loader) player.Loader {
	return func(ref string) (image.Image, error) {
		img, err := base(ref)
		if err == nil {
			return img, nil
		}
		if _, seen := m.errs[ref]; !seen {
			m.errs[ref] = err.Error()
			m.order = append(m.order, ref)
		}
		return image.NewRGBA(image.Rect(0, 0, 1, 1)), nil
	}
}

func (m *exportMissing) refs() []string {
	out := append([]string(nil), m.order...)
	sort.Strings(out)
	return out
}

func (m *exportMissing) list() []string {
	if len(m.order) == 0 {
		return nil
	}
	return m.refs()
}

func writeMissing(path string, m *exportMissing) error {
	var b strings.Builder
	refs := m.refs()
	b.WriteString("# 导出时缺素材（1×1 透明占位继续合成；补素材后重扫/重导）\n")
	for _, ref := range refs {
		b.WriteString(ref)
		b.WriteString("\t")
		b.WriteString(m.errs[ref])
		b.WriteString("\n")
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// ---------------------------------------------------------------
// MP4：ffmpeg 流式编码（rawvideo rgba → libx264 yuv420p，无音轨）
// ---------------------------------------------------------------

type mp4Sink struct {
	cmd *exec.Cmd
	in  interface {
		Write([]byte) (int, error)
		Close() error
	}
	stderr bytes.Buffer
	path   string
}

func newMP4Sink(ffmpeg, path string, w, h, fps int) (*mp4Sink, error) {
	args := []string{
		"-y", "-loglevel", "error",
		"-f", "rawvideo", "-pixel_format", "rgba",
		"-video_size", fmt.Sprintf("%dx%d", w, h),
		"-framerate", strconv.Itoa(fps),
		"-i", "pipe:0",
		"-an",
		"-c:v", "libx264", "-preset", "veryfast", "-crf", "20",
		"-pix_fmt", "yuv420p", "-movflags", "+faststart",
		path,
	}
	cmd := exec.Command(ffmpeg, args...)
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	s := &mp4Sink{cmd: cmd, in: in, path: path}
	cmd.Stderr = &s.stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("启动 ffmpeg 失败：%w", err)
	}
	return s, nil
}

func (s *mp4Sink) write(rgba *image.RGBA) error {
	b := rgba.Bounds()
	if rgba.Stride != b.Dx()*4 {
		// 逐行写，避免把行尾 padding 当成像素喂给 ffmpeg。
		row := make([]byte, b.Dx()*4)
		for y := b.Min.Y; y < b.Max.Y; y++ {
			off := rgba.PixOffset(b.Min.X, y)
			copy(row, rgba.Pix[off:off+len(row)])
			if _, err := s.in.Write(row); err != nil {
				return err
			}
		}
		return nil
	}
	_, err := s.in.Write(rgba.Pix)
	return err
}

func (s *mp4Sink) close() error {
	if err := s.in.Close(); err != nil {
		_ = s.cmd.Wait()
		return err
	}
	if err := s.cmd.Wait(); err != nil {
		msg := strings.TrimSpace(s.stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("ffmpeg 编码失败：%s", msg)
	}
	return nil
}

func (s *mp4Sink) abort() {
	_ = s.in.Close()
	_ = s.cmd.Wait()
	os.Remove(s.path)
}

// findFFmpeg 先查 PATH，再查 WinGet Links（本机实测装在这里）。
func findFFmpeg() string {
	if p, err := exec.LookPath("ffmpeg"); err == nil {
		return p
	}
	var cands []string
	if la := os.Getenv("LOCALAPPDATA"); la != "" {
		cands = append(cands, filepath.Join(la, "Microsoft", "WinGet", "Links", "ffmpeg.exe"))
	}
	cands = append(cands, `C:\ffmpeg\bin\ffmpeg.exe`)
	for _, c := range cands {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c
		}
	}
	return ""
}

// ---------------------------------------------------------------
// 缩放（双线性；go.mod 零外部依赖，不引入 x/image）
// ---------------------------------------------------------------

type axisWeight struct {
	i0, i1 int
	f      float64
}

func makeAxis(src, dst int) []axisWeight {
	out := make([]axisWeight, dst)
	if dst <= 1 {
		for i := range out {
			out[i] = axisWeight{0, 0, 0}
		}
		return out
	}
	for i := 0; i < dst; i++ {
		fx := (float64(i)+0.5)*float64(src)/float64(dst) - 0.5
		if fx < 0 {
			fx = 0
		}
		i0 := int(fx)
		if i0 >= src-1 {
			i0 = src - 1
			out[i] = axisWeight{i0, i0, 0}
			continue
		}
		out[i] = axisWeight{i0, i0 + 1, fx - float64(i0)}
	}
	return out
}

// resizeBilinear 把 src 缩放到 w×h（RGBA，alpha 直接置 255——导出帧恒不透明）。
func resizeBilinear(src *image.RGBA, w, h int) *image.RGBA {
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	if sw == w && sh == h {
		copy(dst.Pix, src.Pix)
		return dst
	}
	xs := makeAxis(sw, w)
	ys := makeAxis(sh, h)
	for y := 0; y < h; y++ {
		ay := ys[y]
		for x := 0; x < w; x++ {
			ax := xs[x]
			p00 := src.PixOffset(b.Min.X+ax.i0, b.Min.Y+ay.i0)
			p10 := src.PixOffset(b.Min.X+ax.i1, b.Min.Y+ay.i0)
			p01 := src.PixOffset(b.Min.X+ax.i0, b.Min.Y+ay.i1)
			p11 := src.PixOffset(b.Min.X+ax.i1, b.Min.Y+ay.i1)
			w00 := (1 - ax.f) * (1 - ay.f)
			w10 := ax.f * (1 - ay.f)
			w01 := (1 - ax.f) * ay.f
			w11 := ax.f * ay.f
			d := dst.PixOffset(x, y)
			for c := 0; c < 3; c++ {
				v := w00*float64(src.Pix[p00+c]) + w10*float64(src.Pix[p10+c]) +
					w01*float64(src.Pix[p01+c]) + w11*float64(src.Pix[p11+c])
				dst.Pix[d+c] = uint8(v + 0.5)
			}
			dst.Pix[d+3] = 255
		}
	}
	return dst
}

func humanSec(ms int64) string {
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}
	return fmt.Sprintf("%.1fs", float64(ms)/1000)
}
