package kif

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// buildPlain 拼一个明文布局的 KIF：槽 0 起就是真实条目（无元数据槽）、名字与数据都不加密。
func buildPlain(nameLen int, names []string, datas [][]byte) []byte {
	count := len(names)
	var buf bytes.Buffer
	var hdr [8]byte
	copy(hdr[:4], magic)
	binary.LittleEndian.PutUint32(hdr[4:], uint32(count))
	buf.Write(hdr[:])

	off := int64(8 + count*(nameLen+8))
	for i, n := range names {
		slot := make([]byte, nameLen+8)
		copy(slot, n)
		binary.LittleEndian.PutUint32(slot[nameLen:], uint32(off))
		binary.LittleEndian.PutUint32(slot[nameLen+4:], uint32(len(datas[i])))
		buf.Write(slot)
		off += int64(len(datas[i]))
	}
	for _, d := range datas {
		buf.Write(d)
	}
	return buf.Bytes()
}

// 明文布局真实样本：甜蜜女友系列的 UPDATE/scene 等（@8 直接就是第一个文件名）。
func TestOpenPlainLayout(t *testing.T) {
	names := []string{"ama_002b.cst", "ed_yuh.mpg"}
	datas := [][]byte{[]byte("CatS1234"), []byte("000001ba")}
	dir := t.TempDir()
	for _, nameLen := range plainNameSizes {
		p := filepath.Join(dir, fmt.Sprintf("plain_%x.int", nameLen))
		if err := os.WriteFile(p, buildPlain(nameLen, names, datas), 0o644); err != nil {
			t.Fatal(err)
		}
		arc, err := Open(p)
		if err != nil {
			t.Fatalf("名字字段 0x%x：Open 失败: %v", nameLen, err)
		}
		if len(arc.Entries) != len(names) {
			t.Fatalf("名字字段 0x%x：条目数 %d != %d", nameLen, len(arc.Entries), len(names))
		}
		for i, e := range arc.Entries {
			if e.Name != names[i] || e.NameKind != NameExactKind {
				t.Errorf("名字字段 0x%x：第 %d 条 = %q/%s，期望 %q/exact",
					nameLen, i, e.Name, e.NameKind, names[i])
			}
			if e.Size != int64(len(datas[i])) {
				t.Errorf("名字字段 0x%x：第 %d 条 size %d != %d", nameLen, i, e.Size, len(datas[i]))
			}
			got, err := arc.Read(e)
			if err != nil {
				t.Fatalf("读取失败: %v", err)
			}
			if !bytes.Equal(got, datas[i]) {
				t.Errorf("名字字段 0x%x：第 %d 条数据不是明文原样: %q", nameLen, i, got)
			}
		}
		arc.Close()
	}
}

func TestPlainLayoutRejectsBadOffset(t *testing.T) {
	raw := buildPlain(0x20, []string{"a.cst"}, [][]byte{[]byte("CatS")})
	binary.LittleEndian.PutUint32(raw[8+0x20:], 0) // offset 非法（指向索引自身）
	dir := t.TempDir()
	p := filepath.Join(dir, "bad.int")
	if err := os.WriteFile(p, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	arc, err := Open(p)
	if err == nil {
		arc.Close()
		t.Fatal("非法 offset 应当报错")
	}
}
