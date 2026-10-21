package catalog

import (
	"path/filepath"
	"testing"

	"ucgr/internal/ir"
)

// showEvent 构造一份最小演出台本：两条台词（说话人沿用 + 切换）、
// 一条空文本（应被跳过）、一个非 text 指令（应被忽略）。
func showEvent() *ir.Event {
	return &ir.Event{
		Show: []ir.ShowOp{
			{Kind: ir.ShowTextRun, TMs: 100, Text: &ir.ShowText{Speaker: "少女", Text: "一起去海边吧。"}},
			{Kind: ir.ShowTextRun, TMs: 200, Text: &ir.ShowText{Text: "  "}}, // 空文本跳过
			{Kind: ir.ShowTextBreak, TMs: 300},
			{Kind: ir.ShowTextRun, TMs: 400, Text: &ir.ShowText{Text: "嗯，走吧。"}}, // speaker 沿用「少女」
			{Kind: ir.ShowLayerShow, TMs: 500, Ref: "kaigan01_bg"},
			{Kind: ir.ShowTextRun, TMs: 600, Text: &ir.ShowText{Speaker: "少年", Text: "等等我。"}},
		},
	}
}

func TestTextLinesFromEvent(t *testing.T) {
	lines := TextLinesFromEvent(showEvent())
	if len(lines) != 3 {
		t.Fatalf("len(lines)=%d, want 3 (%+v)", len(lines), lines)
	}
	want := []TextLine{
		{Kind: "", EventID: "", TMs: 100, Speaker: "少女", Text: "一起去海边吧。"},
		{TMs: 400, Speaker: "少女", Text: "嗯，走吧。"},
		{TMs: 600, Speaker: "少年", Text: "等等我。"},
	}
	for i, w := range want {
		if lines[i].TMs != w.TMs || lines[i].Speaker != w.Speaker || lines[i].Text != w.Text {
			t.Fatalf("line[%d] = %+v, want %+v", i, lines[i], w)
		}
	}
	if TextLinesFromEvent(nil) != nil {
		t.Fatal("nil event should yield nil")
	}
}

func TestNoteRecentAndRecent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "数据库", FileName)
	c, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer c.Close()
	if err := c.SyncGame(testGame(), testLibrary()); err != nil {
		t.Fatalf("SyncGame: %v", err)
	}

	// 空参数必须报错，不静默写入。
	if err := c.NoteRecent("", "cg", "cg001"); err == nil {
		t.Fatal("empty game id should fail")
	}

	if err := c.NoteRecent(testGame().ID, "cg", "cg001"); err != nil {
		t.Fatalf("NoteRecent 1: %v", err)
	}
	if err := c.NoteRecent(testGame().ID, "scene", "01kuukou"); err != nil {
		t.Fatalf("NoteRecent 2: %v", err)
	}
	if err := c.NoteRecent(testGame().ID, "cg", "cg001"); err != nil { // 重复 → play_count=2
		t.Fatalf("NoteRecent 3: %v", err)
	}

	items, err := c.Recent(0)
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len(recent)=%d, want 2 (%+v)", len(items), items)
	}
	// 最新在前：第二次 NoteRecent（cg001）又抢到最前。
	if items[0].EventID != "cg001" || items[0].PlayCount != 2 {
		t.Fatalf("first recent = %+v, want cg001 ×2", items[0])
	}
	if items[1].EventID != "01kuukou" || items[1].PlayCount != 1 {
		t.Fatalf("second recent = %+v, want 01kuukou ×1", items[1])
	}
	if items[0].GameTitle != testGame().Title || items[0].Title != "cg001 · 海面" {
		t.Fatalf("recent titles = %+v", items[0])
	}
}

func TestTextCounts(t *testing.T) {
	c, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer c.Close()
	if err := c.SyncGame(testGame(), testLibrary()); err != nil {
		t.Fatalf("SyncGame: %v", err)
	}
	counts, err := c.TextCounts()
	if err != nil {
		t.Fatalf("TextCounts: %v", err)
	}
	if len(counts) != 0 {
		t.Fatalf("counts = %+v, want empty", counts)
	}
	lines := []TextLine{
		{Kind: "cg", EventID: "cg001", Text: "第一句"},
		{Kind: "cg", EventID: "cg002", Text: "第二句"},
	}
	if err := c.ReplaceTextLines(testGame().ID, "", "", lines); err != nil {
		t.Fatalf("ReplaceTextLines: %v", err)
	}
	counts, err = c.TextCounts()
	if err != nil {
		t.Fatalf("TextCounts 2: %v", err)
	}
	if counts[testGame().ID] != 2 {
		t.Fatalf("counts[game]=%d, want 2 (%+v)", counts[testGame().ID], counts)
	}
}
