package library

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"ucgr/internal/adapter"
)

// ---------------------------------------------------------------
// 合成 BGI .bs 脚本（格式：魔数 + headerSize + 头 + 代码段 + 字符串区）
// 与 internal/adapter/bgi 的解析口径一致。
// ---------------------------------------------------------------

const (
	bsScriptMagic = "BurikoCompiledScriptVer1.00\x00"
	bsOpMsg       = 0x0140 // 显示消息
	bsOpBg        = 0x0280 // 背景（换背景 → 切新事件）
	bsOpExit      = 0x001B // 结束
)

type bsFixup struct {
	pos int
	str string
}

type bsAsm struct {
	code []byte
	fix  []bsFixup
}

func (a *bsAsm) d(v int32) {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], uint32(v))
	a.code = append(a.code, b[:]...)
}

func (a *bsAsm) op(opcode uint32, args ...int32) {
	a.d(int32(opcode))
	for _, v := range args {
		a.d(v)
	}
}

func (a *bsAsm) pushStr(s string) {
	a.fix = append(a.fix, bsFixup{pos: len(a.code) + 4, str: s})
	a.op(0x0003, 0)
}

func (a *bsAsm) finish() []byte {
	offs := map[string]int{}
	var area []byte
	for _, f := range a.fix {
		if _, ok := offs[f.str]; !ok {
			offs[f.str] = len(area)
			area = append(area, []byte(f.str)...)
			area = append(area, 0)
		}
	}
	base := len(a.code)
	for _, f := range a.fix {
		var b [4]byte
		binary.LittleEndian.PutUint32(b[:], uint32(base+offs[f.str]))
		copy(a.code[f.pos:], b[:])
	}
	// 头：外链脚本 1 个 + 标签 1 个（main → 0），其余补零对齐 headerSize。
	var hdr []byte
	var b4 [4]byte
	binary.LittleEndian.PutUint32(b4[:], 1)
	hdr = append(hdr, b4[:]...)
	hdr = append(hdr, []byte("sys.bs\x00")...)
	binary.LittleEndian.PutUint32(b4[:], 1)
	hdr = append(hdr, b4[:]...)
	hdr = append(hdr, []byte("main\x00")...)
	binary.LittleEndian.PutUint32(b4[:], 0)
	hdr = append(hdr, b4[:]...)
	for len(hdr) < 32 {
		hdr = append(hdr, 0)
	}
	out := append([]byte{}, bsScriptMagic...)
	binary.LittleEndian.PutUint32(b4[:], uint32(len(hdr)+4))
	out = append(out, b4[:]...)
	out = append(out, hdr...)
	out = append(out, a.code...)
	out = append(out, area...)
	return out
}

// synthBS 造一份含两段事件的脚本（换背景 → 切新事件）。
func synthBS() []byte {
	a := &bsAsm{}
	a.pushStr("bg1025b")
	a.op(bsOpBg) // 背景
	a.pushStr("由岐")
	a.pushStr("一行目")
	a.op(bsOpMsg)
	a.pushStr("bg1001b")
	a.op(bsOpBg) // 换背景 → 切新事件
	a.pushStr("二行目")
	a.op(bsOpMsg)
	a.op(bsOpExit)
	return a.finish()
}

// TestScanBGIEvents 扫 BGI 目录 → 每个事件一条场景事件（沿用适配器的切分）。
func TestScanBGIEvents(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "scenario")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "op01.bs"), synthBS(), 0o644); err != nil {
		t.Fatal(err)
	}
	conv := adapter.Convention{
		EngineID:    "bgi",
		ScenarioDir: "scenario",
		ScriptExt:   ".bs",
	}
	lib, err := ScanWith(root, conv)
	if err != nil {
		t.Fatalf("ScanWith: %v", err)
	}
	if len(lib.Scenes) < 2 {
		t.Fatalf("场景事件数 = %d, want ≥2（换背景切事件）：%+v", len(lib.Scenes), lib.Scenes)
	}
	// 事件 id 形如 op01#001；Label 记序号。
	if lib.Scenes[0].ID != "op01#001" {
		t.Errorf("第一个事件 id = %q", lib.Scenes[0].ID)
	}
	if lib.Scenes[0].Label != "1" || lib.Scenes[1].Label != "2" {
		t.Errorf("事件 Label = %q, %q", lib.Scenes[0].Label, lib.Scenes[1].Label)
	}
	if lib.Scenes[0].Script != "scenario/op01.bs" {
		t.Errorf("脚本路径 = %q", lib.Scenes[0].Script)
	}
	// 找得到
	if lib.FindScene("op01#001") == nil {
		t.Error("FindScene 应能找到 op01#001")
	}
}
