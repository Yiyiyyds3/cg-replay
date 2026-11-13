// Package cover 从演出台本的时间轴抽取封面帧。
//
// 设计目标是绕开「首帧恰好是入场黑幕」：沿事件时间轴取若干采样点，用
// 非黑像素比例、亮度方差和边缘密度打分，挑信息量最高的一帧。渲染直接复用
// player.Player + player.Compose（与 desktop/replay.go 同一条合成链路），
// 这里只做「把 Frame 的坐标缩到小画布」和「选帧」，不另写渲染器。
package cover

import (
	"errors"
	"image"
	"image/color"
	"math"
	"time"

	"ucgr/internal/ir"
	"ucgr/internal/player"
)

const (
	// Version 封面算法/缓存版本。缓存文件名带版本，旧版黑图不会被复用。
	Version = 2
	// Algorithm 写进 cache meta，便于排查“为什么这张封面和上一版不一样”。
	Algorithm = "timeline-sample-v2"

	// DefaultSamples 采样点数（覆盖开头、中段、结尾）。
	DefaultSamples = 12
	// DefaultWidth 采样画布宽度；高度按舞台比例算。440×248 的卡片封面
	// 从 480px 宽采样帧中心裁切，清晰度足够且比全尺寸合成便宜一个量级。
	DefaultWidth = 480
	// DefaultFrameBudget 单帧渲染上限（任务要求示例 ≤2s）。
	DefaultFrameBudget = 1500 * time.Millisecond
	// DefaultTotalBudget 一轮封面的总渲染上限。超时立即回退，不阻断卡片墙。
	DefaultTotalBudget = 6 * time.Second
	// MinInkRatio 一帧至少要有 5% 像素不是近黑，才算“有效画面”。
	MinInkRatio = 0.05
)

// Metrics 一帧的画面信息量评分。
type Metrics struct {
	NonBlack float64 `json:"non_black"` // 非近黑像素比例 0..1
	Variance float64 `json:"variance"`  // RGB 三通道平均方差归一化后的标准差 0..1
	Edges    float64 `json:"edges"`     // 相邻采样点的边缘密度 0..1
	Score    float64 `json:"score"`     // 加权总分 0..1
}

// Valid 该帧是否算有效画面。只有近黑/全透明帧会被判无效；单色画面
// 虽然信息量低，但不是黑屏，仍可作为兜底画面。
func (m Metrics) Valid() bool { return m.NonBlack >= MinInkRatio }

// Sample 单个采样点结果。
type Sample struct {
	AtMs    int64   `json:"at_ms"`
	Metrics Metrics `json:"metrics"`
	OK      bool    `json:"ok"`
	Err     string  `json:"err,omitempty"`
}

// Result 一轮时间轴采样结果。
type Result struct {
	Image      *image.RGBA
	Metrics    Metrics
	AtMs       int64
	Valid      bool
	Fallback   bool // 时间轴没有有效帧，调用方应回退到静态图
	Reason     string
	DurationMs int64
	Sampled    int // 成功渲染的采样点数
	Attempted  int
	Samples    []Sample
}

// Config 采样配置。零值字段由 fill 补默认。
type Config struct {
	Samples     int
	Width       int
	StageW      int
	StageH      int
	FrameBudget time.Duration
	TotalBudget time.Duration
}

// DefaultConfig 返回默认采样配置。
func DefaultConfig() Config {
	return Config{
		Samples:     DefaultSamples,
		Width:       DefaultWidth,
		FrameBudget: DefaultFrameBudget,
		TotalBudget: DefaultTotalBudget,
	}
}

func (c *Config) fill() {
	d := DefaultConfig()
	if c.Samples < 1 {
		c.Samples = d.Samples
	}
	if c.Samples > 24 {
		c.Samples = 24
	}
	if c.Width < 64 {
		c.Width = d.Width
	}
	if c.FrameBudget <= 0 {
		c.FrameBudget = d.FrameBudget
	}
	if c.TotalBudget <= 0 {
		c.TotalBudget = d.TotalBudget
	}
}

// RenderEvent 沿事件 Show 时间轴采样并选帧。
//
// 只返回 Result；所有渲染错误都记录在 Samples 中，不向卡片墙抛错。
// 若没有一帧通过 MinInkRatio，Valid=false、Fallback=true，由调用方回退到
// 页面首图/缩略图并打「无有效画面」角标。
func RenderEvent(ev *ir.Event, load player.Loader, cfg Config) Result {
	cfg.fill()
	res := Result{Fallback: true}
	if ev == nil || len(ev.Show) == 0 {
		res.Reason = "empty_show"
		return res
	}
	if load == nil {
		res.Reason = "nil_loader"
		return res
	}

	stageW, stageH := cfg.StageW, cfg.StageH
	if stageW <= 0 || stageH <= 0 {
		stageW, stageH = inferStage(ev, load)
	}
	if stageW <= 0 || stageH <= 0 {
		stageW, stageH = 16, 9
	}
	scale := float64(cfg.Width) / float64(stageW)
	outW := cfg.Width
	outH := int(math.Round(float64(stageH) * scale))
	if outH < 1 {
		outH = 1
	}

	res.DurationMs = EstimateDuration(ev)
	targets := sampleTimes(res.DurationMs, cfg.Samples)

	opts := player.DefaultOptions()
	opts.StageW, opts.StageH = stageW, stageH
	p := player.New(ev, opts)
	p.Advance(0, player.Input{})

	deadline := time.Now().Add(cfg.TotalBudget)
	var best *image.RGBA
	bestMetrics := Metrics{}
	bestAt := int64(0)

	for _, target := range targets {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			res.Reason = "total_budget_exceeded"
			break
		}
		advanceTo(p, target)
		res.Attempted++

		frameBudget := cfg.FrameBudget
		if remaining < frameBudget {
			frameBudget = remaining
		}
		img, err := renderWithBudget(p, load, scale, outW, outH, frameBudget)
		if err != nil {
			res.Samples = append(res.Samples, Sample{
				AtMs: p.Now(), OK: false, Err: err.Error(),
			})
			if errors.Is(err, errFrameTimeout) || errors.Is(err, errTotalBudget) {
				res.Reason = err.Error()
				break
			}
			continue
		}
		m := Analyze(img)
		res.Sampled++
		res.Samples = append(res.Samples, Sample{AtMs: p.Now(), Metrics: m, OK: true})
		if m.Valid() && (best == nil || m.Score > bestMetrics.Score) {
			best, bestMetrics, bestAt = img, m, p.Now()
		}
	}

	if best != nil {
		res.Image = best
		res.Metrics = bestMetrics
		res.AtMs = bestAt
		res.Valid = true
		res.Fallback = false
		res.Reason = ""
		return res
	}
	if res.Reason == "" {
		res.Reason = "all_sampled_frames_black"
	}
	return res
}

var (
	errFrameTimeout = errors.New("frame_budget_exceeded")
	errTotalBudget  = errors.New("total_budget_exceeded")
)

type renderResult struct {
	img *image.RGBA
	err error
}

func renderWithBudget(p *player.Player, load player.Loader, scale float64, w, h int, budget time.Duration) (*image.RGBA, error) {
	if budget <= 0 {
		return nil, errTotalBudget
	}
	// p.Frame() 在启动 goroutine 前取好：即使调用方因超时返回，渲染协程
	// 也只操作自己的帧快照，不会与后续播放器状态互相踩踏。
	f := p.Frame()
	done := make(chan renderResult, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- renderResult{err: errors.New("render_panic")}
			}
		}()
		small := scaleFrame(f, scale)
		img, _, err := player.Compose(small, load, w, h)
		done <- renderResult{img: img, err: err}
	}()
	timer := time.NewTimer(budget)
	defer timer.Stop()
	select {
	case r := <-done:
		return r.img, r.err
	case <-timer.C:
		return nil, errFrameTimeout
	}
}

// EstimateDuration 用播放器状态机走一遍台本，得到自动推进下的真实结束时刻。
// 只推进状态，不渲染，开销很小；返回值同时参考 EndMs 与脚本最大 TMs。
func EstimateDuration(ev *ir.Event) int64 {
	if ev == nil {
		return 0
	}
	opts := player.DefaultOptions()
	p := player.New(ev, opts)
	p.Advance(0, player.Input{})
	const step = int64(250)
	const maxSteps = 300000 // 250ms × 300k = 20.8h，防御坏台本死循环
	for i := 0; i < maxSteps && !p.Done(); i++ {
		p.Advance(step, player.Input{})
	}
	d := p.Now()
	if ev.EndMs > d {
		d = ev.EndMs
	}
	for _, op := range ev.Show {
		if op.TMs > d {
			d = op.TMs + op.DurMs
		}
	}
	if d <= 0 {
		d = 1
	}
	return d
}

func sampleTimes(durationMs int64, n int) []int64 {
	if n < 1 {
		n = 1
	}
	if durationMs <= 0 {
		durationMs = 1
	}
	out := make([]int64, n)
	if n == 1 {
		out[0] = durationMs / 2
		return out
	}
	// 2%..96%：保留开头用于识别黑幕，但不把 t=0 的纯黑占位当封面；
	// 末尾留一点余量，避开退场黑幕。
	const start, end = 0.02, 0.96
	for i := 0; i < n; i++ {
		f := start + (end-start)*float64(i)/float64(n-1)
		out[i] = int64(float64(durationMs) * f)
	}
	return out
}

func advanceTo(p *player.Player, target int64) {
	for p.Now() < target {
		step := target - p.Now()
		if step > 250 {
			step = 250
		}
		p.Advance(step, player.Input{})
	}
}

// scaleFrame 把图层坐标/缩放/裁剪/抖动量缩到采样画布尺寸。
// Compose 仍然负责排序、alpha 混合、裁剪与绘制，这里不做任何像素合成。
func scaleFrame(f player.Frame, s float64) player.Frame {
	out := f
	out.Texts = nil // 封面不画台词框，避免文字/框底遮挡画面
	out.QuakeX = int(math.Round(float64(f.QuakeX) * s))
	out.QuakeY = int(math.Round(float64(f.QuakeY) * s))
	out.Layers = make([]player.LayerState, len(f.Layers))
	for i, st := range f.Layers {
		st.X *= s
		st.Y *= s
		if st.Scale <= 0 {
			st.Scale = 1
		}
		st.Scale *= s
		if st.HasClip {
			st.ClipL *= s
			st.ClipT *= s
			st.ClipW *= s
			st.ClipH *= s
		}
		out.Layers[i] = st
	}
	return out
}

func inferStage(ev *ir.Event, load player.Loader) (int, int) {
	for _, op := range ev.Show {
		if op.Kind != ir.ShowLayerShow || op.Ref == "" {
			continue
		}
		img, err := load(op.Ref)
		if err != nil {
			continue
		}
		b := img.Bounds()
		if b.Dx() > 0 && b.Dy() > 0 {
			return b.Dx(), b.Dy()
		}
	}
	return 0, 0
}

// Analyze 计算一帧的信息量。为了不扫描 4K 原图，按网格抽样到约 6 万点。
func Analyze(img image.Image) Metrics {
	b := img.Bounds()
	if b.Dx() <= 0 || b.Dy() <= 0 {
		return Metrics{}
	}
	w, h := b.Dx(), b.Dy()
	step := 1
	if w*h > 60000 {
		step = int(math.Ceil(math.Sqrt(float64(w*h) / 60000)))
		if step < 1 {
			step = 1
		}
	}
	var (
		total               int
		ink                 int
		sumR, sumG, sumB    float64
		sumR2, sumG2, sumB2 float64
		edgeN               int
		edgeHit             int
	)
	for y := b.Min.Y; y < b.Max.Y; y += step {
		for x := b.Min.X; x < b.Max.X; x += step {
			r, g, bl, a := img.At(x, y).RGBA()
			r8, g8, b8 := float64(r>>8), float64(g>>8), float64(bl>>8)
			lum := 0.2126*r8 + 0.7152*g8 + 0.0722*b8
			on := a >= 0x0400
			total++
			if on && lum >= 12 {
				ink++
			}
			sumR += r8
			sumG += g8
			sumB += b8
			sumR2 += r8 * r8
			sumG2 += g8 * g8
			sumB2 += b8 * b8
			if x+step < b.Max.X {
				r2, g2, b2, a2 := img.At(x+step, y).RGBA()
				if a >= 0x0400 && a2 >= 0x0400 {
					l2 := 0.2126*float64(r2>>8) + 0.7152*float64(g2>>8) + 0.0722*float64(b2>>8)
					edgeN++
					if math.Abs(lum-l2) >= 18 {
						edgeHit++
					}
				}
			}
			if y+step < b.Max.Y {
				r2, g2, b2, a2 := img.At(x, y+step).RGBA()
				if a >= 0x0400 && a2 >= 0x0400 {
					l2 := 0.2126*float64(r2>>8) + 0.7152*float64(g2>>8) + 0.0722*float64(b2>>8)
					edgeN++
					if math.Abs(lum-l2) >= 18 {
						edgeHit++
					}
				}
			}
		}
	}
	if total == 0 {
		return Metrics{}
	}
	m := Metrics{}
	m.NonBlack = float64(ink) / float64(total)
	n := float64(total)
	varianceOf := func(sum, sumSq float64) float64 {
		v := sumSq/n - (sum/n)*(sum/n)
		if v < 1e-4 {
			return 0
		}
		return v
	}
	// 三个通道的平均方差：纯色为 0，黑/白或冷暖对比越大分越高。
	variance := (varianceOf(sumR, sumR2) + varianceOf(sumG, sumG2) + varianceOf(sumB, sumB2)) / 3.0 / (255.0 * 255.0)
	std := math.Sqrt(variance)
	if std > 1 {
		std = 1
	}
	m.Variance = std
	if edgeN > 0 {
		m.Edges = float64(edgeHit) / float64(edgeN)
	}
	m.Score = 0.60*m.NonBlack + 0.25*m.Variance + 0.15*m.Edges
	return m
}

// CropFit 中心裁成 targetW:targetH 比例并最近邻缩放到目标尺寸。
// 与 app.go 的 saveRatioCrop 保持同一裁切口径，但返回图像便于先盖角标再落盘。
func CropFit(src image.Image, targetW, targetH int) *image.RGBA {
	if targetW < 1 || targetH < 1 {
		return image.NewRGBA(image.Rect(0, 0, 1, 1))
	}
	out := image.NewRGBA(image.Rect(0, 0, targetW, targetH))
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	if sw <= 0 || sh <= 0 {
		return out
	}
	tr := float64(targetW) / float64(targetH)
	sr := float64(sw) / float64(sh)
	cw, ch := sw, sh
	if sr > tr {
		cw = int(tr*float64(sh) + 0.5)
	} else {
		ch = int(float64(sw)/tr + 0.5)
	}
	ox, oy := (sw-cw)/2, (sh-ch)/2
	for y := 0; y < targetH; y++ {
		sy := oy + y*ch/targetH
		for x := 0; x < targetW; x++ {
			out.Set(x, y, src.At(b.Min.X+ox+x*cw/targetW, b.Min.Y+sy))
		}
	}
	return out
}

// FallbackBadgeBounds 返回「无有效画面」角标区域（用于测试/布局）。
func FallbackBadgeBounds(w, h int) image.Rectangle {
	r := badgeRect(w, h)
	return r
}

// StampFallback 在左上角盖「无有效画面」角标。
//
// 这里用内置 16×16 点阵字，不依赖系统字体（CI/无图形环境也能测试并落盘）。
func StampFallback(img *image.RGBA) {
	if img == nil {
		return
	}
	r := badgeRect(img.Bounds().Dx(), img.Bounds().Dy())
	if r.Empty() {
		return
	}
	// 半透明黑底 + 琥珀色描边。
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			blendPixel(img, x, y, color.RGBA{10, 12, 16, 0}, 0.82)
		}
	}
	border := color.RGBA{255, 196, 80, 255}
	for x := r.Min.X; x < r.Max.X; x++ {
		img.SetRGBA(x, r.Min.Y, border)
		img.SetRGBA(x, r.Max.Y-1, border)
	}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		img.SetRGBA(r.Min.X, y, border)
		img.SetRGBA(r.Max.X-1, y, border)
	}
	const scale = 2
	const glyph = 16
	const gap = 2
	x := r.Min.X + 6
	y := r.Min.Y + 6
	text := []rune("无有效画面")
	for _, ch := range text {
		rows := badgeGlyphs[ch]
		for gy, row := range rows {
			for gx, c := range row {
				if c == '.' {
					continue
				}
				// 2×2 点阵，放大后仍能在 440×248 卡片上看清。
				for dy := 0; dy < scale; dy++ {
					for dx := 0; dx < scale; dx++ {
						img.SetRGBA(x+gx*scale+dx, y+gy*scale+dy, border)
					}
				}
			}
		}
		x += glyph*scale + gap
	}
}

func badgeRect(w, h int) image.Rectangle {
	const badgeW, badgeH = 180, 44
	if w <= 0 || h <= 0 {
		return image.Rectangle{}
	}
	x0, y0 := 8, 8
	x1, y1 := x0+badgeW, y0+badgeH
	if x1 > w {
		x1 = w
	}
	if y1 > h {
		y1 = h
	}
	if x1 <= x0 || y1 <= y0 {
		return image.Rectangle{}
	}
	return image.Rect(x0, y0, x1, y1)
}

func blendPixel(img *image.RGBA, x, y int, c color.RGBA, alpha float64) {
	if !(image.Point{X: x, Y: y}).In(img.Bounds()) {
		return
	}
	if alpha <= 0 {
		return
	}
	if alpha > 1 {
		alpha = 1
	}
	i := img.PixOffset(x, y)
	dr, dg, db := float64(img.Pix[i]), float64(img.Pix[i+1]), float64(img.Pix[i+2])
	img.Pix[i] = uint8(dr + (float64(c.R)-dr)*alpha)
	img.Pix[i+1] = uint8(dg + (float64(c.G)-dg)*alpha)
	img.Pix[i+2] = uint8(db + (float64(c.B)-db)*alpha)
	img.Pix[i+3] = 255
}

var badgeGlyphs = map[rune][]string{
	'无': {
		"................",
		"................",
		"................",
		".##########.....",
		".....#..........",
		".....#..........",
		".....#..........",
		"############....",
		"....+#..........",
		"....#+#+........",
		"...+#.#+........",
		"..+#..#+..+#....",
		".##+..#+..+#....",
		"##....+####+....",
		"................",
		"................",
	},
	'有': {
		"................",
		"................",
		"....++..........",
		"....#+..........",
		"############....",
		"...#+...........",
		"..+#######+.....",
		".+##.....#+.....",
		"+#########+.....",
		"#+##.....#+.....",
		"..########+.....",
		"..##.....#+.....",
		"..##...###......",
		"................",
		"................",
		"................",
	},
	'效': {
		"................",
		"................",
		"..++...#+.......",
		"...#...#+.......",
		"######+#####....",
		".++.+.##..#.....",
		".#+.#+##.+#.....",
		"#+...#.#+++.....",
		".+#.#..+##+.....",
		"..##+...##......",
		"..+##..+##......",
		".##.#++#+##.....",
		"##...+#+..+#....",
		"................",
		"................",
		"................",
	},
	'画': {
		"................",
		"................",
		"................",
		"############....",
		"................",
		".#.######.#.....",
		".#.#.##.#.#.....",
		".#.######.#.....",
		".#.#.##.#.#.....",
		".#.######.#.....",
		".#.#....#.#.....",
		".#........#.....",
		".##########.....",
		"..........#.....",
		"................",
		"................",
	},
	'面': {
		"................",
		"................",
		"................",
		"############....",
		"....+#..........",
		".##########.....",
		".#.+#..#+.#.....",
		".#.+####+.#.....",
		".#.+#..#+.#.....",
		".#.+#..#+.#.....",
		".#.+####+.#.....",
		".#.+#..#+.#.....",
		".##########.....",
		".#........#.....",
		"................",
		"................",
	},
}
