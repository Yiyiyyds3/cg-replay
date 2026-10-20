package catsystem2

import (
	"testing"

	"ucgr/internal/adapter"
	"ucgr/internal/ir"
)

func astFromCommands(cmds []Command) *Ast {
	a := &Ast{Encoding: "utf-8", Commands: cmds}
	for i := range a.Commands {
		a.Seq = append(a.Seq, SeqItem{Kind: "cmd", Cmd: &a.Commands[i]})
	}
	return a
}

func TestLayerRef(t *testing.T) {
	tests := []struct {
		name string
		cmd  Command
		want string
	}{
		{"bg image", Command{Tag: "bg", Args: []string{"0", "bg03_smr_b"}}, "bg03_smr_b"},
		{"bg scale", Command{Tag: "bg", Args: []string{"0", "scale", "63%", "63%"}}, ""},
		{"bg pos", Command{Tag: "bg", Args: []string{"0", "pos", "-80", "-410"}}, ""},
		{"bg m2fade", Command{Tag: "bg", Args: []string{"0", "m2fade"}}, ""},
		{"cg arc", Command{Tag: "cg", Args: []string{"2", "arc", "48"}}, ""},
		{"eg image", Command{Tag: "eg", Args: []string{"1", "bgeffect03"}}, "bgeffect03"},
		{"eg image with offset", Command{Tag: "eg", Args: []string{"2", "bg36_smr_a", "-600"}}, "bg36_smr_a"},
		{"comma argument", Command{Tag: "bg", Args: []string{"0", "bgwhite,$str1"}}, "bgwhite"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := layerRef(&tt.cmd); got != tt.want {
				t.Fatalf("layerRef(%q) = %q, want %q", tt.cmd.Raw, got, tt.want)
			}
		})
	}
}

func TestExtractRefsSkipsTransformsAndEmote(t *testing.T) {
	ast := astFromCommands([]Command{
		{Tag: "bg", Args: []string{"0", "scale", "63%", "63%"}},
		{Tag: "bg", Args: []string{"0", "bg03_smr_b"}},
		{Tag: "bg", Args: []string{"0", "pos", "-80", "-410"}},
		{Tag: "cg", Args: []string{"2", "pos", "#570+100"}},
		{Tag: "eg", Args: []string{"1", "bgeffect03"}},
		{Tag: "fw", Args: []string{"0", "FaChit01,$str338,$str303,a"}},
		{Tag: "mpl", Args: []string{"4", "stchit01l,$str303,1,0"}},
		{Tag: "pl", Args: []string{"16", "pos", "1300"}},
		{Tag: "pcm", Args: []string{"0", "end", "90"}},
		{Tag: "se", Args: []string{"0", "end", "90"}},
		{Tag: "bgm", Args: []string{"1", "vol", "0"}},
		{Tag: "pcm", Args: []string{"AA_01_YU01_001"}},
	})
	got := ast.ExtractRefs()
	// 语音除原名外还登记去 take 后缀的变体（DR/原型同口径）。
	want := []string{"bg03_smr_b", "bgeffect03", "AA_01_YU01_001", "AA_01_YU01"}
	if len(got) != len(want) {
		t.Fatalf("ExtractRefs() = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ExtractRefs()[%d] = %q, want %q (all: %#v)", i, got[i], want[i], got)
		}
	}
}

func TestBuildIRFade(t *testing.T) {
	ast := astFromCommands([]Command{
		{Tag: "bg", Args: []string{"0", "bg03_smr_b"}},
		{Tag: "bg", Args: []string{"0", "fade", "48", "0", "255"}},
	})
	resolve := func(ref string) (*ir.AssetRef, bool) {
		return &ir.AssetRef{Hash: "hash-" + ref, OrigPath: "image00/" + ref + ".hg3", Role: "其它"}, true
	}
	out, err := (&Adapter{}).BuildIR(ast, &adapter.BuildCtx{GameID: "test", Resolver: resolve})
	if err != nil {
		t.Fatal(err)
	}
	ev := &out.Events[0]
	if len(ev.Timeline) != 2 {
		t.Fatalf("timeline = %d ops, want 2: %#v", len(ev.Timeline), ev.Timeline)
	}
	op := ev.Timeline[1]
	if op.Op != "layer_fade" || op.Target != "bg0" {
		t.Fatalf("fade op = %s(%s), want layer_fade(bg0)", op.Op, op.Target)
	}
	p, _ := op.Params.(map[string]interface{})
	if p["from"] != 0.0 || p["to"] != 1.0 || p["duration_ms"] != 768 || p["opacity"] != 1.0 {
		t.Fatalf("fade params = %#v", p)
	}
	if out.HasEmote || ev.HasEmote {
		t.Fatalf("static bg should not set has_emote: ir:%v event:%v", out.HasEmote, ev.HasEmote)
	}
}

func TestBuildIRStaticLayersAndEmote(t *testing.T) {
	ast := astFromCommands([]Command{
		{Tag: "bg", Args: []string{"0", "scale", "63%", "63%"}},
		{Tag: "bg", Args: []string{"0", "bg03_smr_b"}},
		{Tag: "bg", Args: []string{"0", "pos", "-80", "-410"}},
		{Tag: "bg", Args: []string{"1", "bgwhite"}},
		{Tag: "eg", Args: []string{"1", "bgeffect03"}},
		{Tag: "fw", Args: []string{"0", "FaChit01,$str338,$str303,a"}},
		{Tag: "mpl", Args: []string{"4", "stchit01l,$str303,1,0"}},
	})
	resolve := func(ref string) (*ir.AssetRef, bool) {
		switch ref {
		case "bg03_smr_b", "bgwhite", "bgeffect03":
			return &ir.AssetRef{Hash: "hash-" + ref, OrigPath: "image00/" + ref + ".hg3", Role: "其它"}, true
		}
		return nil, false
	}
	a := &Adapter{}
	out, err := a.BuildIR(ast, &adapter.BuildCtx{GameID: "test", Resolver: resolve})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Events) != 1 {
		t.Fatalf("events = %d, want 1", len(out.Events))
	}
	ev := &out.Events[0]
	if len(ev.Assets) != 3 {
		t.Fatalf("assets = %#v, want 3", ev.Assets)
	}
	seenLayer := map[string]bool{}
	for _, l := range ev.Layers {
		seenLayer[l.LayerID] = true
		if l.Visible {
			t.Fatalf("layer %s should start hidden", l.LayerID)
		}
	}
	for _, id := range []string{"bg0", "bg1", "eg1"} {
		if !seenLayer[id] {
			t.Fatalf("missing layer %s (got %#v)", id, seenLayer)
		}
	}
	if out.CanvasW != 1600 || out.CanvasH != 900 {
		t.Fatalf("canvas = %dx%d, want 1600x900", out.CanvasW, out.CanvasH)
	}
	if !out.HasEmote || !ev.HasEmote {
		t.Fatalf("has_emote = ir:%v event:%v, want true (fw/mpl)", out.HasEmote, ev.HasEmote)
	}
	if len(ev.Timeline) != 5 {
		t.Fatalf("timeline = %d ops, want 5: %#v", len(ev.Timeline), ev.Timeline)
	}
	want := []struct {
		op, target string
		params     map[string]interface{}
	}{
		{"layer_set", "bg0", map[string]interface{}{"scale": 0.63}},
		{"image_show", "bg0", map[string]interface{}{"ref": "bg03_smr_b", "scale": 0.63}},
		{"layer_set", "bg0", map[string]interface{}{"left": float64(-80), "top": float64(-410)}},
		{"image_show", "bg1", map[string]interface{}{"ref": "bgwhite"}},
		{"image_show", "eg1", map[string]interface{}{"ref": "bgeffect03"}},
	}
	for i, w := range want {
		op := ev.Timeline[i]
		if op.Op != w.op || op.Target != w.target {
			t.Fatalf("timeline[%d] = %s(%s), want %s(%s)", i, op.Op, op.Target, w.op, w.target)
		}
		p, _ := op.Params.(map[string]interface{})
		for k, v := range w.params {
			if p[k] != v {
				t.Fatalf("timeline[%d].%s = %#v, want %#v (all: %#v)", i, k, p[k], v, p)
			}
		}
		ref, _ := p["ref"].(string)
		switch ref {
		case "63%", "-80", "#570+100", "FaChit01", "stchit01l":
			t.Fatalf("bad ref %q entered timeline: %#v", ref, op)
		}
	}
	if len(ev.Unrecognized) != 2 {
		t.Fatalf("unrecognized = %d, want 2 (fw/mpl): %#v", len(ev.Unrecognized), ev.Unrecognized)
	}
	for _, u := range ev.Unrecognized {
		if u.Reason != "E-mote/动态立绘（不复刻）" {
			t.Fatalf("unrecognized reason = %q", u.Reason)
		}
	}
}
