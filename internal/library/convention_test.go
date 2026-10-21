package library

import (
	"os"
	"path/filepath"
	"testing"

	"ucgr/internal/adapter"
)

// TestScanWithCustomConvention 验证「换一款同引擎游戏 = 改配置不改代码」：
// 同样的脚本内容放到**别的目录、用别的文件命名**，用自定义约定扫出来的
// 事件与默认约定下等价。若哪天 Scan 又写死了目录/命名，这个测试会红。
func TestScanWithCustomConvention(t *testing.T) {
	body := "[wait time=100]\r\n[r]\r\n[cm]\r\n" +
		"*label92|余话-着陆2\r\n[r]\r\n" +
		"*label93|余话-水畔\r\n[r]\r\n"

	// ① 默认约定（raw/data/scenario + 01kuukou.ks 这种命名）。
	rootA := t.TempDir()
	writeScript(t, rootA, "01kuukou.ks", body)
	libA, err := Scan(rootA)
	if err != nil {
		t.Fatalf("默认约定 Scan: %v", err)
	}
	if len(libA.Scenes) == 0 {
		t.Fatal("默认约定应扫出场景")
	}

	// ② 另一套目录 + 命名：data/script/scene_01.ks（约定文件覆盖）。
	conv := adapter.Convention{
		EngineID:     "kirikiri",
		ScenarioDir:  filepath.Join("data", "script"),
		ScriptExt:    ".ks",
		GalleryRe:    `^gallery.*\.ks$`,
		GalleryIndex: "gallery.ks",
		CGLabelRe:    `^cg\d+$`,
		SceneRe:      `^scene_\d+\.ks$`,
	}
	rootB := t.TempDir()
	dirB := filepath.Join(rootB, filepath.FromSlash(conv.ScenarioDir))
	if err := os.MkdirAll(dirB, 0o755); err != nil {
		t.Fatalf("建目录: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dirB, "scene_01.ks"), []byte(body), 0o644); err != nil {
		t.Fatalf("写脚本: %v", err)
	}
	libB, err := ScanWith(rootB, conv)
	if err != nil {
		t.Fatalf("自定义约定 ScanWith: %v", err)
	}
	if len(libB.Scenes) != len(libA.Scenes) {
		t.Fatalf("换约定后场景数 = %d，默认 = %d，应等价",
			len(libB.Scenes), len(libA.Scenes))
	}
	if len(libB.Chapters) != len(libA.Chapters) {
		t.Fatalf("换约定后段落数 = %d，默认 = %d，应等价",
			len(libB.Chapters), len(libA.Chapters))
	}
	if len(libB.Scenes) > 0 {
		// 事件 id 来自文件名（去后缀）——换了命名 id 自然不同，这是预期。
		if libB.Scenes[0].ID != "scene_01" {
			t.Fatalf("自定义约定的场景 id = %q", libB.Scenes[0].ID)
		}
	}

	// ③ 用默认约定扫同一个目录 → 扫不到剧本目录（证明约定确实在起作用）。
	if _, err := Scan(rootB); err == nil {
		t.Fatal("默认约定应读不到 data/script 目录")
	}
}
