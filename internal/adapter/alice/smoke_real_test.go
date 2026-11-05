package alice

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"ucgr/internal/adapter"
)

// 真实语料 smoke 测试：兰斯10（AliceSoft System 4，AI2/AIN + AFA）。
// 语料缺失自动跳过；UCGR_RANCE10_DIR 可覆盖语料目录。

const rance10AinName = "Rance10.ain"

func rance10Dir(t *testing.T) string {
	t.Helper()
	dirs := []string{
		os.Getenv("UCGR_RANCE10_DIR"),
		`E:\Personal_entertainment\game\zfsgalgame\Rance10\兰斯10`,
	}
	for _, d := range dirs {
		if d == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(d, rance10AinName)); err == nil {
			return d
		}
	}
	t.Skip("兰斯10 语料缺失")
	return ""
}

func mustParseAIN(t *testing.T, dir string) *AIN {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, rance10AinName))
	if err != nil {
		t.Fatal(err)
	}
	ain, err := ParseAIN(data)
	if err != nil {
		t.Fatal(err)
	}
	return ain
}

// TestRealAINParse：AI2 解包 + 区段流解析（期望值来自 aindump 实测）。
func TestRealAINParse(t *testing.T) {
	ain := mustParseAIN(t, rance10Dir(t))
	if ain.Version != 12 {
		t.Fatalf("AIN 版本 = %d, want 12", ain.Version)
	}
	if len(ain.Funcs) != 37698 {
		t.Fatalf("函数数 = %d, want 37698", len(ain.Funcs))
	}
	if len(ain.Messages) != 269676 {
		t.Fatalf("消息数 = %d, want 269676", len(ain.Messages))
	}
	if len(ain.Strings) != 15953 {
		t.Fatalf("字符串数 = %d, want 15953", len(ain.Strings))
	}
	if len(ain.Libs) == 0 {
		t.Fatal("HLL 库为空")
	}
	// GBK 消息解码抽查（汉化正文）
	found := false
	for _, m := range ain.Messages {
		if strings.Contains(m, "萨娜琪亚") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("消息区没有解码出中文正文（GBK 解密异常）")
	}
}

// TestRealAINDisasm：反汇编「萨娜琪亚／参战」，产出与 aindump -t 一致的序列。
func TestRealAINDisasm(t *testing.T) {
	ain := mustParseAIN(t, rance10Dir(t))
	idx := ain.FindFunc("萨娜琪亚／参战")
	if idx < 0 {
		t.Fatal("找不到场景函数 萨娜琪亚／参战")
	}
	instrs := ain.DisasmFunc(ain.Funcs[idx].Address)
	var gotS, gotM []string
	for _, in := range instrs {
		switch in.Op {
		case opSPUSH:
			if in.Str != "" {
				gotS = append(gotS, in.Str)
			}
		case opMSG:
			if in.Str != "" {
				gotM = append(gotM, in.Str)
			}
		}
	}
	if len(gotS) == 0 || gotS[0] != "萨娜琪亚／基本" {
		t.Fatalf("首立绘 = %q, want 萨娜琪亚／基本", firstOr(gotS))
	}
	if len(gotM) < 2 || !strings.HasPrefix(gotM[0], "「骑士团长萨娜琪亚") {
		t.Fatalf("首台词 = %q", firstOr(gotM))
	}
}

func firstOr(s []string) string {
	if len(s) == 0 {
		return ""
	}
	return s[0]
}

// TestRealSceneEvents：场景函数筛选 + 事件构建。
func TestRealSceneEvents(t *testing.T) {
	ain := mustParseAIN(t, rance10Dir(t))
	evs, skipped := ain.BuildSceneEvents()
	if len(evs) < 100 {
		t.Fatalf("场景事件 = %d, want >= 100（跳过 %d）", len(evs), skipped)
	}
	// 事件标题应是可读场景名
	var hasNamed bool
	for _, ev := range evs {
		if hasCJK(ev.EventID) && ev.Title == ev.EventID && len(ev.Texts) > 0 {
			hasNamed = true
			break
		}
	}
	if !hasNamed {
		t.Fatal("没有带标题带正文的事件")
	}
}

// TestRealAFA：AFA 表解析 + 条目数据魔数。
func TestRealAFA(t *testing.T) {
	dir := rance10Dir(t)
	af, err := ParseAFA(filepath.Join(dir, "Rance10CG.afa"))
	if err != nil {
		t.Fatal(err)
	}
	if len(af.Entries) != 3460 {
		t.Fatalf("CG.afa 条目 = %d, want 3460", len(af.Entries))
	}
	// 首条 = _シス／帮助／任务画面２.ajp，off=8 size=169036
	e0 := &af.Entries[0]
	if !strings.Contains(e0.Name, "任务画面２.ajp") {
		t.Fatalf("首条目 = %q", e0.Name)
	}
	if e0.Off != 8 || e0.Size != 169036 {
		t.Fatalf("首条目 off=%d size=%d, want 8/169036", e0.Off, e0.Size)
	}
	// 条目数据魔数：AJP / QNT / dcf
	kinds := map[string]int{}
	checked := 0
	for _, e := range af.Entries {
		if checked >= 500 {
			break
		}
		checked++
		data, err := af.ReadEntry(e)
		if err != nil {
			t.Fatalf("读 %s：%v", e.Name, err)
		}
		if len(data) < 4 {
			t.Fatalf("%s 数据过短", e.Name)
		}
		switch {
		case string(data[:4]) == "AJP\x00":
			kinds["ajp"]++
		case string(data[:3]) == "QNT":
			kinds["qnt"]++
		case string(data[:4]) == "dcf ":
			kinds["dcf"]++
		default:
			t.Fatalf("%s 头部异常: % x", e.Name, data[:4])
		}
	}
	if kinds["ajp"] == 0 || kinds["qnt"] == 0 {
		t.Fatalf("图像格式分布异常: %v", kinds)
	}
}

// TestRealAFALookupStem：裸名（萨娜琪亚／基本）尾部匹配立绘条目。
func TestRealAFALookupStem(t *testing.T) {
	dir := rance10Dir(t)
	var af *AFA
	for _, name := range []string{"Rance10CG.afa", "Rance10CG2.afa"} {
		a, err := ParseAFA(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if e := a.LookupStem("萨娜琪亚／基本"); e != nil {
			af = a
			break
		}
		_ = a
	}
	if af == nil {
		t.Fatal("裸名匹配不到立绘条目")
	}
}

// TestRealAliceParseArchiveOffset：ParseArchive 的 Offset 契约是「归档文件内
// 绝对偏移」（adapter.ReadArchiveEntry 直接 Seek 它）——AFA 的 off 相对
// dataStart，回归保护：按 Offset 读回的条目必须以合法图像魔数开头。
func TestRealAliceParseArchiveOffset(t *testing.T) {
	dir := rance10Dir(t)
	ents, err := (&Adapter{}).ParseArchive(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(ents) == 0 {
		t.Fatal("ParseArchive 列出 0 条目")
	}
	afaPath := filepath.Join(dir, "Rance10CG.afa")
	f, err := os.Open(afaPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, e := range ents {
		if !strings.HasPrefix(e.Path, "Rance10CG/") {
			continue
		}
		if e.Offset <= 0 || e.Offset+e.Size > st.Size() {
			t.Fatalf("条目 %s 越界：offset=%d size=%d 文件=%d", e.Path, e.Offset, e.Size, st.Size())
		}
		data, err := adapter.ReadArchiveEntry(f, e)
		if err != nil {
			t.Fatalf("读条目 %s 失败：%v", e.Path, err)
		}
		magic := string(data[:min(4, len(data))])
		if magic != "AJP\x00" && magic != "QNT\x00" && magic != "dcf " {
			t.Fatalf("条目 %s @offset=%d 魔数 %q 不是图像（Offset 语义可能错位）", e.Path, e.Offset, magic)
		}
		checked++
		if checked >= 20 {
			break
		}
	}
	if checked == 0 {
		t.Fatal("没取到 Rance10CG 条目")
	}
}
