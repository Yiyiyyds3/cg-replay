package player

import (
	"math"
	"testing"

	"ucgr/internal/ir"
)

func TestLayerScaleAndFade(t *testing.T) {
	ev := &ir.Event{Show: []ir.ShowOp{
		{Kind: ir.ShowLayerShow, Layer: "bg0", Ref: "bg01",
			Attr: map[string]string{"scale": "0.5", "opacity": "255"}},
		{Kind: ir.ShowLayerAttr, Layer: "bg0", DurMs: 100,
			Attr: map[string]string{"from": "0", "to": "255", "opacity": "255"}},
	}}
	p := New(ev, Options{})
	p.Advance(1, Input{})
	f := p.Frame()
	if len(f.Layers) != 1 {
		t.Fatalf("layers = %d, want 1", len(f.Layers))
	}
	if f.Layers[0].Scale != 0.5 {
		t.Fatalf("scale = %v, want 0.5", f.Layers[0].Scale)
	}
	// fade 刚执行：从 0 开始。
	if got := f.Layers[0].Opacity; got != 0 {
		t.Fatalf("fade start opacity = %v, want 0", got)
	}
	p.Advance(50, Input{})
	if got := p.Frame().Layers[0].Opacity; math.Abs(got-0.5) > 1e-6 {
		t.Fatalf("fade mid opacity = %v, want 0.5", got)
	}
	p.Advance(50, Input{})
	if got := p.Frame().Layers[0].Opacity; got != 1 {
		t.Fatalf("fade end opacity = %v, want 1", got)
	}
}
