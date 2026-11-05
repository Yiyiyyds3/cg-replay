// rpa.go —— Ren'Py 归档（RPA-3.0）索引解析与按名读取。
//
// RPA-3.0 布局：
//
//	RPA-3.0 <16位hex的索引偏移> <8位hex的XOR密钥>\n
//	（其后是 renpy 引导脚本，可跳过）
//	<索引偏移> 处是一段 pickle：dict[加密文件名] -> [(offset, size, prefix), ...]
//
// 文件名解密：hex 解码后逐字节 XOR 密钥的小端字节序（key>>((i%4)*8)）。
// 数据读法：prefix + file[offset+len(prefix) : offset+size]（size 含 prefix 长度）。
//
// pickle 只实现 Ren'Py 实际会产出的操作码子集（协议 2），遇到不认识的
// 操作码就报错——索引坏了宁可明说，不要猜出一份错误的资源表。
package renpy

import (
	"bytes"
	"compress/zlib"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// rpaEntry 归档内一个文件的定位信息。
type rpaEntry struct {
	Offset int64
	Size   int64
	Prefix []byte
}

// RPA 一个 RPA-3.0 归档的已解析索引。
type RPA struct {
	Path  string
	key   uint32
	Files map[string]rpaEntry // 解密后的归档内路径（正斜杠）
}

// OpenRPA 打开并解析一个 RPA-3.0 归档的索引（只读索引，不读数据体）。
func OpenRPA(path string) (*RPA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	head := make([]byte, 128)
	n, _ := io.ReadFull(f, head)
	head = head[:n]
	lineEnd := strings.IndexByte(string(head), '\n')
	if lineEnd < 0 {
		return nil, fmt.Errorf("%s: 没有 RPA 头行", path)
	}
	header := strings.TrimSpace(string(head[:lineEnd]))
	if !strings.HasPrefix(header, "RPA-3.0") {
		return nil, fmt.Errorf("%s: 不支持的归档格式（只认 RPA-3.0，头为 %q）", path, header)
	}
	fields := strings.Fields(header)
	if len(fields) < 2 {
		return nil, fmt.Errorf("%s: RPA 头不完整：%q", path, header)
	}
	offset, err := strconv.ParseUint(fields[1], 16, 64)
	if err != nil {
		return nil, fmt.Errorf("%s: 索引偏移非法：%w", path, err)
	}
	a := &RPA{Path: path, Files: map[string]rpaEntry{}}
	if len(fields) >= 3 {
		k, err := strconv.ParseUint(fields[2], 16, 32)
		if err != nil {
			return nil, fmt.Errorf("%s: XOR 密钥非法：%w", path, err)
		}
		a.key = uint32(k)
	}

	if _, err := f.Seek(int64(offset), io.SeekStart); err != nil {
		return nil, fmt.Errorf("%s: 跳到索引区失败：%w", path, err)
	}
	idxData, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("%s: 读索引失败：%w", path, err)
	}
	// 部分打包器（renpy 自带 rpatool 风格）会把索引 zlib 压缩：0x78 0x9c 开头。
	if len(idxData) >= 2 && idxData[0] == 0x78 {
		zr, zerr := zlib.NewReader(bytes.NewReader(idxData))
		if zerr != nil {
			return nil, fmt.Errorf("%s: 索引 zlib 解压失败：%w", path, zerr)
		}
		raw, rerr := io.ReadAll(zr)
		zr.Close()
		if rerr != nil {
			return nil, fmt.Errorf("%s: 索引 zlib 解压读取出错：%w", path, rerr)
		}
		idxData = raw
	}
	idx, err := parsePickle(idxData)
	if err != nil {
		return nil, fmt.Errorf("%s: 解析索引 pickle 失败：%w", path, err)
	}
	dict, ok := idx.(pickleDict)
	if !ok {
		return nil, fmt.Errorf("%s: 索引顶层不是字典（%T）", path, idx)
	}
	for encName, v := range dict {
		pts, ok := v.([]any)
		if !ok {
			continue
		}
		for _, pt := range pts {
			tuple, ok := pt.([]any)
			if !ok || len(tuple) < 2 {
				continue
			}
			off, ok1 := tuple[0].(int64)
			size, ok2 := tuple[1].(int64)
			if !ok1 || !ok2 {
				continue
			}
			e := rpaEntry{Offset: off, Size: size}
			if len(tuple) >= 3 {
				if p, ok := tuple[2].(string); ok {
					e.Prefix = []byte(p)
				}
			}
			// 两种变体（同解析器兼容）：
			//   标准 RPA-3.0：名字 hex 编码 + XOR 密钥，数值原样；
			//   明文变体（实测 Supower-Re/xxxFiles）：名字原样，
			//   offset/size XOR 密钥（密钥 0x42424242 时数值 0x42424271 → 51）。
			if isHexName(encName) {
				name, err := decodeRPAName(encName, a.key)
				if err != nil {
					return nil, fmt.Errorf("%s: 文件名解密失败：%w", path, err)
				}
				a.Files[strings.ReplaceAll(name, "\\", "/")] = e
			} else {
				name := encName
				e.Offset ^= int64(a.key)
				e.Size ^= int64(a.key)
				a.Files[strings.ReplaceAll(name, "\\", "/")] = e
			}
		}
	}
	if len(a.Files) == 0 {
		return nil, fmt.Errorf("%s: 索引里没有文件条目", path)
	}
	return a, nil
}

// Read 读取一个条目的完整数据。
func (a *RPA) Read(e rpaEntry) ([]byte, error) {
	f, err := os.Open(a.Path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if _, err := f.Seek(e.Offset, io.SeekStart); err != nil {
		return nil, err
	}
	raw := make([]byte, e.Size)
	if _, err := io.ReadFull(f, raw); err != nil {
		return nil, err
	}
	if len(e.Prefix) > len(raw) {
		return nil, fmt.Errorf("%s: 条目 prefix(%d) 大于长度(%d)", a.Path, len(e.Prefix), e.Size)
	}
	if len(e.Prefix) == 0 {
		return raw, nil
	}
	out := make([]byte, 0, e.Size)
	out = append(out, e.Prefix...)
	out = append(out, raw[len(e.Prefix):]...)
	return out, nil
}

// ReadFile 按归档内路径读文件。
func (a *RPA) ReadFile(name string) ([]byte, error) {
	e, ok := a.Files[name]
	if !ok {
		return nil, fmt.Errorf("归档 %s 里没有 %s", a.Path, name)
	}
	return a.Read(e)
}

// decodeRPAName 解密文件名：hex 解码 + 小端字节序 XOR。
func decodeRPAName(enc string, key uint32) (string, error) {
	raw, err := hex.DecodeString(strings.TrimSpace(enc))
	if err != nil {
		return "", err
	}
	out := make([]byte, len(raw))
	for i, b := range raw {
		out[i] = b ^ byte(key>>((i%4)*8))
	}
	return string(out), nil
}

// isHexName 判断索引键是不是「hex 编码的名字」：全 0-9a-f 且偶数长。
// 明文名（含 . 扩展名或路径分隔符以外的可读字符）不会命中。
func isHexName(s string) bool {
	if len(s) == 0 || len(s)%2 != 0 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// ---- 最小 pickle 解析器 ----

// pickleDict pickle 字典（键是字符串；RPA 索引只用字符串键）。
type pickleDict map[string]any

type pickleParser struct {
	stack []any
	marks []int
	memo  map[int]any
	// memoNext MEMOIZE（0x94）的自增索引：dict=0、key=1、list=2…后续
	// BINGET 按这个序号取回（实测 Supower-Re 归档的 h\x03 正是取回
	// 第 4 次 MEMOIZE 的空 bytes 前缀）。
	memoNext int
}

func (p *pickleParser) memoize() {
	p.memoizeAt(p.memoNext)
}

func (p *pickleParser) memoizeAt(n int) {
	if p.memo == nil {
		p.memo = map[int]any{}
	}
	if len(p.stack) > 0 {
		p.memo[n] = p.stack[len(p.stack)-1]
	}
	if n == p.memoNext {
		p.memoNext++
	}
}

func (p *pickleParser) push(v any) { p.stack = append(p.stack, v) }
func (p *pickleParser) pop() any {
	v := p.stack[len(p.stack)-1]
	p.stack = p.stack[:len(p.stack)-1]
	return v
}
func (p *pickleParser) popMark() []any {
	m := p.marks[len(p.marks)-1]
	p.marks = p.marks[:len(p.marks)-1]
	items := append([]any{}, p.stack[m:]...)
	p.stack = p.stack[:m]
	return items
}

// parsePickle 解析索引 pickle，返回 pickleDict / []any / int64 / string。
func parsePickle(data []byte) (any, error) {
	p := &pickleParser{}
	i := 0
	readN := func(n int) ([]byte, error) {
		if i+n > len(data) {
			return nil, fmt.Errorf("pickle 数据在第 %d 字节截断", i)
		}
		b := data[i : i+n]
		i += n
		return b, nil
	}
	for i < len(data) {
		op := data[i]
		i++
		fail := func(format string, args ...any) error {
			from := i - 1
			if from > 16 {
				from = i - 16
			}
			if from < 0 {
				from = 0
			}
			to := i + 8
			if to > len(data) {
				to = len(data)
			}
			return fmt.Errorf("pickle 第 %d 字节操作码 0x%02x：%s（附近 %x…）",
				i-1, op, fmt.Sprintf(format, args...), data[from:to])
		}
		switch op {
		case 0x80: // PROTO
			i++
		case 0x95: // FRAME
			i += 8
		case 0x28: // MARK
			p.marks = append(p.marks, len(p.stack))
		case 0x2e: // STOP
			if len(p.stack) == 0 {
				return nil, fail("空栈结束")
			}
			return p.stack[len(p.stack)-1], nil
		case 0x29, 0x5d: // EMPTY_TUPLE / EMPTY_LIST
			p.push([]any{})
		case 0x7d: // EMPTY_DICT
			p.push(pickleDict{})
		case 0x6c: // LIST（MARK 起的元素构成列表）
			p.push(p.popMark())
		case 0x64: // DICT（MARK 起的键值对构成字典）
			items := p.popMark()
			d := pickleDict{}
			for j := 0; j+1 < len(items); j += 2 {
				d[pickleKey(items[j])] = items[j+1]
			}
			p.push(d)
		case 0x58, 0x8c: // BINUNICODE / SHORT_BINUNICODE
			var n int
			switch op {
			case 0x58:
				b, err := readN(4)
				if err != nil {
					return nil, err
				}
				n = int(le32(b))
			default:
				n = int(data[i])
				i++
			}
			b, err := readN(n)
			if err != nil {
				return nil, err
			}
			p.push(string(b))
		case 0x43, 0x42: // SHORT_BINBYTES / BINBYTES
			var n int
			switch op {
			case 0x42:
				b, err := readN(4)
				if err != nil {
					return nil, err
				}
				n = int(le32(b))
			default:
				n = int(data[i])
				i++
			}
			b, err := readN(n)
			if err != nil {
				return nil, err
			}
			p.push(append([]byte{}, b...))
		case 0x4b: // BININT1
			p.push(int64(data[i]))
			i++
		case 0x4d: // BININT2
			p.push(int64(le16(data[i : i+2])))
			i += 2
		case 0x4a: // BININT4
			p.push(int64(int32(le32(data[i : i+4]))))
			i += 4
		case 0x8a: // LONG1
			n := int(data[i])
			i++
			b, err := readN(n)
			if err != nil {
				return nil, err
			}
			var v int64
			for k := 0; k < n && k < 8; k++ {
				v |= int64(b[k]) << (8 * k)
			}
			p.push(v)
		case 0x85, 0x86, 0x87, 0x74: // TUPLE1/2/3/TUPLE
			var n int
			switch op {
			case 0x85:
				n = 1
			case 0x86:
				n = 2
			case 0x87:
				n = 3
			default: // TUPLE：取 mark 起的元素
				t := p.popMark()
				p.push(t)
				continue
			}
			if len(p.stack) < n {
				return nil, fail("元组元素不足")
			}
			t := append([]any{}, p.stack[len(p.stack)-n:]...)
			p.stack = p.stack[:len(p.stack)-n]
			p.push(t)
		case 0x61: // APPEND
			item := p.pop()
			list, ok := p.pop().([]any)
			if !ok {
				return nil, fail("APPEND 目标不是列表")
			}
			p.push(append(list, item))
		case 0x65: // APPENDS
			items := p.popMark()
			list, ok := p.pop().([]any)
			if !ok {
				return nil, fail("APPENDS 目标不是列表")
			}
			p.push(append(list, items...))
		case 0x73: // SETITEM
			v := p.pop()
			k := p.pop()
			d, ok := p.pop().(pickleDict)
			if !ok {
				return nil, fail("SETITEM 目标不是字典")
			}
			d[pickleKey(k)] = v
			p.push(d)
		case 0x75: // SETITEMS（'u'）
			items := p.popMark()
			d, ok := p.pop().(pickleDict)
			if !ok {
				return nil, fail("SETITEMS 目标不是字典")
			}
			for j := 0; j+1 < len(items); j += 2 {
				d[pickleKey(items[j])] = items[j+1]
			}
			p.push(d)
		case 0x88: // NEWTRUE
			p.push(true)
		case 0x89: // NEWFALSE
			p.push(false)
		case 0x4e: // NONE
			p.push(nil)
		case 0x68, 0x94: // BINGET('h'，实测见上) / MEMOIZE
			if op == 0x68 {
				p.push(p.memo[int(data[i])])
				i++
			} else {
				p.memoize()
			}
		case 0x72: // LONG_BINPUT：栈顶存入指定 memo 槽（不弹栈）
			b, err := readN(4)
			if err != nil {
				return nil, err
			}
			p.memoizeAt(int(le32(b)))
		case 0x67, 0x6a: // BINGET / LONG_BINGET
			var n int
			if op == 0x67 {
				n = int(data[i])
				i++
			} else {
				b, err := readN(4)
				if err != nil {
					return nil, err
				}
				n = int(le32(b))
			}
			p.push(p.memo[n])
		default:
			return nil, fail("操作码暂不支持")
		}
	}
	return nil, fmt.Errorf("pickle 没有结束符（数据截断）")
}

// pickleKey 键统一转字符串（bytes 键也按原字节收）。
func pickleKey(v any) string {
	switch k := v.(type) {
	case string:
		return k
	case []byte:
		return string(k)
	default:
		return fmt.Sprintf("%v", k)
	}
}

func le16(b []byte) uint16 { return uint16(b[0]) | uint16(b[1])<<8 }
func le32(b []byte) uint32 {
	return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
}
func le64(b []byte) uint64 {
	return uint64(le32(b)) | uint64(le32(b[4:]))<<32
}
