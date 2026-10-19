package library

import (
	"os"
	"path/filepath"
	"testing"
)

// writeScript 在临时解包树的剧本目录写一个 .ks。
func writeScript(t *testing.T, root, name, body string) {
	t.Helper()
	dir := ScenarioDir(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("建目录: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatalf("写脚本: %v", err)
	}
}

// TestScanChapters：场景脚本里的章节标记要进库，且按出现序号可区分重号。
func TestScanChapters(t *testing.T) {
	root := t.TempDir()
	// 补足标签行以过明文探测门槛；章节之间故意夹普通标签。
	body := "[wait time=100]\r\n[trans time=200]\r\n" +
		"*label92|余话-着陆2\r\n[r]\r\n*kota1\r\n[cm]\r\n" +
		"*label93|余话-水畔\r\n[r]\r\n" +
		"*label94|余话-聚餐\r\n[cm]\r\n" +
		"*label94|余话-港镇\r\n[r]\r\n"
	writeScript(t, root, "71kaigan01.ks", body)
	writeScript(t, root, "01kuukou.ks", "[wait time=100]\r\n[r]\r\n[cm]\r\n")

	lib, err := Scan(root)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(lib.Scenes) != 2 {
		t.Fatalf("场景数 = %d，期望 2", len(lib.Scenes))
	}
	if len(lib.Chapters) != 4 {
		t.Fatalf("演出段落数 = %d，期望 4：%+v", len(lib.Chapters), lib.Chapters)
	}
	wantTitles := []string{"余话-着陆2", "余话-水畔", "余话-聚餐", "余话-港镇"}
	for i, ch := range lib.Chapters {
		if ch.Scene != "71kaigan01" {
			t.Errorf("第 %d 段 Scene = %q", i, ch.Scene)
		}
		if ch.Title != wantTitles[i] {
			t.Errorf("第 %d 段 Title = %q，期望 %q", i, ch.Title, wantTitles[i])
		}
	}
	// 同号章节必须靠 LabelPos 区分，id 也要唯一。
	ids := map[string]bool{}
	for _, ch := range lib.Chapters {
		if ids[ch.ID] {
			t.Errorf("章节 id 重复：%s", ch.ID)
		}
		ids[ch.ID] = true
	}
}

// TestFindChapter：按 id 与按标题都能定位到章节。
func TestFindChapter(t *testing.T) {
	lib := &Library{Chapters: []ChapterEvent{
		{ID: "71kaigan01#27", Index: 93, Title: "余话-水畔", LabelPos: 27},
		{ID: "71kaigan01#34", Index: 94, Title: "余话-聚餐", LabelPos: 34},
	}}
	if ch := lib.FindChapter("71kaigan01#27"); ch == nil || ch.LabelPos != 27 {
		t.Errorf("按 id 未找到：%+v", ch)
	}
	if ch := lib.FindChapter("余话-聚餐"); ch == nil || ch.LabelPos != 34 {
		t.Errorf("按标题未找到：%+v", ch)
	}
	if ch := lib.FindChapter("不存在"); ch != nil {
		t.Errorf("不该找到：%+v", ch)
	}
}
