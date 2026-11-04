// nscript_test.go —— 不依赖真实语料的合成用例：NSA 索引/解压、
// 脚本词法、事件切分。
package nscript

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"

	"ucgr/internal/adapter"
	"ucgr/internal/ir"
)

// ---- NSA 索引 ----

func buildNsa(entries map[string][]byte) []byte {
	// 头：u16 BE 数量 + u32 BE base（= 头长）
	names := make([]string, 0, len(entries))
	for k := range entries {
		names = append(names, k)
	}
	// 稳定顺序：按名字排序后依次排数据
	for i := 0; i < len(names); i++ {
		for j := i + 1; j < len(names); j++ {
			if names[j] < names[i] {
				names[i], names[j] = names[j], names[i]
			}
		}
	}
	var idx bytes.Buffer
	var data bytes.Buffer
	idx.WriteByte(byte(len(names) >> 8))
	idx.WriteByte(byte(len(names)))
	base := uint32(6)
	var b4h [4]byte
	binary.BigEndian.PutUint32(b4h[:], base)
	idx.Write(b4h[:])
	off := base
	for _, n := range names {
		idx.WriteString(n)
		idx.WriteByte(0)
		idx.WriteByte(compNone)
		var b4 [4]byte
		rel := off - base
		binary.BigEndian.PutUint32(b4[:], rel)
		idx.Write(b4[:])
		binary.BigEndian.PutUint32(b4[:], uint32(len(entries[n])))
		idx.Write(b4[:])
		binary.BigEndian.PutUint32(b4[:], uint32(len(entries[n])))
		idx.Write(b4[:])
		off += uint32(len(entries[n]))
		data.Write(entries[n])
	}
	var out bytes.Buffer
	binary.Write(&out, binary.BigEndian, uint16(len(names)))
	binary.Write(&out, binary.BigEndian, base)
	out.Write(idx.Bytes()[6:])
	out.Write(data.Bytes())
	return out.Bytes()
}

func TestParseNsaDataRoundtrip(t *testing.T) {
	raw := buildNsa(map[string][]byte{
		"bmp\\a.bmp": []byte("AAAA"),
		"bmp\\b.bmp": []byte("BBBBBB"),
	})
	a, err := ParseNsaData(raw, "mem.nsa")
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(a.Entries) != 2 {
		t.Fatalf("条目数 %d != 2", len(a.Entries))
	}
	if e, ok := a.Lookup("BMP/A.BMP"); !ok || e.Length != 4 {
		t.Fatalf("大小写/分隔符查找失败: %v", e)
	}
	if e, ok := a.Lookup("b"); !ok || e.Length != 6 {
		t.Fatalf("省略扩展名查找失败: %v", e)
	}
}

func TestParseNsaRejectsGarbage(t *testing.T) {
	// 压缩类型越界 → 不是 NSA（防误认领加密归档）
	raw := buildNsa(map[string][]byte{"a.bmp": []byte("AAAA")})
	raw[6+5+1+1] = 200 // 第一条目的 comp 字节改坏（名字 a.bmp\0 之后）
	if _, err := ParseNsaData(raw, "bad.nsa"); err == nil {
		t.Fatal("坏压缩类型应报错")
	}
}

// ---- 脚本词法 ----

const sampleScript = `;value 2500

*define
numalias KX 100
stralias Sora,"Bmp\Haikei\Mura\Sora.jpg"
game

*start
bg black,10
ld l,$Sora,3
「　　」テスト。@
;＜べるの
!s0「ふふん。@`

// 注：`;value 2500` 是关键字嗅探的锚，动它要连 scriptPlainText 一起看。

func TestParseScriptBasics(t *testing.T) {
	s, err := ParseScript(sampleScript, "0.txt")
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if s.StrAlias["Sora"] != `Bmp\Haikei\Mura\Sora.jpg` {
		t.Fatalf("stralias 解析错: %v", s.StrAlias)
	}
	texts := 0
	gotLabel := false
	for i := range s.Lines {
		switch s.Lines[i].Kind {
		case LineText:
			texts++
		case LineLabel:
			if s.Lines[i].Label == "start" {
				gotLabel = true
			}
		}
	}
	if texts != 2 {
		t.Fatalf("台词行 %d != 2（含 !s0 与「」开头的行）", texts)
	}
	if !gotLabel {
		t.Fatal("*start 标签没解析到")
	}
	// 说话人注释
	last := s.Lines[len(s.Lines)-1]
	_ = last
}

func TestParseScriptColonSplit(t *testing.T) {
	s, err := ParseScript("game\nbg white,10:bg sora,1\n", "0.txt")
	if err != nil {
		t.Fatal(err)
	}
	bgs := 0
	for i := range s.Lines {
		if s.Lines[i].Kind == LineCmd && s.Lines[i].Cmd == "bg" {
			bgs++
		}
	}
	if bgs != 2 {
		t.Fatalf("冒号连写的 bg 应拆成 2 条，实际 %d", bgs)
	}
}

func TestParseScriptDefineBlockIndent(t *testing.T) {
	// define 块里缩进的 dim 是命令不是台词
	s, err := ParseScript("*define\n   dim ?Tip[200]\ngame\n*start\n　台词。\n", "0.txt")
	if err != nil {
		t.Fatal(err)
	}
	texts := 0
	dims := 0
	for i := range s.Lines {
		switch s.Lines[i].Kind {
		case LineText:
			texts++
		case LineCmd:
			if s.Lines[i].Cmd == "dim" {
				dims++
			}
		}
	}
	if texts != 1 || dims != 1 {
		t.Fatalf("texts=%d dims=%d（应为 1/1）", texts, dims)
	}
}

// ---- BuildIR ----

func nscriptScriptForTest(t *testing.T, text string) *ir.IR {
	t.Helper()
	a := &Adapter{}
	obj, err := a.ParseScript([]byte(text), &adapter.ScriptCtx{EntryPath: "0.txt"})
	if err != nil {
		t.Fatalf("ParseScript: %v", err)
	}
	doc, err := a.BuildIR(obj, &adapter.BuildCtx{GameID: "t"})
	if err != nil {
		t.Fatalf("BuildIR: %v", err)
	}
	return doc
}

func TestBuildIRSplitsOnBGChange(t *testing.T) {
	doc := nscriptScriptForTest(t, `;value 2500
*define
game
*start
bg "Bmp\A.jpg",1
　最初の台詞。@
bg "Bmp\B.jpg",1
　次の台詞。@
`)
	if len(doc.Events) != 2 {
		t.Fatalf("换背景应切 2 个事件，实际 %d", len(doc.Events))
	}
	if doc.Events[0].Title == "" {
		t.Fatal("事件标题应取首句台词")
	}
}

func TestBuildIRSpeakerAndStand(t *testing.T) {
	doc := nscriptScriptForTest(t, `;value 2500
*define
stralias ST1,"Bmp\Tati\X.jpg"
game
*start
ld l,$ST1,3
「こんにちは。」@
;＜ scalar
`)
	if len(doc.Events) != 1 {
		t.Fatalf("事件数 %d != 1", len(doc.Events))
	}
	ev := &doc.Events[0]
	if len(ev.Texts) != 1 || ev.Texts[0].Speaker != "scalar" {
		t.Fatalf("说话人解析错: %+v", ev.Texts)
	}
	stands := 0
	for _, l := range ev.Layers {
		if l.Kind == ir.LayerStand {
			stands++
			if l.Asset == nil || l.Asset.OrigPath != `Bmp\Tati\X.jpg` {
				t.Fatalf("立绘资产应解为 stralias 值: %+v", l.Asset)
			}
		}
	}
	if stands != 1 {
		t.Fatalf("立绘层数 %d != 1", stands)
	}
	if len(ev.Show) == 0 {
		t.Fatal("Show 台本为空")
	}
	hasText := false
	for _, op := range ev.Show {
		if op.Kind == ir.ShowTextRun && op.Text != nil && strings.Contains(op.Text.Text, "こんにちは") {
			hasText = true
		}
	}
	if !hasText {
		t.Fatal("Show 里没有台词指令")
	}
}

func TestBuildIRSelectBranch(t *testing.T) {
	doc := nscriptScriptForTest(t, `;value 2500
*define
game
*start
selgosub "行く",*a,"行かない",*b
　その後。@
`)
	ev := &doc.Events[0]
	if len(ev.Branches) != 1 || len(ev.Branches[0].Options) != 2 {
		t.Fatalf("select 应产出 1 个分支 2 个选项: %+v", ev.Branches)
	}
}

func TestScriptPlainTextSniff(t *testing.T) {
	// 密文（首行是空行的脚本）：XOR 后以 \n;value 开头
	plain := []byte("\n;value 2500\n*define\ngame\n")
	enc := make([]byte, len(plain))
	for i := range plain {
		enc[i] = plain[i] ^ scriptKey
	}
	if got := scriptPlainText(enc); !bytes.Contains(got, []byte(";value")) {
		t.Fatal("密文应被 XOR 解出")
	}
	if got := scriptPlainText(plain); !bytes.Contains(got, []byte(";value")) {
		t.Fatal("明文应原样通过")
	}
}
