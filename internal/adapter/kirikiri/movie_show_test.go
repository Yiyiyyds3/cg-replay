package kirikiri

import (
	"testing"

	"ucgr/internal/adapter"
	"ucgr/internal/ir"
)

// TestMovieToShow KiriKiri 的影片指令要进台本，且能区分「播片」与「关掉视频图层」。
//
// 这里曾经整条丢掉：[movie] 没有分支 → OP/ED 点开必然黑屏（台本里根本没有影片）。
func TestMovieToShow(t *testing.T) {
	build := func(ks string) *ir.Event {
		t.Helper()
		a := &Adapter{}
		parsed, err := a.ParseScript([]byte(ks), &adapter.ScriptCtx{EntryPath: "op.ks"})
		if err != nil {
			t.Fatalf("解析失败：%v", err)
		}
		ev, err := BuildShowFromAst(parsed.(*Ast), "", "op.ks")
		if err != nil || ev == nil {
			t.Fatalf("BuildShowFromAst: %v", err)
		}
		return ev
	}
	videos := func(ev *ir.Event) []string {
		var out []string
		for _, op := range ev.Show {
			if op.Kind == ir.ShowVideoPlay {
				out = append(out, op.Ref)
			}
		}
		return out
	}

	// ① [movie] → 播片
	ev := build("*start\r\n[movie storage=\"op.mpg\"]\r\n[wait time=100]\r\n")
	if got := videos(ev); len(got) != 1 || got[0] != "op.mpg" {
		t.Fatalf("[movie] 应产出 1 条影片指令，实际 %v", got)
	}
	if !ir.HasVideo(ev) {
		t.Fatal("Ir.HasVideo 应认出台本里的影片")
	}

	// ② [video visible=false] → 只是关图层，不是播片
	ev = build("*start\r\n[video visible=false]\r\n[wait time=100]\r\n")
	if got := videos(ev); len(got) != 0 {
		t.Fatalf("[video visible=false] 不该产出影片指令，实际 %v", got)
	}

	// ③ [video storage=...] → 播片
	ev = build("*start\r\n[video storage=\"bg.mpg\" visible=true]\r\n[wait time=100]\r\n")
	if got := videos(ev); len(got) != 1 || got[0] != "bg.mpg" {
		t.Fatalf("[video storage] 应产出 1 条影片指令，实际 %v", got)
	}
}
