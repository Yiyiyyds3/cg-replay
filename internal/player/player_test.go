package player

import (
	"image"
	"image/color"
	"math"
	"strings"
	"testing"

	"ucgr/internal/ir"
)

// ---------------------------------------------------------------
// 测试辅助
// ---------------------------------------------------------------

func near(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

// solid 造一张纯色 RGBA。
func solid(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

// solidLoader 按路径返回纯色图（缺的返回错误）。
func solidLoader(m map[string]color.RGBA) Loader {
	return func(rel string) (image.Image, error) {
		if c, ok := m[rel]; ok {
			return solid(8, 8, c), nil
		}
		return nil, errMissing{rel}
	}
}

type errMissing struct{ rel string }

func (e errMissing) Error() string { return "缺素材：" + e.rel }

func layerByName(f Frame, id string) *LayerState {
	for i := range f.Layers {
		if f.Layers[i].Layer == id {
			return &f.Layers[i]
		}
	}
	return nil
}

// ---------------------------------------------------------------
// 双缓冲 + 转场
// ---------------------------------------------------------------

func TestDoubleBufferAndTransition(t *testing.T) {
	ev := &ir.Event{EventID: "e", Show: []ir.ShowOp{
		{TMs: 0, Kind: ir.ShowScreenCopy},
		{TMs: 0, Kind: ir.ShowLayerShow, Layer: "base", Page: "back", Ref: "bg"},
		{TMs: 0, Kind: ir.ShowTransition, DurMs: 1000},
		{TMs: 0, Kind: ir.ShowWait, Attr: map[string]string{"wt": "1"}},
		{TMs: 1000, Kind: ir.ShowLayerShow, Layer: "layer2", Page: "back", Ref: "st"},
		{TMs: 1000, Kind: ir.ShowTransition, DurMs: 500},
	}}
	p := New(ev, DefaultOptions())

	// [backlay] → 写 back → [trans]：指令在当下这一刻执行，进度从 0 开始
	p.Advance(16, Input{})
	f := p.Frame()
	if !near(f.TransProgress, 0, 1e-9) {
		t.Fatalf("转场刚启动时进度应为 0，实际 %v", f.TransProgress)
	}
	if len(f.Layers) != 1 {
		t.Fatalf("转场中应只有 1 层，实际 %d：%+v", len(f.Layers), f.Layers)
	}
	if st := layerByName(f, "base"); st == nil || !near(st.Opacity, 0, 1e-9) {
		t.Fatalf("base 层刚进入时完全不透明应为 0，实际 %+v", st)
	}

	// 半程：按进度淡入
	p.Advance(500, Input{})
	f = p.Frame()
	if !near(f.TransProgress, 0.5, 1e-9) {
		t.Fatalf("半程进度应为 0.5，实际 %v", f.TransProgress)
	}
	if st := layerByName(f, "base"); st == nil || !near(st.Opacity, 0.5, 1e-6) {
		t.Fatalf("半程 base 不透明度应为 0.5，实际 %+v", st)
	}

	// [wt] 等到转场结束：fore 翻成 back；随后第二段转场启动
	p.Advance(500, Input{})
	f = p.Frame()
	if st := layerByName(f, "base"); st == nil || !near(st.Opacity, 1, 1e-9) {
		t.Fatalf("转场结束后 base 应完全不透明，实际 %+v", st)
	}
	if st := layerByName(f, "layer2"); st == nil || st.Opacity > 1e-9 {
		t.Fatalf("第二段转场刚开始时 layer2 应透明，实际 %+v", st)
	}

	// 转场走完 → fore = back
	p.Advance(500, Input{})
	f = p.Frame()
	for _, id := range []string{"base", "layer2"} {
		st := layerByName(f, id)
		if st == nil || !st.Visible || !near(st.Opacity, 1, 1e-9) {
			t.Fatalf("转场结束后 %s 应可见且不透明，实际 %+v", id, st)
		}
	}
	if !p.Done() {
		t.Fatalf("台本应已走完")
	}
	if f.TransProgress != 0 {
		t.Fatalf("转场结束后 TransProgress 应为 0，实际 %v", f.TransProgress)
	}
}

// ---------------------------------------------------------------
// 文本：自动推进时长 + 手动推进
// ---------------------------------------------------------------

func TestTextWaitAutoAndManual(t *testing.T) {
	ops := []ir.ShowOp{
		{TMs: 0, Kind: ir.ShowTextRun, Layer: "message0",
			Text: &ir.ShowText{Speaker: "池田", Text: "一二三四五六七八九十"}}, // 10 字
		{TMs: 0, Kind: ir.ShowTextWait},
		{TMs: 0, Kind: ir.ShowTextClear},
	}
	p := New(&ir.Event{Show: ops}, DefaultOptions())
	p.Advance(1, Input{})

	f := p.Frame()
	if !f.Waiting {
		t.Fatalf("点击点应处于等待状态")
	}
	if len(f.Texts) != 1 || len(f.Texts[0].Lines) != 1 {
		t.Fatalf("应有 1 个消息层 1 行文字，实际 %+v", f.Texts)
	}
	if got := f.Texts[0].Lines[0].Text; got != "一二三四五六七八九十" {
		t.Fatalf("文本不对：%q", got)
	}
	if got := f.Texts[0].Lines[0].Speaker; got != "池田" {
		t.Fatalf("说话人不对：%q", got)
	}
	// 自动时长 = 800 + 10×75 = 1550ms
	if got := p.blockUntil - p.Now(); got != 1550 {
		t.Fatalf("自动推进时长应为 1550ms，实际 %d", got)
	}
	p.Advance(1000, Input{})
	if !p.Frame().Waiting {
		t.Fatalf("未到时长不应自动翻过")
	}
	p.Advance(600, Input{})
	if p.Frame().Waiting {
		t.Fatalf("超过自动时长应已翻过")
	}
	if len(p.Frame().Texts) != 0 {
		t.Fatalf("[cm] 之后应已清页")
	}

	// 手动推进：不必等自动时长
	p2 := New(&ir.Event{Show: ops}, DefaultOptions())
	p2.Advance(1, Input{})
	p2.Advance(0, Input{Advance: true})
	if p2.Frame().Waiting {
		t.Fatalf("手动推进应立即结束点击点")
	}
	if len(p2.Frame().Texts) != 0 {
		t.Fatalf("手动推进后应已执行 [cm] 清页")
	}
}

func TestTextBreakAndClearPerLayer(t *testing.T) {
	ops := []ir.ShowOp{
		{TMs: 0, Kind: ir.ShowTextRun, Layer: "message0", Text: &ir.ShowText{Text: "甲"}},
		{TMs: 0, Kind: ir.ShowTextRun, Layer: "message0", Text: &ir.ShowText{Text: "乙"}}, // 同行拼接
		{TMs: 0, Kind: ir.ShowTextBreak},
		{TMs: 0, Kind: ir.ShowTextRun, Layer: "message0", Text: &ir.ShowText{Text: "丙"}},
		{TMs: 0, Kind: ir.ShowTextRun, Layer: "message1", Text: &ir.ShowText{Text: "名字"}},
		{TMs: 0, Kind: ir.ShowTextClear, Layer: "message1"},
	}
	p := New(&ir.Event{Show: ops}, DefaultOptions())
	p.Advance(1, Input{})
	f := p.Frame()
	if len(f.Texts) != 1 || f.Texts[0].Layer != "message0" {
		t.Fatalf("message1 应已被单独清掉，实际 %+v", f.Texts)
	}
	lines := f.Texts[0].Lines
	if len(lines) != 2 || lines[0].Text != "甲乙" || lines[1].Text != "丙" {
		t.Fatalf("换行语义不对：%+v", lines)
	}
}

func TestLayerCmdWithoutVisibleKeepsVisibility(t *testing.T) {
	ops := []ir.ShowOp{
		{TMs: 0, Kind: ir.ShowLayerShow, Layer: "layer3", Ref: "st"},
		{TMs: 0, Kind: ir.ShowLayerOpt, Layer: "layer3", Attr: map[string]string{"left": "100"}},
	}
	p := New(&ir.Event{Show: ops}, DefaultOptions())
	p.Advance(1, Input{})
	st := layerByName(p.Frame(), "layer3")
	if st == nil || !st.Visible {
		t.Fatalf("不带 visible 的 [layopt] 不应改可见性：%+v", st)
	}
	if !near(st.X, 100, 1e-9) || !st.HasPos {
		t.Fatalf("left 应写进层几何：%+v", st)
	}
}

// ---------------------------------------------------------------
// 抖动
// ---------------------------------------------------------------

func TestQuakeOffset(t *testing.T) {
	ops := []ir.ShowOp{
		{TMs: 0, Kind: ir.ShowLayerShow, Layer: "base", Ref: "bg"},
		{TMs: 0, Kind: ir.ShowQuake, DurMs: 400, Attr: map[string]string{"vmax": "10", "count": "2"}},
	}
	p := New(&ir.Event{Show: ops}, DefaultOptions())
	p.Advance(100, Input{})
	p.Advance(100, Input{}) // 抖动指令在上一刻才开始，推进到 1/4 处
	if y := p.Frame().QuakeY; y == 0 {
		t.Fatalf("抖动进行中应产生偏移")
	}
	p.Advance(400, Input{})
	if y := p.Frame().QuakeY; y != 0 {
		t.Fatalf("抖动结束后偏移应归零，实际 %d", y)
	}
}

// ---------------------------------------------------------------
// 素材名解析（storage 名 → 原始路径）
// ---------------------------------------------------------------

func TestResolveRefStorageName(t *testing.T) {
	ev := &ir.Event{Assets: []ir.AssetRef{
		{OrigPath: "raw/data/image/kaigan01_bg.jpg"},
		{OrigPath: "raw/data/fgimage/neneko01.png"},
	}}
	cases := map[string]string{
		"kaigan01_bg":                   "raw/data/image/kaigan01_bg.jpg",
		"kaigan01_bg.jpg":               "raw/data/image/kaigan01_bg.jpg",
		"neneko01":                      "raw/data/fgimage/neneko01.png",
		"raw/data/fgimage/neneko01.png": "raw/data/fgimage/neneko01.png",
		"unknown":                       "unknown", // 匹配不上原样返回，交给 Loader
	}
	for in, want := range cases {
		if got := ResolveRef(ev, in); got != want {
			t.Fatalf("ResolveRef(%q) = %q，期望 %q", in, got, want)
		}
	}
}

// ---------------------------------------------------------------
// 画面合成
// ---------------------------------------------------------------

func TestComposeZOrderAndMissing(t *testing.T) {
	f := Frame{Layers: []LayerState{
		{Layer: "layer3", Resolved: "st.png", Visible: true, Opacity: 1},
		{Layer: "base", Resolved: "bg.png", Visible: true, Opacity: 1},
		{Layer: "layer9", Resolved: "missing.png", Visible: true, Opacity: 1}, // 素材缺失
		{Layer: "layer4", Resolved: "hidden.png", Visible: false, Opacity: 1},
	}}
	load := solidLoader(map[string]color.RGBA{
		"bg.png": {R: 0, G: 0, B: 255, A: 255},
		"st.png": {R: 255, G: 0, B: 0, A: 255},
	})
	img, drawn, err := Compose(f, load, 8, 8)
	if err != nil {
		t.Fatalf("合成失败：%v", err)
	}
	if len(drawn) != 3 || drawn[0] != "base←bg.png" || drawn[1] != "layer3←st.png" {
		t.Fatalf("绘制顺序不对（应 base 在下、layer3 在上、hidden 不画）：%v", drawn)
	}
	if !strings.Contains(drawn[2], "missing.png") {
		t.Fatalf("缺素材的层应被登记：%v", drawn)
	}
	if got := img.RGBAAt(4, 4); got.R != 255 || got.B != 0 {
		t.Fatalf("z 序不对：layer3（红）应盖住 base（蓝），实际 %+v", got)
	}
}

func TestDrawBoxBackgroundFrameAndColor(t *testing.T) {
	// 有框图：按原尺寸贴到 (X,Y)
	base := solid(16, 16, color.RGBA{R: 255, G: 255, B: 255, A: 255})
	frameImg := solid(4, 4, color.RGBA{R: 255, G: 0, B: 0, A: 255})
	box := TextBox{Layer: "message0", Frame: "win3.png", X: 2, Y: 2, Visible: true}
	DrawBoxBackground(base, box, frameImg, 0, 0)
	if got := base.RGBAAt(3, 3); got.R != 255 || got.G != 0 {
		t.Fatalf("框图未贴上：%+v", got)
	}
	if got := base.RGBAAt(0, 0); got.R != 255 || got.G != 255 {
		t.Fatalf("框图外的像素不该被改：%+v", got)
	}

	// 无框图 + frameOpacity：按 frameColor 混色
	base2 := solid(16, 16, color.RGBA{R: 255, G: 255, B: 255, A: 255})
	box2 := TextBox{Layer: "message0", FrameColor: [3]uint8{0, 0, 0}, FrameOpacity: 128,
		X: 0, Y: 0, W: 8, H: 8, HasGeom: true, Visible: true}
	DrawBoxBackground(base2, box2, nil, 0, 0)
	if got := base2.RGBAAt(4, 4); !near(float64(got.R), 127, 2) {
		t.Fatalf("半透明黑底应约为 127，实际 %+v", got)
	}
	if got := base2.RGBAAt(12, 12); got.R != 255 {
		t.Fatalf("框外不该被改：%+v", got)
	}
}

// TestVideoPlayBlocksForDuration 影片没有台本时长时按 ffprobe 查到的时长阻塞。
//
// 不这样做的话，OP/ED 只有 3 条指令（movie + next），播放器会立刻走完，
// 影片一帧都显示不出来。
func TestVideoPlayBlocksForDuration(t *testing.T) {
	ev := &ir.Event{Show: []ir.ShowOp{
		{TMs: 0, Kind: ir.ShowVideoPlay, Layer: "video", Ref: "op"},
	}}
	opt := DefaultOptions()
	opt.VideoDur = func(ref string) int64 { return 2000 }
	p := New(ev, opt)

	for i := 0; i < 50; i++ { // ≈800ms
		p.Advance(16, Input{})
	}
	if p.Done() {
		t.Fatal("影片时长 2s，800ms 时不应结束")
	}
	if f := p.Frame(); f.Video == nil || f.Video.Ref != "op" {
		t.Fatalf("帧里应带影片信息：%+v", f.Video)
	}
	for i := 0; i < 100; i++ { // 推进到 ≈2.4s
		p.Advance(16, Input{})
	}
	if !p.Done() {
		t.Fatal("超过影片时长后应结束")
	}
}
