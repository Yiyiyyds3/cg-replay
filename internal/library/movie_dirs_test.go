package library

import (
	"os"
	"path/filepath"
	"testing"

	"ucgr/internal/adapter"
)

// TestMovieDirs 影片脚本目录：只收含影片指令的脚本，系统 UI 脚本不进库。
func TestMovieDirs(t *testing.T) {
	root := t.TempDir()
	scn := filepath.Join(root, "raw", "data", "scenario")
	sys := filepath.Join(root, "raw", "data", "sysscn")
	for _, d := range []string{scn, sys} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(dir, name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// 剧本目录：一个普通场景
	write(scn, "01_story.ks", "[image storage=\"a.png\"]\n[wait time=100]\n")
	// 影片目录：真影片 / 只是关掉视频图层 / 普通 UI 脚本
	write(sys, "end.ks", "[sysmovie state=end]\n")
	write(sys, "title.ks", "[video visible=false]\n")
	write(sys, "brows.ks", "[button storage=\"b.png\"]\n")

	lib, err := ScanWith(root, adapter.DefaultKiriKiri())
	if err != nil {
		t.Fatalf("ScanWith: %v", err)
	}
	var videos []string
	for _, s := range lib.Scenes {
		if s.HasVideo {
			videos = append(videos, s.ID)
		}
	}
	if len(videos) != 1 || videos[0] != "end" {
		t.Fatalf("应只收进 end.ks 一个影片事件，实际 %v", videos)
	}
	// UI 脚本不能进库（否则事件列表里全是 brows/save/option）
	for _, s := range lib.Scenes {
		if s.ID == "title" || s.ID == "brows" {
			t.Fatalf("系统 UI 脚本不该进库：%s", s.ID)
		}
	}
}
