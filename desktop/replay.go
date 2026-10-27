// 回放页会话层（方案 B M3）：事件卡 → IR 台本 → player.Player → JPEG 帧流。
//
// 语义对齐 cmd/cgreplay：IR 构建入口（BuildShowSegment / BuildShowChapterFile）、
// 播放策略（DefaultOptions + FontSize=48）、loader（Cached + GameLoader）、
// 自动推进节奏（停留 > autoWaitMs 注入一次 Input.Advance）。
//
// 两个壳层差异（不碰内核，diff=0 红线不受影响——逐帧回归口径走 CLI 路径）：
//   - 帧给前端是 JPEG dataURL（舞台底为黑、恒不透明，alpha 对本用途无意义）；
//   - 缺素材不中止整帧：loader 记账后回 1×1 透明占位，画面继续叠，
//     缺清单常驻红横幅交代（FR-12 不静默；CLI 语义仍是遇缺中止整帧）。
package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"ucgr/internal/adapter/artemis"
	"ucgr/internal/adapter/bgi"
	"ucgr/internal/adapter/catsystem2"
	"ucgr/internal/adapter/kirikiri"
	"ucgr/internal/audio"
	"ucgr/internal/catalog"
	"ucgr/internal/ir"
	"ucgr/internal/library"
	"ucgr/internal/player"
	"ucgr/internal/replay"
	"ucgr/internal/video"
)

const (
	stageW      = 1600
	stageH      = 1200
	autoWaitMs  = 2500 // 与 cmd/cgreplay 一致
	jpegQuality = 88
	encMinGap   = 50 * time.Millisecond // 播放中两次编码的最小间隔（≈20fps 封顶）
	videoFPS    = 20                    // 影片流的目标帧率（与渲染上限一致）

	durSnapshots = 240 // 时长估计取样点数
)

// replaySession 当前回放会话（同一时刻最多一个）。
type replaySession struct {
	mu    sync.Mutex
	root  string
	kind  string // cg / scene / chapter
	id    string
	title string

	ev   *ir.Event
	opts player.Options
	p    *player.Player

	load   player.Loader // 缺素材记账版 loader
	vs     *video.Stream // 当前影片流（惰性打开）
	vsPath string        // 流对应的文件
	audio  *audio.System

	playing  bool
	auto     bool
	muted    bool
	input    player.Input
	lastTick time.Time
	waited   bool // 正停在等待点（自动推进计时用）
	waitedAt time.Time

	missing map[string]string // ref → 最近一次加载错误

	lastSig  string // 上次编码的画面签名；不变就直接复用
	lastJPEG string
	lastEnc  time.Time // 上次编码时刻（播放中节流，防 16ms 轮询饿死锁）

	// snap 最近一张完整帧快照。s.mu 太忙时 TryLock 不住就直接回它，
	// 避免轮询请求线性堆积把 mutex 排成分钟级队列（2026-10-21 实证：
	// waiter 累到 400+、ToggleAuto 排队 60s+）。
	snapMu   sync.RWMutex
	snapLast FrameDTO
	snapOK   bool

	durMs int64
}

// SessionMeta 回放会话元信息（OpenEvent 返回）。
type SessionMeta struct {
	Kind       string `json:"kind"`
	ID         string `json:"id"`
	Title      string `json:"title"`
	GameID     string `json:"game_id"` // 收藏/跨游戏定位用
	StageW     int    `json:"stage_w"`
	StageH     int    `json:"stage_h"`
	OpCount    int    `json:"op_count"`
	TextCount  int    `json:"text_count"`
	DurationMs int64  `json:"duration_ms"`
}

// FrameDTO 一帧的前端所需全集。
type FrameDTO struct {
	JPEG    string     `json:"jpeg"`
	Now     int64      `json:"now"`
	DurMs   int64      `json:"dur_ms"`
	Playing bool       `json:"playing"`
	Done    bool       `json:"done"`
	Waiting bool       `json:"waiting"`
	Auto    bool       `json:"auto"`
	Muted   bool       `json:"muted"`
	QuakeX  int        `json:"quake_x"`
	QuakeY  int        `json:"quake_y"`
	Missing []string   `json:"missing"`
	Layers  []LayerDTO `json:"layers"`
	Texts   []BoxDTO   `json:"texts"`
	Cached  bool       `json:"cached"`
}

// LayerDTO 调试抽屉 D 的图层一览。
type LayerDTO struct {
	Layer   string  `json:"layer"`
	Ref     string  `json:"ref"`
	Visible bool    `json:"visible"`
	X       float64 `json:"x"`
	Y       float64 `json:"y"`
	Opacity float64 `json:"opacity"`
	Z       int     `json:"z"`
	Missing bool    `json:"missing"`
}

// rgbaDTO FrameColor 的 JSON 友好形态。
type rgbaDTO struct {
	R, G, B uint8
}

// BoxDTO 前端文本覆盖层（几何由后端给，前端照抄定位；框图缺席时可用底色兜底）。
type BoxDTO struct {
	Layer        string    `json:"layer"`
	X            float64   `json:"x"`
	Y            float64   `json:"y"`
	W            float64   `json:"w"`
	H            float64   `json:"h"`
	HasGeom      bool      `json:"has_geom"`
	Visible      bool      `json:"visible"`
	FrameColor   rgbaDTO   `json:"frame_color"`
	FrameOpacity uint8     `json:"frame_opacity"`
	FontSize     float64   `json:"font_size"`
	LineSpacing  float64   `json:"line_spacing"`
	Lines        []LineDTO `json:"lines"`
}

type LineDTO struct {
	Speaker string `json:"speaker"`
	Text    string `json:"text"`
}

// stageLoader 缺素材记账：错误收进 s.missing，回 1×1 透明让合成继续。
// FR-12 的壳层口径：缺素材横幅常驻不静默，但不至于黑掉一整帧。
func (s *replaySession) stageLoader(base player.Loader) player.Loader {
	return func(ref string) (image.Image, error) {
		img, err := base(ref)
		if err == nil {
			return img, nil
		}
		if _, seen := s.missing[ref]; !seen {
			s.missing[ref] = err.Error()
		}
		return image.NewRGBA(image.Rect(0, 0, 1, 1)), nil
	}
}

// buildArtemisShow 构建 Artemis 的 block 演出（事件 Label = block 序号）。
func buildArtemisShow(scriptPath, block string) (*ir.Event, error) {
	idx, err := strconv.Atoi(strings.TrimSpace(block))
	if err != nil {
		return nil, fmt.Errorf("Artemis 事件缺 block 序号（%q）", block)
	}
	data, err := os.ReadFile(scriptPath)
	if err != nil {
		return nil, err
	}
	return artemis.BuildShowBlock(data, filepath.Base(scriptPath), idx)
}

// buildBGIShow 构建 BGI 的第 n 个事件演出（Label = 事件序号）。
func buildBGIShow(scriptPath, ordinal string) (*ir.Event, error) {
	n, err := strconv.Atoi(strings.TrimSpace(ordinal))
	if err != nil {
		return nil, fmt.Errorf("BGI 事件缺序号（%q）", ordinal)
	}
	return bgi.BuildShowEventFile(scriptPath, n)
}

// buildCS2Show 构建 CatSystem2 的 .cst 演出（一份脚本一个事件）。
func buildCS2Show(scriptPath string) (*ir.Event, error) {
	data, err := os.ReadFile(scriptPath)
	if err != nil {
		return nil, err
	}
	return catsystem2.BuildShowScript(data, filepath.Base(scriptPath))
}

// confAuto 读 conf 里的 auto（与旧 CLI 同字段）。
func (a *App) confAuto() bool {
	c, ok := a.loadConf()
	return ok && c.Auto
}

// OpenEvent 打开事件进入回放（先关旧会话；IR 构建与 CLI play() 同口径）。
func (a *App) OpenEvent(kind, id string) (SessionMeta, error) {
	a.sessMu.Lock()
	defer a.sessMu.Unlock()
	a.closeSessionLocked()

	c, _ := a.loadConf()
	reg, g, err := a.activeGame(c)
	if err != nil {
		return SessionMeta{}, err
	}
	root := g.Root

	lib, err := a.loadOrScanLib(g)
	if err != nil {
		return SessionMeta{}, err
	}

	var script, label, title string
	chapterPos := -1
	var chEvent *library.ChapterEvent
	switch kind {
	case "cg":
		e := lib.FindCG(id)
		if e == nil {
			return SessionMeta{}, fmt.Errorf("找不到 CG 事件 %s", id)
		}
		script, label, title = e.Script, e.Label, e.Title
	case "chapter":
		e := lib.FindChapter(id)
		if e == nil {
			return SessionMeta{}, fmt.Errorf("找不到演出段落 %s", id)
		}
		script, chapterPos, title = e.Script, e.LabelPos, e.Title
		chEvent = e
	case "scene":
		e := lib.FindScene(id)
		if e == nil {
			return SessionMeta{}, fmt.Errorf("找不到场景 %s", id)
		}
		script, title = e.Script, e.Title
	default:
		return SessionMeta{}, fmt.Errorf("未知事件类型 %s", kind)
	}
	if script == "" {
		return SessionMeta{}, fmt.Errorf("%s 没有脚本信息", title)
	}
	sp := filepath.Join(root, filepath.FromSlash(script))
	var ev *ir.Event
	switch {
	case chapterPos >= 0 && chEvent != nil:
		// 走 library 入口：章跨文件时自动补演下一场景的章前段（KiriKiri 专有）。
		ev, err = library.BuildChapterEvent(root, chEvent)
	case strings.EqualFold(g.Engine, "artemis"):
		// Artemis：事件单位是剧本里的 block，Label 记 block 序号。
		ev, err = buildArtemisShow(sp, label)
	case strings.EqualFold(g.Engine, "bgi"):
		// BGI：事件由适配器按换背景/换事件切出，Label 记事件序号；
		// builder 已经产好 Show 台本，直接用。
		ev, err = buildBGIShow(sp, label)
	case strings.EqualFold(g.Engine, "catsystem2"):
		// CS2：一份 .cst 一个事件（无 Label），构建时把 Timeline 转成 Show。
		ev, err = buildCS2Show(sp)
	default:
		ev, err = kirikiri.BuildShowSegment(sp, label)
	}
	if err != nil {
		return SessionMeta{}, fmt.Errorf("构建台本失败：%w", err)
	}
	if len(ev.Show) == 0 {
		return SessionMeta{}, fmt.Errorf("%s 没有可回放的演出（台本 0 条）", title)
	}

	s := &replaySession{
		root:    root,
		kind:    kind,
		id:      id,
		title:   title,
		ev:      ev,
		opts:    player.DefaultOptions(),
		auto:    a.confAuto(),
		playing: true,
		missing: map[string]string{},
	}
	s.opts.FontSize = 48
	// 影片时长：台本里通常不写，靠 ffprobe 查；查不到就按「未知」处理。
	s.opts.VideoDur = func(ref string) int64 {
		p, err := replay.AssetPath(root, ref)
		if err != nil {
			return 0
		}
		return video.Duration(p)
	}
	s.load = s.stageLoader(player.Cached(player.Loader(replay.GameLoader(root))))
	s.p = player.New(ev, s.opts)
	if sys, aerr := audio.NewSystem(root); aerr == nil {
		s.audio = sys
		s.p.Audio = sys.Handle
	}
	s.p.Advance(0, player.Input{}) // 先把 t=0 的台本指令执行掉（同 play()）
	s.lastTick = time.Now()

	// 时长口径与 CLI 一致：Show 台本的脚本时钟最大值（TMs 只被
	// [wait]/[wt]/[trans]/fade 推动，作进度条参考刻度；阅读节奏由
	// 播放器自动推进策略补齐，不在脚本数据里）。
	for _, op := range ev.Show {
		if op.TMs > s.durMs {
			s.durMs = op.TMs
		}
	}
	a.sess = s

	// 最近播放时间：派生元数据，写失败不阻断回放。
	reg.Touch(g.ID)
	_ = reg.Save(a.registryPath())
	// 中心库 recent：同样只记元数据；失败只进 errors.log（FR-12 不静默但不阻断）。
	if rerr := a.withCatalog(func(c *catalog.Catalog) error {
		return c.NoteRecent(g.ID, kind, id)
	}); rerr != nil {
		a.logAuxError("catalog recent", rerr)
	}

	return SessionMeta{
		Kind:       kind,
		ID:         id,
		Title:      title,
		GameID:     g.ID,
		StageW:     stageW,
		StageH:     stageH,
		OpCount:    len(ev.Show),
		TextCount:  len(ev.Texts),
		DurationMs: s.durMs,
	}, nil
}

// closeSessionLocked 关掉当前会话（调方必须持有 a.sessMu）。
func (a *App) closeSessionLocked() {
	if a.sess == nil {
		return
	}
	s := a.sess
	a.sess = nil
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.audio != nil {
		s.audio.Close()
	}
	s.closeVideo()
	s.p = nil
}

// GetFrame 拉当前帧。锁空闲才推进+编码；锁忙（前帧还在编码）直接回
// 最近快照并标 Cached，绝不在 mutex 上排队（防轮询请求无上限积压）。
func (a *App) GetFrame() (FrameDTO, error) {
	if a.sess == nil {
		return FrameDTO{}, fmt.Errorf("没有活动会话")
	}
	s := a.sess
	if !s.mu.TryLock() {
		s.snapMu.RLock()
		defer s.snapMu.RUnlock()
		if !s.snapOK {
			return FrameDTO{}, fmt.Errorf("会话忙，首帧尚未就绪")
		}
		d := s.snapLast
		d.Cached = true
		return d, nil
	}
	defer s.mu.Unlock()
	if s.p == nil {
		return FrameDTO{}, fmt.Errorf("会话已关闭")
	}
	return s.frameLocked(), nil
}

// frameLocked 推进 + 编码（调方持有 s.mu）。
func (s *replaySession) frameLocked() FrameDTO {
	// ① 推进：墙钟差 + 手动输入 + 自动推进注入。
	now := time.Now()
	dt := now.Sub(s.lastTick).Milliseconds()
	if dt > 200 { // 后台/卡顿回来不猛跳
		dt = 200
	}
	s.lastTick = now
	autoIn := false
	if s.auto && s.waited && now.Sub(s.waitedAt) > autoWaitMs*time.Millisecond {
		autoIn = true
		s.waitedAt = now
	}
	if dt > 0 || autoIn || s.input.Advance {
		in := s.input
		s.input = player.Input{} // 手动输入是边沿触发，只吃一次
		if autoIn {
			in.Advance = true
		}
		if s.playing || in.Advance {
			s.p.Advance(dt, in)
		}
	}

	f := s.p.Frame()
	if s.waited != f.Waiting {
		s.waited = f.Waiting
		s.waitedAt = now
	}

	// ② 画面指纹：变化才重编码 JPEG（读字/静止时零开销）。
	// 编码在锁内做（RenderFrame 与 Advance 必须串行），但播放中按
	// encMinGap 节流；否则 16ms 轮询×1600×1200 重编码会长期持锁，
	// 其他绑定调用全部排队成假死（ stacks.log 实证）。暂停态遇到
	// 签名变化（seek/点击）立即编码，不受节流。
	sig := frameSig(f, len(s.missing))
	dto := FrameDTO{
		Now:     f.Now,
		DurMs:   s.durMs,
		Playing: s.playing,
		Done:    f.Done,
		Waiting: f.Waiting,
		Auto:    s.auto,
		Muted:   s.muted,
		QuakeX:  f.QuakeX,
		QuakeY:  f.QuakeY,
		Missing: missingSorted(s.missing),
		Layers:  layerDTOs(f, s.missing),
		Texts:   boxDTOs(f),
	}
	dto.Cached = sig == s.lastSig
	if dto.Cached && s.lastJPEG != "" {
		dto.JPEG = s.lastJPEG
		return dto
	}
	if s.playing && time.Since(s.lastEnc) < encMinGap {
		// 播放中节流窗口未到：回上一张（前端 60ms 后自然拿到新的）。
		dto.JPEG = s.lastJPEG
		dto.Cached = true
		return dto
	}
	s.lastSig = sig
	s.lastEnc = time.Now()
	j, err := renderJPEG(s.p, s.load, s.videoProvider(), f.QuakeX, f.QuakeY)
	if err != nil {
		dto.JPEG = s.lastJPEG // 理论上到不了（stageLoader 已吞素材错）
		return dto
	}
	dto.JPEG = j
	s.lastJPEG = j
	// 发快照（TryLock 失败的调用方从这里拿最近帧）。
	s.snapMu.Lock()
	s.snapLast = dto
	s.snapOK = true
	s.snapMu.Unlock()
	return dto
}

// frameSig 便宜的画面指纹（不比像素）。
func frameSig(f player.Frame, missingCount int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "t%d|q%d,%d|d%v|w%v|l%d|m%d",
		f.Now, f.QuakeX, f.QuakeY, f.Done, f.Waiting, len(f.Layers), missingCount)
	for _, t := range f.Texts {
		if !t.Visible {
			continue
		}
		fmt.Fprintf(&b, "|%s", t.Layer)
		if n := len(t.Lines); n > 0 {
			fmt.Fprintf(&b, "/%s:%s", t.Lines[n-1].Speaker, t.Lines[n-1].Text)
		}
	}
	return b.String()
}

func missingSorted(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func layerDTOs(f player.Frame, missing map[string]string) []LayerDTO {
	out := make([]LayerDTO, 0, len(f.Layers))
	for _, l := range f.Layers {
		_, miss1 := missing[l.Ref]
		_, miss2 := missing[l.Resolved]
		out = append(out, LayerDTO{
			Layer: l.Layer, Ref: l.Ref, Visible: l.Visible,
			X: l.X, Y: l.Y, Opacity: l.Opacity, Z: l.Z,
			Missing: miss1 || miss2,
		})
	}
	return out
}

func boxDTOs(f player.Frame) []BoxDTO {
	out := make([]BoxDTO, 0, len(f.Texts))
	for _, t := range f.Texts {
		if !t.Visible {
			continue
		}
		b := BoxDTO{
			Layer:        t.Layer,
			X:            t.X,
			Y:            t.Y,
			W:            t.W,
			H:            t.H,
			HasGeom:      t.HasGeom,
			Visible:      t.Visible,
			FrameColor:   rgbaDTO{t.FrameColor[0], t.FrameColor[1], t.FrameColor[2]},
			FrameOpacity: t.FrameOpacity,
			FontSize:     t.FontSize,
			LineSpacing:  t.LineSpacing,
		}
		for _, ln := range t.Lines {
			b.Lines = append(b.Lines, LineDTO{Speaker: ln.Speaker, Text: ln.Text})
		}
		out = append(out, b)
	}
	return out
}

// videoProvider 影片取帧：引用 → 解包目录里的真实文件 → ffmpeg 流式解码。
//
// 回放时间单调向前，所以开一条进程连续出帧（按时间丢帧），比每帧 seek 省得多；
// 换片或倒退时 Stream 自己会重开。没有 ffmpeg 时返回错误，渲染层记进缺素材、
// 不中断播放。
func (s *replaySession) videoProvider() player.VideoProvider {
	root := s.root
	return func(ref string, tMs int64) (image.Image, error) {
		path, err := replay.AssetPath(root, ref)
		if err != nil {
			return nil, err
		}
		if s.vs == nil || s.vsPath != path {
			if s.vs != nil {
				_ = s.vs.Close()
			}
			s.vs, s.vsPath = nil, path
			st, err := video.OpenStream(path, stageW, stageH, videoFPS)
			if err != nil {
				return nil, err
			}
			s.vs = st
		}
		return s.vs.Frame(tMs)
	}
}

// closeVideo 关掉影片流（换事件/关会话时调用）。
func (s *replaySession) closeVideo() {
	if s.vs != nil {
		_ = s.vs.Close()
		s.vs, s.vsPath = nil, ""
	}
}

// renderJPEG RenderFrame → JPEG dataURL（quake 偏移已由 Compose/定位处理？——
// RenderFrame 只管图层与框底，前端拿 QuakeX/Y 对 <img> 做位移）。
func renderJPEG(p *player.Player, load player.Loader, video player.VideoProvider, _, _ int) (string, error) {
	rgba, _, err := player.RenderFrameWithVideo(p, load, video, stageW, stageH)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, rgba, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return "", err
	}
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

// Play 恢复自动推进。
func (a *App) Play() error {
	if a.sess == nil {
		return fmt.Errorf("没有活动会话")
	}
	s := a.sess
	s.mu.Lock()
	defer s.mu.Unlock()
	s.playing = true
	s.lastTick = time.Now()
	return nil
}

// Pause 暂停（不终止会话）。
func (a *App) Pause() error {
	if a.sess == nil {
		return fmt.Errorf("没有活动会话")
	}
	s := a.sess
	s.mu.Lock()
	defer s.mu.Unlock()
	s.playing = false
	return nil
}

// TogglePlay 播放/暂停切换，返回切换后的状态。
func (a *App) TogglePlay() (bool, error) {
	if a.sess == nil {
		return false, fmt.Errorf("没有活动会话")
	}
	s := a.sess
	s.mu.Lock()
	defer s.mu.Unlock()
	s.playing = !s.playing
	s.lastTick = time.Now()
	return s.playing, nil
}

// InputClick 注入一次点击推进（对齐 CLI「空格/回车」）。
func (a *App) InputClick() error {
	if a.sess == nil {
		return fmt.Errorf("没有活动会话")
	}
	s := a.sess
	s.mu.Lock()
	defer s.mu.Unlock()
	s.input.Advance = true
	return nil
}

// ToggleAuto 切自动推进（与 CLI M 键同语义，写回 conf.Auto）。
func (a *App) ToggleAuto() (bool, error) {
	if a.sess == nil {
		return false, fmt.Errorf("没有活动会话")
	}
	s := a.sess
	s.mu.Lock()
	s.auto = !s.auto
	auto := s.auto
	s.mu.Unlock()

	c, _ := a.loadConf()
	c.Auto = auto
	if err := a.saveConf(c); err != nil {
		return auto, err
	}
	return auto, nil
}

// SetMuted 静音（只是不再把音频 op 派给平台层，会话继续跑）。
func (a *App) SetMuted(m bool) error {
	if a.sess == nil {
		return fmt.Errorf("没有活动会话")
	}
	s := a.sess
	s.mu.Lock()
	defer s.mu.Unlock()
	s.muted = m
	if s.audio != nil {
		if m {
			s.p.Audio = nil
		} else {
			s.p.Audio = s.audio.Handle
		}
	}
	return nil
}

// ExitReplay 退出回放页（关会话、停 BGM/SE）。
func (a *App) ExitReplay() error {
	a.sessMu.Lock()
	defer a.sessMu.Unlock()
	a.closeSessionLocked()
	return nil
}

// SeekTo 跳到目标毫秒：重建播放器 + 快进（纯状态推进，不渲染，倍速批量走）。
func (a *App) SeekTo(ms int64) error {
	if a.sess == nil {
		return fmt.Errorf("没有活动会话")
	}
	s := a.sess
	if ms < 0 {
		ms = 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	opt := s.opts
	p := player.New(s.ev, opt)
	p.Audio = s.p.Audio // 音频挂接保持
	p.Advance(0, player.Input{})
	for t := p.Now(); t < ms && !p.Done(); t = p.Now() {
		step := int64(200)
		if ms-t < step {
			step = ms - t
		}
		p.Advance(step, player.Input{})
	}
	s.p = p
	s.missing = map[string]string{} // 重建后按新时间点重新记账
	s.lastSig = ""
	s.lastJPEG = ""
	s.lastEnc = time.Time{} // 立即允许一次编码
	s.lastTick = time.Now()
	return nil
}

// Screenshot 把当前帧 PNG 落盘到 exe\exports\，返回路径（DR-30 只写派生物）。
func (a *App) Screenshot() (string, error) {
	if a.sess == nil {
		return "", fmt.Errorf("没有活动会话")
	}
	s := a.sess
	s.mu.Lock()
	defer s.mu.Unlock()

	rgba, _, err := player.RenderFrame(s.p, s.load, stageW, stageH)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(dataDir(), "exports", safeName(s.kind)+"_"+safeName(s.id))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	name := fmt.Sprintf("%s.png", time.Now().Format("20060102-150405.000"))
	if err := savePNG(rgba, filepath.Join(dir, name)); err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

// savePNG PNG 落盘。
func savePNG(src image.Image, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, src)
}
