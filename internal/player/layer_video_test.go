package player

import (
	"image"
	"image/color"
	"strings"
	"testing"

	"ucgr/internal/ir"
)

// TestLayerVideo 图层式影片（KiriKiri/Artemis 的 movie 图层）要按 z 序画进去：
// 影片当图片加载必然失败，必须走视频取帧；且压在它上面的图层不能被盖掉。
func TestLayerVideo(t *testing.T) {
	ev := &ir.Event{Show: []ir.ShowOp{
		{TMs: 0, Kind: ir.ShowLayerShow, Layer: "layer0", Ref: "bg.png"},
		{TMs: 0, Kind: ir.ShowLayerShow, Layer: "layer1", Ref: "op.mpg"},
		{TMs: 0, Kind: ir.ShowLayerShow, Layer: "layer2", Ref: "fg.png"},
	}}
	p := New(ev, DefaultOptions())
	for i := 0; i < 5; i++ {
		p.Advance(16, Input{})
	}

	solid := func(c color.RGBA) image.Image {
		img := image.NewRGBA(image.Rect(0, 0, 100, 100))
		for y := 0; y < 100; y++ {
			for x := 0; x < 100; x++ {
				img.SetRGBA(x, y, c)
			}
		}
		return img
	}
	load := func(rel string) (image.Image, error) {
		switch rel {
		case "bg.png":
			return solid(color.RGBA{255, 0, 0, 255}), nil // 红
		case "fg.png":
			return solid(color.RGBA{0, 255, 0, 255}), nil // 绿（最上层）
		}
		return nil, errNotFound{rel}
	}
	gotVideoAt := int64(-1)
	video := func(ref string, tMs int64) (image.Image, error) {
		if ref != "op.mpg" {
			t.Errorf("取帧的引用不对：%s", ref)
		}
		gotVideoAt = tMs
		return solid(color.RGBA{0, 0, 255, 255}), nil // 蓝
	}

	img, drawn, err := RenderFrameWithVideo(p, load, video, 100, 100)
	if err != nil {
		t.Fatalf("渲染失败：%v", err)
	}
	if gotVideoAt < 0 {
		t.Fatalf("影片图层没有取帧（drawn=%v）", drawn)
	}
	// 最上层是 fg（绿），影片被压住 → 中心应是绿，不是蓝也不是红
	r, g, b, _ := img.At(50, 50).RGBA()
	if r>>8 != 0 || g>>8 != 255 || b>>8 != 0 {
		t.Fatalf("中心应是最上层的绿色，实际 (%d,%d,%d)", r>>8, g>>8, b>>8)
	}
}

type errNotFound struct{ ref string }

func (e errNotFound) Error() string { return "找不到：" + e.ref }

// TestVideoMissing 有影片指令但脚本没给文件名时，要明确报「定位不到」，不能静默黑屏。
func TestVideoMissing(t *testing.T) {
	ev := &ir.Event{Show: []ir.ShowOp{
		{TMs: 0, Kind: ir.ShowVideoPlay, Attr: map[string]string{"state": "end", "unresolved": "1"}},
	}}
	p := New(ev, DefaultOptions())
	p.Advance(16, Input{})

	img, drawn, err := RenderFrameWithVideo(p, func(string) (image.Image, error) {
		return nil, errNotFound{"x"}
	}, func(string, int64) (image.Image, error) {
		t.Fatal("不该去取帧（没有文件名）")
		return nil, nil
	}, 10, 10)
	if err != nil {
		t.Fatalf("渲染失败：%v", err)
	}
	_ = img
	found := false
	for _, d := range drawn {
		if strings.Contains(d, "state=end") {
			found = true
		}
	}
	if !found {
		t.Fatalf("应报出「定位不到影片文件」，实际 %v", drawn)
	}
}
