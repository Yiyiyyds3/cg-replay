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
)

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
