package cover

import (
	"image"
	"image/color"
	"testing"
	"time"

	"ucgr/internal/ir"
	"ucgr/internal/player"
)

func solid(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

func checker(w, h int, a, b color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if (x/4+y/4)%2 == 0 {
				img.SetRGBA(x, y, a)
			} else {
				img.SetRGBA(x, y, b)
			}
		}
	}
	return img
}

// 黑幕 → 彩色：采样必须跨过入场黑帧，选中带画面的帧。
func TestRenderEventSkipsBlackIntro(t *testing.T) {
	black := solid(160, 90, color.RGBA{0, 0, 0, 255})
	colored := checker(160, 90, color.RGBA{220, 40, 40, 255}, color.RGBA{30, 70, 230, 255})
	assets := map[string]image.Image{
		"black.png": black,
		"color.png": colored,
	}
	load := func(ref string) (image.Image, error) {
		if img, ok := assets[ref]; ok {
			return img, nil
		}
		return nil, image.ErrFormat
	}
	ev := &ir.Event{
		EventID: "ir-black-then-color",
		Kind:    ir.KindStillCG,
		Show: []ir.ShowOp{
			{Kind: ir.ShowLayerShow, Layer: "layer0", Ref: "black.png",
				Attr: map[string]string{"left": "0", "top": "0"}},
			{Kind: ir.ShowWait, DurMs: 1000},
			{Kind: ir.ShowLayerShow, Layer: "layer0", Ref: "color.png",
				Attr: map[string]string{"left": "0", "top": "0"}},
			{Kind: ir.ShowWait, DurMs: 2000},
		},
	}
	cfg := DefaultConfig()
	cfg.StageW, cfg.StageH = 160, 90
	cfg.Samples = 8
	cfg.FrameBudget = 2 * time.Second
	cfg.TotalBudget = 10 * time.Second
	r := RenderEvent(ev, player.Loader(load), cfg)
	if !r.Valid {
		t.Fatalf("应选到彩色帧，实际 Valid=false reason=%q samples=%+v", r.Reason, r.Samples)
	}
	if r.AtMs < 1000 {
		t.Fatalf("选中的是入场黑帧：at=%dms", r.AtMs)
	}
	if r.Image == nil {
		t.Fatal("选中帧没有图像")
	}
	px := r.Image.RGBAAt(r.Image.Bounds().Dx()/2, r.Image.Bounds().Dy()/2)
	if px.R == 0 && px.G == 0 && px.B == 0 {
		t.Fatalf("选中帧中心仍是黑色：%+v", px)
	}
	if r.Fallback {
		t.Fatal("有效帧不应标记 Fallback")
	}
}

// 全黑：不返回有效帧，调用方会走静态图回退并打角标。
func TestRenderEventAllBlackInvalid(t *testing.T) {
	black := solid(160, 90, color.RGBA{0, 0, 0, 255})
	ev := &ir.Event{
		EventID: "all-black",
		Kind:    ir.KindStillCG,
		Show: []ir.ShowOp{
			{Kind: ir.ShowLayerShow, Layer: "layer0", Ref: "black.png"},
			{Kind: ir.ShowWait, DurMs: 2000},
		},
	}
	cfg := DefaultConfig()
	cfg.StageW, cfg.StageH = 160, 90
	cfg.Samples = 6
	cfg.FrameBudget = time.Second
	cfg.TotalBudget = 5 * time.Second
	r := RenderEvent(ev, player.Loader(func(string) (image.Image, error) { return black, nil }), cfg)
	if r.Valid || !r.Fallback {
		t.Fatalf("全黑应 Valid=false/Fallback=true，实际 valid=%v fallback=%v", r.Valid, r.Fallback)
	}
	if r.Reason != "all_sampled_frames_black" {
		t.Fatalf("reason=%q，想要 all_sampled_frames_black", r.Reason)
	}
	if r.Sampled == 0 {
		t.Fatal("至少应完成一次采样")
	}
}

func TestMetricsBoundaries(t *testing.T) {
	transparent := image.NewRGBA(image.Rect(0, 0, 32, 32))
	black := solid(32, 32, color.RGBA{0, 0, 0, 255})
	solidColor := solid(32, 32, color.RGBA{200, 90, 40, 255})
	colorful := checker(32, 32, color.RGBA{240, 220, 120, 255}, color.RGBA{10, 20, 90, 255})

	mt := Analyze(transparent)
	mb := Analyze(black)
	ms := Analyze(solidColor)
	mc := Analyze(colorful)

	if mt.Score != 0 || mt.Valid() {
		t.Fatalf("全透明应 0 分且无效：%+v", mt)
	}
	if mb.Score != 0 || mb.Valid() {
		t.Fatalf("全黑应 0 分且无效：%+v", mb)
	}
	if !ms.Valid() || ms.NonBlack < 0.99 {
		t.Fatalf("单色不透明应算有效且 NonBlack≈1：%+v", ms)
	}
	if ms.Variance != 0 || ms.Edges != 0 {
		t.Fatalf("单色方差/边缘应为 0：%+v", ms)
	}
	if mc.Score <= ms.Score {
		t.Fatalf("彩色画面评分应高于单色：solid=%v colorful=%v", ms.Score, mc.Score)
	}
	if mc.Variance <= 0 || mc.Edges <= 0 {
		t.Fatalf("彩色画面应有方差与边缘：%+v", mc)
	}
}

func TestFallbackBadgeIsVisible(t *testing.T) {
	img := solid(440, 248, color.RGBA{0, 0, 0, 255})
	before := solid(440, 248, color.RGBA{0, 0, 0, 255})
	StampFallback(img)
	r := FallbackBadgeBounds(440, 248)
	if r.Empty() {
		t.Fatal("角标区域不应为空")
	}
	diff, amber := 0, 0
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			a := img.RGBAAt(x, y)
			b := before.RGBAAt(x, y)
			if a != b {
				diff++
			}
			if a.R > 180 && a.G > 120 && a.B < 130 {
				amber++
			}
		}
	}
	if diff < 100 || amber < 50 {
		t.Fatalf("角标没有明显画上：diff=%d amber=%d", diff, amber)
	}
}

func TestRenderEventHonorsFrameBudget(t *testing.T) {
	ev := &ir.Event{
		EventID: "slow-loader",
		Show: []ir.ShowOp{
			{Kind: ir.ShowLayerShow, Layer: "layer0", Ref: "slow.png"},
			{Kind: ir.ShowWait, DurMs: 1000},
		},
	}
	slow := func(string) (image.Image, error) {
		time.Sleep(250 * time.Millisecond)
		return solid(16, 9, color.RGBA{200, 200, 200, 255}), nil
	}
	cfg := DefaultConfig()
	cfg.StageW, cfg.StageH = 160, 90
	cfg.Samples = 1
	cfg.FrameBudget = 20 * time.Millisecond
	cfg.TotalBudget = 500 * time.Millisecond
	start := time.Now()
	r := RenderEvent(ev, player.Loader(slow), cfg)
	elapsed := time.Since(start)
	if elapsed > 300*time.Millisecond {
		t.Fatalf("单帧超时未生效：耗时 %v", elapsed)
	}
	if r.Valid || r.Reason != "frame_budget_exceeded" {
		t.Fatalf("应因单帧预算超时快速回退：valid=%v reason=%q", r.Valid, r.Reason)
	}
}

func TestSampleTimesCoverOpeningAndEnd(t *testing.T) {
	ts := sampleTimes(1000, 12)
	if len(ts) != 12 {
		t.Fatalf("采样点数=%d", len(ts))
	}
	if ts[0] <= 0 || ts[0] >= 200 {
		t.Fatalf("开头采样点应在 2%% 附近，实际 %d", ts[0])
	}
	if ts[len(ts)-1] < 900 || ts[len(ts)-1] >= 1000 {
		t.Fatalf("结尾采样点应接近但不到结尾，实际 %d", ts[len(ts)-1])
	}
	for i := 1; i < len(ts); i++ {
		if ts[i] < ts[i-1] {
			t.Fatalf("采样时间必须递增：%v", ts)
		}
	}
}
