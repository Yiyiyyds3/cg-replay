// dat.go —— Wolf RPG Editor .dat/.mps 二进制格式（对齐 anopem/WolfTL 的解析，
// MIT License，github.com/Sinflower/WolfTL WolfRPG/*.hpp）。
//
// 容器（非 Pro、未加密形态，塔纳托斯实测）：
//
//	u8 indicator(0x00) + magic 9 字节 + u8 version + 数据体
//	magic 因文件类型而异，第 5 字节（utf8Idx）为 0x55 表示字符串是
//	UTF-8（带尾 \0），否则 Shift-JIS（无 \0）。塔纳托斯是 UTF-8 变体。
//	At(1)==0x50 才是加密容器（wolfcrypt 解密）；本作未加密。
//
// Command（事件指令，自描述 TLV）：
//
//	u8 intArgCount+1 → i32 opcode → i32×N 整数参数 → u8 indent
//	→ u8 strCount → 字符串×N（u32 len + bytes）→ u8 terminator
//	terminator 0x00 结束；0x01 = Move 扩展：5B unknown + 1B flags
//	+ i32 routeCount + N×（u8 id + u8 argc + i32×argc + "01 00"）
//
// 关键 opcode：101=Message 102=Choices 140=Sound 150=Picture
// 210=CommonEvent 300=CommonEventByName 111/112=条件 499=分支结束。
package wolf

import (
	"encoding/binary"
	"fmt"
)

// readErr Wolf 数据读取错误（sticky：首个错误保留，后续读取返回零值）。
type readErr struct{ msg string }

func (e *readErr) Error() string { return "wolf: " + e.msg }

// coder 小端顺序读取器。
type coder struct {
	buf []byte
	i   int
	err error
}

func (r *coder) fail(format string, args ...any) {
	if r.err == nil {
		r.err = fmt.Errorf(format, args...)
	}
}

func (r *coder) ok() bool { return r.err == nil }

func (r *coder) readByte() uint8 {
	if !r.ok() {
		return 0
	}
	if r.i >= len(r.buf) {
		r.fail("数据越界（偏移 %d / %d）", r.i, len(r.buf))
		return 0
	}
	v := r.buf[r.i]
	r.i++
	return v
}

func (r *coder) readInt() int32 {
	if !r.ok() {
		return 0
	}
	if r.i+4 > len(r.buf) {
		r.fail("数据越界（偏移 %d / %d）", r.i, len(r.buf))
		return 0
	}
	v := int32(binary.LittleEndian.Uint32(r.buf[r.i:]))
	r.i += 4
	return v
}

func (r *coder) readUint() uint32 {
	if !r.ok() {
		return 0
	}
	if r.i+4 > len(r.buf) {
		r.fail("数据越界（偏移 %d / %d）", r.i, len(r.buf))
		return 0
	}
	v := binary.LittleEndian.Uint32(r.buf[r.i:])
	r.i += 4
	return v
}

// readString 读 u32 长度前缀字符串（UTF-8 模式：尾部 1 字节 \0 剥离）。
func (r *coder) readString() string {
	if !r.ok() {
		return ""
	}
	n := int(r.readUint())
	if !r.ok() {
		return ""
	}
	if n <= 0 || r.i+n > len(r.buf) {
		r.fail("字符串长度 %d 越界（偏移 %d / %d）", n, r.i, len(r.buf))
		return ""
	}
	raw := r.buf[r.i : r.i+n]
	r.i += n
	if len(raw) > 0 && raw[len(raw)-1] == 0x00 {
		raw = raw[:len(raw)-1]
	}
	return string(raw)
}

func (r *coder) readBytes(n int) []byte {
	if !r.ok() {
		return nil
	}
	if n < 0 || r.i+n > len(r.buf) {
		r.fail("读取 %d 字节越界（偏移 %d / %d）", n, r.i, len(r.buf))
		return nil
	}
	v := r.buf[r.i : r.i+n]
	r.i += n
	return v
}

// at 绝对偏移读单字节（越界返回 0）。
func (r *coder) at(off int) uint8 {
	if off < 0 || off >= len(r.buf) {
		return 0
	}
	return r.buf[off]
}

// command 一条 Wolf 事件指令。
type command struct {
	Op    int32
	Args  []int32
	Strs  []string
	Indent uint8
}

// wolf 命令码（WolfTL README 的 WolfRPG Commands 表）。
const (
	opMessage       = 101 // 消息窗口文本
	opChoices       = 102 // 选项
	opSound         = 140 // SE/BGM
	opPicture       = 150 // 图片显示/移动/移除
	opCommonEvent   = 210 // 按编号调用公共事件
	opCommonByName  = 300 // 按名字调用公共事件
)

// parseCommand 解析一条指令（含 Move 扩展的跳过）。
func parseCommand(r *coder) command {
	argc := int(r.readByte()) - 1
	op := r.readInt()
	args := make([]int32, 0, max(0, argc))
	for i := 0; i < argc; i++ {
		args = append(args, r.readInt())
	}
	indent := r.readByte()
	strCnt := int(r.readByte())
	strs := make([]string, 0, strCnt)
	for i := 0; i < strCnt; i++ {
		strs = append(strs, r.readString())
	}
	term := r.readByte()
	switch term {
	case 0x00: // 常规结束
	case 0x01: // Move 扩展：5B unknown + 1B flags + 路线点数组
		r.readBytes(5)
		r.readByte()
		rc := int(r.readInt())
		for i := 0; i < rc; i++ {
			r.readByte() // route cmd id
			ac := int(r.readByte())
			for j := 0; j < ac; j++ {
				r.readInt()
			}
			if !r.ok() {
				return command{Op: op, Args: args, Strs: strs, Indent: indent}
			}
			if m := r.readBytes(2); len(m) != 2 || m[0] != 0x01 || m[1] != 0x00 {
				r.fail("路线终止符非法（偏移 %d）", r.i)
				return command{Op: op, Args: args, Strs: strs, Indent: indent}
			}
		}
	default:
		r.fail("指令终止符 %#x 非法（偏移 %d，opcode %d）", term, r.i, op)
	}
	return command{Op: op, Args: args, Strs: strs, Indent: indent}
}

// parseCommands 解析 u32 count 前缀的指令数组。
func parseCommands(r *coder) []command {
	if !r.ok() {
		return nil
	}
	cnt := int(r.readUint())
	if !r.ok() {
		return nil
	}
	cmds := make([]command, 0, min(cnt, len(r.buf)/4+1))
	for i := 0; i < cnt && r.ok(); i++ {
		cmds = append(cmds, parseCommand(r))
	}
	return cmds
}
