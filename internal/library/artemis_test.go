package library

import (
	"os"
	"path/filepath"
	"testing"

	"ucgr/internal/adapter"
	"ucgr/internal/adapter/artemis"
)

// artemisAST 一个含两个 block 的 Artemis 剧本（明文 Lua 表）。
const artemisAST = `astver = 2.0
astname = "ast"
ast = {
	block_00000 = {
		{"savetitle", text="银色、遥か"},
		{"bg", time=2000, file="black", path=":bg/"},
		{"bg", file="ev_cut_001_00", id=1, lv=5, path=":cg/", sync=0},
		{"ex", time=2000, func="wait"},
	},
	block_00001 = {
		{"bg", file="ev_cut_002_00", id=1, lv=5, path=":cg/", sync=0},
		{"ex", time=1000, func="wait"},
	},
	label = {
		top = { block="block_00000", label=1 },
	},
}
`

// TestScanArtemisBlocks 扫 Artemis 目录 → 每个 block 一个场景事件。
func TestScanArtemisBlocks(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "scenario")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "01_01.ast"), []byte(artemisAST), 0o644); err != nil {
		t.Fatal(err)
	}
	conv := adapter.Convention{
		EngineID:    "artemis",
		ScenarioDir: "scenario",
		ScriptExt:   ".ast",
	}
	lib, err := ScanWith(root, conv)
	if err != nil {
		t.Fatalf("ScanWith: %v", err)
	}
	if len(lib.Scenes) != 2 {
		t.Fatalf("场景事件数 = %d, want 2（每个 block 一个）：%+v", len(lib.Scenes), lib.Scenes)
	}
	if lib.Scenes[0].ID != "01_01#1" || lib.Scenes[1].ID != "01_01#2" {
		t.Fatalf("事件 id = %q, %q", lib.Scenes[0].ID, lib.Scenes[1].ID)
	}
	// 第一个 block 有 savetitle → 标题取它。
	if lib.Scenes[0].Title != "银色、遥か" {
		t.Errorf("第一个事件标题 = %q", lib.Scenes[0].Title)
	}
	// Label 记 block 序号（回放时据此重建演出）。
	if lib.Scenes[0].Label != "1" || lib.Scenes[1].Label != "2" {
		t.Errorf("事件 Label = %q, %q", lib.Scenes[0].Label, lib.Scenes[1].Label)
	}
	// 脚本路径相对 root。
	if lib.Scenes[0].Script != "scenario/01_01.ast" {
		t.Errorf("脚本路径 = %q", lib.Scenes[0].Script)
	}
	// 找得到
	if lib.FindScene("01_01#2") == nil {
		t.Error("FindScene 应能找到 01_01#2")
	}
}

// TestArtemisBuildShowBlock 按 block 序号构建演出事件。
func TestArtemisBuildShowBlock(t *testing.T) {
	ev, err := artemis.BuildShowBlock([]byte(artemisAST), "01_01.ast", 2)
	if err != nil {
		t.Fatalf("BuildShowBlock: %v", err)
	}
	if ev == nil || len(ev.Show) == 0 {
		t.Fatal("第二个 block 应有演出内容")
	}
	if ev.EventID == "" {
		t.Error("事件应有 ID")
	}
	// 越界要报错，不静默返回空。
	if _, err := artemis.BuildShowBlock([]byte(artemisAST), "01_01.ast", 99); err == nil {
		t.Error("越界的 block 序号应报错")
	}
}
