// ast.go：.ast（Artemis 脚本）的结构化表示与引用提取。
//
// .ast 是明文 Lua 表：顶层 astver / astname / ast；ast 的每个命名字段（除 label）
// 是一个「块」，块内有序列表里位置参数为表的项是命令，text= 字段装对白。
package artemis

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Command 是一条 .ast 命令：{"tag", k=v, k2=v2, ...}。
//
// Tag   命令标签（原样保留大小写，匹配时统一小写）
// Attrs 命名参数（k=v；后出现的同名键覆盖先出现的）
// Args  标签之后的位置参数字符串
// Raw   原始形态的紧凑描述（写进 IR 的 unrecognized.raw）
type Command struct {
	Tag   string
	Attrs map[string]string
	Args  []string
	Raw   string
}

// SeqItem 是块内顺序的一项：命令或一句对白。
type SeqItem struct {
	Kind    string // "cmd" | "text"
	Cmd     *Command
	Text    string
	Speaker string
	Voice   string // text.vo 里的语音素材名（无扩展名）
}

// Block 是一个演出块（block_NNNNN / ["0001_00012"]）。
type Block struct {
	Name     string
	Seq      []SeqItem
	LinkBack string
	LinkNext string
	Line     int    // 块级 line 字段（樱之刻有）
	Title    string // savetitle text=（章节名）
}

// Ast 是单个 .ast 脚本的解析结果。
type Ast struct {
	Name     string
	Encoding string
	Blocks   []*Block
	ByKey    map[string]*Block // 块名 → 块（供 label/link 查表）
	Labels   map[string]int    // label 名 → 块下标
	Warnings []string
}

// Block 按名字取块。
func (a *Ast) Block(name string) *Block {
	if a == nil {
		return nil
	}
	return a.ByKey[name]
}

// TextLines 返回全部对白行（跨块、按块序）。
func (a *Ast) TextLines() []SeqItem {
	var out []SeqItem
	for _, b := range a.Blocks {
		for _, it := range b.Seq {
			if it.Kind == "text" {
				out = append(out, it)
			}
		}
	}
	return out
}

// ---------------- 解析：luaValue → Ast ----------------

// aliasRe 匹配 Artemis 的路径别名前缀（:bg/、:cg/、:fg/xx/z1/、:ani/、:sys/…）。
// 个别命令会把别名写进 file（file=":ani/kiss"），不剥会整批漏匹配。
var aliasRe = regexp.MustCompile(`^:[A-Za-z0-9_/]*/`)

// numRe 用于剔除纯数字操作数（坐标、槽位号等）。
var numRe = regexp.MustCompile(`^-?\d+$`)

func buildAst(chunk *luaValue, name string) (*Ast, error) {
	astVal, ok := chunk.Field("ast")
	if !ok || !astVal.IsTable() {
		return nil, fmt.Errorf("缺少 ast 表")
	}
	a := &Ast{
		Name:     name,
		Encoding: "utf-8",
		ByKey:    map[string]*Block{},
		Labels:   map[string]int{},
	}
	if v, ok := chunk.Field("astver"); ok {
		a.Warnings = append(a.Warnings, "astver="+v.Text())
	}
	for _, f := range astVal.Fields {
		if f.Key == "" {
			continue // ast 表里不该有位置项
		}
		if f.Key == "label" {
			a.parseLabels(f.Val)
			continue
		}
		if !f.Val.IsTable() {
			continue
		}
		b := &Block{Name: f.Key}
		a.parseBlock(b, f.Val)
		a.ByKey[b.Name] = b
		a.Blocks = append(a.Blocks, b)
	}
	if len(a.Blocks) == 0 {
		return nil, fmt.Errorf("ast 表里没有任何块")
	}
	return a, nil
}

func (a *Ast) parseLabels(v luaValue) {
	for _, f := range v.Fields {
		if !f.Val.IsTable() {
			continue
		}
		block, _ := f.Val.Field("block")
		if block.Kind != luaString {
			continue
		}
		idx := len(a.Blocks) // 默认指向末尾（未知块）
		if b := a.ByKey[block.Str]; b != nil {
			idx = indexOfBlock(a.Blocks, b)
		}
		a.Labels[f.Key] = idx
	}
}

func indexOfBlock(bs []*Block, b *Block) int {
	for i, x := range bs {
		if x == b {
			return i
		}
	}
	return -1
}

func (a *Ast) parseBlock(b *Block, v luaValue) {
	for _, f := range v.Fields {
		switch {
		case f.Key == "text":
			a.attachText(b, f.Val)
		case f.Key == "linkback":
			b.LinkBack = f.Val.Text()
		case f.Key == "linknext":
			b.LinkNext = f.Val.Text()
		case f.Key == "line":
			if f.Val.Kind == luaNumber {
				b.Line = int(f.Val.Num)
			}
		case f.Key != "":
			// 其他命名字段（脚本私有）忽略，但记一条告警便于排查
			if !f.Val.IsTable() {
				a.Warnings = append(a.Warnings, fmt.Sprintf("块 %s 字段 %s 未处理", b.Name, f.Key))
			}
		default:
			// 位置项：应为命令表
			if !f.Val.IsTable() {
				continue
			}
			c := parseCommand(f.Val)
			if c == nil {
				continue
			}
			if strings.EqualFold(c.Tag, "savetitle") {
				if t := c.Attrs["text"]; t != "" {
					b.Title = t
				}
			}
			b.Seq = append(b.Seq, SeqItem{Kind: "cmd", Cmd: c})
		}
	}
}

// parseCommand 把 {"tag", k=v, …} 表变成 Command。首个位置字符串为 tag。
func parseCommand(v luaValue) *Command {
	c := &Command{Attrs: map[string]string{}}
	for _, f := range v.Fields {
		if f.Key == "" {
			switch f.Val.Kind {
			case luaString:
				if c.Tag == "" {
					c.Tag = f.Val.Str
				} else {
					c.Args = append(c.Args, f.Val.Str)
				}
			case luaNumber:
				c.Args = append(c.Args, formatLuaNumber(f.Val.Num))
			}
			continue
		}
		c.Attrs[f.Key] = f.Val.Text()
	}
	if c.Tag == "" {
		return nil
	}
	if len(c.Args) > 0 {
		c.Raw = c.Tag + " " + strings.Join(c.Args, " ")
	} else {
		c.Raw = c.Tag
	}
	for k, val := range c.Attrs {
		c.Raw += " " + k + "=" + val
	}
	return c
}

// attachText 解析块的 text = { vo={…}, ja={…}, cn={…} }。
// ja 优先，其次 cn；每条记录 → 一行对白；vo 与记录按序号配对。
func (a *Ast) attachText(b *Block, v luaValue) {
	if !v.IsTable() {
		return
	}
	var lines luaValue
	loc := ""
	if ja, ok := v.Field("ja"); ok && ja.IsTable() && len(ja.Fields) > 0 {
		lines, loc = ja, "ja"
	} else if cn, ok := v.Field("cn"); ok && cn.IsTable() && len(cn.Fields) > 0 {
		lines, loc = cn, "cn"
	}
	voices := voiceList(v)
	if loc == "" || len(lines.Fields) == 0 {
		return
	}
	for i, f := range lines.Fields {
		if !f.Val.IsTable() {
			continue
		}
		it := SeqItem{Kind: "text"}
		it.Speaker, it.Text = parseTextRecord(f.Val)
		if i < len(voices) {
			it.Voice = voices[i]
		}
		if it.Text == "" && it.Speaker == "" {
			continue
		}
		b.Seq = append(b.Seq, it)
	}
}

// voiceList 取 text.vo 里的语音素材名（按序）。
func voiceList(v luaValue) []string {
	vo, ok := v.Field("vo")
	if !ok || !vo.IsTable() {
		return nil
	}
	var out []string
	for _, f := range vo.Fields {
		if !f.Val.IsTable() {
			continue
		}
		for _, g := range f.Val.Fields {
			if g.Key == "file" && g.Val.Kind == luaString {
				out = append(out, cleanRef(g.Val.Str))
			}
		}
	}
	return out
}

// parseTextRecord 解析一条对白记录：
//
//	{ name={"静流","しずる"}, "「せりふ」", {"rt2"}, "続き", }
//
// 位置字符串按序拼接，{"rt2"} 记换行，{"txruby"…}/{"txruby"} 去掉（保留基字）。
func parseTextRecord(v luaValue) (speaker, text string) {
	var sb strings.Builder
	for _, f := range v.Fields {
		if f.Key == "name" {
			speaker = firstName(f.Val)
			continue
		}
		if f.Key != "" {
			continue
		}
		switch f.Val.Kind {
		case luaString:
			sb.WriteString(f.Val.Str)
		case luaNumber:
			sb.WriteString(formatLuaNumber(f.Val.Num))
		case luaTable:
			if tag := firstString(f.Val); strings.EqualFold(tag, "rt2") {
				sb.WriteString("\n")
			}
			// txruby 与其它内联标记：跳过
		}
	}
	text = strings.TrimRight(sb.String(), "\n")
	return speaker, text
}

// firstName 取 name={"…"} 里的名字（第一个位置字符串；{fn} 这类占位符原样保留）。
func firstName(v luaValue) string {
	if v.Kind == luaString {
		return v.Str
	}
	if !v.IsTable() {
		return ""
	}
	for _, f := range v.Fields {
		if f.Key == "" && f.Val.Kind == luaString {
			return f.Val.Str
		}
	}
	return ""
}

func firstString(v luaValue) string {
	for _, f := range v.Fields {
		if f.Key == "" && f.Val.Kind == luaString {
			return f.Val.Str
		}
	}
	return ""
}

// cleanRef 剥掉 :alias/ 前缀并去空白。
func cleanRef(s string) string {
	s = strings.TrimSpace(s)
	s = aliasRe.ReplaceAllString(s, "")
	return s
}

// ---------------- 引用提取（鸭子类型：cmd/ucgr 的 astRefs 调用） ----------------

// refAttrs：资源类标签 → 取哪些属性作为引用。
var refAttrs = map[string][]string{
	"vo":        {"file"},
	"bg":        {"file"},
	"fg":        {"file"},
	"fgact":     {"file"},
	"fgf":       {"frame", "bg", "cg"},
	"se":        {"file"},
	"bgm":       {"file"},
	"evset":     {"file"},
	"cg":        {"file"},
	"cgact":     {"file"},
	"cgdel":     {"file"},
	"movie":     {"file"},
	"scene":     {"file"},
	"selback":   {"file"},
	"sysvo":     {"file"},
	"excall":    {"file"},
	"select":    {"file"},
	"brandlogo": {"file"},
	"txkey":     {"file"},
	"gaiji":     {"file"},
}

// refFallbackAttrs：未登记标签（保守保留口径）也要扫的通用属性名。
// 见 docs/04 §6：不认识的命令里「像包内路径」的操作数一律进引用集。
var refFallbackAttrs = []string{"file", "cg", "bg", "frame", "set", "src", "storage"}

// ExtractRefs 返回脚本里出现的全部资源名（已剥别名前缀、去重、保持出现顺序）。
func (a *Ast) ExtractRefs() []string {
	if a == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	add := func(v string) {
		v = cleanRef(v)
		if v == "" || seen[v] {
			return
		}
		seen[v] = true
		out = append(out, v)
	}
	for _, b := range a.Blocks {
		for _, it := range b.Seq {
			if it.Kind == "text" {
				if it.Voice != "" {
					add(it.Voice)
				}
				continue
			}
			if it.Cmd == nil {
				continue
			}
			tag := strings.ToLower(it.Cmd.Tag)
			keys, ok := refAttrs[tag]
			if !ok {
				keys = refFallbackAttrs
			}
			for _, k := range keys {
				for _, tok := range splitRefValue(it.Cmd.Attrs[k]) {
					add(tok)
				}
			}
		}
	}
	return out
}

// splitRefValue 把属性值拆成候选引用：整串 + 空白分隔的 token；纯数字剔除。
func splitRefValue(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	fields := strings.Fields(s)
	if len(fields) <= 1 {
		if numRe.MatchString(s) {
			return nil
		}
		return []string{s}
	}
	var out []string
	for _, f := range fields {
		if numRe.MatchString(f) {
			continue
		}
		out = append(out, f)
	}
	return out
}

// SortedRefs 是 ExtractRefs 的排序版（调试/报告用）。
func SortedRefs(ast *Ast) []string {
	refs := ast.ExtractRefs()
	sort.Strings(refs)
	return refs
}
