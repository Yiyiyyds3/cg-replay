package main

// replay_test.go —— 回放会话层的交互/透传单测（不依赖 GUI 与音频设备）。
//
// 覆盖三件容易回归的事：
//   1. 暂停 + 等待态时 InputClick 仍能推进（点击推进不是「播放」的附属）；
//   2. 文本与语音随帧透传（FrameDTO.Texts / FrameDTO.Voice）；
//   3. 剧情/鉴赏模式切换只换播放器，元信息（mode/dialogue）跟着变。

import (
	"image"
	"testing"
	"time"

	"ucgr/internal/ir"
	"ucgr/internal/player"
)

// newTestSession 建一个不碰磁盘的最小会话（loader 永远返回 1×1）。
func newTestSession(t *testing.T, ev *ir.Event) *replaySession {
	t.Helper()
	s := &replaySession{
		kind:    "scene",
		id:      "test",
		title:   "测试事件",
		sw:      640,
		sh:      480,
		ev:      ev,
		opts:    player.DefaultOptions(),
		playing: false,
		missing: map[string]string{},
	}
	s.opts.StageW, s.opts.StageH = 640, 480
	s.load = func(string) (image.Image, error) {
		return image.NewRGBA(image.Rect(0, 0, 1, 1)), nil
	}
	s.p = player.New(ev, s.opts)
	s.p.Advance(0, player.Input{})
	s.lastTick = time.Now().Add(-16 * time.Millisecond)
	return s
}

// textEvent 三句台词、两个点击点。
func textEvent() *ir.Event {
	return &ir.Event{EventID: "t", Show: []ir.ShowOp{
		{Kind: ir.ShowTextRun, Layer: "message0", Text: &ir.ShowText{Speaker: "竜", Text: "第一句"}},
		{Kind: ir.ShowTextWait},
		{Kind: ir.ShowTextClear},
		{Kind: ir.ShowTextRun, Layer: "message0", Text: &ir.ShowText{Speaker: "竜", Text: "第二句"}},
		{Kind: ir.ShowTextWait},
		{Kind: ir.ShowTextRun, Layer: "message0", Text: &ir.ShowText{Speaker: "竜", Text: "第三句"}},
	}}
}

// lastLine 取当前帧最后一个可见文本框的最后一行（前端「当前台词」同口径）。
func lastLine(f FrameDTO) LineDTO {
	for i := len(f.Texts) - 1; i >= 0; i-- {
		ls := f.Texts[i].Lines
		if len(ls) > 0 {
			return ls[len(ls)-1]
		}
	}
	return LineDTO{}
}

// TestInputClickAdvancesWhilePaused：暂停且停在点击点时，点击必须推进到下一句。
func TestInputClickAdvancesWhilePaused(t *testing.T) {
	s := newTestSession(t, textEvent())
	app := &App{sess: s}

	f1 := s.frameLocked()
	if !f1.Waiting {
		t.Fatalf("首帧应停在点击点，实际 Waiting=%v", f1.Waiting)
	}
	if got := lastLine(f1); got.Text != "第一句" {
		t.Fatalf("首帧台词 = %q，期望 第一句", got.Text)
	}
	if got := lastLine(f1).Speaker; got != "竜" {
		t.Fatalf("说话人 = %q，期望 竜", got)
	}

	if err := app.InputClick(); err != nil {
		t.Fatalf("InputClick: %v", err)
	}
	f2 := s.frameLocked()
	if got := lastLine(f2); got.Text != "第二句" {
		t.Fatalf("点击后台词 = %q，期望 第二句（暂停态点击没生效？）", got.Text)
	}
	if f2.Playing {
		t.Errorf("点击推进不该把会话切回播放态")
	}
	if !f2.Waiting {
		t.Errorf("推进到下一句后应重新停在点击点")
	}
}

// TestPausedWithoutClickHolds：暂停时不点击就不推进（避免「暂停其实还在跑」）。
func TestPausedWithoutClickHolds(t *testing.T) {
	s := newTestSession(t, textEvent())
	f1 := s.frameLocked()
	now1 := f1.Now
	f2 := s.frameLocked()
	if f2.Now != now1 {
		t.Fatalf("暂停态时间前进了 %dms（%d → %d）", f2.Now-now1, now1, f2.Now)
	}
	if got := lastLine(f2); got.Text != "第一句" {
		t.Fatalf("暂停态台词变成 %q", got.Text)
	}
}

// TestInputClickOnTimeWaitDoesNotSkip：停在 [wait] 时间阻塞点时点击不硬跳
// （原作点击只过台词点；硬跳时间轴由 SeekTo 负责）。
func TestInputClickOnTimeWaitDoesNotSkip(t *testing.T) {
	ev := &ir.Event{EventID: "t", Show: []ir.ShowOp{
		{Kind: ir.ShowTextRun, Layer: "message0", Text: &ir.ShowText{Text: "旁白"}},
		{Kind: ir.ShowWait, DurMs: 5000},
		{Kind: ir.ShowTextRun, Layer: "message0", Text: &ir.ShowText{Text: "之后"}},
	}}
	s := newTestSession(t, ev)
	app := &App{sess: s}
	_ = s.frameLocked()
	if err := app.InputClick(); err != nil {
		t.Fatalf("InputClick: %v", err)
	}
	f := s.frameLocked()
	if got := lastLine(f); got.Text != "旁白" {
		t.Fatalf("点击越过了 [wait]，台词变成 %q", got.Text)
	}
}

// TestVoiceAndTextPassthrough：语音 op 触发后帧里带语音状态，文本带说话人。
func TestVoiceAndTextPassthrough(t *testing.T) {
	ev := &ir.Event{EventID: "t", Show: []ir.ShowOp{
		{Kind: ir.ShowVoicePlay, Ref: "v001"},
		{Kind: ir.ShowTextRun, Layer: "message0", Text: &ir.ShowText{Speaker: "竜", Text: "有语音的台词"}},
		{Kind: ir.ShowTextWait},
		{Kind: ir.ShowVoicePlay, Ref: "v002"},
		{Kind: ir.ShowTextClear},
		{Kind: ir.ShowTextRun, Layer: "message0", Text: &ir.ShowText{Speaker: "竜", Text: "第二句"}},
		{Kind: ir.ShowTextWait},
	}}
	s := newTestSession(t, ev)
	var played []string
	// 音频挂接要在 t=0 那一步之前接上（newTestSession 已经跑过一次 Advance(0)）。
	s.p = player.New(ev, s.opts)
	s.p.Audio = s.audioHandle(func(op ir.ShowOp) { played = append(played, op.Ref) })
	s.p.Advance(0, player.Input{})
	s.lastTick = time.Now().Add(-16 * time.Millisecond)

	f1 := s.frameLocked()
	if f1.Voice == nil || f1.Voice.Ref != "v001" {
		t.Fatalf("首帧语音状态 = %+v，期望 v001", f1.Voice)
	}
	if len(played) != 1 || played[0] != "v001" {
		t.Fatalf("音频回调收到 %v，期望 [v001]", played)
	}
	if got := lastLine(f1); got.Speaker != "竜" || got.Text != "有语音的台词" {
		t.Fatalf("台词透传 = %+v", got)
	}
	if f1.Texts[0].MarginL <= 0 {
		t.Errorf("文本框内边距没透传（margin_l=%v）", f1.Texts[0].MarginL)
	}

	app := &App{sess: s}
	if err := app.InputClick(); err != nil {
		t.Fatalf("InputClick: %v", err)
	}
	f2 := s.frameLocked()
	if f2.Voice == nil || f2.Voice.Ref != "v002" {
		t.Fatalf("点击后语音状态 = %+v，期望 v002", f2.Voice)
	}
	if got := lastLine(f2); got.Text != "第二句" {
		t.Fatalf("点击后台词 = %q", got.Text)
	}
}

// TestSetReplayModeSwapsTimeline：CG 的剧情/鉴赏切换只换播放器，
// 台本、模式、台词统计与时长都要同步换到对应版本。
func TestSetReplayModeSwapsTimeline(t *testing.T) {
	story := textEvent()
	gallery := &ir.Event{EventID: "g", Show: []ir.ShowOp{
		{Kind: ir.ShowTextRun, Layer: "message0", Text: &ir.ShowText{Text: "鉴赏页"}},
	}}
	s := newTestSession(t, story)
	s.kind = "cg"
	s.storyEv = story
	s.galleryEv = gallery
	s.mode = "story"
	s.dialogue = &DialogueDTO{Available: true, TextLines: 3, VoiceLines: 0}
	app := &App{sess: s}

	m, err := app.SetReplayMode("gallery")
	if err != nil {
		t.Fatalf("切鉴赏: %v", err)
	}
	if s.ev != gallery {
		t.Fatal("切鉴赏后台本没换成画廊台本")
	}
	if m.Mode != "gallery" || m.TextCount != 1 {
		t.Errorf("Meta = mode %q / text %d，期望 gallery / 1", m.Mode, m.TextCount)
	}
	if got := lastLine(s.frameLocked()); got.Text != "鉴赏页" {
		t.Errorf("鉴赏台本台词 = %q", got.Text)
	}

	m, err = app.SetReplayMode("story")
	if err != nil {
		t.Fatalf("切剧情: %v", err)
	}
	if s.ev != story {
		t.Fatal("切剧情后台本没换回剧情台本")
	}
	if m.Mode != "story" || m.TextCount != 3 {
		t.Errorf("Meta = mode %q / text %d，期望 story / 3", m.Mode, m.TextCount)
	}
}

// TestMetaLockedCarriesModeAndDialogue：CG 的剧情/鉴赏模式与台词数据要出现在
// SessionMeta 里（前端就靠它显示切换与「无台词」提示）。
func TestMetaLockedCarriesModeAndDialogue(t *testing.T) {
	ev := textEvent()
	s := newTestSession(t, ev)
	s.kind = "cg"
	s.gameID = "g1"
	s.storyEv = ev
	s.mode = "story"
	s.dialogue = &DialogueDTO{Available: true, TextLines: 3, VoiceLines: 1}

	m := s.metaLocked()
	if m.Mode != "story" {
		t.Errorf("Mode = %q，期望 story", m.Mode)
	}
	if len(m.Modes) != 2 || m.Modes[0] != "story" || m.Modes[1] != "gallery" {
		t.Errorf("Modes = %v，期望 [story gallery]", m.Modes)
	}
	if m.Dialogue == nil || !m.Dialogue.Available || m.Dialogue.TextLines != 3 {
		t.Errorf("Dialogue = %+v", m.Dialogue)
	}
	if m.TextCount != 3 {
		t.Errorf("TextCount = %d，期望 3（按 text op 统计）", m.TextCount)
	}
}
