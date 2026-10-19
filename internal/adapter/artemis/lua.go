package artemis

import (
	"fmt"
	"strconv"
	"strings"
)

// lua.go：宽容的 Lua 风格表解析器（零第三方依赖）。
//
// Artemis 的剧本 .ast 是明文 Lua 表（P2 报告已确认「无需解密」），但写法比
// 标准 Lua 表更杂：
//
//	astver = 2.0
//	ast = {
//	    block_00000 = {                       -- 键可以是裸标识符
//	        {"bg", time=2000, file="black"},  -- 位置字段 + k=v 混排
//	        {"text"},
//	        text = { vo = { {"vo", file="x"} }, ja = { {"…"}, {"rt2"} } },
//	    },
//	    ["0001_00012"] = { … },               -- 键也可以是 ["字符串"]
//	}
//
// 因此这里只做「宽容解析」：能读出结构就够，语法细节不追求与 Lua 完全一致。

type luaKind uint8

const (
	luaNil luaKind = iota
	luaBool
	luaNumber
	luaString
	luaTable
)

// luaField 是表里的一个字段：Key 为空表示位置字段（positional）。
type luaField struct {
	Key string
	Val luaValue
}

type luaValue struct {
	Kind   luaKind
	Str    string
	Num    float64
	Bool   bool
	Fields []luaField
}

func (v luaValue) IsTable() bool { return v.Kind == luaTable }

// Field 返回第一个同名（大小写敏感）命名字段。
func (v luaValue) Field(key string) (luaValue, bool) {
	for _, f := range v.Fields {
		if f.Key == key {
			return f.Val, true
		}
	}
	return luaValue{}, false
}

// StrField 返回命名字段的字符串值（数字也转字符串），无则返回 ""。
func (v luaValue) StrField(key string) string {
	f, ok := v.Field(key)
	if !ok {
		return ""
	}
	return f.Text()
}

// Positional 返回全部位置字段（保持出现顺序）。
func (v luaValue) Positional() []luaValue {
	out := make([]luaValue, 0, len(v.Fields))
	for _, f := range v.Fields {
		if f.Key == "" {
			out = append(out, f.Val)
		}
	}
	return out
}

// Text 把值取成字符串：字符串原样、数字用紧凑写法、布尔转 true/false，其余空串。
func (v luaValue) Text() string {
	switch v.Kind {
	case luaString:
		return v.Str
	case luaNumber:
		return formatLuaNumber(v.Num)
	case luaBool:
		if v.Bool {
			return "true"
		}
		return "false"
	}
	return ""
}

func formatLuaNumber(f float64) string {
	if f == float64(int64(f)) {
		return strconv.FormatInt(int64(f), 10)
	}
	return strconv.FormatFloat(f, 'g', -1, 64)
}

type luaParser struct {
	src  []byte
	pos  int
	line int
}

// parseLuaChunk 解析整份 .ast 顶层：返回一个「表」，其字段即 astver / astname / ast。
func parseLuaChunk(src []byte) (*luaValue, error) {
	// 去 UTF-8 BOM
	if len(src) >= 3 && src[0] == 0xEF && src[1] == 0xBB && src[2] == 0xBF {
		src = src[3:]
	}
	p := &luaParser{src: src, line: 1}
	top := &luaValue{Kind: luaTable}
	for {
		p.skipSpace()
		if p.eof() {
			break
		}
		key, err := p.parseKey()
		if err != nil {
			return nil, err
		}
		p.skipSpace()
		if p.eof() || p.src[p.pos] != '=' {
			return nil, p.errf("顶层赋值缺少 '='（键 %q）", key)
		}
		p.pos++
		val, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		top.Fields = append(top.Fields, luaField{Key: key, Val: val})
		// 顶层允许 `;` 或 `,` 分隔（Lua 两种都合法）
		p.skipSpace()
		if !p.eof() && (p.src[p.pos] == ';' || p.src[p.pos] == ',') {
			p.pos++
		}
	}
	return top, nil
}

func (p *luaParser) eof() bool { return p.pos >= len(p.src) }

func (p *luaParser) errf(format string, args ...interface{}) error {
	return fmt.Errorf("第 %d 行：%s", p.line, fmt.Sprintf(format, args...))
}

// skipSpace 跳过空白与注释（-- 行注释、--[[ ]] 块注释）。
func (p *luaParser) skipSpace() {
	for !p.eof() {
		c := p.src[p.pos]
		switch {
		case c == '\n':
			p.line++
			p.pos++
		case c == ' ' || c == '\t' || c == '\r' || c == '\v' || c == '\f':
			p.pos++
		case c == '-' && p.pos+1 < len(p.src) && p.src[p.pos+1] == '-':
			// 块注释 --[[ … ]] 或 --[=[ … ]=]
			if rest, ok := trimLongBracketOpen(p.src[p.pos+2:]); ok {
				if end := strings.Index(string(rest), "]"); end >= 0 {
					consumed := len(p.src) - p.pos - 2 - len(rest) + end + 1
					for i := 0; i < consumed && !p.eof(); i++ {
						if p.src[p.pos] == '\n' {
							p.line++
						}
						p.pos++
					}
					continue
				}
			}
			// 行注释
			for !p.eof() && p.src[p.pos] != '\n' {
				p.pos++
			}
		default:
			return
		}
	}
}

func trimLongBracketOpen(b []byte) ([]byte, bool) {
	if len(b) == 0 || b[0] != '[' {
		return nil, false
	}
	i := 1
	for i < len(b) && b[i] == '=' {
		i++
	}
	if i < len(b) && b[i] == '[' {
		return b[i+1:], true
	}
	return nil, false
}

// parseLongString 解析 Lua 长字符串 `[[…]]` / `[=[…]=]`：
// たねつみの歌 的译文（en/cn 段）用它书写，形如 `[["I'm sorry…"]],`。
func (p *luaParser) parseLongString() (string, error) {
	start := p.pos
	eqs := 0
	i := start + 1
	for i < len(p.src) && p.src[i] == '=' {
		eqs++
		i++
	}
	if i >= len(p.src) || p.src[i] != '[' {
		return "", p.errf("无法识别的值（首字节 0x%02x）", p.src[start])
	}
	body := p.src[i+1:]
	// Lua 长字符串会吞掉紧跟开括号的第一个换行
	if len(body) > 0 && body[0] == '\n' {
		body = body[1:]
		p.line++
	}
	close := "]" + strings.Repeat("=", eqs) + "]"
	idx := strings.Index(string(body), close)
	if idx < 0 {
		return "", p.errf("长字符串缺少收尾 %s", close)
	}
	text := string(body[:idx])
	p.line += strings.Count(text, "\n")
	p.pos = i + 1 + idx + len(close)
	return text, nil
}

func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c >= 0x80
}

func isIdentChar(c byte) bool {
	return isIdentStart(c) || (c >= '0' && c <= '9')
}

// parseKey 读一个字段键：`ident` 或 `["str"]` / `[123]`。
func (p *luaParser) parseKey() (string, error) {
	if p.eof() {
		return "", p.errf("期望键，已到文件末尾")
	}
	if p.src[p.pos] == '[' {
		p.pos++
		p.skipSpace()
		if p.eof() {
			return "", p.errf("'[' 后缺少键")
		}
		if c := p.src[p.pos]; c == '"' || c == '\'' {
			s, err := p.parseString()
			if err != nil {
				return "", err
			}
			p.skipSpace()
			if p.eof() || p.src[p.pos] != ']' {
				return "", p.errf("字符串键缺少 ']'")
			}
			p.pos++
			return s, nil
		}
		// 数字键
		n, err := p.parseNumber()
		if err != nil {
			return "", err
		}
		p.skipSpace()
		if p.eof() || p.src[p.pos] != ']' {
			return "", p.errf("数字键缺少 ']'")
		}
		p.pos++
		return formatLuaNumber(n), nil
	}
	var b strings.Builder
	for !p.eof() && isIdentChar(p.src[p.pos]) {
		b.WriteByte(p.src[p.pos])
		p.pos++
	}
	if b.Len() == 0 {
		return "", p.errf("无法识别的键（首字节 0x%02x）", p.src[p.pos])
	}
	return b.String(), nil
}

func (p *luaParser) parseValue() (luaValue, error) {
	p.skipSpace()
	if p.eof() {
		return luaValue{}, p.errf("期望值，已到文件末尾")
	}
	c := p.src[p.pos]
	switch {
	case c == '{':
		return p.parseTable()
	case c == '[':
		s, err := p.parseLongString()
		if err != nil {
			return luaValue{}, err
		}
		return luaValue{Kind: luaString, Str: s}, nil
	case c == '"' || c == '\'':
		s, err := p.parseString()
		if err != nil {
			return luaValue{}, err
		}
		return luaValue{Kind: luaString, Str: s}, nil
	case c == '-' || c == '+' || c == '.' || (c >= '0' && c <= '9'):
		n, err := p.parseNumber()
		if err != nil {
			return luaValue{}, err
		}
		return luaValue{Kind: luaNumber, Num: n}, nil
	case isIdentStart(c):
		ident := p.parseIdent()
		switch ident {
		case "true":
			return luaValue{Kind: luaBool, Bool: true}, nil
		case "false":
			return luaValue{Kind: luaBool, Bool: false}, nil
		case "nil":
			return luaValue{Kind: luaNil}, nil
		}
		// 其它裸标识符（例如值位置的 enum 名）当字符串处理，宽容优先
		return luaValue{Kind: luaString, Str: ident}, nil
	}
	return luaValue{}, p.errf("无法识别的值（首字节 0x%02x）", c)
}

func (p *luaParser) parseIdent() string {
	var b strings.Builder
	for !p.eof() && isIdentChar(p.src[p.pos]) {
		b.WriteByte(p.src[p.pos])
		p.pos++
	}
	return b.String()
}

func (p *luaParser) parseNumber() (float64, error) {
	start := p.pos
	if !p.eof() && (p.src[p.pos] == '-' || p.src[p.pos] == '+') {
		p.pos++
	}
	for !p.eof() {
		c := p.src[p.pos]
		if (c >= '0' && c <= '9') || c == '.' || c == 'e' || c == 'E' ||
			((c == '-' || c == '+') && p.pos > start && (p.src[p.pos-1] == 'e' || p.src[p.pos-1] == 'E')) {
			p.pos++
			continue
		}
		break
	}
	tok := string(p.src[start:p.pos])
	if tok == "" {
		return 0, p.errf("期望数字，得到 0x%02x", p.src[p.pos])
	}
	f, err := strconv.ParseFloat(tok, 64)
	if err != nil {
		return 0, p.errf("数字 %q 解析失败：%v", tok, err)
	}
	return f, nil
}

// parseString 读字符串字面量。字符串内允许裸换行（样本里确实存在）。
func (p *luaParser) parseString() (string, error) {
	quote := p.src[p.pos]
	p.pos++
	var b strings.Builder
	for {
		if p.eof() {
			return "", p.errf("字符串缺少收尾引号")
		}
		c := p.src[p.pos]
		if c == quote {
			p.pos++
			return b.String(), nil
		}
		if c == '\n' {
			// 裸换行：原样保留
			p.line++
			b.WriteByte(c)
			p.pos++
			continue
		}
		if c != '\\' {
			b.WriteByte(c)
			p.pos++
			continue
		}
		// 转义
		p.pos++
		if p.eof() {
			return "", p.errf("字符串以 '\\' 结尾")
		}
		e := p.src[p.pos]
		switch e {
		case 'n':
			b.WriteByte('\n')
			p.pos++
		case 't':
			b.WriteByte('\t')
			p.pos++
		case 'r':
			b.WriteByte('\r')
			p.pos++
		case 'a':
			b.WriteByte(0x07)
			p.pos++
		case 'b':
			b.WriteByte(0x08)
			p.pos++
		case 'f':
			b.WriteByte(0x0C)
			p.pos++
		case 'v':
			b.WriteByte(0x0B)
			p.pos++
		case '\\', '"', '\'':
			b.WriteByte(e)
			p.pos++
		case 'x', 'X':
			p.pos++
			hex := ""
			for len(hex) < 2 && !p.eof() && isHexDigit(p.src[p.pos]) {
				hex += string(p.src[p.pos])
				p.pos++
			}
			if n, err := strconv.ParseUint(hex, 16, 8); err == nil {
				b.WriteByte(byte(n))
			}
		case 'z':
			// \z 跳过后续空白
			p.pos++
			for !p.eof() && (p.src[p.pos] == ' ' || p.src[p.pos] == '\t' || p.src[p.pos] == '\n' || p.src[p.pos] == '\r') {
				if p.src[p.pos] == '\n' {
					p.line++
				}
				p.pos++
			}
		default:
			if e >= '0' && e <= '9' {
				dec := ""
				for len(dec) < 3 && !p.eof() && p.src[p.pos] >= '0' && p.src[p.pos] <= '9' {
					dec += string(p.src[p.pos])
					p.pos++
				}
				if n, err := strconv.Atoi(dec); err == nil {
					b.WriteRune(rune(n))
				}
				continue
			}
			// 未知转义：原样保留反斜杠后的字符
			b.WriteByte(e)
			p.pos++
		}
	}
}

func isHexDigit(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

// parseTable 读 `{ … }`；字段可以是位置值、`key = value` 或 `[key] = value`。
func (p *luaParser) parseTable() (luaValue, error) {
	if p.src[p.pos] != '{' {
		return luaValue{}, p.errf("期望 '{'")
	}
	p.pos++
	out := luaValue{Kind: luaTable}
	for {
		p.skipSpace()
		if p.eof() {
			return luaValue{}, p.errf("表缺少收尾 '}'")
		}
		if p.src[p.pos] == '}' {
			p.pos++
			return out, nil
		}
		// 先尝试解析「键 = 值」形态；失败则回退成位置值
		save := p.pos
		saveLine := p.line
		if key, ok := p.tryParseKey(); ok {
			p.pos = key.pos
			p.line = key.line
			p.skipSpace()
			if !p.eof() && p.src[p.pos] == '=' {
				p.pos++
				val, err := p.parseValue()
				if err != nil {
					return luaValue{}, err
				}
				out.Fields = append(out.Fields, luaField{Key: key.key, Val: val})
			} else {
				// 不是键：回退，按位置值处理
				p.pos = save
				p.line = saveLine
				val, err := p.parseValue()
				if err != nil {
					return luaValue{}, err
				}
				out.Fields = append(out.Fields, luaField{Val: val})
			}
		} else {
			p.pos = save
			p.line = saveLine
			val, err := p.parseValue()
			if err != nil {
				return luaValue{}, err
			}
			out.Fields = append(out.Fields, luaField{Val: val})
		}
		// 分隔符：`,` / `;`（可省略，紧跟 `}` 也合法）
		p.skipSpace()
		if !p.eof() && (p.src[p.pos] == ',' || p.src[p.pos] == ';') {
			p.pos++
		}
	}
}

type keyProbe struct {
	pos  int
	line int
	key  string
}

func (p *luaParser) save() keyProbe { return keyProbe{pos: p.pos, line: p.line} }

// tryParseKey 试读一个键（不改动解析器状态）。
func (p *luaParser) tryParseKey() (keyProbe, bool) {
	start, startLine := p.pos, p.line
	if p.eof() {
		return keyProbe{}, false
	}
	if p.src[p.pos] == '[' {
		p.pos++
		p.skipSpace()
		if p.eof() {
			p.pos, p.line = start, startLine
			return keyProbe{}, false
		}
		var key string
		if c := p.src[p.pos]; c == '"' || c == '\'' {
			s, err := p.parseString()
			if err != nil {
				p.pos, p.line = start, startLine
				return keyProbe{}, false
			}
			key = s
		} else {
			n, err := p.parseNumber()
			if err != nil {
				p.pos, p.line = start, startLine
				return keyProbe{}, false
			}
			key = formatLuaNumber(n)
		}
		p.skipSpace()
		if p.eof() || p.src[p.pos] != ']' {
			p.pos, p.line = start, startLine
			return keyProbe{}, false
		}
		p.pos++
		res := keyProbe{pos: p.pos, line: p.line, key: key}
		p.pos, p.line = start, startLine
		return res, true
	}
	if !isIdentStart(p.src[p.pos]) {
		p.pos, p.line = start, startLine
		return keyProbe{}, false
	}
	key := p.parseIdent()
	probe := keyProbe{pos: p.pos, line: p.line, key: key}
	p.pos, p.line = start, startLine
	return probe, true
}
