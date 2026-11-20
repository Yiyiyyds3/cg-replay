// 脚本图片名静态提取：把 code 355/655 里的「图片函数 + 字符串实参」展开成
// 等价的事件指令，让切段（Segments）与渲染（BuildShow）走同一条路径。
//
// 动机：有些游戏完全不写「显示图片」（code 231），图片名直接写在脚本调用里：
//
//	$gameScreen.showPicture(10, "CG07_01", 0, 0, 0, 100, 100, 255, 0);
//	this.picsChange2('立绘_02');
//	this.loadPicture('CG/03');
//
// 这些名字是字面量，静态可读。真正读不出来的是 $portraitManager.init() 这种
// 由插件在运行时拼名字的调用——那时才该保留「需插件级解析器」的诊断。
package rmmz

import (
	"regexp"
	"strconv"
	"strings"
)

// scriptPicCallRe 抓「函数名(实参)」。实参里不再嵌套括号：嵌套的多半是
// 表达式，不会是纯字面量图片名。
var scriptPicCallRe = regexp.MustCompile(`([A-Za-z_$][A-Za-z0-9_$]*)\s*\(([^()]*)\)`)

// scriptPicFuncRe 函数名里带这些词才算图片相关：showPicture / loadPicture /
// picsChange2 / setImage …。cgLoad（整组加载差分套图）另有 scriptCGName
// 处理，这里不碰，免得同一套图被算两次。
var scriptPicFuncRe = regexp.MustCompile(`(?i)picture|pics|pict|image`)

// scriptPicLitRe 取实参里的字符串字面量（单双引号都认）。
var scriptPicLitRe = regexp.MustCompile(`'([^']*)'|"([^"]*)"`)

// scriptNumRe 单个数字实参。
var scriptNumRe = regexp.MustCompile(`^\s*(-?\d+(?:\.\d+)?)\s*$`)

// looksLikePictureName 判断字面量「像不像图片名」。
//
// 认：字母、数字、下划线、连字符、空格、斜杠（pictures/ 子目录）、点（扩展名）、
// 日文/中文等宽字符。不认：$ 开头的变量、#{...} 本地化占位、带运算符或括号的
// 表达式片段（'stands/' + raw 这种拼接只剩 'stands/'，末尾斜杠会被挡掉）。
func looksLikePictureName(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > 120 {
		return false
	}
	if strings.HasPrefix(s, "$") || strings.HasSuffix(s, "/") || strings.HasSuffix(s, "\\") {
		return false
	}
	hasWord := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			hasWord = true
		case r == '_' || r == '-' || r == ' ' || r == '/' || r == '.' || r >= 0x2E80:
			// 允许：分隔符、扩展名点、日文/中文
		default:
			return false
		}
	}
	return hasWord
}

// splitCallArgs 按顶层逗号切实参（引号里的逗号、[a,b] 里的逗号不切）。
func splitCallArgs(s string) []string {
	var out []string
	var quote rune
	depth, start := 0, 0
	for i, r := range s {
		if quote != 0 {
			if r == quote {
				quote = 0
			}
			continue
		}
		switch r {
		case '\'', '"':
			quote = r
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				out = append(out, s[start:i])
				start = i + 1
			}
		}
	}
	out = append(out, s[start:])
	for i := range out {
		out[i] = strings.TrimSpace(out[i])
	}
	return out
}

// argNum 取第 i 个实参的数字值。
func argNum(args []string, i int) (float64, bool) {
	if i < 0 || i >= len(args) {
		return 0, false
	}
	m := scriptNumRe.FindStringSubmatch(args[i])
	if m == nil {
		return 0, false
	}
	v, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// ScriptPictureNames 列出脚本指令里的图片名（同一条指令内按出现顺序）。
func ScriptPictureNames(c Command) []string {
	shows := scriptPictureShows(c)
	if len(shows) == 0 {
		return nil
	}
	out := make([]string, 0, len(shows))
	for _, s := range shows {
		out = append(out, s.str(1))
	}
	return out
}

// scriptPictureShows 把一条脚本指令里的图片调用展开成等价的「显示图片」指令。
//
// 参数按 RPG Maker 的 showPicture(id, name, origin, x, y, scaleX, scaleY,
// opacity, blend) 位置读取；插件自定义函数（picsChange2('名字') 之类）只取
// 名字，位置/缩放/透明度沿用引擎默认值——宁可摆得朴素，也不要乱摆。
func scriptPictureShows(c Command) []Command {
	if c.Code != 355 && c.Code != 655 {
		return nil
	}
	src := c.str(0)
	if src == "" {
		return nil
	}
	var out []Command
	for _, m := range scriptPicCallRe.FindAllStringSubmatch(src, -1) {
		fn, rawArgs := m[1], m[2]
		if !scriptPicFuncRe.MatchString(fn) {
			continue
		}
		args := splitCallArgs(rawArgs)
		name := ""
		for _, a := range args {
			lm := scriptPicLitRe.FindStringSubmatch(a)
			if lm == nil {
				continue
			}
			lit := lm[1]
			if lm[2] != "" {
				lit = lm[2]
			}
			if looksLikePictureName(lit) {
				name = strings.TrimSpace(lit)
				break
			}
		}
		if name == "" {
			continue
		}
		id := 0
		if v, ok := argNum(args, 0); ok && v >= 1 && v <= 999 {
			id = int(v)
		}
		origin, x, y, sx, sy, opacity := 0.0, 0.0, 0.0, 100.0, 100.0, 255.0
		if len(args) >= 8 {
			if v, ok := argNum(args, 2); ok {
				origin = v
			}
			if v, ok := argNum(args, 3); ok {
				x = v
			}
			if v, ok := argNum(args, 4); ok {
				y = v
			}
			if v, ok := argNum(args, 5); ok && v > 0 {
				sx = v
			}
			if v, ok := argNum(args, 6); ok && v > 0 {
				sy = v
			}
			if v, ok := argNum(args, 7); ok {
				opacity = v
			}
		}
		out = append(out, Command{Code: 231, Indent: c.Indent, Params: []any{
			float64(id), name, origin, 0.0, x, y, sx, sy, opacity, 0.0,
		}})
	}
	return out
}

// expandScriptPictures 把整段指令流里的脚本图片调用展开成 231 指令。
//
// 编号：调用里第一个整数实参就是图片编号（showPicture 的第一个参数），拿不到
// 时从 900 起顺延，避开 1..99 的常规占用；层名仍是 layer<编号>，与真实 231
// 生成的演出事件结构完全一致。
func expandScriptPictures(cmds []Command) []Command {
	next := 900
	for _, c := range cmds {
		if c.Code == 231 {
			if id := int(c.num(0)); id >= next {
				next = id + 1
			}
		}
	}
	out := make([]Command, 0, len(cmds))
	for _, c := range cmds {
		shots := scriptPictureShows(c)
		if len(shots) == 0 {
			out = append(out, c)
			continue
		}
		for _, s := range shots {
			if int(s.num(0)) <= 0 {
				s.Params[0] = float64(next)
				next++
			}
			out = append(out, s)
		}
	}
	return out
}
