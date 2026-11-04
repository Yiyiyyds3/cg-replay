package psb

import (
	"encoding/binary"
	"os"
	"testing"
)

// psbArray 以最小宽度编码一个 PsbArray（仅用于测试构造）。
func psbArray(vals []uint32) []byte {
	out := []byte{0x0D, byte(len(vals)), 0x0D}
	for _, v := range vals {
		out = append(out, byte(v))
	}
	return out
}

// buildTiny 造一个最小的 v3 PSB：根对象 {"a": "hello"}。
func buildTiny() []byte {
	return buildTinyValue([]byte{0x15, 0x00})
}

// buildTinyValue 同 buildTiny，但根对象 "a" 的值换成给定字节。
func buildTinyValue(value []byte) []byte {
	// 名字表：charset=[0]；nodes[0x61]=0；indexes=[0x61] → 名字 "a"。
	charset := psbArray([]uint32{0})
	nodes := psbArray(make([]uint32, 0x62))
	indexes := psbArray([]uint32{0x61})
	names := append(append(append([]byte{}, charset...), nodes...), indexes...)

	// 根对象：names=[0], offsets=[0], 值 = 参数给的字节
	entries := []byte{0x21}
	entries = append(entries, psbArray([]uint32{0})...)
	entries = append(entries, psbArray([]uint32{0})...)
	entries = append(entries, value...)

	strings := psbArray([]uint32{0})
	stringsData := []byte("hello\x00")

	const headerLen = 44
	offNames := uint32(headerLen)
	offEntries := offNames + uint32(len(names))
	offStrings := offEntries + uint32(len(entries))
	offStringsData := offStrings + uint32(len(strings))
	offChunkOffsets := offStringsData + uint32(len(stringsData))
	emptyArr := psbArray(nil)
	offChunkLengths := offChunkOffsets + uint32(len(emptyArr))
	offChunkData := offChunkLengths + uint32(len(emptyArr))

	out := make([]byte, 0, offChunkData)
	hdr := make([]byte, 44)
	copy(hdr, "PSB\x00")
	binary.LittleEndian.PutUint16(hdr[4:], 3)
	fields := []uint32{headerLen, offNames, offStrings, offStringsData,
		offChunkOffsets, offChunkLengths, offChunkData, offEntries}
	for i, v := range fields {
		binary.LittleEndian.PutUint32(hdr[8+i*4:], v)
	}
	out = append(out, hdr...)
	out = append(out, names...)
	out = append(out, entries...)
	out = append(out, strings...)
	out = append(out, stringsData...)
	out = append(out, emptyArr...)
	out = append(out, emptyArr...)
	return out
}

func TestParseTiny(t *testing.T) {
	f, err := Parse(buildTiny())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if f.Version != 3 {
		t.Fatalf("version = %d", f.Version)
	}
	if len(f.Names) != 1 || f.Names[0] != "a" {
		t.Fatalf("names = %#v", f.Names)
	}
	if len(f.Strings) != 1 || f.Strings[0] != "hello" {
		t.Fatalf("strings = %#v", f.Strings)
	}
	root, ok := f.Root.(Object)
	if !ok {
		t.Fatalf("root 类型 = %T", f.Root)
	}
	if got := StringOf(root["a"]); got != "hello" {
		t.Fatalf("root[a] = %q", got)
	}
}

// TestParseTolerant 验证局部坏节点只丢自己，不连累兄弟：解包产物常有
// 「一处字符串索引越界」，整份剧本报废太浪费。
func TestParseTolerant(t *testing.T) {
	// 列表两个元素：["hello", <字符串索引 9 越界>]
	list := []byte{0x20}
	list = append(list, psbArray([]uint32{0, 2})...)
	list = append(list, 0x15, 0x00, 0x15, 0x09)

	f, err := Parse(buildTinyValue(list))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if f.Dropped != 1 {
		t.Fatalf("Dropped = %d，期望 1", f.Dropped)
	}
	arr := ArrayOf(ObjectOf(f.Root)["a"])
	if len(arr) != 2 {
		t.Fatalf("元素数 = %d，期望 2", len(arr))
	}
	if StringOf(arr[0]) != "hello" {
		t.Fatalf("arr[0] = %#v", arr[0])
	}
	if arr[1] != nil {
		t.Fatalf("坏元素应为 nil，实际 %#v", arr[1])
	}
}

// TestParseReal 用真实 9nine .scn 验证（设置 UCGR_9NINE_SCN 才跑）。
func TestParseReal(t *testing.T) {
	path := os.Getenv("UCGR_9NINE_SCN")
	if path == "" {
		t.Skip("未设置 UCGR_9NINE_SCN")
	}
	f, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile(%s): %v", path, err)
	}
	scenes := ArrayOf(Get(f.Root, "scenes"))
	if len(scenes) == 0 {
		t.Fatalf("没有 scenes；root keys = %v", objectKeys(f.Root))
	}
	first := ObjectOf(scenes[0])
	if first == nil {
		t.Fatalf("scene[0] 不是对象：%T", scenes[0])
	}
	if len(ArrayOf(first["lines"])) == 0 {
		t.Fatalf("scene[0] 没有 lines")
	}
	t.Logf("scenes=%d lines=%d texts=%d", len(scenes),
		len(ArrayOf(first["lines"])), len(ArrayOf(first["texts"])))
}

func objectKeys(v any) []string {
	o := ObjectOf(v)
	out := make([]string, 0, len(o))
	for k := range o {
		out = append(out, k)
	}
	return out
}
