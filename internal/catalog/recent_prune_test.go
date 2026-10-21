package catalog

import (
	"path/filepath"
	"testing"
	"time"
)

func TestPruneRecent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "数据库", FileName)
	c, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer c.Close()
	if err := c.SyncGame(testGame(), testLibrary()); err != nil {
		t.Fatalf("SyncGame: %v", err)
	}

	// 参数校验：负数与过大窗口报错（不静默改语义）；0 = 默认 3 天。
	if _, err := c.PruneRecent(-1); err == nil {
		t.Fatal("负数窗口应报错")
	}
	if _, err := c.PruneRecent(4000); err == nil {
		t.Fatal("4000 天窗口应报错")
	}

	// 造数据：今天 / 2 天前 / 5 天前 各一条（直接写 played_at，绕过 NoteRecent）。
	ago := func(d int) string {
		return time.Now().UTC().AddDate(0, 0, -d).Format(time.RFC3339)
	}
	rows := []struct{ id, at string }{
		{"cg001", ago(0)},
		{"cg002", ago(2)},
		{"cg003", ago(5)},
	}
	for _, r := range rows {
		if _, err := c.db.Exec(
			`INSERT INTO recent(game_id, event_id, kind, played_at, play_count) VALUES (?, ?, 'cg', ?, 1)`,
			testGame().ID, r.id, r.at); err != nil {
			t.Fatalf("造数据失败：%v", err)
		}
	}

	// 清理 3 天：只有 5 天前那条该走。
	n, err := c.PruneRecent(3)
	if err != nil {
		t.Fatalf("PruneRecent: %v", err)
	}
	if n != 1 {
		t.Fatalf("删除条数 = %d, want 1", n)
	}
	items, err := c.Recent(0)
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("剩余 %d 条, want 2：%+v", len(items), items)
	}
	for _, it := range items {
		if it.EventID == "cg003" {
			t.Fatal("5 天前那条没被清理")
		}
	}

	// 再清理一次（没得删）→ 0，不报错。
	n, err = c.PruneRecent(3)
	if err != nil || n != 0 {
		t.Fatalf("二次清理 = (%d, %v), want (0, nil)", n, err)
	}

	// 默认窗口（days=0 → 3 天）：造一条 4 天前，应被删（用没占用过的事件 id）。
	if _, err := c.db.Exec(
		`INSERT INTO recent(game_id, event_id, kind, played_at, play_count) VALUES (?, 'cg004', 'cg', ?, 1)`,
		testGame().ID, ago(4)); err != nil {
		t.Fatalf("造数据失败：%v", err)
	}
	if n, err := c.PruneRecent(0); err != nil || n != 1 {
		t.Fatalf("默认窗口清理 = (%d, %v), want (1, nil)", n, err)
	}

	// 关掉再开，清理结果持久（不是内存假象）。
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	c2, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer c2.Close()
	items, err = c2.Recent(0)
	if err != nil || len(items) != 2 {
		t.Fatalf("重开后剩余 = %d 条 err=%v, want 2（cg001/cg002）", len(items), err)
	}
	for _, it := range items {
		if it.EventID == "cg004" || it.EventID == "cg003" {
			t.Fatalf("清理结果没持久：%+v", items)
		}
	}
	// 空表清理 no-op。
	empty, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open mem: %v", err)
	}
	defer empty.Close()
	if n, err := empty.PruneRecent(3); err != nil || n != 0 {
		t.Fatalf("空表清理 = (%d, %v), want (0, nil)", n, err)
	}
}
