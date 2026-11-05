// ain.go —— AliceSoft System 4 的 .ain 脚本解析。
//
// 文件形态（兰斯10 实测）：头 16 字节 = "AI2\0" + u32(0) + u32(packedSize)
// + u32(rawSize)，其后是整文件 zlib 流；解压产物是「区段流」——
// 一串 `<4 字节 ASCII tag><内容>`，内容长度由 tag 语义决定（下一 tag
// 出现即上一段结束）。
//
// 区段（AIN v12，对齐 nunuhara/libsys4 ain.c 的逆向成果）：
//
//	VERS u32 版本 | KEYC u32 | CODE u32 size + data | FUNC u32 n + n×函数
//	GLOB u32 n + n×变量 | GSET u32 n + n×初值 | STRT u32 n + n×结构体
//	MSG0 u32 n + n×zstr | MSG1 u32 n + u32 + n×加密串（b[i] -= i+0x60）
//	MAIN/MSGF/OJMP u32 | HLL0 u32 n + n×库 | SWI0 u32 n + n×switch
//	GVER u32 | STR0 u32 n + n×zstr | SLBL u32 n + n×(zstr+u32)
//
// 兰斯10 = v12：有 MSG1（加密消息）无 MSG0、无 FNAM/OJMP；汉化版文本是 GBK。
package alice

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"
)

// AIN 解析产物（只留演出提取需要的区段）。
type AIN struct {
	Version  int
	Code     []byte    // CODE 区段（字节码）
	Funcs    []AFunc   // 函数表（地址 + 名字）
	Messages []string  // MSG0/MSG1 文本
	Strings  []string  // STR0 字符串
	Libs     []ALib    // HLL0 库
}

// AFunc 函数表项（v12：address + name；参数/变量已跳过）。
type AFunc struct {
	Address int32
	Name    string
}

// ALib HLL 库（CALLHLL 的目标）。
type ALib struct {
	Name     string
	FuncName []string // 扁平索引 = 前面库的函数数累加
}

// LooksLikeAIN 报告文件是不是 AI2 包裹的 AIN。
func LooksLikeAIN(head []byte) bool {
	return len(head) >= 16 && bytes.Equal(head[:4], []byte("AI2\x00"))
}

// LooksLikeAINFile 报告路径指向 AI2 形态的 .ain。
func LooksLikeAINFile(path string) bool {
	data, err := readHead(path, 16)
	if err != nil {
		return false
	}
	return LooksLikeAIN(data)
}

// ParseAIN 解析 .ain（AI2 zlib 包裹或裸区段流）。
func ParseAIN(data []byte) (*AIN, error) {
	raw, err := unpackAI2(data)
	if err != nil {
		return nil, err
	}
	a := &AIN{}
	r := &reader{buf: raw}
	for r.i+4 <= len(raw) {
		tag := string(raw[r.i : r.i+4])
		r.i += 4
		switch tag {
		case "VERS":
			a.Version = int(r.i32())
		case "KEYC", "MAIN", "MSGF", "OJMP", "GVER":
			r.i32()
		case "CODE":
			n := int(r.u32())
			if n < 0 || r.i+n > len(raw) {
				return nil, fmt.Errorf("alice: CODE 区段越界（%d 字节）", n)
			}
			a.Code = raw[r.i : r.i+n]
			r.i += n
		case "FUNC":
			a.parseFuncs(r)
		case "GLOB":
			a.skipGlobals(r)
		case "GSET":
			a.skipInitvals(r)
		case "STRT":
			a.skipStructs(r)
		case "MSG0":
			n := int(r.u32())
			a.Messages = make([]string, n)
			for i := 0; i < n; i++ {
				a.Messages[i] = r.zstr()
			}
		case "MSG1":
			n := int(r.u32())
			r.i32() // uk
			a.Messages = make([]string, n)
			for i := 0; i < n; i++ {
				a.Messages[i] = r.msg1()
			}
		case "HLL0":
			a.parseLibs(r)
		case "SWI0":
			a.skipSwitches(r)
		case "STR0":
			n := int(r.u32())
			a.Strings = make([]string, n)
			for i := 0; i < n; i++ {
				a.Strings[i] = r.zstr()
			}
		case "SLBL":
			n := int(r.u32())
			for i := 0; i < n; i++ {
				r.zstr()
				r.i32()
			}
		case "FNAM":
			n := int(r.u32())
			for i := 0; i < n; i++ {
				r.zstr()
			}
		default:
			// 未知 tag（v13+ 的新区段等）：无法推断长度，就地停止。
			// 已解析的区段照常可用。
			return a, nil
		}
	}
	return a, nil
}

// unpackAI2 解 AI2 zlib 包裹；裸区段流（VERS 开头）原样返回。
func unpackAI2(data []byte) ([]byte, error) {
	if LooksLikeAIN(data) {
		zr, err := zlib.NewReader(bytes.NewReader(data[16:]))
		if err != nil {
			return nil, fmt.Errorf("alice: AI2 zlib 流打开失败：%w", err)
		}
		raw, err := io.ReadAll(zr)
		if err != nil {
			return nil, fmt.Errorf("alice: AI2 zlib 解压失败：%w", err)
		}
		return raw, nil
	}
	if len(data) >= 4 && string(data[:4]) == "VERS" {
		return data, nil
	}
	return nil, fmt.Errorf("alice: 不是 AIN 文件（头 % x）", head3(data))
}

func head3(b []byte) []byte {
	if len(b) > 8 {
		return b[:8]
	}
	return b
}

// ---- 区段内容解析（对齐 libsys4 ain.c v12 路径） ----

// AIN 数据类型（libsys4 ain.h enum ain_data_type）。
const (
	ainString  = 12
	ainStruct  = 13
	ainDelegat = 63
	ainArray   = 79
)

// ainRefTypes 引用/容器类类型（initval 跳过判定用）。
var ainRefTypes = map[int32]bool{
	18: true, 19: true, 20: true, 21: true, 22: true, 23: true, 24: true,
	25: true, 26: true, 27: true, 30: true, 31: true, 32: true, 51: true,
	52: true, 55: true, 58: true, 59: true, 60: true, 66: true, 67: true,
	69: true, 71: true, 74: true, 75: true, 80: true, 82: true,
}

func (a *AIN) parseFuncs(r *reader) {
	n := int(r.u32())
	a.Funcs = make([]AFunc, 0, n)
	for i := 0; i < n; i++ {
		addr := r.i32()
		name := r.zstr()
		skipType(r)
		r.i32() // nr_args
		nvars := int(r.i32())
		r.i32() // is_lambda
		r.i32() // crc
		for v := 0; v < nvars; v++ {
			skipVar(r)
		}
		a.Funcs = append(a.Funcs, AFunc{Address: addr, Name: name})
	}
}

func (a *AIN) skipGlobals(r *reader) {
	n := int(r.u32())
	for i := 0; i < n; i++ {
		r.zstr() // name
		r.zstr() // name2 (v12+)
		skipType(r)
		r.i32() // group_index (v5+)
	}
}

func (a *AIN) skipInitvals(r *reader) {
	n := int(r.u32())
	for i := 0; i < n; i++ {
		r.i32() // global_index
		dt := r.i32()
		if dt == ainString {
			r.zstr()
		} else {
			r.i32()
		}
	}
}

func (a *AIN) skipStructs(r *reader) {
	n := int(r.u32())
	for i := 0; i < n; i++ {
		r.zstr() // name
		nif := int(r.i32())
		for j := 0; j < nif; j++ {
			r.i32() // struct_type
			r.i32() // vtable_offset
		}
		r.i32() // ctor
		r.i32() // dtor
		nm := int(r.i32())
		for v := 0; v < nm; v++ {
			skipVar(r)
		}
		// v14.1+ 的 vtable 函数表：兰斯10 是 v12，没有；遇到也不识别
	}
}

func (a *AIN) parseLibs(r *reader) {
	n := int(r.u32())
	a.Libs = make([]ALib, 0, n)
	for i := 0; i < n; i++ {
		name := r.zstr()
		nf := int(r.u32())
		lib := ALib{Name: name, FuncName: make([]string, nf)}
		for j := 0; j < nf; j++ {
			lib.FuncName[j] = r.zstr()
			r.i32() // return type data
			na := int(r.u32())
			for k := 0; k < na; k++ {
				r.zstr() // arg name
				r.i32()  // arg type data
			}
		}
		a.Libs = append(a.Libs, lib)
	}
}

func (a *AIN) skipSwitches(r *reader) {
	n := int(r.u32())
	for i := 0; i < n; i++ {
		r.i32() // case_type
		r.i32() // default_address
		nc := int(r.u32())
		for c := 0; c < nc; c++ {
			r.i32() // value
			r.i32() // address
		}
	}
}

// skipType 跳过变量类型（v11+：data/struc/rank，rank 为 bool 可嵌套）。
func skipType(r *reader) {
	r.i32() // data
	r.i32() // struc
	rank := r.i32()
	for rank != 0 {
		r.i32() // sub data
		r.i32() // sub struc
		rank = r.i32()
	}
}

// skipVar 跳过变量（v12：name + name2 + type + initval）。
func skipVar(r *reader) {
	r.zstr()
	r.zstr()
	dt := skipTypeRet(r)
	if r.i32() != 0 { // has_initval
		switch {
		case dt == ainString:
			r.zstr()
		case dt == ainStruct || dt == ainDelegat || dt == ainArray || ainRefTypes[dt]:
			// 无负载
		default:
			r.i32()
		}
	}
}

func skipTypeRet(r *reader) int32 {
	data := r.i32()
	r.i32() // struc
	rank := r.i32()
	for rank != 0 {
		data = r.i32()
		r.i32() // struc
		rank = r.i32()
	}
	return data
}

// reader 小端顺序读取器。
type reader struct {
	buf []byte
	i   int
}

func (r *reader) u32() uint32 {
	v := binary.LittleEndian.Uint32(r.buf[r.i:])
	r.i += 4
	return v
}

func (r *reader) i32() int32 {
	v := int32(binary.LittleEndian.Uint32(r.buf[r.i:]))
	r.i += 4
	return v
}

// zstr 读 \0 结尾串（GBK → UTF-8）。
func (r *reader) zstr() string {
	end := bytes.IndexByte(r.buf[r.i:], 0)
	if end < 0 {
		s := decodeGBK(r.buf[r.i:])
		r.i = len(r.buf)
		return s
	}
	s := decodeGBK(r.buf[r.i : r.i+end])
	r.i += end + 1
	return s
}

// msg1 读 MSG1 加密串（u32 len + len 字节，b[i] -= i+0x60）。
func (r *reader) msg1() string {
	n := int(r.u32())
	if n < 0 || r.i+n > len(r.buf) {
		r.i = len(r.buf)
		return ""
	}
	b := make([]byte, n)
	copy(b, r.buf[r.i:r.i+n])
	r.i += n
	for i := range b {
		b[i] -= byte(i)
		b[i] -= 0x60
	}
	return decodeGBK(b)
}
