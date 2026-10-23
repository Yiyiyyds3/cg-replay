package catalog

import (
	"os"
	"path/filepath"
	"testing"
)

// TestOpenWithStaleWAL 已存在（未 checkpoint 的）WAL 时，Open 必须成功且数据可读。
//
// 背景：强杀/崩溃退出时 SQLite 不会 checkpoint，`catalog.db-wal` 会留在盘上；
// Open 现在会在启动期做一次 `wal_checkpoint(TRUNCATE)`。这个用例守住两点：
//  1. 该 PRAGMA 在本驱动下不会把 Open 弄失败（它返回一行结果，不能用 Exec）；
//  2. 残留 WAL 里的数据在打开后仍然读得到。
func TestOpenWithStaleWAL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)

	// 第一个连接：写一条收藏后**不 Close**（模拟强杀，WAL 留在盘上）。
	c1, err := Open(path)
	if err != nil {
		t.Fatalf("Open#1: %v", err)
	}
	if err := c1.AddFavorite("event", "g1", "e1", 4, "留档"); err != nil {
		t.Fatalf("写收藏: %v", err)
	}
	walPath := path + "-wal"
	if st, err := os.Stat(walPath); err != nil || st.Size() == 0 {
		t.Fatalf("应留下非空 WAL（err=%v）", err)
	}

	// 第二个连接：同一文件再开一次（触发启动期 checkpoint）。
	c2, err := Open(path)
	if err != nil {
		t.Fatalf("Open#2（有残留 WAL）: %v", err)
	}
	favs, err := c2.ListFavorites("", 0)
	if err != nil {
		_ = c2.Close()
		_ = c1.Close()
		t.Fatalf("ListFavorites: %v", err)
	}
	if len(favs) != 1 || favs[0].ItemID != "e1" {
		_ = c2.Close()
		_ = c1.Close()
		t.Fatalf("残留 WAL 里的数据应可读：%+v", favs)
	}
	// 两个连接都关掉，TempDir 清理才不会被占用。
	_ = c2.Close()
	_ = c1.Close()
}
