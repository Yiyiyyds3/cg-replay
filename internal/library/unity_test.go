package library

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"ucgr/internal/adapter"
)

// writeUnityIndex 造一个导出器产出形状的最小索引。
func writeUnityIndex(t *testing.T, root string, cgs []map[string]any) {
	t.Helper()
	idx := map[string]any{"schema": 1, "engine": "unity", "game": "probe", "cgs": cgs}
	data, err := json.Marshal(idx)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "cg-index.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestScanUnity frames / still / spine 现在都能播（spine 由前端 WebGL 渲染）。
func TestScanUnity(t *testing.T) {
	root := t.TempDir()
	writeUnityIndex(t, root, []map[string]any{
		{"id": "Witch01", "kind": "frames", "category": "战斗H·女巫",
			"frames": []string{"1.png", "2.png"}},
		{"id": "OpeningCG00", "kind": "still", "category": "开场",
			"files": map[string]any{"png": []string{"OpeningCG00.png"}}},
		{"id": "HCG02", "kind": "spine", "category": "战斗H",
			"files": map[string]any{
				"text": []string{"HCG_02.json", "HCG_02.atlas"},
				"png":  []string{"HCG_02.png"},
			}},
	})

	lib, err := ScanWith(root, adapter.Convention{EngineID: "unity"})
	if err != nil {
		t.Fatalf("ScanWith: %v", err)
	}
	if lib.EngineID != "unity" {
		t.Fatalf("引擎应为 unity，实际 %q", lib.EngineID)
	}
	if len(lib.Scenes) != 3 {
		t.Fatalf("应列出 3 部可播 CG，实际 %d", len(lib.Scenes))
	}
	ids := map[string]bool{}
	for _, s := range lib.Scenes {
		ids[s.ID] = true
		if s.Script == "" {
			t.Fatalf("%s 没有 script 字段", s.ID)
		}
	}
	if !ids["Witch01"] || !ids["OpeningCG00"] || !ids["HCG02"] {
		t.Fatalf("frames/still/spine 都应列出，实际 %v", ids)
	}
}

// TestScanUnityOnlySpine 只有 spine 也能出库（前端 WebGL 渲染）。
func TestScanUnityOnlySpine(t *testing.T) {
	root := t.TempDir()
	writeUnityIndex(t, root, []map[string]any{
		{"id": "HCG02", "kind": "spine", "category": "战斗H",
			"files": map[string]any{
				"text": []string{"HCG_02.json", "HCG_02.atlas"},
				"png":  []string{"HCG_02.png"},
			}},
	})
	lib, err := ScanWith(root, adapter.Convention{EngineID: "unity"})
	if err != nil {
		t.Fatalf("只有 spine 时不应报错：%v", err)
	}
	if len(lib.Scenes) != 1 || lib.Scenes[0].ID != "HCG02" {
		t.Fatalf("应列出 spine 事件，实际 %+v", lib.Scenes)
	}
}

// TestBuildUnityEvent 按 id 取到 CG 记录。
func TestBuildUnityEvent(t *testing.T) {
	root := t.TempDir()
	writeUnityIndex(t, root, []map[string]any{
		{"id": "Witch01", "kind": "frames", "frames": []string{"1.png"}},
	})
	cg, err := BuildUnityEvent(root, "Witch01")
	if err != nil {
		t.Fatalf("BuildUnityEvent: %v", err)
	}
	if cg.Kind != "frames" || len(cg.Frames) != 1 {
		t.Fatalf("取到的记录不对：%+v", cg)
	}
	if _, err := BuildUnityEvent(root, "nope"); err == nil {
		t.Fatal("不存在的 id 应报错")
	}
}
