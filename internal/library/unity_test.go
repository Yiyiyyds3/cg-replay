package library

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

// TestScanUnityBinarySkel：SkeletonBinary 通道打通后，binary-skel（有文本+贴图）
// 应被列为可播事件；只有真正无法识别的 unknown 才被门禁拦下并如实报类型分布。
func TestScanUnityBinarySkel(t *testing.T) {
	root := t.TempDir()
	writeUnityIndex(t, root, []map[string]any{
		{"id": "bed_anna_1", "kind": "binary-skel", "category": "床",
			"files": map[string]any{
				"text": []string{"bed_anna_1.skel", "bed_anna_1.atlas"},
				"png":  []string{"bed_anna_1.png"},
			}},
		{"id": "orphan", "kind": "unknown"},
	})
	lib, err := ScanWith(root, adapter.Convention{EngineID: "unity"})
	if err != nil {
		t.Fatalf("含 binary-skel 的库不应报错：%v", err)
	}
	if len(lib.Scenes) != 1 || lib.Scenes[0].ID != "bed_anna_1" {
		t.Fatalf("应只列出 binary-skel 事件，实际 %+v", lib.Scenes)
	}
	if lib.Scenes[0].Label != "binary-skel" {
		t.Fatalf("Label 应保留 kind=binary-skel，实际 %q", lib.Scenes[0].Label)
	}
}

// TestScanUnityAllUnknown 全部不可识别时仍要报错，且错误里给出类型分布。
func TestScanUnityAllUnknown(t *testing.T) {
	root := t.TempDir()
	writeUnityIndex(t, root, []map[string]any{
		{"id": "orphan", "kind": "unknown"},
		{"id": "broken", "kind": "spine", "files": map[string]any{"text": []string{"x.json"}}},
	})
	_, err := ScanWith(root, adapter.Convention{EngineID: "unity"})
	if err == nil {
		t.Fatal("全部不可播时应报错")
	}
	msg := err.Error()
	for _, want := range []string{"unknown=1", "spine=1"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("错误应含 %q，实际：%s", want, msg)
		}
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
