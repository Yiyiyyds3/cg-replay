package library

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"ucgr/internal/adapter"
	"ucgr/internal/adapter/catsystem2"
)

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
