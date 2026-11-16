// AddGame 端到端测试：通过 t.Setenv 把 %LOCALAPPDATA% 重定向到临时目录
// （datadir_test.go 同款机制），测试只在临时目录读写 games.json/libraries/
// catalog.db，**不会污染真实 %LOCALAPPDATA%\cgreplay-ui**。
//
// 覆盖用户报的端到端假通过：导出 CG 库（cg-index.json + cg/）与原生
// RPG Maker MV/MZ 目录此前都在 AddGame 第二步（resolveGameConvention）被拒。
package main

import (
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ucgr/internal/library"
	"ucgr/internal/registry"
)

// redirectDataDir 把可写数据目录指到临时目录并返回其路径。
func redirectDataDir(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("LOCALAPPDATA", tmp)
	t.Setenv("APPDATA", filepath.Join(tmp, "roaming"))
	return filepath.Join(tmp, "cgreplay-ui")
}

func writeFileT(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// writePNGT 生成一张 400×300 全不透明 PNG（pictureLarge 判定要读图）。
func writePNGT(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 400, 300))
	for y := 0; y < 300; y++ {
		for x := 0; x < 400; x++ {
			img.Set(x, y, color.RGBA{R: 210, G: 90, B: 130, A: 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

// makeUnityExportFixture 造一个与真实导出器产物同布局的 CG 库目录。
func makeUnityExportFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFileT(t, filepath.Join(root, "cg-index.json"), `{
  "schema": 1,
  "engine": "unity",
  "game": "测试导出库",
  "cgs": [
    {"id": "cg001", "kind": "still", "category": "卧室",
     "files": {"png": ["cg001_0.png"]}},
    {"id": "cg002", "kind": "frames",
     "files": {"png": ["cg002_0.png", "cg002_1.png"]}}
  ]
}`)
	// 导出器会带一份约定文件（engine_id=unity）；修复前 DetectConvention
	// 对它报「未知引擎 "unity"」。
	writeFileT(t, filepath.Join(root, "ucgr-convention.json"),
		`{"engine_id": "unity", "scenario_dir": "cg"}`)
	writeFileT(t, filepath.Join(root, "cg", "cg001", "cg001_0.png"), "png")
	return root
}

// makeRMMZFixture 造一个原生 RPG Maker MV/MZ 目录（www/data + 一段 CG 演出）。
func makeRMMZFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFileT(t, filepath.Join(root, "www", "data", "System.json"),
		`{"gameTitle":"RMMZ测试"}`)
	writeFileT(t, filepath.Join(root, "www", "data", "Map001.json"), `{
  "displayName": "房间",
  "events": [
    {"id": 1, "name": "CG演出", "pages": [{"list": [
      {"code": 231, "indent": 0, "parameters": [1, "cg001", 0, 0, 0, 0, 100, 100, 255]},
      {"code": 401, "indent": 0, "parameters": ["这是测试台词，用来凑一个文本指令"]},
      {"code": 235, "indent": 0, "parameters": [0]}
    ]}]}
  ]
}`)
	writePNGT(t, filepath.Join(root, "www", "img", "pictures", "cg001.png"))
	return root
}

func TestAddGameExportedCGLibrary(t *testing.T) {
	dd := redirectDataDir(t)
	root := makeUnityExportFixture(t)

	var a App
	// AddGame 里 syncCatalog 会打开 catalog.db：测试结束前必须关闭，
	// 否则 Windows 上 t.TempDir 清理删不掉被占用的文件。
	defer a.shutdown(context.Background())
	lib, err := a.AddGame(root)
	if err != nil {
		t.Fatalf("导出 CG 库 AddGame 失败（用户报的假通过）：%.400v", err)
	}
	if lib.EngineID != "unity" {
		t.Errorf("lib.EngineID = %q, want unity", lib.EngineID)
	}
	// ScanUnity 把 CG 事件列进 Scenes（lib.CGs/Chapters 恒空）；
	// 事件总数 > 0 才算真正入库。
	if len(lib.CGs)+len(lib.Scenes)+len(lib.Chapters) == 0 {
		t.Fatal("导出库扫出 0 个事件")
	}
	if len(lib.Scenes) != 2 {
		t.Errorf("Scenes = %d, want 2（索引里 2 部可播 CG）", len(lib.Scenes))
	}

	// 登记表落在重定向的 dataDir，且引擎号已回填。
	reg, err := registry.Load(filepath.Join(dd, registry.FileName))
	if err != nil {
		t.Fatalf("读登记表：%v", err)
	}
	g := reg.Find(registry.IDFor(root))
	if g == nil {
		t.Fatal("导出库没有登记进 games.json")
	}
	if g.Engine != "unity" {
		t.Errorf("登记引擎 = %q, want unity", g.Engine)
	}
	// 库清单已落盘且可读回。
	if _, err := library.Load(g.Library); err != nil {
		t.Fatalf("库清单落盘不可读：%v", err)
	}
}

func TestAddGameNativeRMMZ(t *testing.T) {
	dd := redirectDataDir(t)
	root := makeRMMZFixture(t)

	var a App
	defer a.shutdown(context.Background())
	lib, err := a.AddGame(root)
	if err != nil {
		t.Fatalf("原生 rmmz 目录 AddGame 失败（junction 登记场景同根因）：%.400v", err)
	}
	if lib.EngineID != "rmmz" {
		t.Errorf("lib.EngineID = %q, want rmmz", lib.EngineID)
	}
	if len(lib.Scenes) == 0 {
		t.Fatal("rmmz 扫出 0 个事件（231+401 演出应产出 1 段）")
	}

	reg, err := registry.Load(filepath.Join(dd, registry.FileName))
	if err != nil {
		t.Fatalf("读登记表：%v", err)
	}
	g := reg.Find(registry.IDFor(root))
	if g == nil {
		t.Fatal("rmmz 目录没有登记进 games.json")
	}
	if g.Engine != "rmmz" {
		t.Errorf("登记引擎 = %q, want rmmz", g.Engine)
	}
}

func TestAddGameFakeDirActionableError(t *testing.T) {
	dd := redirectDataDir(t)
	root := t.TempDir()
	writeFileT(t, filepath.Join(root, "readme.txt"), "随便放点东西")

	var a App
	_, err := a.AddGame(root)
	if err == nil {
		t.Fatal("陌生目录应该被拒")
	}
	// 报错要可操作：说明这不是可识别的游戏目录、给出引擎特征清单
	// （含 cg-index.json），并保留底层原因（%w 包装）。
	msg := err.Error()
	for _, want := range []string{
		"这不是可识别的游戏目录",
		"cg-index.json",
		"没有找到可回放的剧本",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("报错缺关键信息 %q：%.400v", want, err)
		}
	}
	// 失败路径不落登记表（games.json 不应被创建）。
	if _, serr := os.Stat(filepath.Join(dd, registry.FileName)); serr == nil {
		t.Fatalf("AddGame 失败却写了 %s", registry.FileName)
	}
}
