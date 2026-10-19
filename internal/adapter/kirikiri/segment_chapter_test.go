package kirikiri

import (
	"strings"
	"testing"

	"ucgr/internal/adapter"
)

// parseTestScript 解析一段脚本源码（补齐足够的标签行以过明文探测门槛）。
func parseTestScript(t *testing.T, src string) *Ast {
	t.Helper()
	raw, err := (&Adapter{}).ParseScript([]byte(src), &adapter.ScriptCtx{})
	if err != nil {
		t.Fatalf("ParseScript: %v", err)
	}
	ast, ok := raw.(*Ast)
	if !ok {
		t.Fatalf("ParseScript 返回 %T，期望 *Ast", raw)
	}
	return ast
}

// TestParseLabelTitle：`*label93|水畔` 的显示名要保留（切章节全靠它）。
func TestParseLabelTitle(t *testing.T) {
	src := "[wait time=100]\r\n" +
		"[trans time=200]\r\n" +
		"*label92|余话-着陆2\r\n" +
		"[r]\r\n" +
		"*kota1\r\n" +
		"[cm]\r\n" +
		"*label93|水畔\r\n" +
		"[r]\r\n"
	ast := parseTestScript(t, src)

	byName := map[string]Label{}
	for _, lb := range ast.Labels {
		byName[lb.Name] = lb
	}
	if got := byName["label92"].Title; got != "余话-着陆2" {
		t.Errorf("label92.Title = %q，期望「余话-着陆2」", got)
	}
	if got := byName["label93"].Title; got != "水畔" {
		t.Errorf("label93.Title = %q，期望「水畔」", got)
	}
	if got := byName["kota1"].Title; got != "" {
		t.Errorf("普通标签不该有显示名，kota1.Title = %q", got)
	}
}

// TestChaptersSpanPastPlainLabels：章节要跨过中间的普通标签（实测 150 处）。
func TestChaptersSpanPastPlainLabels(t *testing.T) {
	ast := &Ast{
		Seq: []SeqItem{
			{Kind: "text", Text: "序"}, // 0 章前内容
			{Kind: "text", Text: "甲"}, // 1
			{Kind: "text", Text: "乙"}, // 2
			{Kind: "text", Text: "丙"}, // 3
			{Kind: "text", Text: "丁"}, // 4
		},
		Labels: []Label{
			{Name: "label92", Title: "着陆2", SeqPos: 1},
			{Name: "kota1", SeqPos: 2},
			{Name: "label93", Title: "水畔", SeqPos: 3},
			{Name: "owa2", SeqPos: 4},
		},
	}
	chs := Chapters(ast)
	if len(chs) != 2 {
		t.Fatalf("章节数 = %d，期望 2", len(chs))
	}
	if chs[0].SeqStart != 1 || chs[0].SeqEnd != 3 {
		t.Errorf("第一段 [%d,%d)，期望 [1,3)——要跨过 kota1", chs[0].SeqStart, chs[0].SeqEnd)
	}
	if chs[1].SeqStart != 3 || chs[1].SeqEnd != 5 {
		t.Errorf("第二段 [%d,%d)，期望 [3,5)——末段到脚本末", chs[1].SeqStart, chs[1].SeqEnd)
	}
	if chs[0].Index != 92 || chs[1].Index != 93 {
		t.Errorf("Index = %d,%d，期望 92,93", chs[0].Index, chs[1].Index)
	}
}

// TestChaptersDuplicateIndex：同号异名（分支路线）要按出现序号区分。
func TestChaptersDuplicateIndex(t *testing.T) {
	ast := &Ast{
		Seq: []SeqItem{{Kind: "text"}, {Kind: "text"}, {Kind: "text"}, {Kind: "text"}},
		Labels: []Label{
			{Name: "label94", Title: "余话-聚餐", SeqPos: 0},
			{Name: "label94", Title: "余话-港镇", SeqPos: 1},
			{Name: "label94", Title: "余话-复仇", SeqPos: 2},
		},
	}
	chs := Chapters(ast)
	if len(chs) != 3 {
		t.Fatalf("章节数 = %d，期望 3", len(chs))
	}
	wantTitles := []string{"余话-聚餐", "余话-港镇", "余话-复仇"}
	seen := map[int]bool{}
	for i, ch := range chs {
		if ch.Index != 94 {
			t.Errorf("第 %d 段 Index = %d，期望 94", i, ch.Index)
		}
		if ch.Title != wantTitles[i] {
			t.Errorf("第 %d 段 Title = %q，期望 %q", i, ch.Title, wantTitles[i])
		}
		if seen[ch.LabelPos] {
			t.Errorf("LabelPos %d 重复，无法区分同号章节", ch.LabelPos)
		}
		seen[ch.LabelPos] = true
	}
}

// TestChaptersIgnoresUntitledLabelNN：没有 `|显示名` 的 labelNN 不算章节标记。
func TestChaptersIgnoresUntitledLabelNN(t *testing.T) {
	ast := &Ast{
		Seq: []SeqItem{{Kind: "text"}, {Kind: "text"}},
		Labels: []Label{
			{Name: "label90", SeqPos: 0},
			{Name: "cg17", Title: "cg17", SeqPos: 1},
		},
	}
	if got := Chapters(ast); len(got) != 0 {
		t.Fatalf("章节数 = %d，期望 0（label90 无显示名、cg17 不是 labelNN）", len(got))
	}
}

// TestBuildShowChapterTrimsToRange：构建的台本只含本章节的指令。
func TestBuildShowChapterTrimsToRange(t *testing.T) {
	ast := &Ast{
		Seq: []SeqItem{
			{Kind: "cmd", Cmd: mkCmd("image", map[string]string{"storage": "before"})},
			{Kind: "cmd", Cmd: mkCmd("image", map[string]string{"storage": "inside"})},
			{Kind: "cmd", Cmd: mkCmd("image", map[string]string{"storage": "after"})},
		},
		Labels: []Label{
			{Name: "label92", Title: "着陆2", SeqPos: 1},
			{Name: "label93", Title: "水畔", SeqPos: 2},
		},
	}
	chs := Chapters(ast)
	ev, err := BuildShowChapter(ast, chs[0], "scenario/71kaigan01.ks", "", "")
	if err != nil {
		t.Fatalf("BuildShowChapter: %v", err)
	}
	if ev.Title != "着陆2" {
		t.Errorf("Title = %q，期望「着陆2」", ev.Title)
	}
	if ev.EventID != "71kaigan01_label92" {
		t.Errorf("EventID = %q，期望 71kaigan01_label92", ev.EventID)
	}
	var refs []string
	for _, op := range ev.Show {
		if op.Ref != "" {
			refs = append(refs, op.Ref)
		}
	}
	if len(refs) != 1 || refs[0] != "inside" {
		t.Errorf("台本引用 = %v，期望只含本章的 inside", refs)
	}
}

// TestBuildShowChapterEmptyErrors：空章节要报错而不是给出空台本。
func TestBuildShowChapterEmptyErrors(t *testing.T) {
	ast := &Ast{Seq: []SeqItem{{Kind: "text"}}, Labels: []Label{{Name: "label1", Title: "空", SeqPos: 1}}}
	chs := Chapters(ast)
	if len(chs) != 1 {
		t.Fatalf("章节数 = %d", len(chs))
	}
	if _, err := BuildShowChapter(ast, chs[0], "x.ks", "", ""); err == nil {
		t.Fatal("空章节应报错")
	} else if !strings.Contains(err.Error(), "空") {
		t.Errorf("错误信息应说明是空章节: %v", err)
	}
}
