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

// TestScanArtemisScripts 扫 Artemis 目录 → 一份 .ast 一个场景事件。
func TestScanArtemisScripts(t *testing.T) {
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
	if len(lib.Scenes) != 1 {
		t.Fatalf("场景事件数 = %d, want 1（一份剧本一个事件）：%+v", len(lib.Scenes), lib.Scenes)
	}
	if lib.Scenes[0].ID != "01_01" {
		t.Fatalf("事件 id = %q", lib.Scenes[0].ID)
	}
	// 标题取首个有 savetitle 的 block。
	if lib.Scenes[0].Title != "银色、遥か" {
		t.Errorf("事件标题 = %q", lib.Scenes[0].Title)
	}
	// 新口径的事件不带 block 序号（回放走整份剧本）。
	if lib.Scenes[0].Label != "" {
		t.Errorf("事件 Label = %q, want 空", lib.Scenes[0].Label)
	}
	// 脚本路径相对 root。
	if lib.Scenes[0].Script != "scenario/01_01.ast" {
		t.Errorf("脚本路径 = %q", lib.Scenes[0].Script)
	}
	// 找得到
	if lib.FindScene("01_01") == nil {
		t.Error("FindScene 应能找到 01_01")
	}
}

// TestArtemisBuildShowScript 整份剧本拼成一个事件：两个 block 的演出都在。
func TestArtemisBuildShowScript(t *testing.T) {
	ev, err := artemis.BuildShowScript([]byte(artemisAST), "01_01.ast")
	if err != nil {
		t.Fatalf("BuildShowScript: %v", err)
	}
	if ev == nil || len(ev.Show) == 0 {
		t.Fatal("整份剧本应有演出内容")
	}
	// 两个 block 各自的背景都会出现。
	refs := map[string]bool{}
	for _, op := range ev.Show {
		if op.Ref != "" {
			refs[op.Ref] = true
		}
	}
	if !refs["black"] || !refs["ev_cut_001_00"] || !refs["ev_cut_002_00"] {
		t.Fatalf("两个 block 的图都应出现，实际 refs=%v", refs)
	}
	if ev.EndMs <= 0 {
		t.Error("整份剧本的结束时刻应大于 0")
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
