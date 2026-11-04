// smoke_real_test.go —— 真实语料冒烟（语料缺失自动 skip）。
//
// Baldr Sky：Script.pac 是 GBK 官方中文版（部分日文残留），
// 索引 Huffman 尾部差 3 字节是官方打包器特性，走残缺兜底。
package nexas

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const baldrSkyRoot = `E:\Personal_entertainment\game\zfsgalgame\Baldr Sky`

func baldrSkyAvailable(t *testing.T) bool {
	t.Helper()
	if _, err := os.Stat(filepath.Join(baldrSkyRoot, "Script.pac")); err != nil {
		t.Skip("本机没有 Baldr Sky 语料")
		return false
	}
	return true
}

// TestRealBaldrSkyPacIndex 索引解码：条目几何与流边界吻合（208/209）。
func TestRealBaldrSkyPacIndex(t *testing.T) {
	if !baldrSkyAvailable(t) {
		return
	}
	a, err := ParsePac(filepath.Join(baldrSkyRoot, "Script.pac"))
	if err != nil {
		t.Fatalf("ParsePac: %v", err)
	}
	if a.Count != 209 {
		t.Fatalf("条目数 = %d, 期望 209", a.Count)
	}
	if len(a.Entries) < 208 {
		t.Fatalf("解出条目 %d, 期望 >= 208", len(a.Entries))
	}
	// 第一个条目：00：ADV体験版.bin，offset=12, encoded=64855, decoded=359046
	e := a.Entries[0]
	if e.Offset != 12 || e.Size != 64855 || e.Unpacked != 359046 {
		t.Fatalf("条目0 几何错误: %+v", e)
	}
	if !strings.Contains(e.Name, "ADV") || !strings.HasSuffix(e.Name, ".bin") {
		t.Fatalf("条目0 名字异常: %q", e.Name)
	}
}

// TestRealBaldrSkyScript 内容：解压第一个脚本并解析段落。
func TestRealBaldrSkyScript(t *testing.T) {
	if !baldrSkyAvailable(t) {
		return
	}
	a, err := ParsePac(filepath.Join(baldrSkyRoot, "Script.pac"))
	if err != nil {
		t.Fatalf("ParsePac: %v", err)
	}
	// 条目 0（体験版序章）是日文残留，拿它验证 CP932 兜底与解析。
	e := a.Entries[0]
	data, err := a.ReadEntry(e)
	if err != nil {
		t.Fatalf("ReadEntry: %v", err)
	}
	s, err := ParseBin(data, e.Name)
	if err != nil {
		t.Fatalf("ParseBin: %v", err)
	}
	texts, voices := 0, 0
	for _, seg := range s.Segs {
		switch seg.Kind {
		case SegText:
			texts++
			if seg.Voice != "" {
				voices++
			}
		}
	}
	if texts < 100 {
		t.Fatalf("台词条数过少: %d（总段 %d）", texts, len(s.Segs))
	}
	if voices < 50 {
		t.Fatalf("语音绑定过少: %d", voices)
	}
	t.Logf("%s: %d 段, %d 台词, %d 语音, cp=%d", s.Name, len(s.Segs), texts, voices, s.TextCP)
}

// TestRealBaldrSkyResource 图像条目是标准 PNG/BMP。
func TestRealBaldrSkyResource(t *testing.T) {
	if !baldrSkyAvailable(t) {
		return
	}
	a, err := ParsePac(filepath.Join(baldrSkyRoot, "Bmp.pac"))
	if err != nil {
		t.Fatalf("ParsePac(Bmp): %v", err)
	}
	if len(a.Entries) < 2000 {
		t.Fatalf("Bmp.pac 条目过少: %d", len(a.Entries))
	}
	// 抽一个 png 条目验证头。
	var e *Entry
	for i := range a.Entries {
		if strings.HasSuffix(a.Entries[i].Name, ".png") && a.Entries[i].Size > 10000 {
			e = &a.Entries[i]
			break
		}
	}
	if e == nil {
		t.Fatal("没有找到 png 条目")
	}
	data, err := a.ReadEntry(*e)
	if err != nil {
		t.Fatalf("ReadEntry(%s): %v", e.Name, err)
	}
	if len(data) < 8 || string(data[1:4]) != "PNG" {
		t.Fatalf("%s 不是 PNG（头 %x）", e.Name, data[:8])
	}
}
