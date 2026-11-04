package library

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"ucgr/internal/adapter"
	"ucgr/internal/adapter/catsystem2"
)

// ---------------------------------------------------------------
// 两种合成 .cst 的辅助：两种参数风格各自贴近自己的测试用例。
// ---------------------------------------------------------------

// buildCSTFor 造一份最小 CatScene 文件（魔数 + u32 压缩长 + 16 字节头 + zlib）。
func buildCSTFor(t *testing.T, entries []struct {
	typ  byte
	body string
}) []byte {
	t.Helper()
	var raw []byte
	for _, e := range entries {
		raw = append(raw, 0x01, e.typ)
		raw = append(raw, []byte(e.body)...)
		raw = append(raw, 0)
	}
	var zbuf bytes.Buffer
	zw := zlib.NewWriter(&zbuf)
	if _, err := zw.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	head := make([]byte, 16)
	copy(head, "CatScene")
	binary.LittleEndian.PutUint32(head[8:12], uint32(zbuf.Len()))
	return append(head, zbuf.Bytes()...)
}

// makeCST 造一个最小 CatScene 容器：magic + u32 压缩长 + zlib(01 <type> <bytes> 00 …)。
func makeCST(t *testing.T, entries ...[2]interface{}) []byte {
	t.Helper()
	var raw []byte
	for _, e := range entries {
		typ := byte(e[0].(int))
		body := []byte(e[1].(string))
		raw = append(raw, 0x01, typ)
		raw = append(raw, body...)
		raw = append(raw, 0)
	}
	var zbuf bytes.Buffer
	zw := zlib.NewWriter(&zbuf)
	if _, err := zw.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	z := zbuf.Bytes()
	out := make([]byte, 16+len(z))
	copy(out, "CatScene")
	binary.LittleEndian.PutUint32(out[8:], uint32(len(z)))
	copy(out[16:], z)
	return out
}

// writePNG 写一张纯色 PNG（用于舞台尺寸推断）。
func writePNG(t *testing.T, path string, w, h int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x += 16 {
		img.Set(x, 0, color.RGBA{255, 0, 0, 255})
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

// TestScanCS2OneEventPerScript 一份 .cst = 一个场景事件，id 取文件名。
func TestScanCS2OneEventPerScript(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "scene")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	cst := buildCSTFor(t, []struct {
		typ  byte
		body string
	}{
		{0x30, "bg 0 bg01"},
		{0x21, "名前"},
		{0x20, "一行目"},
	})
	for _, n := range []string{"01_YU01.cst", "02_CH28.cst"} {
		if err := os.WriteFile(filepath.Join(dir, n), cst, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// 坏文件不该进事件库。
	if err := os.WriteFile(filepath.Join(dir, "broken.cst"), []byte("not a cat scene"), 0o644); err != nil {
		t.Fatal(err)
	}
	conv := adapter.Convention{EngineID: "catsystem2", ScenarioDir: "scene", ScriptExt: ".cst"}
	lib, err := ScanWith(root, conv)
	if err != nil {
		t.Fatalf("ScanWith(cs2): %v", err)
	}
	if len(lib.Scenes) != 2 {
		t.Fatalf("事件数 = %d, want 2（坏文件应被跳过）：%+v", len(lib.Scenes), lib.Scenes)
	}
	if lib.Scenes[0].ID != "01_YU01" {
		t.Errorf("事件 id = %q, want 01_YU01", lib.Scenes[0].ID)
	}
	if lib.Scenes[0].Script != "scene/01_YU01.cst" {
		t.Errorf("脚本路径 = %q", lib.Scenes[0].Script)
	}
	if lib.FindScene("02_CH28") == nil {
		t.Error("FindScene 应能找到 02_CH28")
	}
}

// TestCS2BuildShowScript .cst → 可播放事件（Show 非空）。
func TestCS2BuildShowScript(t *testing.T) {
	cst := buildCSTFor(t, []struct {
		typ  byte
		body string
	}{
		{0x30, "bg 0 bg01"},
		{0x20, "一行目"},
	})
	ev, err := catsystem2.BuildShowScript(cst, "01_YU01.cst")
	if err != nil {
		t.Fatalf("BuildShowScript: %v", err)
	}
	if len(ev.Show) == 0 {
		t.Fatal("Show 台本不应为空（播放器只吃 Show）")
	}
	if ev.Show[0].Kind == "" {
		t.Error("Show 指令应有 Kind")
	}
}

// TestScanCatSystem2 一个 .cst = 一个场景事件；标题取首句对白；舞台从 bg 图推断。
func TestScanCatSystem2(t *testing.T) {
	root := t.TempDir()
	scene := filepath.Join(root, "原始归档解包", "scene")
	if err := os.MkdirAll(scene, 0o755); err != nil {
		t.Fatal(err)
	}
	cst := makeCST(t,
		[2]interface{}{0x30, "bg 0 bg01"},
		[2]interface{}{0x20, "こんにちは"},
	)
	if err := os.WriteFile(filepath.Join(scene, "ama01.cst"), cst, 0o644); err != nil {
		t.Fatal(err)
	}
	// 两张 1280×720 的背景图 + 一张乱七八糟尺寸的立绘部件。
	img := filepath.Join(root, "图片", "bg01")
	chr := filepath.Join(root, "图片", "chr")
	for _, d := range []string{img, chr} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writePNG(t, filepath.Join(img, "bg01_0000.png"), 1280, 720)
	writePNG(t, filepath.Join(img, "bg02_0000.png"), 1280, 720)
	writePNG(t, filepath.Join(img, "bg03_0000.png"), 1280, 720)
	writePNG(t, filepath.Join(chr, "Tama01f_0001_0000.png"), 700, 800)

	conv := adapter.Convention{EngineID: "catsystem2"}
	lib, err := ScanWith(root, conv)
	if err != nil {
		t.Fatalf("ScanWith: %v", err)
	}
	if len(lib.Scenes) != 1 {
		t.Fatalf("场景事件数 = %d, want 1：%+v", len(lib.Scenes), lib.Scenes)
	}
	if lib.Scenes[0].ID != "ama01" {
		t.Errorf("事件 id = %q", lib.Scenes[0].ID)
	}
	if lib.Scenes[0].Title != "こんにちは" {
		t.Errorf("标题 = %q, want 首句对白", lib.Scenes[0].Title)
	}
	if lib.StageW != 1280 || lib.StageH != 720 {
		t.Errorf("舞台 = %d×%d, want 1280×720（按背景图推断）", lib.StageW, lib.StageH)
	}
}

// TestConventionByEngineAutoScenario 新接入的引擎都应有默认约定，
// 且允许剧本目录留空（由扫描器自动探测）。
func TestConventionByEngineAutoScenario(t *testing.T) {
	for _, id := range []string{"artemis", "bgi", "catsystem2"} {
		c, ok := adapter.ConventionByEngine(id)
		if !ok {
			t.Fatalf("%s 应有默认约定", id)
		}
		if c.EngineID != id {
			t.Errorf("%s 的 EngineID = %q", id, c.EngineID)
		}
		if err := c.Validate(); err != nil {
			t.Errorf("%s 的默认约定应允许剧本目录留空：%v", id, err)
		}
	}
}
