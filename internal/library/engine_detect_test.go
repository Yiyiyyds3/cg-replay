package library

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ucgr/internal/adapter"
)

// TestPackedEngineOf 只靠原作的打包签名也能认出引擎（用来把「没解包」说清楚）。
func TestPackedEngineOf(t *testing.T) {
	cases := []struct {
		files []string
		want  string
	}{
		{[]string{"data.xp3"}, "kirikiri"},
		{[]string{"BGI.hvl", "BGI.sdb"}, "bgi"},
		{[]string{"灰色的果实/boot.dfn"}, "catsystem2"},
		{[]string{"GrisaiaAno1_cn.int"}, "catsystem2"},
		{[]string{"tanetsumi.pfs", "tanetsumi.pfs.000"}, "artemis"},
		{[]string{"readme.txt", "说明.txt"}, ""},
	}
	for _, c := range cases {
		root := t.TempDir()
		for _, name := range c.files {
			p := filepath.Join(root, filepath.FromSlash(name))
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		got, evidence := PackedEngineOf(root)
		if got != c.want {
			t.Errorf("%v → 引擎 %q, want %q", c.files, got, c.want)
			continue
		}
		if got != "" && evidence == "" {
			t.Errorf("%v → 认出 %q 却没给证据文件名", c.files, got)
		}
	}
}

// TestDetectConventionNotUnpacked 没解包的目录要报「还没解包」，不能报成引擎不匹配。
func TestDetectConventionNotUnpacked(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "data.xp3"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err := DetectConvention(root)
	if err == nil || !strings.Contains(err.Error(), "还没解包") {
		t.Fatalf("打包目录的报错 = %v, want 含「还没解包」", err)
	}

	empty := t.TempDir()
	_, _, err = DetectConvention(empty)
	if err == nil || !strings.Contains(err.Error(), "没有找到可回放的剧本") {
		t.Fatalf("空目录的报错 = %v, want 含「没有找到可回放的剧本」", err)
	}
}

// TestSCN整理布局 9nine「解包整理/data_脚本与数据/scn」这种布局不必手写约定。
func TestSCN整理布局(t *testing.T) {
	root := t.TempDir()
	scnDir := filepath.Join(root, "解包整理", "data_脚本与数据", "scn")
	if err := os.MkdirAll(scnDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"b01.txt.scn", "b02.txt.scn"} {
		if err := os.WriteFile(filepath.Join(scnDir, n), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cgDir := filepath.Join(root, "解包整理", "data_脚本与数据", "main")
	if err := os.MkdirAll(cgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cgPath := filepath.Join(cgDir, "cglist.csv")
	if err := os.WriteFile(cgPath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ResolveScenarioDir(root, adapter.Convention{EngineID: "scn"})
	if err != nil {
		t.Fatalf("ResolveScenarioDir: %v", err)
	}
	if got != scnDir {
		t.Errorf("剧本目录 = %q, want %q", got, scnDir)
	}
	if cg := findCGList(root, ""); cg != cgPath {
		t.Errorf("cglist = %q, want %q", cg, cgPath)
	}
}

// TestDetectEngineUnityExported 导出 CG 库（cg-index.json + cg/）要认成 unity，
// 约定文件 engine_id=unity 也不能再报「未知引擎」——UI 添加导出库就死在这两步。
func TestDetectEngineUnityExported(t *testing.T) {
	write := func(root, name, content string) {
		p := filepath.Join(root, name)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// 真实导出布局：cg/ + cg-index.json + ucgr-convention.json(engine_id=unity)。
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "cg", "cg001"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(root, "cg-index.json", `{"schema":1,"engine":"unity","game":"x","cgs":[]}`)
	write(root, "ucgr-convention.json", `{"engine_id":"unity","scenario_dir":"cg"}`)

	if got := DetectEngine(root); got != "unity" {
		t.Fatalf("DetectEngine = %q, want unity", got)
	}
	engineID, conv, err := DetectConvention(root)
	if err != nil {
		t.Fatalf("DetectConvention（约定文件 engine_id=unity 的导出库）：%v", err)
	}
	if engineID != "unity" || conv.EngineID != "unity" {
		t.Fatalf("引擎 = %q/%q, want unity", engineID, conv.EngineID)
	}
	if conv.ScenarioDir != "cg" {
		t.Fatalf("剧本目录 = %q, want cg（约定文件指定）", conv.ScenarioDir)
	}

	// 没有约定文件的裸导出库也要过（ScanUnity 自己找索引，scenario_dir 留空合法）。
	bare := t.TempDir()
	write(bare, "cg-index.json", `{"schema":1,"engine":"unity","game":"x","cgs":[]}`)
	if got := DetectEngine(bare); got != "unity" {
		t.Fatalf("裸导出库 DetectEngine = %q, want unity", got)
	}
	if _, conv, err := DetectConvention(bare); err != nil {
		t.Fatalf("裸导出库 DetectConvention：%v", err)
	} else if err := conv.Validate(); err != nil {
		t.Fatalf("裸导出库约定 Validate：%v", err)
	}
}

// TestDetectEngineRMMZ 原生 RPG Maker MV/MZ 目录（www/data、data、内嵌/data）
// 不能再被当成「没解包的陌生目录」——junction 登记的 rmmz 游戏就死在这里。
func TestDetectEngineRMMZ(t *testing.T) {
	for _, layout := range []string{"www/data", "data", "内嵌/data"} {
		root := t.TempDir()
		dataDir := filepath.Join(root, filepath.FromSlash(layout))
		if err := os.MkdirAll(dataDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dataDir, "System.json"), []byte(`{"gameTitle":"t"}`), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := DetectEngine(root); got != "rmmz" {
			t.Errorf("%s 布局 DetectEngine = %q, want rmmz", layout, got)
		}
		_, conv, err := DetectConvention(root)
		if err != nil {
			t.Fatalf("%s 布局 DetectConvention：%v", layout, err)
		}
		if conv.EngineID != "rmmz" {
			t.Errorf("%s 布局引擎 = %q, want rmmz", layout, conv.EngineID)
		}
	}
}

// TestStageSkipsExtraAssets 壁纸/特典目录不参与舞台推断。
func TestStageSkipsExtraAssets(t *testing.T) {
	root := t.TempDir()
	wall := filepath.Join(root, "壁紙集")
	if err := os.MkdirAll(wall, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		writePNG(t, filepath.Join(wall, "壁紙0"+string(rune('1'+i))+".png"), 1920, 1170)
	}
	img := filepath.Join(root, "图片")
	if err := os.MkdirAll(img, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		writePNG(t, filepath.Join(img, "bg0"+string(rune('1'+i))+".png"), 1280, 720)
	}
	if w, h := DetectStageSize(root); w != 1280 || h != 720 {
		t.Errorf("舞台 = %d×%d, want 1280×720（壁纸不参与）", w, h)
	}

	only := t.TempDir()
	wall2 := filepath.Join(only, "壁紙集")
	if err := os.MkdirAll(wall2, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		writePNG(t, filepath.Join(wall2, "壁紙0"+string(rune('1'+i))+".png"), 1920, 1170)
	}
	if w, h := DetectStageSize(only); w != 0 || h != 0 {
		t.Errorf("只有壁纸时舞台 = %d×%d, want 未推断", w, h)
	}
}
