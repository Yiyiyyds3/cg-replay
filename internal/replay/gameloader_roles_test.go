package replay

import (
	"image/color"
	"os"
	"path/filepath"
	"testing"
)

// TestGameLoaderRoleSubdirFallback：新版按角色分目录后，引用名里的子路径
// 要能在对应角色目录里命中；同名 CG/背景不能串。
func TestGameLoaderRoleSubdirFallback(t *testing.T) {
	root := t.TempDir()
	write := func(rel string, c color.RGBA) {
		t.Helper()
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, solidPNG(t, 2, 2, c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("raw/data/image/cg/ev/001.png", color.RGBA{R: 255, A: 255})
	write("raw/data/image/bg/001.png", color.RGBA{B: 255, A: 255})
	write("raw/data/image/cg/chara_a.png", color.RGBA{R: 255, A: 255})
	write("raw/data/image/stand/chara_a.png", color.RGBA{G: 255, A: 255})

	load := GameLoader(root)
	img, err := load("ev/001")
	if err != nil {
		t.Fatalf("加载 CG：%v", err)
	}
	if c := color.RGBAModel.Convert(img.At(0, 0)).(color.RGBA); c.R != 255 {
		t.Fatalf("ev/001 应命中 cg 目录的红色图，实际 %+v", c)
	}
	img, err = load("bg/001")
	if err != nil {
		t.Fatalf("加载背景：%v", err)
	}
	if c := color.RGBAModel.Convert(img.At(0, 0)).(color.RGBA); c.B != 255 {
		t.Fatalf("bg/001 应命中 bg 目录的蓝色图，实际 %+v", c)
	}
	// 裸名 + 同名跨角色：按引用角色（chara_a -> stand）选，而不是按目录字典序。
	img, err = load("chara_a")
	if err != nil {
		t.Fatalf("加载立绘：%v", err)
	}
	if c := color.RGBAModel.Convert(img.At(0, 0)).(color.RGBA); c.G != 255 {
		t.Fatalf("chara_a 应命中 stand 目录的绿色图，实际 %+v", c)
	}

	path, err := AssetPath(root, "ev/001")
	if err != nil {
		t.Fatalf("AssetPath：%v", err)
	}
	want := filepath.Join(root, "raw", "data", "image", "cg", "ev", "001.png")
	if filepath.Clean(path) != filepath.Clean(want) {
		t.Fatalf("AssetPath(ev/001) = %q，want %q", path, want)
	}
}

// TestGameLoaderLegacyFlatLayout：旧解包目录没有角色层，平铺文件仍要能加载。
func TestGameLoaderLegacyFlatLayout(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "raw", "data", "image", "legacy.png")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, solidPNG(t, 2, 2, color.RGBA{G: 255, A: 255}), 0o644); err != nil {
		t.Fatal(err)
	}
	img, err := GameLoader(root)("legacy")
	if err != nil {
		t.Fatalf("旧平铺目录加载失败：%v", err)
	}
	if c := color.RGBAModel.Convert(img.At(0, 0)).(color.RGBA); c.G != 255 {
		t.Fatalf("旧平铺图颜色不对：%+v", c)
	}
	// 影片时长探测也走 AssetPath：旧平铺目录必须能查回真实路径。
	if got, err := AssetPath(root, "legacy"); err != nil || filepath.Clean(got) != filepath.Clean(p) {
		t.Fatalf("AssetPath 旧平铺回退 = %q, %v；want %q", got, err, p)
	}
}
