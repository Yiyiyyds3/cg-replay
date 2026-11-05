package library

import (
	"os"
	"path/filepath"
	"testing"

	"ucgr/internal/adapter"
)

// writeRenPyFixture 一个最小 Ren'Py 游戏目录。
func writeRenPyFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	game := filepath.Join(root, "game")
	if err := os.MkdirAll(filepath.Join(game, "images"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(game, "images", "cg01.png"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	script := "label start:\n    scene cg01\n    \"旁白\"\n    return\n"
	if err := os.WriteFile(filepath.Join(game, "script.rpy"), []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestScanRenPy(t *testing.T) {
	root := writeRenPyFixture(t)
	lib, err := ScanWith(root, adapter.Convention{EngineID: "renpy"})
	if err != nil {
		t.Fatal(err)
	}
	if lib.EngineID != "renpy" {
		t.Fatalf("EngineID = %q", lib.EngineID)
	}
	if len(lib.Scenes) != 1 || lib.Scenes[0].ID != "start" {
		t.Fatalf("Scenes = %+v", lib.Scenes)
	}
	if lib.Scenes[0].Script != "game/script.rpy" {
		t.Fatalf("Script = %q", lib.Scenes[0].Script)
	}
}

func TestScanRenPyNestedGameDir(t *testing.T) {
	root := writeRenPyFixture(t)
	// 挪进两层壳目录（整合包套壳形态）。
	gameAbs := filepath.Join(root, "game")
	nested := filepath.Join(root, "壳", "Game")
	if err := os.MkdirAll(filepath.Dir(nested), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(gameAbs, nested); err != nil {
		t.Fatal(err)
	}
	lib, err := ScanWith(root, adapter.DefaultRenPy())
	if err != nil {
		t.Fatal(err)
	}
	if len(lib.Scenes) != 1 {
		t.Fatalf("套壳布局应扫到 1 个事件，实为 %d", len(lib.Scenes))
	}
	if lib.Scenes[0].Script != "壳/Game/script.rpy" {
		t.Fatalf("Script = %q", lib.Scenes[0].Script)
	}
}

func TestScanRenPyNoGameDir(t *testing.T) {
	_, err := ScanWith(t.TempDir(), adapter.DefaultRenPy())
	if err == nil {
		t.Fatal("没有 game 目录应报错")
	}
}
