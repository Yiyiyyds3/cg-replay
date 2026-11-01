package unitycg

import (
	"testing"

	"ucgr/internal/ir"
)

// TestBuildShowFrames 逐帧 CG：按文件名里的数字排序，每帧一条 layer_show + wait。
func TestBuildShowFrames(t *testing.T) {
	cg := CG{ID: "Witch01", Kind: "frames"}
	cg.Frames = []string{"10.png", "2.png", "1.png"}
	ev, err := BuildShow(cg)
	if err != nil {
		t.Fatalf("BuildShow: %v", err)
	}
	if len(ev.Show) != 6 { // 3 帧 × (show + wait)
		t.Fatalf("台本应为 6 条，实际 %d", len(ev.Show))
	}
	want := []string{"cg/Witch01/1.png", "cg/Witch01/2.png", "cg/Witch01/10.png"}
	var got []string
	for _, op := range ev.Show {
		if op.Kind == ir.ShowLayerShow {
			got = append(got, op.Ref)
		}
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("帧顺序不对：want %v, got %v", want, got)
		}
	}
	if ev.EndMs <= 0 {
		t.Fatal("EndMs 应大于 0")
	}
}

// TestBuildShowStill 单图 CG：一条 show + 一条长 wait。
func TestBuildShowStill(t *testing.T) {
	cg := CG{ID: "OpeningCG00", Kind: "still"}
	cg.Files.PNG = []string{"OpeningCG00.png"}
	ev, err := BuildShow(cg)
	if err != nil {
		t.Fatalf("BuildShow: %v", err)
	}
	if len(ev.Show) != 2 {
		t.Fatalf("单图台本应为 2 条，实际 %d", len(ev.Show))
	}
	if ev.Show[1].DurMs < 1000 {
		t.Fatalf("单图停留应 >= 1s，实际 %d ms", ev.Show[1].DurMs)
	}
}

// TestBuildShowSpine 骨骼动画还没接渲染：要明确报错，不能返回空台本装没事。
func TestBuildShowSpine(t *testing.T) {
	cg := CG{ID: "HCG02", Kind: "spine"}
	cg.Files.Text = []string{"HCG_02.json"}
	cg.Spine = &SpineInfo{Animations: []AnimRef{{Name: "1(loop)", Loop: true, Dur: 6}}}
	if _, err := BuildShow(cg); err == nil {
		t.Fatal("spine 类型应当返回错误（渲染器未接入）")
	}
}

// TestPlayable 只有 frames/still 现在能放。
func TestPlayable(t *testing.T) {
	for k, want := range map[string]bool{"frames": true, "still": true, "spine": false} {
		if got := Playable(CG{Kind: k}); got != want {
			t.Fatalf("%s: Playable=%v，应为 %v", k, got, want)
		}
	}
}
