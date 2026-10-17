package replay

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"

	"ucgr/internal/ir"
)

// solidImg 造一张 w×h 的纯色图，当作素材（真实解码路径的替身）。
func solidImg(t *testing.T, w, h int, c color.RGBA) image.Image {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	return img
}

// solidPNG 造一张纯色 PNG 的字节（验证 DecodeImage 用）。
func solidPNG(t *testing.T, w, h int, c color.RGBA) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, solidImg(t, w, h, c)); err != nil {
		t.Fatalf("PNG 编码失败: %v", err)
	}
	return buf.Bytes()
}

func f64(v float64) *float64 { return &v }

// 事件：背景层 base（1600×1200）+ 立绘层 layer3（20×20）。
// 时间轴：0ms 显示立绘在 (100,220)，2000ms 隐藏，3000ms 再显示在 (500,600)。
func testEvent() *ir.Event {
	return &ir.Event{
		EventID: "EV1",
		Kind:    ir.KindScene,
		Layers: []ir.Layer{
			{LayerID: "base", Kind: ir.LayerBackground, Z: 0, Visible: true,
				Asset:     &ir.AssetRef{Hash: "h1", OrigPath: "bg.png"},
				Transform: &ir.Transform{X: f64(0), Y: f64(0), Anchor: "top-left"}},
			{LayerID: "layer3", Kind: ir.LayerStand, Z: 3, Visible: true,
				Asset: &ir.AssetRef{Hash: "h2", OrigPath: "st.png"}},
		},
		Timeline: []ir.TimelineOp{
			{TMs: 0, Op: "image_show", Target: "layer3",
				Params: map[string]interface{}{"ref": "st.png", "left": "100", "top": "220"}},
			{TMs: 2000, Op: "image_hide", Target: "layer3"},
			{TMs: 3000, Op: "image_show", Target: "layer3",
				Params: map[string]interface{}{"ref": "st.png", "left": 500.0, "top": 600.0}},
			{TMs: 4000, Op: "text_show", Target: "L1"},
		},
		Texts: []ir.TextLine{{LineID: "L1", TMs: 4000, Text: "台词"}},
	}
}

func TestStatesAtInitialAndTimeline(t *testing.T) {
	ev := testEvent()
	st := StatesAt(ev, 0)
	if len(st) != 2 {
		t.Fatalf("图层数应为 2，实为 %d", len(st))
	}
	m := map[string]LayerState{}
	for _, s := range st {
		m[s.LayerID] = s
	}
	// layer3 的初态：位置由 0ms 的 image_show 给出
	if got := m["layer3"]; !got.Visible || got.X != 100 || got.Y != 220 || !got.HasPos {
		t.Fatalf("t=0 时 layer3 状态错: %+v", got)
	}
	// 初态资产来自 Layer.Asset（事件初态口径）
	if m["base"].Ref != "bg.png" || !m["base"].Visible {
		t.Fatalf("base 初态错: %+v", m["base"])
	}

	// 2000ms：隐藏
	if got := StatesAt(ev, 2000)[1]; got.Visible {
		t.Fatalf("t=2000 时 layer3 应隐藏: %+v", got)
	}
	// 3000ms：重新显示且位置更新（数值型 left/top 也要认）
	got := StatesAt(ev, 3500)[1]
	if !got.Visible || got.X != 500 || got.Y != 600 {
		t.Fatalf("t=3500 时 layer3 状态错: %+v", got)
	}
	// 事件初态不被时间轴污染：再取一次 t=0，仍是初态
	if again := StatesAt(ev, 0)[1]; again.X != 100 {
		t.Fatalf("StatesAt 有副作用: %+v", again)
	}
}

func TestComposeZOrderAndGeometry(t *testing.T) {
	ev := testEvent()
	blue := color.RGBA{R: 0, G: 0, B: 255, A: 255}
	red := color.RGBA{R: 255, G: 0, B: 0, A: 255}
	imgs := map[string]image.Image{
		"bg.png": solidImg(t, 1600, 1200, blue),
		"st.png": solidImg(t, 20, 20, red),
	}
	load := func(rel string) (image.Image, error) { return imgs[rel], nil }

	// t=0：立绘在 (100,220)
	img, drawn, err := Compose(StatesAt(ev, 0), load, 1600, 1200)
	if err != nil {
		t.Fatal(err)
	}
	if len(drawn) != 2 {
		t.Fatalf("t=0 应画 2 层，实为 %v", drawn)
	}
	if c := img.RGBAAt(100, 220); c.R != 255 {
		t.Fatalf("(100,220) 应为立绘: %+v", c)
	}
	if c := img.RGBAAt(500, 600); c.R != 0 || c.B != 255 {
		t.Fatalf("(500,600) 应为背景: %+v", c)
	}

	// t=2000：立绘隐藏 → 只剩背景
	hidden, drawn, err := Compose(StatesAt(ev, 2000), load, 1600, 1200)
	if err != nil {
		t.Fatal(err)
	}
	if len(drawn) != 1 {
		t.Fatalf("t=2000 只应画背景，实为 %v", drawn)
	}
	if c := hidden.RGBAAt(100, 220); c.R != 0 || c.B != 255 {
		t.Fatalf("t=2000 (100,220) 应为背景: %+v", c)
	}

	// t=3000：立绘移到 (500,600)
	big, _, err := Compose(StatesAt(ev, 3000), load, 1600, 1200)
	if err != nil {
		t.Fatal(err)
	}
	if c := big.RGBAAt(100, 220); c.R != 0 {
		t.Fatalf("(100,220) 立绘已移走，应为背景: %+v", c)
	}
	if c := big.RGBAAt(500, 600); c.R != 255 || c.B != 0 {
		t.Fatalf("(500,600) 应被立绘覆盖: %+v", c)
	}

	// 画布裁剪：画布只有 100×100，立绘在 (500,600) 完全在外 → 画上去但画布内无红；
	// 背景 1600×1200 被裁到画布内（同时覆盖「源比画布大」的路径，不会越界写坏内存）
	small, drawn, err := Compose(StatesAt(ev, 3000), load, 100, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(drawn) != 2 {
		t.Fatalf("裁剪用例仍应记录 2 层: %v", drawn)
	}
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			if c := small.RGBAAt(x, y); c.R != 0 || c.B != 255 {
				t.Fatalf("裁剪后 (%d,%d) 应为背景: %+v", x, y, c)
			}
		}
	}
}

func TestStatesAtResolvesStorageName(t *testing.T) {
	// KAG 的 [image storage=kaigan01_bg] 不带扩展名，时间轴上记的是脚本原文，
	// 回放时要对到事件资产表里的相对路径（重返蓝鲸岛 71kaigan01.ks 的真实形态）。
	ev := &ir.Event{
		EventID: "EV3",
		Layers: []ir.Layer{
			{LayerID: "layerbase", Z: 0, Visible: true,
				Asset: &ir.AssetRef{OrigPath: "raw/data/image/kaigan01_bg.jpg"}},
		},
		Assets: []ir.AssetRef{
			{OrigPath: "raw/data/image/kaigan01_bg.jpg"},
			{OrigPath: "raw/data/fgimage/akira12.png"},
		},
		Timeline: []ir.TimelineOp{
			{TMs: 0, Op: "image_show", Target: "layerbase",
				Params: map[string]interface{}{"ref": "kaigan01_bg", "page": "fore"}},
			{TMs: 100, Op: "image_show", Target: "layerbase",
				Params: map[string]interface{}{"ref": "akira12"}},
			{TMs: 200, Op: "image_show", Target: "layerbase",
				Params: map[string]interface{}{"ref": "没这个文件"}},
		},
	}
	if got := StatesAt(ev, 0)[0].Ref; got != "raw/data/image/kaigan01_bg.jpg" {
		t.Fatalf("无扩展名 storage 未解析: %q", got)
	}
	if got := StatesAt(ev, 150)[0].Ref; got != "raw/data/fgimage/akira12.png" {
		t.Fatalf("文件名匹配未生效: %q", got)
	}
	if got := StatesAt(ev, 250)[0].Ref; got != "没这个文件" {
		t.Fatalf("解析不了时应原样保留: %q", got)
	}
}

func TestComposeOpacityAndAutoCanvas(t *testing.T) {
	ev := &ir.Event{
		EventID: "EV2",
		Layers: []ir.Layer{
			{LayerID: "base", Z: 0, Visible: true, Asset: &ir.AssetRef{OrigPath: "bg.png"}},
			{LayerID: "veil", Z: 1, Visible: true,
				Asset:     &ir.AssetRef{OrigPath: "veil.png"},
				Transform: &ir.Transform{Opacity: f64(0.5)}},
		},
	}
	imgs := map[string]image.Image{
		"bg.png":   solidImg(t, 8, 8, color.RGBA{R: 0, G: 0, B: 0, A: 255}),
		"veil.png": solidImg(t, 8, 8, color.RGBA{R: 255, G: 255, B: 255, A: 255}),
	}
	load := func(rel string) (image.Image, error) { return imgs[rel], nil }

	// 画布 0×0 → 自动取 z 最小的可见层尺寸
	img, drawn, err := Compose(StatesAt(ev, 0), load, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(drawn) != 2 {
		t.Fatalf("应画 2 层: %v", drawn)
	}
	if img.Bounds().Dx() != 8 || img.Bounds().Dy() != 8 {
		t.Fatalf("自动画布尺寸错: %v", img.Bounds())
	}
	// 黑底 + 50% 白纱 → 约 127（±2）
	if c := img.RGBAAt(0, 0); c.R < 125 || c.R > 129 {
		t.Fatalf("不透明度合成结果异常: %+v", c)
	}
}

func TestDecodeImagePNG(t *testing.T) {
	data := solidPNG(t, 4, 3, color.RGBA{R: 10, G: 20, B: 30, A: 255})
	img, err := DecodeImage(data)
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 4 || img.Bounds().Dy() != 3 {
		t.Fatalf("尺寸错: %v", img.Bounds())
	}
	r, g, b, _ := img.At(1, 1).RGBA()
	if r>>8 != 10 || g>>8 != 20 || b>>8 != 30 {
		t.Fatalf("像素错: %d %d %d", r>>8, g>>8, b>>8)
	}
}

func TestSampleTimes(t *testing.T) {
	ev := testEvent()
	ts := SampleTimes(ev, 5)
	if len(ts) != 5 {
		t.Fatalf("应 5 帧，实为 %d", len(ts))
	}
	if ts[0] != 0 || ts[4] != 4000 {
		t.Fatalf("采样区间错: %v", ts)
	}
	for i := 1; i < len(ts); i++ {
		if ts[i] < ts[i-1] {
			t.Fatalf("采样应非降序: %v", ts)
		}
	}
}
