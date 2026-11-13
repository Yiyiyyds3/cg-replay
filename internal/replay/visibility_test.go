package replay

import (
	"errors"
	"image"
	"image/color"
	"testing"

	"ucgr/internal/ir"
	"ucgr/internal/vision"
)

func solidVisionImg(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

// 入场淡入首帧是纯黑，但后面有 CG：CheckEventVisibility 不能报错。
func TestCheckEventVisibilityBlackIntroThenCG(t *testing.T) {
	ev := &ir.Event{
		EventID: "EV-black-intro",
		EndMs:   3000,
		Show: []ir.ShowOp{
			{TMs: 0, Kind: ir.ShowLayerShow, Layer: "base", Ref: "black.png"},
			{TMs: 0, Kind: ir.ShowWait, DurMs: 1000},
			{TMs: 2000, Kind: ir.ShowLayerShow, Layer: "base", Ref: "cg.png"},
		},
	}
	load := func(rel string) (image.Image, error) {
		switch rel {
		case "black.png":
			return solidVisionImg(80, 45, color.RGBA{A: 255}), nil
		case "cg.png":
			return solidVisionImg(80, 45, color.RGBA{R: 240, G: 160, B: 80, A: 255}), nil
		default:
			return solidVisionImg(80, 45, color.RGBA{A: 255}), nil
		}
	}
	r, err := CheckEventVisibility(ev, load, nil, 80, 45, 3)
	if err != nil {
		t.Fatalf("首帧纯黑的正常事件被拒绝：%v（%+v）", err, r)
	}
	if !r.HasPicture || r.AllBlack {
		t.Fatalf("应看到后续 CG：%+v", r)
	}
	if r.Stats[0].HasPicture {
		t.Fatalf("首帧本就该是纯黑：%+v", r.Stats[0])
	}
}

// 只有 [wait] 推进时间、EndMs 为 0 的台本也必须跨过等待采样。
// 这是「所有采样点都落在 t=0 入场黑幕」的回归点。
func TestCheckEventVisibilityUsesWaitDurations(t *testing.T) {
	ev := &ir.Event{
		EventID: "EV-wait-only",
		Show: []ir.ShowOp{
			{TMs: 0, Kind: ir.ShowLayerShow, Layer: "base", Ref: "black.png"},
			{TMs: 0, Kind: ir.ShowWait, DurMs: 1000},
			{TMs: 0, Kind: ir.ShowLayerShow, Layer: "base", Ref: "cg.png"},
		},
	}
	load := func(rel string) (image.Image, error) {
		if rel == "cg.png" {
			return solidVisionImg(80, 45, color.RGBA{R: 240, G: 160, B: 80, A: 255}), nil
		}
		return solidVisionImg(80, 45, color.RGBA{A: 255}), nil
	}
	r, err := CheckEventVisibility(ev, load, nil, 80, 45, 3)
	if err != nil {
		t.Fatalf("等待后出现的画面不应被判全黑：%v（%+v）", err, r)
	}
	if !r.HasPicture || r.AllBlack {
		t.Fatalf("应采样到等待后的 CG：%+v", r)
	}
}

// 全黑事件在足够采样后如实报错，并且不 panic。
func TestCheckEventVisibilityAllBlack(t *testing.T) {
	ev := &ir.Event{
		EventID: "EV-all-black",
		EndMs:   3000,
		Show: []ir.ShowOp{
			{TMs: 0, Kind: ir.ShowLayerShow, Layer: "base", Ref: "black.png"},
		},
	}
	load := func(string) (image.Image, error) {
		return solidVisionImg(80, 45, color.RGBA{A: 255}), nil
	}
	r, err := CheckEventVisibility(ev, load, nil, 80, 45, 3)
	if !errors.Is(err, vision.ErrAllBlack) {
		t.Fatalf("全黑事件应报 ErrAllBlack，实际 %v（%+v）", err, r)
	}
	if !r.AllBlack {
		t.Fatalf("报告应标记 AllBlack：%+v", r)
	}
}
