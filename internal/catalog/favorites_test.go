package catalog

import (
	"path/filepath"
	"testing"
)

func TestFavoritesCRUD(t *testing.T) {
	path := filepath.Join(t.TempDir(), "数据库", FileName)
	c, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer c.Close()
	if err := c.SyncGame(testGame(), testLibrary()); err != nil {
		t.Fatalf("SyncGame: %v", err)
	}

	// 参数校验：不静默报错。
	if err := c.AddFavorite("nope", testGame().ID, "cg001", 0, ""); err == nil {
		t.Fatal("非法 item_type 应报错")
	}
	if err := c.AddFavorite(FavEven, "", "cg001", 0, ""); err == nil {
		t.Fatal("空 game_id 应报错")
	}
	if err := c.AddFavorite(FavEven, testGame().ID, "", 0, ""); err == nil {
		t.Fatal("空 item_id 应报错")
	}
	if err := c.AddFavorite(FavEven, testGame().ID, "cg001", 6, ""); err == nil {
		t.Fatal("rating 6 应报错")
	}

	// 新增 + upsert：同键第二次只更新 rating/note，不产生第二行。
	if err := c.AddFavorite(FavEven, testGame().ID, "cg001", 0, ""); err != nil {
		t.Fatalf("AddFavorite 1: %v", err)
	}
	if err := c.AddFavorite(FavEven, testGame().ID, "cg002", 4, "海边"); err != nil {
		t.Fatalf("AddFavorite 2: %v", err)
	}
	if err := c.AddFavorite(FavEven, testGame().ID, "cg001", 5, "重温"); err != nil {
		t.Fatalf("AddFavorite upsert: %v", err)
	}
	items, err := c.ListFavorites(FavEven, 0)
	if err != nil {
		t.Fatalf("ListFavorites: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len(favorites)=%d, want 2 (%+v)", len(items), items)
	}
	// upsert 后 updated_at 最新 → cg001 排最前。
	if items[0].ItemID != "cg001" || items[0].Rating != 5 || items[0].Note != "重温" {
		t.Fatalf("upsert 行 = %+v, want cg001 5 重温", items[0])
	}
	// 展示标题走 events join。
	if items[1].ItemID != "cg002" || items[1].Title != "cg002 · 码头" || items[1].GameTitle != testGame().Title {
		t.Fatalf("join 标题 = %+v, want cg002 · 码头 / %s", items[1], testGame().Title)
	}

	// game 级收藏：标题取 games.title；类型过滤互不串。
	if err := c.AddFavorite(FavGame, testGame().ID, testGame().ID, 0, "通关纪念"); err != nil {
		t.Fatalf("AddFavorite game: %v", err)
	}
	all, _ := c.ListFavorites("", 0)
	if len(all) != 3 {
		t.Fatalf("len(all)=%d, want 3", len(all))
	}
	games, _ := c.ListFavorites(FavGame, 0)
	if len(games) != 1 || games[0].Title != testGame().Title || games[0].Note != "通关纪念" {
		t.Fatalf("game 收藏 = %+v", games)
	}

	// 删除幂等：删一次生效，再删同一键不报错也不动其他行。
	if err := c.RemoveFavorite(FavEven, testGame().ID, "cg002"); err != nil {
		t.Fatalf("RemoveFavorite: %v", err)
	}
	if err := c.RemoveFavorite(FavEven, testGame().ID, "cg002"); err != nil {
		t.Fatalf("RemoveFavorite 重复应幂等: %v", err)
	}
	items, _ = c.ListFavorites(FavEven, 0)
	if len(items) != 1 || items[0].ItemID != "cg001" {
		t.Fatalf("删除后 = %+v, want 仅 cg001", items)
	}
	if err := c.RemoveFavorite("bad", testGame().ID, "cg001"); err == nil {
		t.Fatal("删除时非法类型应报错")
	}

	// 重扫删除事件后，事件收藏仍在（收藏是用户数据，不随索引重建丢失），
	// 但标题回落 item_id（join 不到 events）。
	lib := testLibrary()
	lib.CGs = lib.CGs[1:] // 删 cg001
	if err := c.SyncGame(testGame(), lib); err != nil {
		t.Fatalf("SyncGame rescan: %v", err)
	}
	items, _ = c.ListFavorites(FavEven, 0)
	if len(items) != 1 || items[0].Title != "cg001" {
		t.Fatalf("事件删后收藏标题应回落 item_id：%+v", items)
	}

	// 关掉再开，收藏仍在。
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	c2, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer c2.Close()
	items, err = c2.ListFavorites("", 0)
	if err != nil || len(items) != 2 {
		t.Fatalf("reopen favorites=%+v err=%v, want 2 条", items, err)
	}
}

func TestFavoriteKey(t *testing.T) {
	if got := FavoriteKey(FavEven, "g1", "cg001"); got != "event|g1|cg001" {
		t.Fatalf("FavoriteKey = %q", got)
	}
}
