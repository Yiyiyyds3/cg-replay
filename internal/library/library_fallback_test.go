package library

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFallbackStoryScenes：没有编号剧本的 KiriKiri 标题（如 g01.ks）也能
// 扫出场景；系统脚本即使有章节标记，因体积太小不会被当成故事场景。
func TestFallbackStoryScenes(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "raw", "data", "scenario")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, src string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// 补足 8 KiB：回退只收有实际体量的故事脚本。
	pad := strings.Repeat("; padding\n", 1000)
	write("g01.ks", "*label1|Prologue\r\n"+
		"line one[l][r]\r\n"+pad)
	write("start.ks", "*start|开始\r\n[image storage=\"title\"]\r\n")

	lib, err := Scan(root)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(lib.Scenes) != 1 || lib.Scenes[0].ID != "g01" {
		t.Fatalf("回退场景 = %+v，期望只有 g01", lib.Scenes)
	}
	if len(lib.Chapters) != 1 {
		t.Fatalf("章节数 = %d，期望 1", len(lib.Chapters))
	}
	if got := lib.Chapters[0].Title; got != "Prologue" {
		t.Errorf("章节标题 = %q，期望 Prologue", got)
	}
}
