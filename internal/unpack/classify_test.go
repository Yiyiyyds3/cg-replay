package unpack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ucgr/internal/asset"
)

// TestUnpackClassifiesAssetsByRole：CG/背景/立绘/影片/UI 分目录，且同一
// 基名来自不同原目录时不会互相覆盖。这是「事件库把背景当 CG」的回归点。
func TestUnpackClassifiesAssetsByRole(t *testing.T) {
	dir := t.TempDir()
	xp3 := filepath.Join(dir, "data.xp3")
	script := "*cgrai\n" +
		"[sysbutton graphic=\"cg001\" target=\"*cg001\"]\n" +
		"*cg001\n" +
		"[image storage=\"ev/001\"]\n" +
		"[image storage=\"bg/001\"]\n" +
		"[image storage=\"fg/chara_a\"]\n" +
		"[image storage=\"sys/button\"]\n" +
		"[movie storage=\"op.mpg\"]\n"
	writeXP3(t, xp3, []xp3File{
		{path: "data/scenario/cgrai.ks", data: []byte(script)},
		{path: "data/image/ev/001.png", data: []byte("CG-CONTENT")},
		{path: "data/image/bg/001.png", data: []byte("BG-CONTENT")},
		{path: "data/image/fg/chara_a.png", data: []byte("STAND-CONTENT")},
		{path: "data/image/sys/button.png", data: []byte("UI-CONTENT")},
		{path: "data/movie/op.mpg", data: []byte("MOVIE-CONTENT")},
	})
	dest := filepath.Join(t.TempDir(), "game")
	res, err := ExtractWith(xp3, dest, Options{Scope: ScopeCG})
	if err != nil {
		t.Fatalf("ExtractWith: %v", err)
	}
	t.Logf("解包结果：files=%d missing=%v warnings=%v", res.Files, res.Missing, res.Warnings)
	want := map[string]string{
		"raw/data/image/cg/ev/001.png":        "CG-CONTENT",
		"raw/data/image/bg/001.png":           "BG-CONTENT",
		"raw/data/image/stand/fg/chara_a.png": "STAND-CONTENT",
		"raw/data/image/ui/sys/button.png":    "UI-CONTENT",
		"raw/data/image/movie/op.mpg":         "MOVIE-CONTENT",
	}
	for rel, content := range want {
		got, err := os.ReadFile(filepath.Join(dest, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("缺 %s：%v", rel, err)
		}
		if string(got) != content {
			t.Fatalf("%s 内容 = %q，want %q（同名文件被覆盖或取错源）", rel, got, content)
		}
	}
	// 旧版平铺路径不应再出现同名文件。
	if _, err := os.Stat(filepath.Join(dest, "raw", "data", "image", "001.png")); err == nil {
		t.Fatal("不应再平铺同名 001.png")
	}
}

// TestUnpackSameRoleSameBasenameKeepsBoth：同一角色目录下、不同原目录的
// 同名图也要各自落盘；文件名相同不能靠 map 随机选一份。
func TestUnpackSameRoleSameBasenameKeepsBoth(t *testing.T) {
	dir := t.TempDir()
	xp3 := filepath.Join(dir, "data.xp3")
	script := "*cgrai\n" +
		"[sysbutton graphic=\"cg001\" target=\"*cg001\"]\n" +
		"*cg001\n" +
		"[image storage=\"ev/room_a/001\"]\n" +
		"[image storage=\"ev/room_b/001\"]\n"
	writeXP3(t, xp3, []xp3File{
		{path: "data/scenario/cgrai.ks", data: []byte(script)},
		{path: "data/image/ev/room_a/001.png", data: []byte("ROOM-A")},
		{path: "data/image/ev/room_b/001.png", data: []byte("ROOM-B")},
	})
	dest := filepath.Join(t.TempDir(), "game")
	res, err := ExtractWith(xp3, dest, Options{Scope: ScopeCG})
	if err != nil {
		t.Fatalf("ExtractWith: %v", err)
	}
	t.Logf("解包结果：files=%d missing=%v warnings=%v", res.Files, res.Missing, res.Warnings)
	for rel, want := range map[string]string{
		"raw/data/image/cg/ev/room_a/001.png": "ROOM-A",
		"raw/data/image/cg/ev/room_b/001.png": "ROOM-B",
	} {
		got, err := os.ReadFile(filepath.Join(dest, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("缺 %s：%v", rel, err)
		}
		if string(got) != want {
			t.Fatalf("%s 内容 = %q，want %q", rel, got, want)
		}
	}
}

// TestUniqueAssetPathSuffix：归一化后仍撞到同一路径时，不同源条目要有
// 稳定且互不覆盖的后缀；同一源重复调用不能改名。
func TestUniqueAssetPathSuffix(t *testing.T) {
	dest := t.TempDir()
	base := filepath.Join(dest, "raw", "data", "image", "cg", "ev", "001.png")
	used := map[string]string{}
	a1 := uniqueAssetPath(base, "data/image/ev/001.png", used)
	a2 := uniqueAssetPath(base, "data/image/ev/001.png", used)
	b := uniqueAssetPath(base, "data/evimage/ev/001.png", used)
	b2 := uniqueAssetPath(base, "data/evimage/ev/001.png", used)
	if a1 != a2 {
		t.Fatalf("同一源重复调用不应改名：%q vs %q", a1, a2)
	}
	if b != b2 {
		t.Fatalf("撞名后的源重复调用也不应改道：%q vs %q", b, b2)
	}
	if b == a1 {
		t.Fatalf("不同源条目撞到同一目标路径：%q", b)
	}
	if !strings.Contains(filepath.Base(b), "~") {
		t.Fatalf("兜底路径应带稳定哈希后缀：%q", b)
	}
}

// TestDestPathKeepsNestedPaths：只裁掉归档根与最外层素材根，保留原相对
// 子路径（同名不同目录靠它分开）。
func TestDestPathKeepsNestedPaths(t *testing.T) {
	dest := t.TempDir()
	cases := map[string]string{
		"data/image/ev/a.png":     "raw/data/image/cg/ev/a.png",
		"data/image/bg/a.png":     "raw/data/image/bg/a.png",
		"data/fgimage/tami/a.png": "raw/data/image/stand/tami/a.png",
		"data/movie/op.mpg":       "raw/data/image/movie/op.mpg",
		"data/image/sys/menu.png": "raw/data/image/ui/sys/menu.png",
	}
	for src, want := range cases {
		got, err := filepath.Rel(dest, asset.DestPath(dest, "raw/data/image", src))
		if err != nil {
			t.Fatal(err)
		}
		if filepath.ToSlash(got) != want {
			t.Errorf("DestPath(%q) = %q, want %q", src, filepath.ToSlash(got), want)
		}
	}
}
