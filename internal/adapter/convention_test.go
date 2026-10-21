package adapter

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConventionDefaults(t *testing.T) {
	c := DefaultKiriKiri()
	if err := c.Validate(); err != nil {
		t.Fatalf("默认约定应合法：%v", err)
	}
	if c.ScenarioDir != filepath.Join("raw", "data", "scenario") {
		t.Fatalf("剧本目录 = %q", c.ScenarioDir)
	}
	if c.GalleryIndex != "cgrai00.ks" {
		t.Fatalf("画廊索引 = %q", c.GalleryIndex)
	}
	// 引擎未知时按 KiriKiri 试（约定文件可覆盖）。
	got, ok := ConventionByEngine("kirikiri")
	if !ok || got.ScenarioDir != c.ScenarioDir {
		t.Fatalf("按引擎取约定失败：%+v ok=%v", got, ok)
	}
	if _, ok := ConventionByEngine("nosuchengine"); ok {
		t.Fatal("未知引擎应返回 false")
	}
}

func TestConventionFileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	c := Convention{
		EngineID:     "kirikiri",
		ScenarioDir:  filepath.Join("data", "script"),
		ScriptExt:    ".ks",
		GalleryRe:    `^gallery.*\.ks$`,
		GalleryIndex: "gallery.ks",
		CGLabelRe:    `^cg\d+$`,
		SceneRe:      `^scene_\d+\.ks$`,
	}
	if err := SaveConventionFile(dir, c); err != nil {
		t.Fatalf("SaveConventionFile: %v", err)
	}
	got, exists, err := LoadConventionFile(dir)
	if err != nil || !exists {
		t.Fatalf("读约定失败：exists=%v err=%v", exists, err)
	}
	if got.ScenarioDir != c.ScenarioDir || got.GalleryIndex != c.GalleryIndex ||
		got.GalleryRe != c.GalleryRe || got.SceneRe != c.SceneRe {
		t.Fatalf("往返不一致：%+v", got)
	}

	// 不存在 → (零值, false, nil)。
	if _, exists, err := LoadConventionFile(t.TempDir()); exists || err != nil {
		t.Fatalf("无约定文件应返回 exists=false err=nil，实际 %v %v", exists, err)
	}

	// 写坏了 → 报错（不静默忽略：用户以为配置生效了更糟）。
	bad := filepath.Join(dir, ConventionFileName)
	if err := os.WriteFile(bad, []byte("{不是 json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadConventionFile(dir); err == nil {
		t.Fatal("坏约定文件应报错")
	}
}

func TestConventionMergeAndValidate(t *testing.T) {
	base := DefaultKiriKiri()
	// 覆盖文件只填部分字段 → 其余沿用默认。
	over := Convention{ScenarioDir: filepath.Join("data", "script"), GalleryIndex: "g.ks"}
	m := over.Merge(base)
	if m.ScenarioDir != over.ScenarioDir || m.GalleryIndex != over.GalleryIndex {
		t.Fatalf("覆盖字段未生效：%+v", m)
	}
	if m.GalleryRe != base.GalleryRe || m.CGLabelRe != base.CGLabelRe {
		t.Fatalf("未填字段应沿用默认：%+v", m)
	}
	// 非法正则 / 绝对路径 → Validate 报错。
	if err := (Convention{ScenarioDir: "x", GalleryRe: "("}).Validate(); err == nil {
		t.Fatal("非法正则应报错")
	}
	if err := (Convention{ScenarioDir: `C:\abs`}).Validate(); err == nil {
		t.Fatal("绝对路径应报错")
	}
	if err := (Convention{}).Validate(); err == nil {
		t.Fatal("缺 scenario_dir 应报错")
	}
}
