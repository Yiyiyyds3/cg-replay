package kirikiri

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"ucgr/internal/adapter"
	"ucgr/internal/ir"
)

// Segment 取脚本中某个标签对应的片段（`*cg01` → 该标签到下一个标签之间）。
// label 为空时返回整脚本；标签名允许带或不带前导 `*`。找不到标签返回 nil。
func Segment(ast *Ast, label string) []SeqItem {
	if ast == nil {
		return nil
	}
	name := strings.TrimPrefix(strings.TrimSpace(label), "*")
	if name == "" {
		return ast.Seq
	}
	for i, lb := range ast.Labels {
		if lb.Name != name {
			continue
		}
		start := lb.SeqPos
		if start < 0 {
			start = 0
		}
		if start > len(ast.Seq) {
			start = len(ast.Seq)
		}
		end := len(ast.Seq)
		if i+1 < len(ast.Labels) {
			end = ast.Labels[i+1].SeqPos
		}
		if end < start {
			end = start
		}
		return ast.Seq[start:end]
	}
	return nil
}

// Labels 返回脚本内全部标签名（按出现顺序，去掉 `*`）。
func Labels(ast *Ast) []string {
	if ast == nil {
		return nil
	}
	out := make([]string, 0, len(ast.Labels))
	for _, lb := range ast.Labels {
		out = append(out, lb.Name)
	}
	return out
}

// Section 一个「标签段」：原作自己的事件单位（CG 鉴赏的 *cgNN、场景的 *hajime…）。
type Section struct {
	Name   string
	SeqPos int
}

// Sections 返回脚本内的标签段（含段首位置），空名标签段（脚本开头到第一个标签）不返回。
func Sections(ast *Ast) []Section {
	if ast == nil {
		return nil
	}
	out := make([]Section, 0, len(ast.Labels))
	for _, lb := range ast.Labels {
		out = append(out, Section{Name: lb.Name, SeqPos: lb.SeqPos})
	}
	return out
}

// ParseScriptFile 读一个 .ks 并解析（库与播放器复用）。
func ParseScriptFile(path string) (*Ast, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读不到脚本 %s：%w", path, err)
	}
	obj, err := (&Adapter{}).ParseScript(data, &adapter.ScriptCtx{EntryPath: path})
	if err != nil {
		return nil, fmt.Errorf("解析脚本 %s 失败：%w", path, err)
	}
	a, ok := obj.(*Ast)
	if !ok {
		return nil, fmt.Errorf("脚本 %s 的解析结果类型异常", path)
	}
	return a, nil
}

// BuildShowFromAst 把脚本的某个标签段变成「只含演出台本」的事件。
// 不解析资产（Assets 留空）——台本只记怎么演，素材由播放器的 Loader 按名字找。
func BuildShowFromAst(ast *Ast, label, scriptPath string) (*ir.Event, error) {
	seg := Segment(ast, label)
	if seg == nil && strings.TrimPrefix(strings.TrimSpace(label), "*") != "" {
		return nil, fmt.Errorf("脚本 %s 里没有标签 %s", filepath.Base(scriptPath), label)
	}
	name := strings.TrimPrefix(strings.TrimSpace(label), "*")
	id := name
	if id == "" {
		id = strings.TrimSuffix(filepath.Base(scriptPath), filepath.Ext(scriptPath))
	}
	kind := ir.KindScene
	if strings.HasPrefix(strings.ToLower(name), "cg") {
		kind = ir.KindStillCG
	}
	b := &irBuilder{}
	return &ir.Event{
		EventID: id,
		Kind:    kind,
		Title:   id,
		Show:    b.buildShow(seg),
	}, nil
}

// BuildShowSegment 读脚本文件并把某个标签段变成事件（GUI/CLI 的便捷入口）。
func BuildShowSegment(scriptPath, label string) (*ir.Event, error) {
	ast, err := ParseScriptFile(scriptPath)
	if err != nil {
		return nil, err
	}
	return BuildShowFromAst(ast, label, scriptPath)
}

// Chapter 一个「演出段落」= 原作自己的章节标记 `*labelNN|显示名` 到同文件下一个章节标记。
//
// 与 Section 的区别（这是长场景切段的关键）：
//   - Section 到「下一个任意标签」就截断，而章节标记之间常夹着普通标签
//     （重返蓝鲸岛实测 150 处），那样会把一章切碎；
//   - 章节标记会重号（分支路线同号异名，实测 9 处），所以按出现序号寻址，
//     不能按名字找。
type Chapter struct {
	Index int    `json:"index"` // `label94` → 94
	Name  string `json:"name"`  // 标签名，如 label94
	Title string `json:"title"` // `|` 之后的显示名，如「余话-聚餐」
	// SeqStart/SeqEnd 是 ast.Seq 的下标区间 [start, end)
	SeqStart int `json:"seq_start"`
	SeqEnd   int `json:"seq_end"`
	// LabelPos 是该章节标记在 ast.Labels 中的下标（区分重号用）
	LabelPos int `json:"label_pos"`
}

// chapterRe 章节标记的标签名形式：`label93`。
var chapterRe = regexp.MustCompile(`^label(\d+)$`)

// Chapters 返回脚本内的演出段落（按出现顺序）。只认带 `|显示名` 的 `labelNN`。
func Chapters(ast *Ast) []Chapter {
	if ast == nil {
		return nil
	}
	var marks []int // ast.Labels 的下标
	for i, lb := range ast.Labels {
		if lb.Title == "" {
			continue
		}
		if !chapterRe.MatchString(lb.Name) {
			continue
		}
		marks = append(marks, i)
	}
	out := make([]Chapter, 0, len(marks))
	for k, li := range marks {
		lb := ast.Labels[li]
		start := lb.SeqPos
		if start < 0 {
			start = 0
		}
		if start > len(ast.Seq) {
			start = len(ast.Seq)
		}
		// 到「下一个章节标记」为止，而不是下一个任意标签
		end := len(ast.Seq)
		if k+1 < len(marks) {
			end = ast.Labels[marks[k+1]].SeqPos
		}
		if end < start {
			end = start
		}
		n, _ := strconv.Atoi(chapterRe.FindStringSubmatch(lb.Name)[1])
		out = append(out, Chapter{
			Index:    n,
			Name:     lb.Name,
			Title:    lb.Title,
			SeqStart: start,
			SeqEnd:   end,
			LabelPos: li,
		})
	}
	return out
}

// SegmentRange 取 ast.Seq 的 [start,end) 区间（章节切段用）。
func SegmentRange(ast *Ast, start, end int) []SeqItem {
	if ast == nil {
		return nil
	}
	if start < 0 {
		start = 0
	}
	if end > len(ast.Seq) {
		end = len(ast.Seq)
	}
	if start > len(ast.Seq) {
		start = len(ast.Seq)
	}
	if end < start {
		end = start
	}
	return ast.Seq[start:end]
}

// BuildShowChapter 把一个演出段落变成事件（长场景不再是一整块）。
// id/title 由调用方给（同一文件内章节会重号，id 里带上出现序号才能唯一）。
func BuildShowChapter(ast *Ast, ch Chapter, scriptPath, id, title string) (*ir.Event, error) {
	if ast == nil {
		return nil, fmt.Errorf("脚本 %s 未解析", filepath.Base(scriptPath))
	}
	seg := SegmentRange(ast, ch.SeqStart, ch.SeqEnd)
	if len(seg) == 0 {
		return nil, fmt.Errorf("脚本 %s 的章节 %s 是空的", filepath.Base(scriptPath), ch.Title)
	}
	if id == "" {
		id = fmt.Sprintf("%s_%s", strings.TrimSuffix(filepath.Base(scriptPath), filepath.Ext(scriptPath)), ch.Name)
	}
	if title == "" {
		title = ch.Title
	}
	b := &irBuilder{}
	return &ir.Event{
		EventID: id,
		Kind:    ir.KindScene,
		Title:   title,
		Show:    b.buildShow(seg),
	}, nil
}

// BuildShowChapterFile 读脚本文件并构建某个演出段落（GUI/CLI 的便捷入口）。
func BuildShowChapterFile(scriptPath string, labelPos int) (*ir.Event, error) {
	ast, err := ParseScriptFile(scriptPath)
	if err != nil {
		return nil, err
	}
	for _, ch := range Chapters(ast) {
		if ch.LabelPos != labelPos {
			continue
		}
		base := strings.TrimSuffix(filepath.Base(scriptPath), filepath.Ext(scriptPath))
		id := fmt.Sprintf("%s_%d_%s", base, ch.LabelPos, ch.Name)
		return BuildShowChapter(ast, ch, scriptPath, id, ch.Title)
	}
	return nil, fmt.Errorf("脚本 %s 里没有位于 %d 的演出段落", filepath.Base(scriptPath), labelPos)
}

// SegmentImages 取标签段里 `[image]/[img]` 依次用到的 storage 名（用于 CG 鉴赏表）。
func SegmentImages(ast *Ast, label string) []string {
	seg := Segment(ast, label)
	out := make([]string, 0, 4)
	seen := map[string]bool{}
	for _, it := range seg {
		if it.Kind != "cmd" || it.Cmd == nil {
			continue
		}
		tag := strings.ToLower(it.Cmd.Tag)
		if tag != "image" && tag != "img" {
			continue
		}
		ref := firstResourceRef(it.Cmd)
		if ref == "" || seen[ref] {
			continue
		}
		seen[ref] = true
		out = append(out, ref)
	}
	return out
}

// LabelIndex 从 `cg017` 这类名字里取数字（取不到返回 0）。
func LabelIndex(name string) int {
	digits := saatail(name)
	if digits == "" {
		return 0
	}
	n, _ := strconv.Atoi(digits)
	return n
}

// saatail 取字符串末尾的连续数字。
func saatail(s string) string {
	i := len(s)
	for i > 0 && s[i-1] >= '0' && s[i-1] <= '9' {
		i--
	}
	return s[i:]
}
