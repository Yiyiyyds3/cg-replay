package kif

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 参考值来自独立的 Python 原型（GARbro 规范的等价实现），用作跨实现一致性锚点。
func TestBlowfishStandardVector(t *testing.T) {
	bf := newBlowfish(make([]byte, 8))
	xl, xr := uint32(0), uint32(0)
	bf.encipherBlock(&xl, &xr)
	if xl != 0x4ef99745 || xr != 0x6198dd78 {
		t.Fatalf("Blowfish 标准向量（key=0/明文=0）不符: %08x %08x", xl, xr)
	}
	bf.decipherBlock(&xl, &xr)
	if xl != 0 || xr != 0 {
		t.Fatalf("解密未还原明文: %08x %08x", xl, xr)
	}
}

func TestMTMatchesPrototype(t *testing.T) {
	cases := []struct {
		seed uint32
		want []uint32
	}{
		{1, []uint32{3556162021, 4012392791, 221657543, 586320575}},
		{57851327, []uint32{3040291990, 2600497728, 470781196, 3434544681}},
	}
	for _, c := range cases {
		var m mt19937
		m.seed(c.seed)
		for i, w := range c.want {
			if got := m.next(); got != w {
				t.Fatalf("MT(seed=%d) 第 %d 个输出 = %d，期望 %d", c.seed, i+1, got, w)
			}
		}
	}
}

func TestBlowfishKeyFromSeed(t *testing.T) {
	var m mt19937
	m.seed(57851327)
	bf := newBlowfish(le4(m.next()))
	xl, xr := uint32(0), uint32(0)
	bf.encipherBlock(&xl, &xr)
	if xl != 0xeb2f24b8 || xr != 0x6a689c2a {
		t.Fatalf("seed 派生的 Blowfish 密钥不符: %08x %08x", xl, xr)
	}
}

func TestPassphraseToKey(t *testing.T) {
	if got := PassphraseToKey("windmill"); got != 0xf7c368cc {
		t.Fatalf("EncodePassPhrase(windmill) = %08x，期望 f7c368cc", got)
	}
}

func TestDecipherNameIsInvertible(t *testing.T) {
	names := []string{"SE001.OGG", "AA_01_common_01_001.lpb", "bgm03.ogg", "A.cst", "x"}
	for _, k0 := range []byte{0, 7, 26, 51, 130, 255} {
		for _, name := range names {
			buf := encodeNameK(name, k0)
			if got := decipherNameK(buf, k0); got != name {
				t.Fatalf("名字 %q k0=%d 往返失败: 得到 %q", name, k0, got)
			}
		}
	}
}

// 名字的字母可靠、大小写不可判定：给定魔数提示时应判为 exact 且字母一致。
func TestResolveNameLetters(t *testing.T) {
	buf := encodeNameK("SE001.OGG", 5)
	name, kind := resolveName(buf, []string{".ogg"})
	if !strings.EqualFold(name, "SE001.OGG") {
		t.Fatalf("字母应完全一致，得到 %q", name)
	}
	if kind != NameExactKind {
		t.Fatalf("给了唯一魔数提示时应为 exact，得到 %q", kind)
	}
}

// 造一个带 2 个文件的 KIF 归档（含加密索引与加密数据），验证整条读取链路。
func TestArchiveRoundTrip(t *testing.T) {
	seed := uint32(0x12345678)
	files := []struct {
		name string
		data []byte
	}{
		{"SE001.OGG", append([]byte("OggS"), bytes.Repeat([]byte{0x11}, 12)...)},
		{"bgm03.ogg", append([]byte("OggS"), bytes.Repeat([]byte{0x22}, 8)...)},
	}
	path := writeTestArchive(t, seed, files)

	a, err := Open(path)
	if err != nil {
		t.Fatalf("Open 失败: %v", err)
	}
	defer a.Close()

	if a.Count != 3 || len(a.Entries) != 2 || a.Skipped != 0 {
		t.Fatalf("条目数不符: count=%d entries=%d skipped=%d", a.Count, len(a.Entries), a.Skipped)
	}
	off := int64(indexStart + 3*entryStride)
	for i, want := range files {
		e := a.Entries[i]
		if !strings.EqualFold(e.Name, want.name) {
			t.Errorf("第 %d 条名字 = %q，期望（忽略大小写）%q", i, e.Name, want.name)
		}
		if e.NameKind != NameExactKind {
			t.Errorf("第 %d 条 NameKind = %q，期望 exact", i, e.NameKind)
		}
		if e.Offset != off || e.Size != int64(len(want.data)) {
			t.Errorf("第 %d 条 offset/size = %d/%d，期望 %d/%d",
				i, e.Offset, e.Size, off, len(want.data))
		}
		data, err := a.Read(e)
		if err != nil {
			t.Fatalf("第 %d 条 Read 失败: %v", i, err)
		}
		if !bytes.Equal(data, want.data) {
			t.Errorf("第 %d 条内容不符: %x", i, data)
		}
		off += int64(len(want.data))
	}
}

// 已知 main_key 时应走精确路径（名字与原始归档逐字节一致）。
func TestSetMainKeyExactNames(t *testing.T) {
	seed := uint32(0x0badf00d)
	want := "MixedCase_Name.TLG"
	data := append([]byte("TLG5.0"), bytes.Repeat([]byte{0x33}, 16)...)
	path := writeTestArchive(t, seed, []struct {
		name string
		data []byte
	}{{want, data}})

	a, err := Open(path)
	if err != nil {
		t.Fatalf("Open 失败: %v", err)
	}
	defer a.Close()

	key := PassphraseToKey("windmill")
	a.SetMainKey(key)
	var m mt19937
	m.seed(key)
	key1 := m.next()
	if got := decipherNameKey(encodeNameK(want, byte((key1>>24)+(key1>>16)+(key1>>8)+key1)), key1); got != want {
		t.Fatalf("精确路径解密名字不符: %q", got)
	}
	if len(a.Entries) != 1 {
		t.Fatalf("条目数 = %d", len(a.Entries))
	}
}

func TestOpenRejects(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.int")
	if err := os.WriteFile(bad, []byte("NOTKIF!!\x00\x00\x00\x00"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(bad); err == nil || !strings.Contains(err.Error(), "魔数不符") {
		t.Fatalf("应报魔数不符，得到 %v", err)
	}

	plain := filepath.Join(dir, "plain.int")
	head := make([]byte, indexStart+2*entryStride)
	copy(head, magic)
	binary.LittleEndian.PutUint32(head[4:], 2)
	copy(head[indexStart:], "plainname.txt\x00")
	if err := os.WriteFile(plain, head, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(plain); err == nil || !strings.Contains(err.Error(), "明文") {
		t.Fatalf("应报明文布局未支持，得到 %v", err)
	}
}

// ---- 测试辅助 ----

// encodeNameK 是 decipherNameK 的逆：把名字按 k0 混淆成归档里的名字字节。
func encodeNameK(name string, k0 byte) []byte {
	buf := make([]byte, nameField)
	ki := int(k0)
	for i := 0; i < len(name); i++ {
		c := name[i]
		if j := indexByte(nameAlphabet, c); j >= 0 {
			j = (ki + 0x33 - j) % 0x34
			if j < 0 {
				j += 0x34
			}
			c = nameAlphabet[j]
		}
		buf[i] = c
		ki++
	}
	return buf
}

// encipherBytes 是 decipherBytes 的逆（同样小端组字）。
func encipherBytes(b *blowfish, data []byte) {
	n := len(data) / 8 * 8
	for i := 0; i < n; i += 8 {
		xl := binary.LittleEndian.Uint32(data[i:])
		xr := binary.LittleEndian.Uint32(data[i+4:])
		b.encipherBlock(&xl, &xr)
		binary.LittleEndian.PutUint32(data[i:], xl)
		binary.LittleEndian.PutUint32(data[i+4:], xr)
	}
}

func writeTestArchive(t *testing.T, seed uint32, files []struct {
	name string
	data []byte
}) string {
	t.Helper()
	count := len(files) + 1
	var m mt19937
	m.seed(seed)
	bf := newBlowfish(le4(m.next()))

	indexLen := indexStart + count*entryStride
	head := make([]byte, indexLen)
	copy(head, magic)
	binary.LittleEndian.PutUint32(head[4:], uint32(count))
	copy(head[indexStart:], keyDatMarker)
	binary.LittleEndian.PutUint32(head[seedOffset:], seed)

	off := int64(indexLen)
	body := make([]byte, 0, 64)
	for i, f := range files {
		slot := indexStart + (i+1)*entryStride
		copy(head[slot:], encodeNameK(f.name, byte(7+i)))
		encL, encR := uint32(off), uint32(len(f.data))
		bf.encipherBlock(&encL, &encR)
		binary.LittleEndian.PutUint32(head[slot+nameField:], encL-uint32(i+1))
		binary.LittleEndian.PutUint32(head[slot+nameField+4:], encR)
		chunk := append([]byte(nil), f.data...)
		encipherBytes(bf, chunk)
		body = append(body, chunk...)
		off += int64(len(f.data))
	}

	path := filepath.Join(t.TempDir(), "test.int")
	if err := os.WriteFile(path, append(head, body...), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
