package alice

import (
	"encoding/binary"
	"testing"
)

// ---- opcode 表 ----

func TestOpcodeTableKeyOps(t *testing.T) {
	key := map[uint16]int{
		0x00: 1, // PUSH
		0x2c: 1, // JUMP
		0x30: 1, // CALLFUNC
		0x41: 1, // S_PUSH
		0x59: 1, // MSG
		0x5a: 3, // CALLHLL (v11+)
		0x5c: 1, // CALLMETHOD
		0x63: 1, // CALLSYS
		0x7e: 1, // ENDFUNC
	}
	for op, n := range key {
		if got, ok := opcodeNArgs[op]; !ok || got != n {
			t.Fatalf("opcode 0x%02x = (%d,%v), want nargs=%d", op, got, ok, n)
		}
	}
	// SH 系列 0xb2 起连续无空洞
	for op := uint16(0xb2); op < 0xf1; op++ {
		if _, ok := opcodeNArgs[op]; !ok {
			t.Fatalf("SH 指令 0x%02x 缺失", op)
		}
	}
	if opcodeNArgs[0x101] != 1 {
		t.Fatal("DG_STR_TO_METHOD (v11+) 应为 1 参")
	}
}

// ---- msg1 解密 ----

func TestMsg1Decrypt(t *testing.T) {
	// 加密串 b[i] 原文 = 明文[i] + i + 0x60（模 256）
	plain := []byte("AB")
	enc := make([]byte, len(plain))
	for i, b := range plain {
		enc[i] = b + byte(i) + 0x60
	}
	buf := make([]byte, 4+len(enc))
	binary.LittleEndian.PutUint32(buf, uint32(len(enc)))
	copy(buf[4:], enc)
	r := &reader{buf: buf}
	if got := r.msg1(); got != "AB" {
		t.Fatalf("msg1 = %q, want AB", got)
	}
}

// ---- hasCJK / 工具 ----

func TestHasCJK(t *testing.T) {
	cases := map[string]bool{
		"萨娜琪亚／参战":                  true,
		"◎ＭＡＰ制御「２０１／进入鱿鱼山」": true,
		"parts::NumofConstructProcess": false,
		"CASConstructionProcess@0":     false,
		"t０５／洞窟中的兰斯等人２":        true, // 全角数字 FF10-FF19 在 FF00-FFEF
	}
	for s, want := range cases {
		if got := hasCJK(s); got != want {
			t.Fatalf("hasCJK(%q) = %v, want %v", s, got, want)
		}
	}
}

func TestTruncateRunes(t *testing.T) {
	if got := truncateRunes("あいうえお", 3); got != "あいう…" {
		t.Fatalf("truncateRunes = %q", got)
	}
	if got := truncateRunes("短い", 10); got != "短い" {
		t.Fatalf("truncateRunes = %q", got)
	}
}

// ---- 合成 AIN 解析（无语料环境可跑） ----

func synthAIN(t *testing.T) []byte {
	// VERS(12) + MSG0(2条) + STR0(1条) + FUNC(1个场景函数)；中文按 GBK 编码
	g := func(s string) []byte {
		b, err := gbkEncode(s)
		if err != nil {
			t.Skipf("GBK 编码不可用: %v", err)
		}
		return b
	}
	var b []byte
	put32 := func(v uint32) {
		b = append(b, byte(v), byte(v>>8), byte(v>>16), byte(v>>24))
	}
	put := func(s string) { b = append(b, s...) }
	put("VERS")
	put32(12)
	put("MSG0")
	put32(2)
	b = append(b, g("消息一")...)
	b = append(b, 0)
	b = append(b, g("消息二")...)
	b = append(b, 0)
	put("STR0")
	put32(1)
	b = append(b, g("立／测试立绘")...)
	b = append(b, 0)
	// FUNC：1 个函数（addr=0，void 返回，0 参 0 变量）
	put("FUNC")
	put32(1)
	put32(0)
	b = append(b, g("测试场景")...)
	b = append(b, 0)
	put32(0); put32(0) // type: data + struc
	put32(0)          // rank
	put32(0)          // nr_args
	put32(0)          // nr_vars
	put32(0)          // is_lambda
	put32(0)          // crc
	return b
}

func TestSynthAIN(t *testing.T) {
	ain, err := ParseAIN(synthAIN(t))
	if err != nil {
		t.Fatal(err)
	}
	if ain.Version != 12 {
		t.Fatalf("版本 = %d", ain.Version)
	}
	if len(ain.Messages) != 2 || ain.Messages[1] != "消息二" {
		t.Fatalf("消息 = %v", ain.Messages)
	}
	if len(ain.Strings) != 1 || ain.Strings[0] != "立／测试立绘" {
		t.Fatalf("字符串 = %v", ain.Strings)
	}
	if len(ain.Funcs) != 1 || ain.Funcs[0].Name != "测试场景" {
		t.Fatalf("函数 = %+v", ain.Funcs)
	}
	if !hasCJK(ain.Funcs[0].Name) {
		t.Fatal("场景函数判 CJK 失败")
	}
}
