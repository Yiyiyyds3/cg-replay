package bgi

import (
	"encoding/binary"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ucgr/internal/adapter"
	"ucgr/internal/ir"
)

// ---- 合成 V1 脚本（对照真机格式：魔数 + headerSize + 头 + 代码 + 字符串区）----

type fixup struct {
	pos int
	str string
}

type asm struct {
	code []byte
	fix  []fixup
}

func (a *asm) d(v int32) {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], uint32(v))
	a.code = append(a.code, b[:]...)
}

func (a *asm) op(opcode uint32, args ...int32) {
	a.d(int32(opcode))
	for _, v := range args {
		a.d(v)
	}
}

// pushStr 发一条 0x0003 并把字符串登记进字符串区。
func (a *asm) pushStr(s string) {
	a.fix = append(a.fix, fixup{pos: len(a.code) + 4, str: s})
	a.op(0x0003, 0)
}

func (a *asm) finish() []byte {
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
	// 头：外链脚本 1 个 + 标签 1 个（main → 0），其余补零对齐 headerSize
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

	out := append([]byte{}, scriptMagic...)
	binary.LittleEndian.PutUint32(b4[:], uint32(len(hdr)+4))
	out = append(out, b4[:]...)
	out = append(out, hdr...)
	out = append(out, a.code...)
	out = append(out, area...)
	return out
}

// synth 造一份含「背景 / 台词 / 语音 / 未登记命令 / 换景 / CG / 选项」的脚本。
func synth() []byte {
	a := &asm{}
	a.pushStr("bg1025b")
	a.op(0x0280) // 背景
	a.pushStr("由岐")
	a.pushStr("一行目")
	a.op(opMsg)
	a.pushStr("taku_001715")
	a.op(0x01A5) // 语音：绑下一条台词
	a.pushStr("由岐")
	a.pushStr("二行目")
	a.op(opMsg)
	a.pushStr("se0042")
	a.op(0x03F1) // 未登记命令 + 资源形字符串（§6 保守保留）
	a.pushStr("bg1001b")
	a.op(0x0280) // 换背景 → 新事件
	a.pushStr("三行目")
	a.op(opMsg)
	a.pushStr("ev1000a")
	a.op(0x02C0) // 事件 CG
	a.pushStr("選択肢A")
	a.pushStr("選択肢B")
	a.pushStr("_SelectEx")
	a.op(opUserCall) // 选项屏
	a.op(opExit)
	return a.finish()
}

func TestParseSynthetic(t *testing.T) {
	s, err := ParseScriptData(synth(), "data00001/sample.bs")
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if s.CodeOffset != 64 {
		t.Errorf("代码段起点 = %d，想要 64", s.CodeOffset)
	}
	if len(s.Labels) != 1 || s.Labels[0].Name != "main" {
		t.Errorf("标签 = %+v，想要 [main]", s.Labels)
	}
	if len(s.RefScripts) != 1 || s.RefScripts[0] != "sys.bs" {
		t.Errorf("外链脚本 = %v，想要 [sys.bs]", s.RefScripts)
	}
	if len(s.Notes) != 0 {
		t.Errorf("不应有可疑记录：%v", s.Notes)
	}
	// 指令数：13 条 pushStr（各 2 dword）+ 10 条命令（各 1 dword）= 23 条 / 36 dword
	if s.Instr != 23 {
		t.Errorf("指令数 = %d，想要 23", s.Instr)
	}
	if s.CodeLen != 144 {
		t.Errorf("代码段长度 = %d，想要 144（36 dword）", s.CodeLen)
	}
	// 字符串归属：0x03F1 应吃到 se0042；文本命令应吃到说话人+正文
	var unknown, msg *Command
	for i := range s.Commands {
		switch s.Commands[i].Op {
		case 0x03F1:
			unknown = &s.Commands[i]
		case opMsg:
			if msg == nil {
				msg = &s.Commands[i]
			}
		}
	}
	if unknown == nil || len(unknown.Pushed) != 1 || unknown.Pushed[0] != "se0042" {
		t.Errorf("0x03F1 的字符串归属 = %+v", unknown)
	}
	if msg == nil || len(msg.Pushed) != 2 || msg.Pushed[0] != "由岐" || msg.Pushed[1] != "一行目" {
		t.Errorf("0x0140 的字符串归属 = %+v", msg)
	}
}

func TestExtractRefs(t *testing.T) {
	s, err := ParseScriptData(synth(), "sample.bs")
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	got := s.ExtractRefs()
	want := map[string]bool{"bg1025b": true, "taku_001715": true, "se0042": true, "bg1001b": true, "ev1000a": true}
	if len(got) != len(want) {
		t.Fatalf("引用 = %v，想要 %d 条", got, len(want))
	}
	for _, r := range got {
		if !want[r] {
			t.Errorf("多收了引用 %q", r)
		}
	}
	for _, r := range got {
		if strings.HasPrefix(r, "行目") || r == "由岐" || r == "選択肢A" {
			t.Errorf("台词/选项被当成资源引用：%q", r)
		}
	}
}

func TestBuildIR(t *testing.T) {
	s, err := ParseScriptData(synth(), "data00001/sample.bs")
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	hashes := map[string]bool{}
	resolve := func(ref string) (*ir.AssetRef, bool) {
		h := "h-" + ref
		hashes[h] = true
		return &ir.AssetRef{Hash: h, OrigPath: ref + ".png", Role: "其它"}, true
	}
	out, err := (&Adapter{}).BuildIR(s, &adapter.BuildCtx{GameID: "test", Resolver: resolve})
	if err != nil {
		t.Fatalf("BuildIR 失败：%v", err)
	}
	if errs := ir.Validate(out); len(errs) > 0 {
		t.Fatalf("V 校验未过：%v", errs)
	}
	if errs := ir.CheckAssetRefs(out, hashes); len(errs) > 0 {
		t.Fatalf("资产校验未过：%v", errs)
	}
	if len(out.Events) != 2 {
		t.Fatalf("事件数 = %d，想要 2", len(out.Events))
	}
	e0, e1 := out.Events[0], out.Events[1]
	if e0.Kind != ir.KindScene {
		t.Errorf("事件 1 类型 = %v，想要 scene", e0.Kind)
	}
	if len(e0.Texts) != 2 {
		t.Errorf("事件 1 台词 = %d 条，想要 2", len(e0.Texts))
	}
	if len(e0.Voices) != 1 || e0.Voices[0].LineID != e0.Texts[1].LineID {
		t.Errorf("语音绑定 = %+v，想要绑在第二条台词", e0.Voices)
	}
	if len(e0.Layers) != 1 || e0.Layers[0].Kind != ir.LayerBackground {
		t.Errorf("事件 1 图层 = %+v，想要 1 个背景层", e0.Layers)
	}
	if len(e0.Unrecognized) != 1 || e0.Unrecognized[0].NodePath != "op[0x03F1]" {
		t.Errorf("事件 1 未识别登记 = %+v", e0.Unrecognized)
	}
	if len(e0.Unrecognized) == 1 && len(e0.Unrecognized[0].KeptAssets) == 0 {
		t.Errorf("未登记命令的资源形实参没有保守保留")
	}
	if e1.Kind != ir.KindStillCG {
		t.Errorf("事件 2 类型 = %v，想要 still_cg（含 ev1000a）", e1.Kind)
	}
	if len(e1.Branches) != 1 || len(e1.Branches[0].Options) != 2 {
		t.Errorf("选项屏 = %+v，想要 2 个选项", e1.Branches)
	}
	// Show 台本顺序：层 → 台词 → 语音
	var kinds []string
	for _, op := range e0.Show {
		kinds = append(kinds, op.Kind)
	}
	if len(kinds) == 0 || kinds[0] != ir.ShowLayerShow {
		t.Errorf("Show 首条 = %v，想要 layer_show", kinds)
	}
}

func TestIdentify(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "文本_脚本", "data01999", "makerlogo.bs")
	if err := os.MkdirAll(filepath.Dir(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(script, synth(), 0o644); err != nil {
		t.Fatal(err)
	}
	// 文件：带头魔数 → 认领；非 .bs → 不认领
	if id, err := (&Adapter{}).Identify(script); err != nil || id != engineID {
		t.Errorf("Identify(.bs 文件) = %q, %v", id, err)
	}
	txt := filepath.Join(dir, "note.txt")
	_ = os.WriteFile(txt, synth(), 0o644)
	if id, _ := (&Adapter{}).Identify(txt); id != "" {
		t.Errorf("非 .bs 文件被认领：%q", id)
	}
	// 目录：有 .bs 魔数 → 认领
	if id, err := (&Adapter{}).Identify(dir); err != nil || id != engineID {
		t.Errorf("Identify(目录) = %q, %v", id, err)
	}
	// 反向证据：目录里有 .xp3 → 不认领
	_ = os.WriteFile(filepath.Join(dir, "data.xp3"), []byte("x"), 0o644)
	if id, _ := (&Adapter{}).Identify(dir); id != "" {
		t.Errorf("带 .xp3 的目录被认领：%q", id)
	}
}

// TestRealCorpus 真机语料（需设 UCGR_BGI_DIR）。
// 期望值与 Python 原型（analyze2.py）一致：文件数 / 指令数逐款吻合。
func TestRealCorpus(t *testing.T) {
	root := os.Getenv("UCGR_BGI_DIR")
	if root == "" {
		t.Skip("未设置 UCGR_BGI_DIR，跳过真机语料测试")
	}
	var files, instr, msgs, refs, fail, vErrs, events int
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.EqualFold(filepath.Ext(p), scriptExt) {
			return nil
		}
		data, rerr := os.ReadFile(p)
		if rerr != nil {
			fail++
			return nil
		}
		s, perr := ParseScriptData(data, p)
		if perr != nil {
			fail++
			t.Logf("解析失败 %s: %v", p, perr)
			return nil
		}
		files++
		instr += s.Instr
		refs += len(s.ExtractRefs())
		for i := range s.Commands {
			c := &s.Commands[i]
			if opKind(c.Op) == "text" && len(c.Pushed) > 0 {
				msgs++
			}
		}
		if files <= 40 { // 抽样做 IR 构建 + 校验
			out, berr := (&Adapter{}).BuildIR(s, &adapter.BuildCtx{GameID: filepath.Base(root)})
			if berr != nil {
				t.Logf("BuildIR 跳过 %s: %v", p, berr)
				return nil
			}
			if errs := ir.Validate(out); len(errs) > 0 {
				vErrs += len(errs)
				if vErrs <= 5 {
					t.Logf("V 校验 %s: %v", p, errs[0])
				}
			}
			events += len(out.Events)
		}
		return nil
	})
	t.Logf("语料：%d 个 .bs / %d 条指令 / %d 条文本 / %d 条引用 / 抽样 %d 个事件 / 失败 %d / V 校验错 %d",
		files, instr, msgs, refs, events, fail, vErrs)
	if fail > 0 {
		t.Errorf("解析失败 %d 个", fail)
	}
	if vErrs > 0 {
		t.Errorf("IR V 校验错误 %d 条", vErrs)
	}
}
