// Package psb 解析 M2/KiriKiri Evolution 的 PSB 二进制容器。
//
// PSB 是 9nine 这类 KiriKiri Evolution 作品的剧本/图像/立绘统一容器：
// 文件头之后依次是名字表（Trie）、根对象、字符串表、资源块。本项目只用它
// 读剧情脚本（.scn）和 CG 列表所需的资源名，不做回写。
//
// 格式参考 FreeMote（MIT）的 PsbFile/PsbHeader/PsbValues 实现：
//   - 头部 8 字节固定，随后 8 个 uint32 偏移；v3 起追加 Adler32 校验和，
//     v4 起再追加三组 ExtraChunk 偏移。
//   - 值类型的第一字节同时编码“类型 + 宽度”：NumberNk / ArrayNk /
//     StringNk / ResourceNk / ExtraChunkNk 的 k = 类型字节 - 基值 + 1。
//   - 名字表是字符集 + Trie 节点 + 根索引三个数组；字符串表是偏移数组 +
//     零结尾 UTF-8 数据区。
//
// 解密（headerEncrypt != 0 / body 加密）不在本包范围内：9nine 的解包产物
// 是明文，遇到加密头会明确报错，绝不猜测。
package psb

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
)

// 值类型编码（与 FreeMote PsbObjType 一致）。
const (
	typeNone       = 0x00
	typeNull       = 0x01
	typeFalse      = 0x02
	typeTrue       = 0x03
	typeNumberN0   = 0x04
	typeNumberN1   = 0x05
	typeNumberN8   = 0x0C
	typeArrayN1    = 0x0D
	typeArrayN8    = 0x14
	typeStringN1   = 0x15
	typeStringN4   = 0x18
	typeResourceN1 = 0x19
	typeResourceN4 = 0x1C
	typeFloat0     = 0x1D
	typeFloat      = 0x1E
	typeDouble     = 0x1F
	typeList       = 0x20
	typeObjects    = 0x21
	typeExtraN1    = 0x22
	typeExtraN4    = 0x25
)

// File 一个已解析的 PSB。
type File struct {
	Version uint16
	Names   []string
	Strings []string
	// Resources 资源块（按索引）。剧本通常为空。
	Resources [][]byte
	Root      any
	// Dropped 记录值树里被跳过的坏节点数。
	//
	// 每个子节点都带独立偏移，解包产物局部损坏时坏一个不该让整份剧本报废；
	// 跳过但计数，让上层能如实报告「这份文件残缺了多少」。
	Dropped int
}

// Object 是 PSB 字典；键已去掉名字表里的结尾 NUL。
type Object map[string]any

// Resource 资源引用（值树里的 thunk）。
type Resource struct {
	Index int
}

// ParseFile 读取并解析一个 PSB 文件。
func ParseFile(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	f, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return f, nil
}

// Parse 解析 PSB 字节流（支持 mdf\0 外壳与 0x20 异或混淆）。
func Parse(data []byte) (*File, error) {
	raw, err := unwrapMDF(data)
	if err != nil {
		return nil, err
	}
	if x, ok := unwrapXOR(raw); ok {
		raw = x
	}
	r := &reader{data: raw}
	magic := r.bytes(4)
	if len(magic) < 4 || string(magic) != "PSB\x00" {
		return nil, errors.New("不是 PSB 文件（magic 不是 PSB\\0）")
	}
	f := &File{Version: r.u16()}
	headerEncrypt := r.u16()
	headerLength := r.u32()
	offsetNames := r.u32()
	offsetStrings := r.u32()
	offsetStringsData := r.u32()
	offsetChunkOffsets := r.u32()
	offsetChunkLengths := r.u32()
	offsetChunkData := r.u32()
	offsetEntries := r.u32()
	if err := r.err; err != nil {
		return nil, fmt.Errorf("读 PSB 头失败：%w", err)
	}
	if headerLength == 0 {
		headerLength = offsetNames
	}
	// 未加密的 PSB 头部长度固定（v2=40 / v3=44 / v4=56），且 Names 紧跟头部。
	// 不满足就说明头被加密过——9nine 解包产物不会出现，直接报错不猜。
	if headerEncrypt != 0 || (offsetNames != 0 && offsetNames != headerLength) {
		return nil, fmt.Errorf("PSB 头疑似加密（header_encrypt=%d, header_length=%d, names=%d），本解析器只支持明文头",
			headerEncrypt, headerLength, offsetNames)
	}
	if !validOffset(offsetNames, len(raw)) || !validOffset(offsetEntries, len(raw)) ||
		!validOffset(offsetStrings, len(raw)) || !validOffset(offsetStringsData, len(raw)) {
		return nil, fmt.Errorf("PSB 偏移越界（names=%d entries=%d strings=%d strings_data=%d 文件=%d）",
			offsetNames, offsetEntries, offsetStrings, offsetStringsData, len(raw))
	}

	names, err := parseNames(raw, int(offsetNames))
	if err != nil {
		return nil, fmt.Errorf("解析名字表失败：%w", err)
	}
	f.Names = names

	r.pos = int(offsetStrings)
	strOffsets, err := r.array()
	if err != nil {
		return nil, fmt.Errorf("解析字符串偏移表失败：%w", err)
	}
	f.Strings = make([]string, 0, len(strOffsets))
	for _, off := range strOffsets {
		start := int(offsetStringsData) + int(off)
		if start < 0 || start > len(raw) {
			return nil, fmt.Errorf("字符串偏移越界：%d", start)
		}
		end := bytes.IndexByte(raw[start:], 0)
		if end < 0 {
			end = len(raw) - start
		}
		f.Strings = append(f.Strings, string(raw[start:start+end]))
	}

	if offsetChunkOffsets > 0 && offsetChunkLengths > 0 && offsetChunkData > 0 &&
		validOffset(offsetChunkOffsets, len(raw)) && validOffset(offsetChunkLengths, len(raw)) &&
		validOffset(offsetChunkData, len(raw)) {
		f.Resources, err = parseResources(raw, int(offsetChunkOffsets), int(offsetChunkLengths), int(offsetChunkData))
		if err != nil {
			return nil, fmt.Errorf("解析资源块失败：%w", err)
		}
	}

	r.pos = int(offsetEntries)
	root, err := r.value(f)
	if err != nil {
		return nil, fmt.Errorf("解析 PSB 根对象失败：%w", err)
	}
	f.Root = root
	return f, nil
}

// unwrapMDF 处理 mdf\0 外壳：4 字节 magic + 4 字节原始长度 + zlib 流。
func unwrapMDF(data []byte) ([]byte, error) {
	if len(data) >= 8 && string(data[:4]) == "mdf\x00" {
		zr, err := zlib.NewReader(bytes.NewReader(data[8:]))
		if err != nil {
			return nil, fmt.Errorf("MDF 解压失败：%w", err)
		}
		defer zr.Close()
		out, err := io.ReadAll(zr)
		if err != nil {
			return nil, fmt.Errorf("MDF 解压失败：%w", err)
		}
		return out, nil
	}
	return data, nil
}

func validOffset(off uint32, n int) bool {
	return off > 0 && int(off) >= 0 && int(off) < n
}

// xorKey 9nine 部分发行版把整份 PSB 与 0x20 逐字节异或：头 4 字节从
// "PSB\0" 变成 "psb "（0x70 0x73 0x62 0x20）。这不是加密（无密钥），
// 只是轻量混淆，整份还原后再按普通 PSB 解析即可。
const xorKey = 0x20

// IsPSB 报告字节流是否是 PSB（含 0x20 混淆变体）。只读前 4 字节。
func IsPSB(data []byte) bool {
	if len(data) < 4 {
		return false
	}
	return string(data[:4]) == "PSB\x00" || string(data[:4]) == "psb "
}

// IsObfuscated 报告字节流是否是 0x20 混淆变体（头写作 "psb "）。
func IsObfuscated(data []byte) bool {
	return len(data) >= 4 && string(data[:4]) == "psb "
}

// unwrapXOR 还原 0x20 混淆；不是该变体时返回 (原数据, false)。
func unwrapXOR(data []byte) ([]byte, bool) {
	if !IsPSB(data) || string(data[:4]) == "PSB\x00" {
		return data, false
	}
	out := make([]byte, len(data))
	for i, b := range data {
		out[i] = b ^ xorKey
	}
	return out, true
}

// parseNames 解析名字表：字符集数组 + Trie 节点数组 + 根索引数组。
func parseNames(data []byte, off int) ([]string, error) {
	r := &reader{data: data, pos: off}
	charset, err := r.array()
	if err != nil {
		return nil, err
	}
	nodes, err := r.array()
	if err != nil {
		return nil, err
	}
	indexes, err := r.array()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(indexes))
	for _, idx := range indexes {
		var bs []byte
		chr := int(idx)
		for steps := 0; chr != 0; steps++ {
			if steps > len(nodes) || chr < 0 || chr >= len(nodes) {
				return nil, fmt.Errorf("Trie 节点越界：%d", chr)
			}
			code := int(nodes[chr])
			if code < 0 || code >= len(charset) {
				return nil, fmt.Errorf("Trie 字符集索引越界：%d", code)
			}
			bs = append(bs, byte(chr-int(charset[code])))
			chr = code
		}
		// 反转字节序
		for i, j := 0, len(bs)-1; i < j; i, j = i+1, j-1 {
			bs[i], bs[j] = bs[j], bs[i]
		}
		name := string(bs)
		// 部分实现会把终止节点的 0 也带出来；键名本身不应含 NUL。
		name = strings.TrimRight(name, "\x00")
		names = append(names, name)
	}
	return names, nil
}

func parseResources(data []byte, offOffsets, offLengths, offData int) ([][]byte, error) {
	ro := &reader{data: data, pos: offOffsets}
	offsets, err := ro.array()
	if err != nil {
		return nil, err
	}
	rl := &reader{data: data, pos: offLengths}
	lengths, err := rl.array()
	if err != nil {
		return nil, err
	}
	n := len(offsets)
	if len(lengths) < n {
		n = len(lengths)
	}
	out := make([][]byte, n)
	for i := 0; i < n; i++ {
		start := offData + int(offsets[i])
		end := start + int(lengths[i])
		if start < 0 || end < start || end > len(data) {
			return nil, fmt.Errorf("资源 %d 越界（%d..%d, 文件 %d）", i, start, end, len(data))
		}
		out[i] = data[start:end]
	}
	return out, nil
}

// reader 带越界保护的顺序读取器。
type reader struct {
	data []byte
	pos  int
	err  error
}

func (r *reader) bytes(n int) []byte {
	if r.err != nil {
		return nil
	}
	if n < 0 || r.pos+n > len(r.data) {
		r.err = fmt.Errorf("越界读取 %d 字节（位置 %d，文件 %d）", n, r.pos, len(r.data))
		return nil
	}
	b := r.data[r.pos : r.pos+n]
	r.pos += n
	return b
}

func (r *reader) u8() byte {
	b := r.bytes(1)
	if len(b) == 0 {
		return 0
	}
	return b[0]
}

func (r *reader) u16() uint16 {
	b := r.bytes(2)
	if len(b) < 2 {
		return 0
	}
	return binary.LittleEndian.Uint16(b)
}

func (r *reader) u32() uint32 {
	b := r.bytes(4)
	if len(b) < 4 {
		return 0
	}
	return binary.LittleEndian.Uint32(b)
}

// uintN 读 n 字节无符号数（小端，高位补 0）。
func (r *reader) uintN(n int) uint32 {
	if n <= 0 || n > 8 {
		r.err = fmt.Errorf("非法宽度 %d", n)
		return 0
	}
	b := r.bytes(n)
	var v uint64
	for i := n - 1; i >= 0; i-- {
		v = v<<8 | uint64(b[i])
	}
	return uint32(v)
}

// sintN 读 n 字节有符号数（小端，按最高位符号扩展）。
func (r *reader) sintN(n int) int64 {
	if n <= 0 || n > 8 {
		r.err = fmt.Errorf("非法宽度 %d", n)
		return 0
	}
	b := r.bytes(n)
	var v uint64
	for i := n - 1; i >= 0; i-- {
		v = v<<8 | uint64(b[i])
	}
	if n < 8 && b[n-1]&0x80 != 0 {
		v |= ^uint64(0) << (uint(n) * 8)
	}
	return int64(v)
}

// array 读一个 PsbArray：类型字节（宽度）→ count → 元素宽度 → 元素。
func (r *reader) array() ([]uint32, error) {
	t := int(r.u8())
	if t < typeArrayN1 || t > typeArrayN8 {
		return nil, fmt.Errorf("期望数组类型 0x0D..0x14，实际 0x%02X（位置 %d）", t, r.pos-1)
	}
	count := int(r.uintN(t - typeArrayN1 + 1))
	if r.err != nil {
		return nil, r.err
	}
	entryLen := int(r.u8()) - typeNumberN8
	if entryLen < 1 || entryLen > 8 {
		return nil, fmt.Errorf("数组元素宽度非法：%d", entryLen)
	}
	if count < 0 || count > len(r.data) {
		return nil, fmt.Errorf("数组长度非法：%d", count)
	}
	out := make([]uint32, count)
	// 批量读取，避免 count 次函数调用；仍然是 O(元素字节数)。
	buf := r.bytes(count * entryLen)
	if r.err != nil {
		return nil, r.err
	}
	for i := 0; i < count; i++ {
		var v uint64
		base := i * entryLen
		for j := entryLen - 1; j >= 0; j-- {
			v = v<<8 | uint64(buf[base+j])
		}
		out[i] = uint32(v)
	}
	return out, nil
}

// value 读一个值树节点。
//
// 注意：Object/List 的偏移基准是「两个数组读完后的位置」，与 FreeMote 一致。
func (r *reader) value(f *File) (any, error) {
	start := r.pos
	t := r.u8()
	if r.err != nil {
		return nil, r.err
	}
	switch {
	case t == typeNone || t == typeNull:
		return nil, nil
	case t == typeFalse:
		return false, nil
	case t == typeTrue:
		return true, nil
	case t >= typeNumberN0 && t <= typeNumberN8:
		n := int(t - typeNumberN0)
		if n == 0 {
			return int64(0), nil
		}
		return r.sintN(n), nil
	case t >= typeArrayN1 && t <= typeArrayN8:
		r.pos = start
		vals, err := r.array()
		if err != nil {
			return nil, err
		}
		out := make([]any, len(vals))
		for i, v := range vals {
			out[i] = int64(v)
		}
		return out, nil
	case t >= typeStringN1 && t <= typeStringN4:
		idx := int(r.uintN(int(t-typeStringN1) + 1))
		if r.err != nil {
			return nil, r.err
		}
		if idx < 0 || idx >= len(f.Strings) {
			return nil, fmt.Errorf("字符串索引越界：%d（共 %d，位置 %d）", idx, len(f.Strings), start)
		}
		return f.Strings[idx], nil
	case t >= typeResourceN1 && t <= typeResourceN4:
		idx := int(r.uintN(int(t-typeResourceN1) + 1))
		if r.err != nil {
			return nil, r.err
		}
		return &Resource{Index: idx}, nil
	case t >= typeExtraN1 && t <= typeExtraN4:
		idx := int(r.uintN(int(t-typeExtraN1) + 1))
		if r.err != nil {
			return nil, r.err
		}
		return &Resource{Index: idx}, nil
	case t == typeFloat0:
		return float64(0), nil
	case t == typeFloat:
		b := r.bytes(4)
		if r.err != nil {
			return nil, r.err
		}
		return float64(math.Float32frombits(binary.LittleEndian.Uint32(b))), nil
	case t == typeDouble:
		b := r.bytes(8)
		if r.err != nil {
			return nil, r.err
		}
		return math.Float64frombits(binary.LittleEndian.Uint64(b)), nil
	case t == typeList:
		offsets, err := r.array()
		if err != nil {
			return nil, err
		}
		base := r.pos
		out := make([]any, 0, len(offsets))
		bad := 0
		for _, off := range offsets {
			r.pos = base + int(off)
			v, err := r.value(f)
			if err != nil {
				// 元素各自带独立偏移，坏一个不会让兄弟节点错位：
				// 记一次丢弃、继续读下一个，比整份文件报废划算。
				bad++
				f.Dropped++
				out = append(out, nil)
				continue
			}
			out = append(out, v)
		}
		r.pos = base
		if bad > 0 && bad == len(offsets) {
			return nil, fmt.Errorf("列表 %d 个元素全部不可读", len(offsets))
		}
		return out, nil
	case t == typeObjects:
		nameIdx, err := r.array()
		if err != nil {
			return nil, err
		}
		offsets, err := r.array()
		if err != nil {
			return nil, err
		}
		base := r.pos
		out := Object{}
		bad := 0
		for i, off := range offsets {
			if i >= len(nameIdx) {
				break
			}
			ni := int(nameIdx[i])
			if ni < 0 || ni >= len(f.Names) {
				return nil, fmt.Errorf("名字索引越界：%d（共 %d）", ni, len(f.Names))
			}
			r.pos = base + int(off)
			v, err := r.value(f)
			if err != nil {
				bad++
				f.Dropped++
				continue
			}
			out[f.Names[ni]] = v
		}
		r.pos = base
		if bad > 0 && len(out) == 0 {
			return nil, fmt.Errorf("对象 %d 个键全部不可读", len(offsets))
		}
		return out, nil
	default:
		return nil, fmt.Errorf("未知值类型 0x%02X（位置 %d）", t, start)
	}
}

// Get 按路径取嵌套值（键名不需要带 NUL）：Get(root, "scenes", 0, "texts")。
// 数组下标用 int。
func Get(root any, path ...any) any {
	cur := root
	for _, p := range path {
		switch key := p.(type) {
		case string:
			obj, ok := cur.(Object)
			if !ok {
				return nil
			}
			cur = obj[key]
		case int:
			arr, ok := cur.([]any)
			if !ok || key < 0 || key >= len(arr) {
				return nil
			}
			cur = arr[key]
		default:
			return nil
		}
	}
	return cur
}

// ObjectOf 把值断言成 Object（不存在返回 nil）。
func ObjectOf(v any) Object { o, _ := v.(Object); return o }

// ArrayOf 把值断言成 []any（不存在返回 nil）。
func ArrayOf(v any) []any { a, _ := v.([]any); return a }

// StringOf 把值断言成 string（不存在返回 ""）。
func StringOf(v any) string { s, _ := v.(string); return s }

// IntOf 把值断言成整数（float 会截断）。
func IntOf(v any) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case float64:
		return int64(x)
	case bool:
		if x {
			return 1
		}
	}
	return 0
}

// FloatOf 把值断言成浮点。
func FloatOf(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int64:
		return float64(x)
	}
	return 0
}
