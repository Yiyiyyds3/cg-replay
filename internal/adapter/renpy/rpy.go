// rpy.go —— 明文 .rpy 剧本的语句提取。
//
// 只认「CG 回放用得到」的语句：label / image 定义 / scene / show / hide /
// 台词（引号行）/ with 转场 / pause。python 块、菜单、if/else 分支结构
// 一律不进台本——回放沿默认路线走，宁缺勿错。
//
// Ren'Py 语法要点（对本解析器有影响的）：
//   - 顶层语句从列 0 开始；label 体内的语句有缩进（解析时只认语法不认层级，
//     缩进的 scene/show 大多是 if 分支里的变体，收进来作备用画面）。
//   - `image 名字 = "路径"` 定义命名图像，名字可含空格（多词）。
//   - `scene`/`show` 后面是图像名 + 可选 as/at/with 子句。
//   - 台词是裸字符串或「角色名 属性 "台词"」。
package renpy

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Stmt 一条回放相关语句。
type Stmt struct {
	Kind string // scene / show / hide / say / with / pause
	Name string // scene/show/hide 的图像名（原样大小写）；say 的说话人
	Text string // say 的台词
	Line int    // 在脚本文件里的行号（1 起，诊断用）
}

// Script 一个 .rpy 文件的提取结果。
type Script struct {
	Path string // 相对 gameDir 的斜杠路径
	// Labels 按出现顺序的 label 名。
	Labels []string
	// Body label 名 → 语句序列（label 之前的全局语句挂在 "" 下）。
	Body map[string][]Stmt
	// ImageDefs `image 名 = "路径"` 定义：小写空格名 → 归档相对路径。
	ImageDefs map[string]string
	Warnings  []string
}

var (
	reLabel   = regexp.MustCompile(`^label\s+([A-Za-z_][\w.]*)\s*(\([^)]*\))?\s*:`)
	reImage   = regexp.MustCompile(`^image\s+(.+?)\s*=\s*"(.*)"\s*$`)
	reScene   = regexp.MustCompile(`^(scene|show|hide)\s+(.+)$`)
	reSay     = regexp.MustCompile(`^(?:([A-Za-z_][\w ]*?)\s+)?"((?:[^"\\]|\\.)*)"\s*$`)
	reWith    = regexp.MustCompile(`^with\s+(.+)$`)
	rePause   = regexp.MustCompile(`^pause\b`)
	reTextTag = regexp.MustCompile(`\{[^}]*\}`)
	reDefine  = regexp.MustCompile(`^(?:define|default)\s+\w+`) // 变量定义，跳过
)

// ParseScriptFile 读一个 .rpy 文件并提取语句。
func ParseScriptFile(path string) (*Script, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parseScript(bufio.NewScanner(f)), nil
}

func parseScript(sc *bufio.Scanner) *Script {
	s := &Script{Body: map[string][]Stmt{}, ImageDefs: map[string]string{}}
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	cur := "" // 当前 label（"" = 全局区）
	line := 0
	for sc.Scan() {
		line++
		raw := strings.TrimPrefix(sc.Text(), "\uFEFF") // 每行容忍 BOM（拼接文件的常见病）
		line2 := strings.TrimRight(raw, "\r")
		text := strings.TrimSpace(line2)
		if text == "" {
			continue
		}
		indent := len(line2) - len(strings.TrimLeft(line2, " \t"))

		if m := reLabel.FindStringSubmatch(text); m != nil && indent == 0 {
			cur = m[1]
			s.Labels = append(s.Labels, cur)
			continue
		}
		if reDefine.MatchString(text) || text == "return" || text == "init python:" ||
			strings.HasPrefix(text, "python:") || strings.HasPrefix(text, "$ ") {
			continue
		}
		if m := reImage.FindStringSubmatch(text); m != nil && indent == 0 {
			name := strings.ToLower(strings.Join(strings.Fields(m[1]), " "))
			if name != "" {
				s.ImageDefs[name] = filepath.ToSlash(m[2])
			}
			continue
		}
		if m := reScene.FindStringSubmatch(text); m != nil {
			name, ok := imageExprName(m[2])
			if !ok {
				s.Warnings = append(s.Warnings, fmt.Sprintf("第 %d 行 %s 后面是表达式，暂不支持：%s",
					line, m[1], truncateStr(m[2], 60)))
				continue
			}
			s.Body[cur] = append(s.Body[cur], Stmt{Kind: m[1], Name: name, Line: line})
			continue
		}
		if m := reWith.FindStringSubmatch(text); m != nil {
			s.Body[cur] = append(s.Body[cur], Stmt{Kind: "with", Name: firstToken(m[1]), Line: line})
			continue
		}
		if rePause.MatchString(text) {
			s.Body[cur] = append(s.Body[cur], Stmt{Kind: "pause", Line: line})
			continue
		}
		// 台词：`"旁白"` 或 `角色 属性... "台词"`（缩进无所谓，但要在 label 体内或全局）。
		if strings.HasPrefix(text, `"`) || reSay.MatchString(text) {
			if m := reSay.FindStringSubmatch(text); m != nil {
				st := Stmt{Kind: "say", Name: strings.TrimSpace(m[1]), Text: unescapeRenPy(m[2]), Line: line}
				s.Body[cur] = append(s.Body[cur], st)
			}
			continue
		}
	}
	return s
}

// stripComment 去掉行尾注释（引号外的 #）。
func stripComment(line string) string {
	inQuote := false
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case '"':
			if i == 0 || line[i-1] != '\\' {
				inQuote = !inQuote
			}
		case '#':
			if !inQuote {
				return strings.TrimRight(line[:i], " \t")
			}
		}
	}
	return line
}

// imageExprName 从 scene/show/hide 的参数里抠图像名：
// 去掉 as/at/with 子句，表达式（带括号/加号）不支持。
func imageExprName(rest string) (string, bool) {
	// ToLower 对个别字符（如 İ）会变长，直接拿它的下标切 rest 会越界，
	// 所以只在 lower 上找位置，切割时守住 rest 的长度。
	cut := len(rest)
	lower := strings.ToLower(rest)
	for _, kw := range []string{" with ", " as ", " at ", " behind ", " onlayer ", " zorder "} {
		if i := strings.Index(lower, kw); i >= 0 && i <= len(rest) && i < cut {
			cut = i
		}
	}
	rest = strings.TrimSpace(rest[:cut])
	if rest == "" {
		return "", false
	}
	for _, ch := range rest {
		if ch == '(' || ch == '+' || ch == '[' {
			return "", false
		}
	}
	return rest, true
}

func firstToken(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, " \t"); i >= 0 {
		return s[:i]
	}
	return s
}

// unescapeRenPy 还原台词里的转义（\" \\ \n）并剥掉 Ren'Py 文本标签
// （{color=#…}{/color}{nw} 等——回放器不排版富文本）。`{{` 是字面大括
// 号的转义，先占位再还原。
func unescapeRenPy(s string) string {
	s = strings.ReplaceAll(s, "{{", "\x00")
	if strings.Contains(s, "}") {
		s = reTextTag.ReplaceAllString(s, "")
	}
	s = strings.ReplaceAll(s, "\x00", "{")
	if !strings.Contains(s, "\\") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case '"', '\\':
				b.WriteByte(s[i+1])
				i++
				continue
			case 'n', 'N':
				b.WriteByte(' ')
				i++
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func truncateStr(s string, n int) string {
	rs := []rune(s)
	if len(rs) <= n {
		return s
	}
	return string(rs[:n]) + "…"
}
