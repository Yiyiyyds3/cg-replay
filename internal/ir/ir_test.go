package ir

import "testing"

// TestHasVideo 影片判定只有这一处口径：指令式（CS2）与图层式（KiriKiri/Artemis）都算。
func TestHasVideo(t *testing.T) {
	if HasVideo(nil) {
		t.Fatal("nil 不该算影片")
	}
	// 指令式
	ev := &Event{Show: []ShowOp{{Kind: ShowVideoPlay, Ref: "op.mpg"}}}
	if !HasVideo(ev) {
		t.Fatal("video_play 指令应算影片")
	}
	// 图层式
	ev = &Event{Layers: []Layer{{LayerID: "mv", Kind: LayerMovie}}}
	if !HasVideo(ev) {
		t.Fatal("movie 图层应算影片")
	}
	// 都没有
	ev = &Event{Show: []ShowOp{{Kind: ShowTextRun}}, Layers: []Layer{{Kind: LayerBackground}}}
	if HasVideo(ev) {
		t.Fatal("普通演出不该算影片")
	}
}
