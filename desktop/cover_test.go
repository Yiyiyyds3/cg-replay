package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestReadGameCover 根目录有 cover.png 就优先用它（这类 ACT 库统一用主界面当封面）。
func TestReadGameCover(t *testing.T) {
	root := t.TempDir()
	if _, ok := readGameCover(root); ok {
		t.Fatal("没有 cover.png 时不应命中")
	}
	if err := os.WriteFile(filepath.Join(root, "cover.png"), []byte("fake-png"), 0o644); err != nil {
		t.Fatal(err)
	}
	url, ok := readGameCover(root)
	if !ok {
		t.Fatal("有 cover.png 时应命中")
	}
	if !strings.HasPrefix(url, "data:image/png;base64,") {
		t.Fatalf("应返回 dataURL，实际 %q", url)
	}
	// 空文件不算封面（避免导出中断留下 0 字节文件时卡片全黑）
	if err := os.WriteFile(filepath.Join(root, "cover.png"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := readGameCover(root); ok {
		t.Fatal("0 字节的 cover.png 不应命中")
	}
	if _, ok := readGameCover(""); ok {
		t.Fatal("空 root 不应命中")
	}
}
