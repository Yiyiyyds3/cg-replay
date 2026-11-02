package catsystem2

import (
	"testing"

	"ucgr/internal/ir"
)

func TestBuildShowFromLayerOps(t *testing.T) {
	ev := &ir.Event{Timeline: []ir.TimelineOp{
		{TMs: 10, Op: "layer_set", Target: "bg0",
			Params: map[string]interface{}{"scale": 0.5, "opacity": 0.5}},
		{TMs: 20, Op: "layer_fade", Target: "bg0",
			Params: map[string]interface{}{"from": 0.0, "to": 1.0, "opacity": 1.0, "duration_ms": 320}},
		{TMs: 30, Op: "image_show", Target: "bg0",
			Params: map[string]interface{}{"ref": "bg01", "opacity": 0.5}},
	}}
	ops := BuildShowFromEvent(ev)
	if len(ops) != 3 {
		t.Fatalf("ops = %d, want 3: %#v", len(ops), ops)
	}
	if ops[0].Kind != ir.ShowLayerAttr || ops[0].Attr["scale"] != "0.5" {
		t.Fatalf("layer_set = %#v", ops[0])
	}
	// CS2 的 alpha 是 0~1，播放器按 0~255 解释：转换侧必须乘 255。
	if ops[0].Attr["opacity"] != "127.5" {
		t.Fatalf("opacity = %q, want 127.5", ops[0].Attr["opacity"])
	}
	if ops[1].Kind != ir.ShowLayerAttr || ops[1].DurMs != 320 {
		t.Fatalf("layer_fade = %#v", ops[1])
	}
	if ops[1].Attr["from"] != "0" || ops[1].Attr["to"] != "255" {
		t.Fatalf("fade from/to = %q/%q, want 0/255", ops[1].Attr["from"], ops[1].Attr["to"])
	}
	if ops[2].Kind != ir.ShowLayerShow || ops[2].Ref != "bg01" || ops[2].Attr["opacity"] != "127.5" {
		t.Fatalf("image_show = %#v", ops[2])
	}
}
