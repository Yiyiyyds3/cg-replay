package library

import (
	"os"
	"path/filepath"
	"testing"
)

// TestChapterPrefixAttribution：章前段按章节号归属到前序场景的末章。
//
// 夹具：01a 有 label1，02b 的开头（label2 之前）是 01a 未完的演出。
// 期望：01a 的章节带 cont_script=02b，合并后比本体多出台词。
func TestChapterPrefixAttribution(t *testing.T) {
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
	// 两个文件都要有足够的标签行过明文探测门槛。
	write("01a.ks", "*label1|甲\r\n"+
		"甲台词一[l][r]\r\n"+
		"甲台词二[l][r]\r\n")
	write("02b.ks", "乙章前台词一[l][r]\r\n"+
		"乙章前台词二[l][r]\r\n"+
		"*label2|乙\r\n"+
		"乙台词一[l][r]\r\n")

	lib, err := Scan(root)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(lib.Chapters) != 2 {
		t.Fatalf("章节数 = %d，期望 2", len(lib.Chapters))
	}
	var first, second *ChapterEvent
	for i := range lib.Chapters {
		switch lib.Chapters[i].Index {
		case 1:
			first = &lib.Chapters[i]
		case 2:
			second = &lib.Chapters[i]
		}
	}
	if first == nil || second == nil {
		t.Fatalf("缺少 label1/label2 章节：%+v", lib.Chapters)
	}
	if first.ContScript != second.Script {
		t.Errorf("第一章 cont_script = %q，期望 %q", first.ContScript, second.Script)
	}
	if second.ContScript != "" {
		t.Errorf("末章不该再接续，得到 %q", second.ContScript)
	}

	base, err := BuildChapterEvent(root, &ChapterEvent{
		ID: first.ID, Script: first.Script, LabelPos: first.LabelPos,
		Scene: first.Scene, Name: first.Name, Index: first.Index, Title: first.Title,
	})
	if err != nil {
		t.Fatalf("基准构建: %v", err)
	}
	merged, err := BuildChapterEvent(root, first)
	if err != nil {
		t.Fatalf("合并构建: %v", err)
	}
	if len(merged.Show) <= len(base.Show) {
		t.Fatalf("合并后 Show = %d，本体 = %d，章前段没有接进来", len(merged.Show), len(base.Show))
	}
	for i := 1; i < len(merged.Show); i++ {
		if merged.Show[i].TMs < merged.Show[i-1].TMs {
			t.Fatalf("合并后 TMs 不单调：op[%d]=%d < op[%d]=%d",
				i, merged.Show[i].TMs, i-1, merged.Show[i-1].TMs)
		}
	}
}
