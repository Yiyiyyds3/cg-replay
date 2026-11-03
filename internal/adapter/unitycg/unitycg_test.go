package unitycg

import (
	"os"
	"path/filepath"
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

// TestSortFramesWithCGNumber CG 名自带编号时，必须按末尾帧号排，而不是字典序。
func TestSortFramesWithCGNumber(t *testing.T) {
	got := sortFrames([]string{"Witch01_10.png", "Witch01_2.png", "Witch01_1.png"})
	want := []string{"Witch01_1.png", "Witch01_2.png", "Witch01_10.png"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("帧顺序不对：want %v, got %v", want, got)
		}
	}
}

// TestBuildShowRejectsTraversal 索引/帧名里的 .. 不能拼进 root 之外。
func TestBuildShowRejectsTraversal(t *testing.T) {
	cg := CG{ID: "Witch01", Kind: "frames", Frames: []string{"../evil.png", "2.png"}}
	if _, err := BuildShow(cg); err == nil {
		t.Fatal("带 ../ 的帧名应被拒绝")
	}
	cg = CG{ID: "../Witch01", Kind: "still"}
	cg.Files.PNG = []string{"1.png"}
	if _, err := BuildShow(cg); err == nil {
		t.Fatal("带 ../ 的 CG id 应被拒绝")
	}
}

func TestLoadIndexRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	data := `{"schema":1,"cgs":[{"id":"HCG01","kind":"frames","frames":["../evil.png"]}]}`
	if err := os.WriteFile(filepath.Join(root, IndexFile), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadIndex(root); err == nil {
		t.Fatal("索引里的 ../ 应被拒绝")
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

// TestBuildShowSpine 骨骼动画：player 只拿一条等待撑时间轴，画面交给前端。
func TestBuildShowSpine(t *testing.T) {
	cg := CG{ID: "HCG02", Kind: "spine"}
	cg.Files.Text = []string{"HCG_02.json"}
	cg.Spine = &SpineInfo{Animations: []AnimRef{{Name: "1(loop)", Loop: true, Dur: 6}}}
	ev, err := BuildShow(cg)
	if err != nil {
		t.Fatalf("BuildShow: %v", err)
	}
	if len(ev.Show) != 1 || ev.Show[0].Kind != ir.ShowWait {
		t.Fatalf("spine 台本应只有一条 wait，实际 %+v", ev.Show)
	}
	if ev.Show[0].DurMs != 6000 || ev.EndMs != 6000 {
		t.Fatalf("循环动画应播满 6s，实际 wait=%d end=%d", ev.Show[0].DurMs, ev.EndMs)
	}
}

// TestPickSpineAnimation 优先 idle，时长按动画自身/循环下限计算。
func TestPickSpineAnimation(t *testing.T) {
	cg := CG{Kind: "spine"}
	cg.Spine = &SpineInfo{Animations: []AnimRef{
		{Name: "start", Dur: 2},
		{Name: "Idle_loop", Loop: true, Dur: 1.2},
	}}
	a, ok := cg.PickSpineAnimation()
	if !ok || a.Name != "Idle_loop" {
		t.Fatalf("应优先选 idle，实际 %+v ok=%v", a, ok)
	}
	if got := cg.SpineDurationMs(); got != 5000 {
		t.Fatalf("循环动画时长下限应为 5000ms，实际 %d", got)
	}
	cg.Spine.Animations = nil
	if got := cg.SpineDurationMs(); got != 3000 {
		t.Fatalf("无动画统计应退回 3000ms，实际 %d", got)
	}
}

// TestPlayable spine 由前端渲染；缺素材时仍不可播。
func TestPlayable(t *testing.T) {
	for k, want := range map[string]bool{"frames": true, "still": true, "unknown": false} {
		if got := Playable(CG{Kind: k}); got != want {
			t.Fatalf("%s: Playable=%v，应为 %v", k, got, want)
		}
	}
	spine := CG{Kind: "spine"}
	if Playable(spine) {
		t.Fatal("缺 skeleton/贴图的 spine 不应可播")
	}
	spine.Files.Text = []string{"HCG_02.json", "HCG_02.atlas"}
	spine.Files.PNG = []string{"HCG_02.png"}
	if !Playable(spine) {
		t.Fatal("素材齐全的 spine 应可播")
	}
}
