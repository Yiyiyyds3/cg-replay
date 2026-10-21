package registry

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIDForStableAndCaseInsensitive(t *testing.T) {
	a := IDFor(`C:\Games\Foo`)
	b := IDFor(`c:/games/foo/`)
	if a != b {
		t.Fatalf("同一目录不同写法应得同一 id：%s != %s", a, b)
	}
	if a[0] != 'g' || len(a) != 11 {
		t.Fatalf("id 形状不符合预期：%q", a)
	}
	if IDFor(`C:\Games\Foo`) == IDFor(`C:\Games\Bar`) {
		t.Fatal("不同目录不应撞 id")
	}
}

func TestEnsureDedupesAndKeepsLibrary(t *testing.T) {
	r := New()
	root := t.TempDir()
	g, added := r.Ensure(root, filepath.Join("libs", "a.json"))
	if !added || g.ID == "" || g.Title == "" {
		t.Fatalf("首次登记失败：%+v added=%v", g, added)
	}
	firstID := g.ID
	// 同目录换写法（尾分隔符/斜杠方向）：不新增，库路径更新。
	alt := filepath.ToSlash(filepath.Clean(root)) + "/"
	g2, added := r.Ensure(alt, filepath.Join("libs", "b.json"))
	if added {
		t.Fatal("同一根目录不应重复登记")
	}
	if g2.ID != firstID {
		t.Fatalf("重复登记换了 id：%s != %s", g2.ID, firstID)
	}
	if len(r.Games) != 1 {
		t.Fatalf("登记表条数 = %d，想要 1", len(r.Games))
	}
}

func TestSaveLoadActiveRemove(t *testing.T) {
	dir := t.TempDir()
	r := New()
	g1, _ := r.Ensure(filepath.Join(dir, "one"), "")
	g2, _ := r.Ensure(filepath.Join(dir, "two"), "")
	if err := r.SetActive(g2.ID); err != nil {
		t.Fatal(err)
	}
	if err := r.Save(Path(dir)); err != nil {
		t.Fatal(err)
	}
	got, err := Load(Path(dir))
	if err != nil {
		t.Fatal(err)
	}
	if got.Active != g2.ID || got.ActiveGame() == nil {
		t.Fatalf("active 未持久化：%+v", got)
	}
	if len(got.Games) != 2 {
		t.Fatalf("games = %d，想要 2", len(got.Games))
	}

	// 删当前游戏 → 自动落到剩下那款。
	if !got.Remove(g2.ID) {
		t.Fatal("删除已登记游戏失败")
	}
	if got.Active != g1.ID {
		t.Fatalf("删除当前游戏后 active = %q，想要 %q", got.Active, g1.ID)
	}
	if !got.Touch(g1.ID) || got.Find(g1.ID).LastPlayedAt == "" {
		t.Fatal("Touch 未写入最近播放时间")
	}
}

func TestLoadMissingAndBadVersion(t *testing.T) {
	dir := t.TempDir()
	if _, err := Load(Path(dir)); !os.IsNotExist(err) {
		t.Fatalf("缺文件应返回 os.ErrNotExist，得到 %v", err)
	}
	p := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(p, []byte(`{"version":99,"games":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err == nil {
		t.Fatal("高版本登记表应显式报错")
	}
}
