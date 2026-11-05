// opcode.go —— System 4 字节码的指令宽度表。
//
// 指令 = u16 opcode + nargs 个 i32 参数。opcode → nargs 的映射来自
// nunuhara/libsys4 instructions.c（SLC 社区逆向成果），v11+ 修正：
// CALLHLL=3 参、NEW=2 参、S_MOD=1、OBJSWAP=1、DG_STR_TO_METHOD=1。
package alice

// opcode 常量（演出提取关心的指令）。
const (
	opPUSH       = 0x00
	opCALLFUNC   = 0x30
	opFPUSH      = 0x40
	opSPUSH      = 0x41 // 压字符串（参数 = STR0 索引）
	opMSG        = 0x59 // 显示消息（参数 = MSG 索引）
	opCALLHLL    = 0x5a // 调 HLL 库函数（3 参：lib, func, ?）
	opCALLMETHOD = 0x5c
	opCALLSYS    = 0x63
	opENDFUNC    = 0x7e
	opRETURN     = 0x2f
)

// opcodeNArgs v12 全量指令宽度表（0x00-0x101）。
var opcodeNArgs = func() map[uint16]int {
	m := map[uint16]int{}
	// 0x00-0xb0 基表（源自 libsys4 instructions.c 的 OP/JMP/_OP 宏展开）
	fixed := map[uint16]int{
		0x00: 1, 0x01: 0, 0x02: 0, 0x03: 0, 0x04: 0, 0x05: 0, 0x06: 0, 0x07: 0, 0x08: 0,
		0x09: 0, 0x0a: 0, 0x0b: 0, 0x0c: 0, 0x0d: 0, 0x0e: 0, 0x0f: 0, 0x10: 0, 0x11: 0, 0x12: 0,
		0x13: 0, 0x14: 0, 0x15: 0, 0x16: 0, 0x17: 0, 0x18: 0, 0x19: 0, 0x1a: 0, 0x1b: 0, 0x1c: 0,
		0x1d: 0, 0x1e: 0, 0x1f: 0, 0x20: 0, 0x21: 0, 0x22: 0, 0x23: 0, 0x24: 0, 0x25: 0, 0x26: 0,
		0x27: 0, 0x28: 0, 0x29: 0, 0x2a: 0, 0x2b: 0,
		0x2c: 1, 0x2d: 1, 0x2e: 1, 0x2f: 0, 0x30: 1,
		0x31: 0, 0x32: 0, 0x33: 0, 0x34: 0, 0x35: 0, 0x36: 0, 0x37: 0, 0x38: 0, 0x39: 0, 0x3a: 0,
		0x3b: 0, 0x3c: 0, 0x3d: 0, 0x3e: 0, 0x3f: 0,
		0x40: 1, 0x41: 1, 0x42: 0, 0x43: 0, 0x44: 0, 0x45: 0, 0x46: 0, 0x47: 0, 0x48: 0, 0x49: 0,
		0x4a: 0, 0x4b: 0, 0x4c: 0, 0x4d: 0, 0x4e: 0,
		0x4f: 1, 0x50: 0, 0x51: 0, 0x52: 0, 0x53: 0, 0x54: 0, 0x55: 0, 0x56: 0, 0x57: 0, 0x58: 0,
		0x59: 1, // MSG
		0x5a: 3, // CALLHLL (v11+)
		0x5b: 0, 0x5c: 1, 0x5d: 1, 0x5e: 1, 0x5f: 1, 0x60: 1, 0x61: 1, 0x62: 1,
		0x63: 1, 0x64: 0, 0x65: 0, 0x66: 0, 0x67: 1, 0x68: 0, 0x69: 0, 0x6a: 0,
		0x6b: 0, 0x6c: 0, 0x6d: 0, 0x6e: 0, 0x6f: 0, 0x70: 0, 0x71: 0, 0x72: 0,
		0x73: 0, 0x74: 0, 0x75: 0, 0x76: 0, 0x77: 0, 0x78: 0, 0x79: 0, 0x7a: 0,
		0x7b: 0, 0x7c: 0, 0x7d: 0,
		0x7e: 1, 0x7f: 0, 0x80: 0,
		0x81: 2, 0x82: 1, 0x83: 0, 0x84: 0, 0x85: 0, 0x86: 0, 0x87: 0, 0x88: 0, 0x89: 0,
		0x8a: 1, 0x8b: 1, 0x8c: 2, 0x8d: 0, 0x8e: 0, 0x8f: 0, 0x90: 0, 0x91: 0, 0x92: 0,
		0x93: 0, 0x94: 1, 0x95: 0, 0x96: 1, 0x97: 0, 0x98: 1, 0x99: 0, 0x9a: 0, 0x9b: 0,
		0x9c: 0, 0x9d: 0, 0x9e: 0, 0x9f: 0, 0xa0: 0, 0xa1: 0, 0xa2: 0, 0xa3: 0, 0xa4: 0,
		0xa5: 0, 0xa6: 0, 0xa7: 0, 0xa8: 0, 0xa9: 0, 0xaa: 0, 0xab: 0, 0xac: 0, 0xad: 0,
		0xae: 0, 0xaf: 0, 0xb0: 0,
	}
	for op, n := range fixed {
		m[uint16(op)] = n
	}
	// 0xb2 起 SH_* 系列（按 instructions.c 顺序）
	sh := []int{
		0, // SH_SR_ASSIGN
		2, // SH_MEM_ASSIGN_LOCAL
		1, // A_NUMOF_GLOB_1
		1, // A_NUMOF_STRUCT_1
		2, // SH_MEM_ASSIGN_IMM
		1, // SH_LOCALREFREF
		2, // SH_LOCALASSIGN_SUB_IMM
		3, // SH_IF_LOC_LT_IMM
		3, // SH_IF_LOC_GE_IMM
		2, // SH_LOCREF_ASSIGN_MEM
		1, // PAGE_REF
		2, // SH_GLOBAL_ASSIGN_LOCAL
		2, // SH_STRUCTREF_GT_IMM
		2, // SH_STRUCT_ASSIGN_LOCALREF_ITOB
		2, // SH_LOCAL_ASSIGN_STRUCTREF
		3, // SH_IF_STRUCTREF_NE_LOCALREF
		3, // SH_IF_STRUCTREF_GT_IMM
		2, // SH_STRUCTREF_CALLMETHOD_NO_PARAM
		2, // SH_STRUCTREF2
		2, // SH_REF_STRUCTREF2
		3, // SH_STRUCTREF3
		3, // SH_STRUCTREF2_CALLMETHOD_NO_PARAM
		2, // SH_IF_STRUCTREF_Z
		2, // SH_IF_STRUCT_A_NOT_EMPTY
		3, // SH_IF_LOC_GT_IMM
		3, // SH_IF_STRUCTREF_NE_IMM
		1, // THISCALLMETHOD_NOPARAM
		3, // SH_IF_LOC_NE_IMM
		3, // SH_IF_STRUCTREF_EQ_IMM
		2, // SH_GLOBAL_ASSIGN_IMM
		3, // SH_LOCALSTRUCT_ASSIGN_IMM
		2, // SH_STRUCT_A_PUSHBACK_LOCAL_STRUCT
		2, // SH_GLOBAL_A_PUSHBACK_LOCAL_STRUCT
		2, // SH_LOCAL_A_PUSHBACK_LOCAL_STRUCT
		2, // SH_IF_SREF_NE_STR0
		0, // SH_S_ASSIGN_REF
		0, // SH_A_FIND_SREF
		0, // SH_SREF_EMPTY
		2, // SH_STRUCTSREF_EQ_LOCALSREF
		2, // SH_LOCALSREF_EQ_STR0
		2, // SH_STRUCTSREF_NE_LOCALSREF
		2, // SH_LOCALSREF_NE_STR0
		2, // SH_STRUCT_SR_REF
		1, // SH_STRUCT_S_REF
		1, // S_REF2
		3, // SH_REF_LOCAL_ASSIGN_STRUCTREF2
		1, // SH_GLOBAL_S_REF
		1, // SH_LOCAL_S_REF
		2, // SH_LOCALREF_SASSIGN_LOCALSREF
		2, // SH_LOCAL_APUSHBACK_LOCALSREF
		0, // SH_S_ASSIGN_CALLSYS19
		1, // SH_S_ASSIGN_STR0
		1, // SH_SASSIGN_LOCALSREF
		2, // SH_STRUCTREF_SASSIGN_LOCALSREF
		1, // SH_LOCALSREF_EMPTY
		2, // SH_GLOBAL_APUSHBACK_LOCALSREF
		2, // SH_STRUCT_APUSHBACK_LOCALSREF
		1, // SH_STRUCTSREF_EMPTY
		1, // SH_GLOBALSREF_EMPTY
		1, // SH_SASSIGN_STRUCTSREF
		1, // SH_SASSIGN_GLOBALSREF
		2, // SH_STRUCTSREF_NE_STR0
		2, // SH_GLOBALSREF_NE_STR0
		3, // SH_LOC_LT_IMM_OR_LOC_GE_IMM
	}
	for i, n := range sh {
		m[uint16(0xb2+i)] = n
	}
	// A_SORT_MEM=0xf1, DG_SET=0xf2 ... DG_STR_TO_METHOD=0x101
	dgN := map[uint16]int{
		0x0f1: 0, // A_SORT_MEM
		0x0f2: 0, // DG_SET
		0x0f3: 0, // DG_ADD
		0x0f4: 2, // DG_CALL
		0x0f5: 0, // DG_NUMOF
		0x0f6: 0, // DG_EXIST
		0x0f7: 0, // DG_ERASE
		0x0f8: 0, // DG_CLEAR
		0x0f9: 0, // DG_COPY
		0x0fa: 0, // DG_ASSIGN
		0x0fb: 0, // DG_PLUSA
		0x0fc: 0, // DG_POP
		0x0fd: 0, // DG_NEW_FROM_METHOD
		0x0fe: 0, // DG_MINUSA
		0x0ff: 1, // DG_CALLBEGIN
		0x100: 0, // DG_NEW
		0x101: 1, // DG_STR_TO_METHOD (v11+)
	}
	for op, n := range dgN {
		m[op] = n
	}
	return m
}()
