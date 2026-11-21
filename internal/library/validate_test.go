package library

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 造一个最小素材树：root/img/EV001.png + root/img/EV002.rpgmvp（加密形态）。
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, content := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// 用例 ①：Pages 指向不存在素材的 CG 事件被剔除，命中的保留。
func TestValidateDropsEmptyCG(t *testing.T) {
	root := writeTree(t, map[string]string{"img/cg001.png": "x"})
	lib := &Library{
		EngineID: "kirikiri",
		CGs: []CGEvent{
			{ID: "cg001", Pages: []string{"img/cg001"}},
			{ID: "cg099", Pages: []string{"img/cg099"}},
		},
	}
	rep := lib.Validate(root)
	if rep.CGKept != 1 || rep.CGDroppedEmpty != 1 {
		t.Fatalf("kept=%d dropped=%d，want 1/1", rep.CGKept, rep.CGDroppedEmpty)
	}
	if len(lib.CGs) != 1 || lib.CGs[0].ID != "cg001" {
		t.Fatalf("留下的 CG = %v，want [cg001]", lib.CGs)
	}
	if len(rep.Notes) == 0 || len(rep.Dropped) == 0 {
		t.Fatalf("报告缺 Notes/Dropped：%v / %v", rep.Notes, rep.Dropped)
	}
}

// 用例 ②：rmmz 特殊事件（标题带 EV）保留；纯常规事件剔除；加密图也算素材在。
func TestValidateRMMSpecialScenes(t *testing.T) {
	root := writeTree(t, map[string]string{
		"img/pictures/EV001.rpgmvp": "enc",
		"img/pictures/bg001.png":    "x",
	})
	lib := &Library{
		EngineID: "rmmz",
		CGs:      []CGEvent{},
		Scenes: []SceneEvent{
			{ID: "map:Map002.json:15:0#0", Title: "芙拉村 · EV015", Script: "www/data/Map002.json"},
			{ID: "map:Map001.json:3:0#0", Title: "村口 · EV003", Script: "www/data/Map001.json"},
		},
	}
	rep := lib.Validate(root)
	// rmmz 的 map: 场景由扫描器 pictureLarge 把关，validate 全保留（名字特征
	// 二次筛会误杀「CG演出」这类命名）。
	if rep.SceneKept != 2 || rep.SceneDropped != 0 {
		t.Fatalf("kept=%d dropped=%d，want 2/0", rep.SceneKept, rep.SceneDropped)
	}
	// 纯常规命名事件同样保留（扫描器已筛过）。
	lib2 := &Library{
		EngineID: "rmmz",
		Scenes: []SceneEvent{
			{ID: "map:Map003.json:1:0#0", Title: "路边捡到十枚金币", Script: "www/data/Map003.json"},
		},
	}
	rep2 := lib2.Validate(root)
	if rep2.SceneKept != 1 || rep2.SceneDropped != 0 {
		t.Fatalf("map场景 kept=%d dropped=%d，want 1/0", rep2.SceneKept, rep2.SceneDropped)
	}
}

// 用例 ③：安全阀——root 下看不到任何素材时不许把库杀光。
func TestValidateSkipWhenIndexEmpty(t *testing.T) {
	root := t.TempDir() // 空目录
	lib := &Library{
		EngineID: "rmmz",
		CGs:      []CGEvent{{ID: "cg001", Pages: []string{"img/cg001"}}},
		Scenes:   []SceneEvent{{ID: "map:Map001.json:1:0#0", Title: "事件一"}},
	}
	rep := lib.Validate(root)
	if rep.CGKept != 1 || rep.SceneKept != 1 {
		t.Fatalf("安全阀失效：cg=%d scene=%d，want 原样保留", rep.CGKept, rep.SceneKept)
	}
	if len(lib.CGs) != 1 || len(lib.Scenes) != 1 {
		t.Fatalf("库被空索引误杀")
	}
	skipped := false
	for _, n := range rep.Notes {
		if strings.Contains(n, "未执行过滤") {
			skipped = true
		}
	}
	if !skipped {
		t.Fatalf("Notes 未交代跳过原因：%v", rep.Notes)
	}
}

// 用例 ④：rmmz map 场景与其章节在新口径下全保留（扫描器 pictureLarge 已把关）。
func TestValidateChaptersFollowScenes(t *testing.T) {
	root := writeTree(t, map[string]string{"img/x.png": "x"})
	lib := &Library{
		EngineID: "rmmz",
		Scenes: []SceneEvent{
			{ID: "map:Map001.json:2:0#0", Title: "普通名字的CG演出事件"},
		},
		Chapters: []ChapterEvent{
			{ID: "map:Map001.json:2:0#0#0", Scene: "map:Map001.json:2:0#0", Title: "章"},
		},
	}
	rep := lib.Validate(root)
	if rep.SceneKept != 1 || rep.SceneDropped != 0 || len(lib.Chapters) != 1 {
		t.Fatalf("map 场景应保留：kept=%d dropped=%d chapters=%d", rep.SceneKept, rep.SceneDropped, len(lib.Chapters))
	}
}
