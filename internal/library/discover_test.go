package library

import (
	"os"
	"path/filepath"
	"testing"
)

// TestScriptFilesAutoDiscover 剧本目录不在约定路径时按扩展名自动探测。
func TestScriptFilesAutoDiscover(t *testing.T) {
	root := t.TempDir()
	// 模拟 Artemis 的多卷解包布局：文本/<卷>.pfs/script/*.ast
	deep := filepath.Join(root, "文本", "game.pfs", "script")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(deep, "01_01.ast"), []byte("ast"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 干扰项：别处的同名后缀（备份目录）也应被收集到，但排序稳定。
	other := filepath.Join(root, "备份", "script")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(other, "01_01.ast"), []byte("ast"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := scriptFiles(root, "", ".ast", discoverMaxDepth)
	if len(got) != 2 {
		t.Fatalf("收集到 %d 个 .ast，want 2：%v", len(got), got)
	}
	// 重名时事件 id 退回相对路径，避免撞车。
	stem := uniqueStems(root, got)
	if stem(got[0]) == stem(got[1]) {
		t.Fatalf("重名剧本的 id 不应相同：%q", stem(got[0]))
	}

	// 约定指定了存在的目录时只在该目录下找。
	if got := scriptFiles(root, "文本", ".ast", discoverMaxDepth); len(got) != 1 {
		t.Fatalf("约定收窄后 = %d 个，want 1：%v", len(got), got)
	}
}

// TestFindKiriKiriScriptDir 优先选名为 scenario 的目录，而不是 .ks 更多的系统目录。
func TestFindKiriKiriScriptDir(t *testing.T) {
	root := t.TempDir()
	scn := filepath.Join(root, "raw", "data_zh", "scenario")
	sys := filepath.Join(root, "raw", "data", "sysscn")
	for _, d := range []string{scn, sys} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, n := range []string{"a.ks", "b.ks"} {
		if err := os.WriteFile(filepath.Join(scn, n), []byte("*label1|x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, n := range []string{"s1.ks", "s2.ks", "s3.ks", "s4.ks"} {
		if err := os.WriteFile(filepath.Join(sys, n), []byte("system\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if got := findKiriKiriScriptDir(root); got != scn {
		t.Fatalf("探测到 %q，want %q", got, scn)
	}
}
