// dasm.go —— System 4 字节码线性反汇编（演出提取用）。
//
// 不做控制流分析：从函数地址线性扫描到 ENDFUNC，收集三类演出事件：
//
//	S_PUSH n  → 引用 STR0[n]（立绘/CG/部件名）
//	MSG n     → 显示 MSG[n]（台词）
//	CALLHLL…  → HLL 库函数调用（名字可得，v1 只记录不解释）
package alice

import (
	"encoding/binary"
	"fmt"
)

// Instr 一条反汇编出的演出相关指令。
type Instr struct {
	Op     uint16
	Args   []int32
	Str    string // S_PUSH/MSG 解出的文本
	CallNm string // CALLHLL/CALLFUNC 解出的名字
}

// DisasmFunc 从 addr 线性反汇编到 ENDFUNC（或越界/坏指令）。
func (a *AIN) DisasmFunc(addr int32) []Instr {
	var out []Instr
	pos := int(addr)
	for pos+2 <= len(a.Code) {
		op := binary.LittleEndian.Uint16(a.Code[pos:])
		n, ok := opcodeNArgs[op]
		if !ok {
			break // 未知指令（v13+ 或表缺）——停在线性扫描
		}
		if pos+2+n*4 > len(a.Code) {
			break
		}
		args := make([]int32, n)
		for i := 0; i < n; i++ {
			args[i] = int32(binary.LittleEndian.Uint32(a.Code[pos+2+i*4:]))
		}
		pos += 2 + n*4
		switch op {
		case opENDFUNC:
			return out
		case opSPUSH:
			s := a.strAt(args[0])
			out = append(out, Instr{Op: op, Args: args, Str: s})
		case opMSG:
			s := a.msgAt(args[0])
			out = append(out, Instr{Op: op, Args: args, Str: s})
		case opCALLHLL:
			out = append(out, Instr{Op: op, Args: args, CallNm: a.hllName(args)})
		case opCALLFUNC, opCALLMETHOD, opCALLSYS, opPUSH, opFPUSH:
			// CALLFUNC 的参数是函数索引（演出提取不需要调用图，v1 记名字便于调试）
			in := Instr{Op: op, Args: args}
			if op == opCALLFUNC || op == opCALLMETHOD {
				fi := int(args[0])
				if fi >= 0 && fi < len(a.Funcs) {
					in.CallNm = a.Funcs[fi].Name
				}
			}
			out = append(out, in)
		default:
			// 其余指令不产出演出信息
		}
	}
	return out
}

func (a *AIN) strAt(i int32) string {
	if i >= 0 && int(i) < len(a.Strings) {
		return a.Strings[i]
	}
	return ""
}

func (a *AIN) msgAt(i int32) string {
	if i >= 0 && int(i) < len(a.Messages) {
		return a.Messages[i]
	}
	return ""
}

// hllName CALLHLL 的 3 参 = (lib, func, ?)；lib/func 是 HLL0 的二级索引。
func (a *AIN) hllName(args []int32) string {
	if len(args) < 2 {
		return ""
	}
	lib, fn := int(args[0]), int(args[1])
	if lib < 0 || lib >= len(a.Libs) {
		return fmt.Sprintf("hll#%d", lib)
	}
	l := &a.Libs[lib]
	if fn < 0 || fn >= len(l.FuncName) {
		return l.Name + "#?"
	}
	return l.Name + "::" + l.FuncName[fn]
}

// FindFunc 按名字找函数索引（-1 = 没有）。
func (a *AIN) FindFunc(name string) int {
	for i := range a.Funcs {
		if a.Funcs[i].Name == name {
			return i
		}
	}
	return -1
}
