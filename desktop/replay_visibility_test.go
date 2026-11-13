package main

// replay_visibility_test.go —— 黑屏守卫接线的单测。
//
// 钉死三件事：
//   1. 守卫只跑在「有图可看的普通事件」上：含影片指令的事件不采样（OP/ED 黑场
//      是正常演出），纯台词事件也不采样（舞台底本来就是黑的）；
//   2. 判定走 replay.CheckEventVisibility：1s 入场黑幕后再出图不拦会话，
//      整段纯黑才拦（这正是验收项「纯黑界面不触发交互判定」的落地口径）；
//   3. 缺素材不算「确认全黑」：采样失败只记 errors.log，会话照开。

import (
	"errors"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ucgr/internal/ir"
	"ucgr/internal/vision"
)

// solidImg 合成一张纯色素材（测试不碰真实游戏目录）。
func solidImg(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

// TestNeedsVisibilityCheck：普通图事件要查；含 ShowVideoPlay 的影片事件不能查。
func TestNeedsVisibilityCheck(t *testing.T) {
	cg := &ir.Event{EventID: "EV-cg", EndMs: 2000, Show: []ir.ShowOp{
		{TMs: 0, Kind: ir.ShowLayerShow, Layer: "base", Ref: "cg.png"},
		{TMs: 0, Kind: ir.ShowWait, DurMs: 1000},
	}}
	if !needsVisibilityCheck(cg) {
		t.Fatal("普通图事件应做可见度守卫")
	}

	movie := &ir.Event{EventID: "EV-movie", Show: []ir.ShowOp{
		{TMs: 0, Kind: ir.ShowLayerShow, Layer: "base", Ref: "op.mpg"},
		{TMs: 0, Kind: ir.ShowVideoPlay, Ref: "op.mpg"},
	}}
	if needsVisibilityCheck(movie) {
		t.Fatal("含 ShowVideoPlay 的事件必须跳过守卫（影片黑场不是全黑）")
	}

	textOnly := &ir.Event{EventID: "EV-text", Show: []ir.ShowOp{
		{Kind: ir.ShowTextRun, Layer: "message0", Text: &ir.ShowText{Text: "只有台词"}},
	}}
	if needsVisibilityCheck(textOnly) {
		t.Fatal("纯台词事件没有图，舞台黑底不能当「整段纯黑」")
	}
}

// TestGuardEventVisibilityBlackThenPicture：1s 黑幕后出彩图，单帧黑幕不能拦会话。
func TestGuardEventVisibilityBlackThenPicture(t *testing.T) {
	ev := &ir.Event{EventID: "EV-guard-ok", EndMs: 3000, Show: []ir.ShowOp{
		{TMs: 0, Kind: ir.ShowLayerShow, Layer: "base", Ref: "black.png"},
		{TMs: 0, Kind: ir.ShowWait, DurMs: 1000},
		{TMs: 2000, Kind: ir.ShowLayerShow, Layer: "base", Ref: "cg.png"},
	}}
	load := func(rel string) (image.Image, error) {
		if rel == "cg.png" {
			return solidImg(80, 45, color.RGBA{R: 240, G: 160, B: 80, A: 255}), nil
		}
		return solidImg(80, 45, color.RGBA{A: 255}), nil
	}
	app := &App{}
	if err := app.guardEventVisibility("黑幕开场", ev, load, 80, 45); err != nil {
		t.Fatalf("首帧黑幕的正常事件被拦截：%v", err)
	}
}

// TestGuardEventVisibilityAllBlack：整段只有黑图才报 ErrAllBlack 拦会话。
func TestGuardEventVisibilityAllBlack(t *testing.T) {
	ev := &ir.Event{EventID: "EV-guard-black", EndMs: 3000, Show: []ir.ShowOp{
		{TMs: 0, Kind: ir.ShowLayerShow, Layer: "base", Ref: "black.png"},
		{TMs: 0, Kind: ir.ShowWait, DurMs: 3000},
	}}
	load := func(string) (image.Image, error) {
		return solidImg(80, 45, color.RGBA{A: 255}), nil
	}
	app := &App{}
	err := app.guardEventVisibility("全黑事件", ev, load, 80, 45)
	if err == nil {
		t.Fatal("整段纯黑必须拦会话")
	}
	if !errors.Is(err, vision.ErrAllBlack) {
		t.Fatalf("错误链应保留 vision.ErrAllBlack，实际 %v", err)
	}
	if want := "全黑事件 整段没有有效画面（采样 5 帧均为纯黑）"; err.Error() != want {
		t.Fatalf("错误文案 = %q，期望 %q", err.Error(), want)
	}
}

// TestGuardEventVisibilityMissingAssetsPasses：缺素材是「采样失败」，不是「确认全黑」。
// 必须放行并留痕（errors.log），否则缺图的正常事件会被当成纯黑挡在门外。
func TestGuardEventVisibilityMissingAssetsPasses(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("LOCALAPPDATA", tmp) // 让 errors.log 落在临时目录

	ev := &ir.Event{EventID: "EV-guard-missing", EndMs: 3000, Show: []ir.ShowOp{
		{TMs: 0, Kind: ir.ShowLayerShow, Layer: "base", Ref: "missing.png"},
		{TMs: 0, Kind: ir.ShowWait, DurMs: 3000},
	}}
	load := func(string) (image.Image, error) {
		return nil, errors.New("素材不存在")
	}
	app := &App{}
	if err := app.guardEventVisibility("缺素材事件", ev, load, 80, 45); err != nil {
		t.Fatalf("缺素材被误判成确认全黑：%v", err)
	}

	logPath := filepath.Join(tmp, "cgreplay-ui", "errors.log")
	data, rerr := os.ReadFile(logPath)
	if rerr != nil {
		t.Fatalf("采样失败没有留痕（读 %s）：%v", logPath, rerr)
	}
	if !strings.Contains(string(data), "可见度采样缺 1 个素材") {
		t.Fatalf("errors.log 里没有缺素材记录：%s", data)
	}
}
