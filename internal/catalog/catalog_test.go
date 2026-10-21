package catalog

import (
	"path/filepath"
	"testing"

	"ucgr/internal/library"
	"ucgr/internal/registry"
)

func testLibrary() *library.Library {
	return &library.Library{
		Root:        `C:\games\blue`,
		Game:        "blue",
		GeneratedAt: "2026-10-21T00:00:00Z",
		CGs: []library.CGEvent{
			{ID: "cg001", Index: 1, Title: "cg001 · 海面", Thumb: "cg001", Script: "raw/data/scenario/cgrai00.ks", Label: "cg1", Pages: []string{"cg001", "cg001b"}},
			{ID: "cg002", Index: 2, Title: "cg002 · 码头", Thumb: "cg002", Script: "raw/data/scenario/cgrai00.ks", Label: "cg2", Pages: []string{"cg002"}},
		},
		Scenes: []library.SceneEvent{
			{ID: "01kuukou", Title: "01kuukou", Script: "raw/data/scenario/01kuukou.ks"},
		},
		Chapters: []library.ChapterEvent{
			{ID: "01kuukou#3", Index: 3, Title: "水畔", Script: "raw/data/scenario/01kuukou.ks", Scene: "01kuukou", LabelPos: 3, Name: "label3"},
		},
	}
}

func testGame() registry.Game {
	return registry.Game{
		ID:      "gtest000001",
		Title:   "重返蓝鲸岛",
		Root:    `C:\games\blue`,
		Library: `C:\data\libraries\gtest000001.json`,
		Engine:  "kirikiri",
		AddedAt: "2026-10-20T00:00:00Z",
	}
}

func TestSyncGameAndEvents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "数据库", FileName)
	c, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer c.Close()

	if _, err := c.Stats(); err != nil {
		t.Fatalf("Stats empty: %v", err)
	}
	if err := c.SyncGame(testGame(), testLibrary()); err != nil {
		t.Fatalf("SyncGame: %v", err)
	}
	s, err := c.Stats()
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if s.Games != 1 || s.Events != 4 || s.TextLines != 0 {
		t.Fatalf("stats = %+v, want 1/4/0", s)
	}

	events, err := c.Events(0)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	if len(events) != 4 {
		t.Fatalf("len(events)=%d, want 4", len(events))
	}
	if events[0].Kind != "cg" || events[0].ID != "cg001" || !events[0].HasCover {
		t.Fatalf("first event = %+v", events[0])
	}
	if events[2].Kind != "scene" || events[3].Kind != "chapter" || events[3].Title != "水畔" {
		t.Fatalf("kind order = %+v", events)
	}

	// 覆盖式同步：删掉一个 CG 后旧行必须消失。
	lib := testLibrary()
	lib.CGs = lib.CGs[:1]
	lib.Chapters[0].ContScript = "raw/data/scenario/02.ks"
	lib.Chapters[0].ContLabelPos = 1
	if err := c.SyncGame(testGame(), lib); err != nil {
		t.Fatalf("SyncGame rescan: %v", err)
	}
	s, _ = c.Stats()
	if s.Events != 3 {
		t.Fatalf("after rescan events=%d, want 3", s.Events)
	}
	events, _ = c.Events(0)
	if len(events) != 3 || events[2].Kind != "chapter" {
		t.Fatalf("after rescan events = %+v", events)
	}

	// 关掉再开，数据仍在。
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	c2, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer c2.Close()
	s, err = c2.Stats()
	if err != nil || s.Games != 1 || s.Events != 3 {
		t.Fatalf("reopen stats=%+v err=%v", s, err)
	}
}

func TestSearchEvents(t *testing.T) {
	c, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer c.Close()
	if err := c.SyncGame(testGame(), testLibrary()); err != nil {
		t.Fatalf("SyncGame: %v", err)
	}

	cases := []struct {
		q    string
		want int
	}{
		{"蓝鲸", 4},    // 游戏标题命中所有事件
		{"cg002", 1}, // 事件 id
		{"码头", 1},    // 事件标题
		{"不存在的", 0},
		{"100%", 0}, // 通配符必须按字面量处理
	}
	for _, tc := range cases {
		got, err := c.SearchEvents(tc.q, 0)
		if err != nil {
			t.Fatalf("SearchEvents(%q): %v", tc.q, err)
		}
		if len(got) != tc.want {
			t.Fatalf("SearchEvents(%q)=%d, want %d (%+v)", tc.q, len(got), tc.want, got)
		}
	}
}

func TestTextLinesFTS(t *testing.T) {
	c, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer c.Close()
	if err := c.SyncGame(testGame(), testLibrary()); err != nil {
		t.Fatalf("SyncGame: %v", err)
	}
	lines := []TextLine{
		{GameID: testGame().ID, Kind: "cg", EventID: "cg001", TMs: 1200, Speaker: "少女", Text: "重返蓝鲸岛的故事开始了。"},
		{GameID: testGame().ID, Kind: "cg", EventID: "cg002", TMs: 3000, Speaker: "少年", Text: "hello world, 100% 晴。"},
	}
	if err := c.ReplaceTextLines(testGame().ID, "", "", lines); err != nil {
		t.Fatalf("ReplaceTextLines: %v", err)
	}
	s, _ := c.Stats()
	if s.TextLines != 2 {
		t.Fatalf("text lines=%d, want 2", s.TextLines)
	}

	cases := []struct {
		q    string
		want int
	}{
		{"蓝鲸岛", 1},   // FTS trigram
		{"蓝鲸", 1},    // 2 字符 → LIKE 回退
		{"hello", 1}, // FTS
		{"少女", 1},    // speaker
		{"100%", 1},  // LIKE 通配符转义
		{"不存在", 0},
	}
	for _, tc := range cases {
		got, err := c.SearchText(tc.q, 0)
		if err != nil {
			t.Fatalf("SearchText(%q): %v", tc.q, err)
		}
		if len(got) != tc.want {
			t.Fatalf("SearchText(%q)=%d, want %d (%+v)", tc.q, len(got), tc.want, got)
		}
	}
	hits, err := c.SearchText("蓝鲸岛", 0)
	if err != nil || len(hits) != 1 || hits[0].Kind != "cg" || hits[0].EventID != "cg001" {
		t.Fatalf("text hit identity = %+v err=%v", hits, err)
	}

	// 删除/更新要经过 FTS 触发器，不能留下旧索引。
	var lineID int64
	if err := c.db.QueryRow(`SELECT line_id FROM text_lines WHERE event_id='cg001'`).Scan(&lineID); err != nil {
		t.Fatalf("line_id: %v", err)
	}
	if _, err := c.db.Exec(`DELETE FROM text_lines WHERE line_id=?`, lineID); err != nil {
		t.Fatalf("delete line: %v", err)
	}
	if got, _ := c.SearchText("蓝鲸岛", 0); len(got) != 0 {
		t.Fatalf("deleted line still searchable: %+v", got)
	}
	if _, err := c.db.Exec(`UPDATE text_lines SET text='新的台词' WHERE event_id='cg002'`); err != nil {
		t.Fatalf("update line: %v", err)
	}
	if got, _ := c.SearchText("hello", 0); len(got) != 0 {
		t.Fatalf("old text still searchable: %+v", got)
	}
	if got, _ := c.SearchText("新的台词", 0); len(got) != 1 {
		t.Fatalf("updated text not searchable: %+v", got)
	}
}

func TestSyncGameDropsStaleTextLines(t *testing.T) {
	c, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer c.Close()
	if err := c.SyncGame(testGame(), testLibrary()); err != nil {
		t.Fatalf("SyncGame: %v", err)
	}
	line := TextLine{Kind: "cg", EventID: "cg001", Text: "会被重扫清掉"}
	if err := c.ReplaceTextLines(testGame().ID, "cg", "cg001", []TextLine{line}); err != nil {
		t.Fatalf("ReplaceTextLines: %v", err)
	}
	if s, _ := c.Stats(); s.TextLines != 1 {
		t.Fatalf("text lines=%d, want 1", s.TextLines)
	}
	lib := testLibrary()
	lib.CGs = lib.CGs[1:] // 删除 cg001
	if err := c.SyncGame(testGame(), lib); err != nil {
		t.Fatalf("SyncGame rescan: %v", err)
	}
	if s, _ := c.Stats(); s.TextLines != 0 {
		t.Fatalf("stale text lines=%d, want 0", s.TextLines)
	}
	if got, _ := c.SearchText("会被重扫清掉", 0); len(got) != 0 {
		t.Fatalf("stale text still searchable: %+v", got)
	}
}
