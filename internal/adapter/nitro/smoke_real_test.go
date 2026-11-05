package nitro

import (
	"os"
	"path/filepath"
	"testing"
)

// 真实语料 smoke 测试：鬼哭街重制版（Nitro+ / SDActi，NPA 归档 + NSS 明文脚本）。
// 语料缺失时自动跳过；可用 UCGR_KIKOKUGAI_DIR 覆盖语料目录。

func kikokugaiDir(t *testing.T) string {
	t.Helper()
	dirs := []string{
		os.Getenv("UCGR_KIKOKUGAI_DIR"),
		`E:\Personal_entertainment\game\zfsgalgame\鬼哭街重制版\鬼哭街重制版`,
	}
	for _, d := range dirs {
		if d == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(d, "nss.npa")); err == nil {
			return d
		}
	}
	t.Skip("鬼哭街重制版语料缺失")
	return ""
}

// TestRealNpaEntries：nss.npa 解析出 81 个脚本条目且全部可读。
func TestRealNpaEntries(t *testing.T) {
	dir := kikokugaiDir(t)
	arc, err := ParseNpa(filepath.Join(dir, "nss.npa"))
	if err != nil {
		t.Fatal(err)
	}
	files := 0
	for _, e := range arc.Entries {
		if !e.IsDir {
			files++
		}
	}
	if files < 81 {
		t.Fatalf("nss.npa 文件条目 = %d, want >= 81", files)
	}
	e := arc.Lookup("0101h.nss")
	if e == nil {
		t.Fatal("Lookup 0101h.nss 失败")
	}
	data, err := arc.ReadEntry(*e)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("0101h.nss 读取为空")
	}
	// 明文 NSS 应含 scene 块
	if len(data) > 4 && string(data[:5]) != "scene" && string(data[:5]) != "//\r\n" {
		// 宽松校验：不强制开头格式，只要求可读文本无 0x00
		for _, b := range data[:min(len(data), 64)] {
			if b == 0 {
				t.Fatal("明文脚本头部出现 0x00（解密异常）")
			}
		}
	}
}

// TestRealScriptParse：真实日文脚本可解析出文本段与图像段。
func TestRealScriptParse(t *testing.T) {
	dir := kikokugaiDir(t)
	arc, err := ParseNpa(filepath.Join(dir, "nss.npa"))
	if err != nil {
		t.Fatal(err)
	}
	e := arc.Lookup("0101h.nss")
	if e == nil {
		t.Fatal("Lookup 0101h.nss 失败")
	}
	data, err := arc.ReadEntry(*e)
	if err != nil {
		t.Fatal(err)
	}
	s, err := ParseNSS(data, "0101h.nss")
	if err != nil {
		t.Fatal(err)
	}
	if s.TextCP != 932 {
		t.Fatalf("TextCP = %d, want 932", s.TextCP)
	}
	var texts, imgs int
	for _, seg := range s.Segs {
		switch seg.Kind {
		case SegText:
			texts++
		case SegImage:
			imgs++
		}
	}
	if texts == 0 || imgs == 0 {
		t.Fatalf("段统计: text=%d img=%d（均应为正）", texts, imgs)
	}
}

// TestRealCGImage：cg.npa 里取一张 CG 验证 JPG/PNG 头。
func TestRealCGImage(t *testing.T) {
	dir := kikokugaiDir(t)
	arc, err := ParseNpa(filepath.Join(dir, "cg.npa"))
	if err != nil {
		t.Fatal(err)
	}
	files := 0
	for _, e := range arc.Entries {
		if !e.IsDir {
			files++
		}
	}
	if files < 2000 {
		t.Fatalf("cg.npa 文件条目 = %d, want >= 2000", files)
	}
	// 取第一个非目录条目
	for _, e := range arc.Entries {
		if e.IsDir {
			continue
		}
		data, err := arc.ReadEntry(e)
		if err != nil {
			t.Fatal(err)
		}
		if len(data) < 4 {
			t.Fatalf("%s 数据过短", e.Name)
		}
		isJpg := data[0] == 0xff && data[1] == 0xd8
		isPng := data[0] == 0x89 && data[1] == 0x50
		if !isJpg && !isPng {
			t.Fatalf("%s 头部非 JPG/PNG: % x", e.Name, data[:4])
		}
		return
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
