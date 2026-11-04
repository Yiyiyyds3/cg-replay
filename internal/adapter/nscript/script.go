// script.go —— NScripter 脚本词法。
//
// 行分类（ONScripter readScript 同口径）：
//   - `;` 开头 = 注释（07th 系用 `;＜名前` 给上一行台词标注说话人）；
//   - `*` 开头 = 标签；
//   - 半角/全角空格开头，或全角引号「『开头 = 台词行（07th 的对话行
//     直接以「开头，前面没有空格）；
//   - 其余 = 命令行（`命令 参数,…`）。
//
// 台词行里的内联标记（@ 点击等待、\ 换页、!w… 等待、%/$ 变量、#颜色）
// 只影响显示，CG 回放不关心：剥掉后整行存成一条台词。
package nscript

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// LineKind 行种类。
type LineKind int

const (
	LineText  LineKind = iota // 台词行
	LineCmd                   // 命令行
	LineLabel                 // 标签行（*name）
)

// Line 一行脚本。
type Line struct {
	Kind    LineKind
	Cmd     string   // 命令名（小写）
	Args    []string // 参数（引号已剥、别名未解）
	Text    string   // 台词（标记已剥）
	Label   string   // 标签名
	RawNo   int      // 原始行号（1 起）
	Comment string   // 紧随其后的 `;…` 注释体（`;＜南條` → `＜南條`）
}

// Script 解析后的脚本。
type Script struct {
	EntryPath string
	Text      string // UTF-8 全文
	Lines     []Line
	StrAlias  map[string]string // $别名 → 字符串
	NumAlias  map[string]string // %别名 → 字符串（数值表达式，直接存原文）
	strLower  map[string]string // stralias 小写键 → 值（图像命令裸名解析）
}

// ParseScript 把 UTF-8 脚本文本解析成 AST。
func ParseScript(text, entryPath string) (*Script, error) {
	s := &Script{
		EntryPath: entryPath,
		Text:      text,
		StrAlias:  map[string]string{},
		NumAlias:  map[string]string{},
		strLower:  map[string]string{},
	}
	sc := bufio.NewScanner(strings.NewReader(text))
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	sc.Split(bufio.ScanLines)

	lastText := -1 // s.Lines 里最近一条台词的下标（接 ;＜说话人 注释）
	inDefine := false
	for no := 1; sc.Scan(); no++ {
		raw := strings.TrimRight(sc.Text(), "\r")
		if raw == "" {
			continue
		}
		switch {
		case strings.HasPrefix(raw, ";"):
			body := strings.TrimPrefix(raw, ";")
			if !inDefine && strings.HasPrefix(body, "＜") && lastText >= 0 {
				name := strings.TrimSpace(strings.TrimPrefix(body, "＜"))
				s.Lines[lastText].Comment = name
			}
			continue
		case strings.HasPrefix(raw, "*"):
			name := strings.TrimSpace(raw[1:])
			if name == "" {
				continue // 裸 * 是标签定义里的占位
			}
			s.Lines = append(s.Lines, Line{Kind: LineLabel, Label: name, RawNo: no})
			lastText = -1
			inDefine = strings.EqualFold(name, "define")
			continue
		}
		if inDefine {
			// define 块里没有台词：缩进的 dim 等也是定义命令。
			if cmd, args := splitCmd(raw); cmd != "" {
				s.Lines = append(s.Lines, Line{Kind: LineCmd, Cmd: cmd, Args: args, RawNo: no})
				s.noteAlias(cmd, args)
			}
			if strings.EqualFold(strings.TrimSpace(raw), "game") {
				inDefine = false
			}
			continue
		}
		if isTextLine(raw) || !cmdShaped(raw) {
			ln := Line{Kind: LineText, Text: cleanText(raw), RawNo: no}
			if ln.Text != "" {
				s.Lines = append(s.Lines, ln)
				lastText = len(s.Lines) - 1
			}
			continue
		}
		ln := Line{Kind: LineCmd, RawNo: no}
		// 一行可用 `:` 连写多条命令（gosub *X:bg white,10,10:bg sora2,1），
		// 引号内的冒号不算分隔。
		for _, seg := range splitColon(raw) {
			cmd, args := splitCmd(seg)
			if cmd == "" {
				continue
			}
			ln = Line{Kind: LineCmd, RawNo: no}
			ln.Cmd, ln.Args = cmd, args
			s.Lines = append(s.Lines, ln)
			lastText = -1
			s.noteAlias(cmd, args)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("nscript: 扫描 %s 失败: %w", entryPath, err)
	}
	return s, nil
}

// isTextLine 台词行判定：半角/全角空格开头，或全角引号开头。
func isTextLine(raw string) bool {
	switch {
	case strings.HasPrefix(raw, " "):
		return true
	case strings.HasPrefix(raw, "　"): // U+3000
		return true
	case strings.HasPrefix(raw, "「"), strings.HasPrefix(raw, "『"):
		return true
	}
	return false
}

// cmdShaped 判断一行是否像命令行：首字段是纯小写 ASCII 命令形状
// （NScripter 命令名全部是 [a-z][a-z0-9_]*）。台词行即使不以空格开头
// （07th 的「…对话、!s0 重音、@ 等内联标记前缀），首字段也含非 ASCII
// 或以 ! @ \ 开头——靠这个兜底把它们判成台词。
func cmdShaped(raw string) bool {
	fields := splitFields(raw)
	if len(fields) == 0 {
		return false
	}
	f := fields[0]
	if f == "" {
		return false
	}
	for i, r := range f {
		ok := r >= 'a' && r <= 'z' || r >= '0' && r <= '9' && i > 0 || r == '_' && i > 0
		if !ok {
			return false
		}
	}
	return f[0] >= 'a' && f[0] <= 'z'
}

var (
	inlineWaitRe  = regexp.MustCompile(`![wsd]\d*`)
	inlineColorRe = regexp.MustCompile(`#[0-9A-Fa-f]{6}`)
	inlineVarRe   = regexp.MustCompile(`%[A-Za-z0-9_]+|\$[A-Za-z0-9_]+`)
)

// cleanText 剥台词里的内联标记与结构符（@ 点击等待 / \ 换页，可出现在行中）。
func cleanText(raw string) string {
	t := strings.TrimSpace(raw)
	t = strings.TrimPrefix(t, "　")
	t = inlineWaitRe.ReplaceAllString(t, "")
	t = inlineColorRe.ReplaceAllString(t, "")
	t = inlineVarRe.ReplaceAllString(t, "")
	t = strings.ReplaceAll(t, "@", "")
	t = strings.ReplaceAll(t, `\`, "")
	return strings.TrimSpace(t)
}

// splitCmd 拆命令行：首 token 是命令名，其余按逗号/空格分隔，
// 引号内可含逗号与空格。
func splitCmd(raw string) (string, []string) {
	fields := splitFields(raw)
	if len(fields) == 0 {
		return "", nil
	}
	return strings.ToLower(fields[0]), fields[1:]
}

// splitFields 按空白/逗号切分，双引号字符串作为一个 field（剥引号）。
func splitFields(raw string) []string {
	var out []string
	var cur strings.Builder
	inQuote := false
	flush := func() {
		if cur.Len() > 0 {
			out = append(out, strings.TrimSpace(cur.String()))
			cur.Reset()
		}
	}
	for _, r := range raw {
		switch {
		case r == '"':
			inQuote = !inQuote
		case !inQuote && (r == ' ' || r == '\t' || r == ',' || r == '　'):
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return out
}

// noteAlias 收别名定义（stralias / numalias，define 块与正文都可能出现）。
func (s *Script) noteAlias(cmd string, args []string) {
	switch cmd {
	case "stralias":
		if len(args) >= 2 {
			name := strings.TrimPrefix(args[0], "$")
			s.StrAlias[name] = args[1]
			s.strLower[strings.ToLower(name)] = args[1]
		}
	case "numalias":
		if len(args) >= 2 {
			s.NumAlias[strings.TrimPrefix(args[0], "%")] = args[1]
		}
	}
}

// ResolveStr 解一个参数：$别名 → stralias 表；其余原样返回。
func (s *Script) ResolveStr(v string) string {
	if strings.HasPrefix(v, "$") {
		if got, ok := s.StrAlias[strings.TrimPrefix(v, "$")]; ok {
			return got
		}
	}
	return v
}

// ResolveImage 解图像命令参数（07th 系的 bg/ld 大量用不带 $ 的别名）：
//  1. `$别名` → stralias 表；
//  2. 已是路径形态（含分隔符或扩展名）→ 原样；
//  3. 裸名 → stralias 大小写不敏感查一遍（bg sora → stralias Sora）；
//     查不到原样返回（引擎此时按 `<名>.bmp` 找文件）。
func (s *Script) ResolveImage(v string) string {
	if strings.HasPrefix(v, "$") {
		return s.ResolveStr(v)
	}
	if strings.ContainsAny(v, `\/.`) {
		return v
	}
	if got, ok := s.strLower[strings.ToLower(v)]; ok {
		return got
	}
	return v
}

// splitColon 按 `:` 拆命令段（引号内的冒号不拆）。
func splitColon(raw string) []string {
	var out []string
	var cur strings.Builder
	inQuote := false
	for _, r := range raw {
		switch {
		case r == '"':
			inQuote = !inQuote
			cur.WriteRune(r)
		case r == ':' && !inQuote:
			out = append(out, cur.String())
			cur.Reset()
		default:
			cur.WriteRune(r)
		}
	}
	out = append(out, cur.String())
	return out
}

// ParseTextLines 从 io.Reader 直接解析（UTF-8 文本，测试/管道用）。
func ParseTextLines(r io.Reader, entryPath string) (*Script, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return ParseScript(string(data), entryPath)
}
