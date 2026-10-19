// Package bgi 实现 BGI / Ethornell（Buriko）引擎（.bs 编译脚本）的适配器。
//
// 格式（BurikoCompiledScriptVer1.00）：
//
//	magic(28B) headerSize(i32) 头(外链脚本名 + 跳转标签) 指令流(dword…) 字符串区(NUL 结尾)
//
// _codeOffset = 28 + headerSize；指令 = [opcode][按模板的实参 dword…]，
// 'm'/'n' 类实参是「相对 _codeOffset 的字符串偏移」。字符串由 0x0003 压入，
// 随后被「下一个 opcode >= 0x100 的命令」消耗 —— 这条归属规则来自对
// 樱之诗 / 秽翼的尤斯蒂娅 / 美好的每一天 三款语料 4,265,970 条指令的统计，
// 并用「字符串与真实文件名同词干」做了交叉验证（见 reports/）。
//
// 采集纪律 §6（保守保留）：未登记的命令只要带「形似资源名」的字符串实参，
// 一样要进引用，不因为不认识命令就丢。
package bgi

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	engineID = "bgi"

	// scriptMagic 是 V1 编译脚本头；不带该头的旧版（ver0）本适配器明确报错。
	scriptMagic = "BurikoCompiledScriptVer1.00\x00"

	// maxOpcode 是已知指令上界：超过它说明 walk 已走进字符串区（把 ASCII 当指令）。
	maxOpcode = 0x1000
	// maxCString 单条字符串读取上限（防御性）。
	maxCString = 64 << 10

	scriptExt        = ".bs"
	archiveExt       = ".arc"
	dirProbeMaxDepth = 5
)

// 已确认语义的指令。其余指令一律按 §6 保守保留：不解释语义，但登记 + 收引用。
const (
	opPushString = 0x0003 // push 字符串地址（进 stringStack）
	opUserCall   = 0x001C // 用户函数调用（被调函数名 = 栈顶字符串，_SelectEx 即选项屏）
	opMsg        = 0x0140 // 显示消息
	opBacklog    = 0x0143 // 回想/回溯消息
	opRuby       = 0x014B // 注音（假名 → 汉字）
	opChoice     = 0x0160 // 选项屏（剩余栈内容全是选项）
	opExit       = 0x001B // 返回/结束（终止候选）
	opExit2      = 0x00F4 // 返回/结束（终止候选）
)

// operandTemplates 是 V1 指令的实参模板（照抄 VNTextPatch EthornellV1Disassembler
// 的 OperandTemplates 表）：'i' = int32，'c' = 代码地址，'m'/'n' = 字符串地址。
// 未列出的 opcode 一律按 0 实参处理。
var operandTemplates = map[uint32]string{
	0x0000: "i", // push 常量
	0x0001: "c", // push 代码地址
	0x0002: "i", // push 变量地址（帧基址 - 值）
	0x0003: "m", // push 字符串地址
	0x0008: "i", // load（0 = byte，其余 = dword）
	0x0009: "i", // store
	0x000A: "i",
	0x0017: "i",
	0x0019: "i",
	0x003F: "i",
	0x007B: "iii",
	0x007E: "i",
	0x007F: "ii",
}

// 资源类指令的 opcode（用「字符串实参与真实文件名同词干」的命中率验证过）。
var (
	imageOps = map[uint32]bool{
		0x0280: true, 0x0281: true, 0x0288: true, 0x0289: true,
		0x02C0: true, 0x02C2: true, 0x02C4: true, 0x02C6: true,
		0x02C8: true, 0x02DC: true,
		// 以下三项在《秽翼的尤斯蒂娅》（汉化版）实测为图类命令：
		// 0x0248 收 bg_2010/bg_1510（背景）、0x0151/0x0300/0x0304 收 view_base00/view_mask02/view_pen。
		0x0248: true, 0x0151: true, 0x0300: true, 0x0304: true,
	}
	bgmOps   = map[uint32]bool{0x0180: true, 0x0184: true, 0x0185: true, 0x0186: true}
	seOps    = map[uint32]bool{0x01A0: true, 0x01A1: true, 0x01A2: true, 0x01A3: true}
	voiceOps = map[uint32]bool{0x01A5: true, 0x01A9: true}
)

// opKind 报告 opcode 的资源类别（"" = 未分类）。
func opKind(op uint32) string {
	switch {
	case imageOps[op]:
		return "image"
	case bgmOps[op]:
		return "bgm"
	case seOps[op]:
		return "se"
	case voiceOps[op]:
		return "voice"
	}
	switch op {
	case opMsg, opBacklog:
		return "text"
	case opRuby:
		return "ruby"
	case opChoice:
		return "choice"
	}
	return ""
}

// Command 是一条已解码的指令。
type Command struct {
	Op     uint32
	Offset int      // 相对代码段起点
	Args   []int32  // 模板实参
	Raw    string   // 反汇编文本（诊断/登记用）
	Pushed []string // 归因到本命令的 0x0003 字符串（编码解码后）
}

// Label 是脚本头里的跳转标签。
type Label struct {
	Name string
	Addr int32
}

// Script 是解析后的 .bs 脚本。
type Script struct {
	EntryPath  string
	HeaderSize int32
	CodeOffset int
	CodeLen    int
	RefScripts []string // 头里的外链脚本名
	Labels     []Label
	Commands   []Command
	Encoding   string
	Notes      []string // 解析过程中的可疑情况（照实记录，不静默）
	Unresolved int      // 越界/无终止的字符串地址条数
	Leftover   int      // 读完代码仍未归属的字符串条数
	Instr      int      // 走过的指令总数（含终止指令）
}

// ParseScriptData 解析一个 V1 编译脚本。
func ParseScriptData(data []byte, entry string) (*Script, error) {
	if !bytes.HasPrefix(data, []byte(scriptMagic)) {
		return nil, fmt.Errorf("bgi: 不是 BurikoCompiledScriptVer1.00（起始 % x）", preview(data, 16))
	}
	if len(data) < 32 {
		return nil, fmt.Errorf("bgi: 文件过短（%d 字节）", len(data))
	}
	s := &Script{EntryPath: entry, HeaderSize: le32s(data[28:32])}
	if s.HeaderSize < 4 {
		return nil, fmt.Errorf("bgi: headerSize 非法（%d）", s.HeaderSize)
	}
	codeOff := 28 + int(s.HeaderSize)
	if codeOff > len(data) {
		return nil, fmt.Errorf("bgi: 代码段起点 0x%X 超出文件（%d 字节）", codeOff, len(data))
	}
	s.CodeOffset = codeOff
	s.parseHeader(data)
	s.walk(data)
	return s, nil
}

// parseHeader 解析头里的外链脚本与标签。头字段不完整时只记录、不失败：
// 代码段起点（_codeOffset = 28 + headerSize）是唯一权威字段。
func (s *Script) parseHeader(data []byte) {
	pos, end := 32, s.CodeOffset
	if pos >= end {
		return
	}
	rd32 := func() (int32, bool) {
		if pos+4 > end {
			return 0, false
		}
		v := le32s(data[pos:])
		pos += 4
		return v, true
	}
	rdStr := func() (string, bool) {
		i := pos
		for i < end && data[i] != 0 {
			i++
		}
		if i >= end {
			return "", false
		}
		out := string(data[pos:i])
		pos = i + 1
		return out, true
	}
	n, ok := rd32()
	if !ok {
		s.Notes = append(s.Notes, "头里缺外链脚本计数")
		return
	}
	if n < 0 || n > 4096 {
		s.Notes = append(s.Notes, fmt.Sprintf("外链脚本计数可疑（%d），跳过头部解析", n))
		return
	}
	for i := int32(0); i < n; i++ {
		name, ok := rdStr()
		if !ok {
			s.Notes = append(s.Notes, "外链脚本名越界")
			return
		}
		if name != "" {
			s.RefScripts = append(s.RefScripts, name)
		}
	}
	ln, ok := rd32()
	if !ok {
		return
	}
	if ln < 0 || ln > 65536 {
		s.Notes = append(s.Notes, fmt.Sprintf("标签数可疑（%d），跳过标签解析", ln))
		return
	}
	for i := int32(0); i < ln; i++ {
		name, ok := rdStr()
		if !ok {
			s.Notes = append(s.Notes, "标签名越界")
			return
		}
		addr, ok := rd32()
		if !ok {
			s.Notes = append(s.Notes, "标签地址越界")
			return
		}
		s.Labels = append(s.Labels, Label{Name: name, Addr: addr})
	}
}

// walk 按模板走一遍指令流，顺手做字符串归属，最后统一选编码解码。
func (s *Script) walk(data []byte) {
	codeOff := s.CodeOffset
	var cmds []Command
	var raws [][][]byte // 与 cmds 一一对应：归因到该命令的字符串原始字节
	var pend [][]byte
	pos := codeOff
	maxCode := int32(0)

	for pos+4 <= len(data) {
		op := le32u(data[pos:])
		if op > maxOpcode {
			s.Notes = append(s.Notes,
				fmt.Sprintf("在 0x%X 处遇到可疑指令 0x%08X（疑已走进字符串区），代码段到此为止", pos, op))
			break
		}
		tpl := operandTemplates[op]
		s.Instr++
		p := pos + 4
		args := make([]int32, 0, len(tpl))
		overflow := false
		for i := 0; i < len(tpl); i++ {
			if p+4 > len(data) {
				overflow = true
				break
			}
			v := le32s(data[p:])
			p += 4
			args = append(args, v)
			if tpl[i] == 'c' && v > maxCode {
				maxCode = v
			}
		}
		if overflow {
			s.Notes = append(s.Notes, fmt.Sprintf("指令 0x%04X（0x%X）的实参越过文件末尾", op, pos))
			break
		}
		switch {
		case op == opPushString:
			if len(args) == 1 {
				if raw, ok := cstring(data, codeOff+int(args[0])); ok {
					pend = append(pend, raw)
				} else {
					s.Unresolved++
				}
			}
		case op >= 0x100 || op == opUserCall:
			// 字符串归属：>= 0x100 的命令（含用户函数调用）吃掉此前压入的字符串。
			cmds = append(cmds, Command{
				Op:     op,
				Offset: pos - codeOff,
				Args:   args,
				Raw:    fmt.Sprintf("0x%04X% s", op, argsText(args)),
			})
			raws = append(raws, pend)
			pend = nil
		}
		pos = p
		if (op == opExit || op == opExit2) && maxCode < int32(pos-codeOff) {
			break
		}
	}
	s.CodeLen = pos - codeOff
	s.Leftover = len(pend)

	// 选编码：把本脚本所有字符串拼一起判一次，避免逐条抖动。
	var all [][]byte
	for _, r := range raws {
		all = append(all, r...)
	}
	s.Encoding = pickEncoding(all)
	for i := range cmds {
		for _, b := range raws[i] {
			cmds[i].Pushed = append(cmds[i].Pushed, decodeBy(s.Encoding, b))
		}
	}
	s.Commands = cmds
}

// refNameRe 与 CatSystem2 适配器一致：只收「像资源名」的串，滤掉自然语言噪声。
var refNameRe = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.\-/]*$`)

// assetExtRe / codeNameRe 识别「资源名形态」：带素材扩展名，或 引擎式代号（ev1000a、bg1025b、taku_001715）。
var (
	assetExtRe = regexp.MustCompile(`(?i)\.(png|bmp|jpg|jpeg|tga|ogg|wav|mp3|mpg|mpeg|avi|wmv|bss|h|txt|csv|dat)$`)
	codeNameRe = regexp.MustCompile(`^[a-z][a-z0-9]{0,7}[_\-]?\d{2,6}[a-z]{0,2}$`)
)

func looksLikeRef(s string) bool {
	return assetExtRe.MatchString(s) || codeNameRe.MatchString(s)
}

// plausibleRef 资源名形态的宽松底线：滤掉空串/单字符等噪声。
// 实例：立绘命令 0x02C0 会把变体标记 a/y 与基础名 tati_ 一起压栈。
func plausibleRef(s string) bool {
	if s == "" || !refNameRe.MatchString(s) {
		return false
	}
	if assetExtRe.MatchString(s) || codeNameRe.MatchString(s) {
		return true
	}
	n := len([]rune(s))
	return n >= 4 || strings.ContainsAny(s, "_.")
}

// ExtractRefs 提取资源引用（候选集，不做池内校验）。
//
// 口径（§6 保守保留）：
//   - 图像/音频类命令的字符串实参全收；
//   - 文本类（消息/回想/注音/选项）的字符串是台词，不收；
//   - 其余未登记命令（>= 0x100）里「形似资源名」的字符串实参照收。
func (s *Script) ExtractRefs() []string {
	seen := map[string]bool{}
	var out []string
	add := func(v string) {
		if v == "" || seen[v] {
			return
		}
		seen[v] = true
		out = append(out, v)
	}
	for i := range s.Commands {
		c := &s.Commands[i]
		kind := opKind(c.Op)
		switch kind {
		case "text":
			// 消息组 [语音名, 说话人, 正文]（《秽翼》实测）：语音名形如 aiy340300010，
			// 对应 音频\data04034\aiy340300010.ogg，必须收进引用集，否则语音会被裁掉。
			if len(c.Pushed) >= 3 {
				if v := c.Pushed[0]; plausibleRef(v) && !looksLikeNaturalText(v) {
					add(v)
				}
			}
			continue
		case "ruby", "choice":
			continue
		case "image", "bgm", "se", "voice":
			for _, v := range c.Pushed {
				if plausibleRef(v) {
					add(v)
				}
			}
			continue
		}
		for _, v := range c.Pushed {
			if refNameRe.MatchString(v) && looksLikeRef(v) {
				add(v)
			}
		}
	}
	return out
}

// looksLikeNaturalText 粗略排除「像台词/人名」的串（含空格或全 CJK 且长度 >= 4）。
// 仅用于消息组首条串的语音名判定，避免把「名字」当资源收。
func looksLikeNaturalText(s string) bool {
	if strings.ContainsAny(s, " \u3000") && len([]rune(s)) >= 4 {
		return true
	}
	cjk := 0
	for _, r := range s {
		if r >= 0x2E80 {
			cjk++
		}
	}
	return cjk >= 2
}

// 编码：目标平台是 Windows，CP932/GB18030 都走 MultiByteToWideChar（包内自带，不改核心）。

// pickEncoding 判定整份脚本的文本编码。
// 汉化版（如秽翼的尤斯蒂娅）是 GBK：把这些字节按 CP932 解会得到大量半角片假名，
// 这是与日文原版最稳的分界（与 CatSystem2 的判据一致）。
func pickEncoding(parts [][]byte) string {
	if len(parts) == 0 {
		return "cp932"
	}
	joined := bytes.Join(parts, []byte("\n"))
	if _, ok := tryUTF8(joined); ok {
		return "utf-8"
	}
	s := decodeMB(joined, 932)
	total, hw, hi := 0, 0, 0
	for _, r := range s {
		total++
		switch {
		case r >= 0xFF61 && r <= 0xFF9F:
			hw++
		case r >= 0x3040 && r <= 0x309F:
			hi++
		}
	}
	if total > 0 && float64(hw)/float64(total) > 0.02 && hw >= hi {
		return "gb18030"
	}
	return "cp932"
}

func tryUTF8(b []byte) (string, bool) {
	if !utf8.Valid(b) {
		return "", false
	}
	s := string(b)
	if badChar(s) {
		return "", false
	}
	return s, true
}

func badChar(s string) bool {
	for _, r := range s {
		if r == utf8.RuneError {
			return true
		}
		if r < 0x20 && r != '\n' && r != '\r' && r != '\t' {
			return true
		}
	}
	return false
}

// decodeBy 按编码名解码字节串。
func decodeBy(enc string, b []byte) string {
	switch enc {
	case "utf-8":
		return string(b)
	case "gb18030":
		return decodeMB(b, 54936)
	default:
		return decodeMB(b, 932)
	}
}

func decodeMB(b []byte, cp int) string {
	if len(b) == 0 {
		return ""
	}
	if s, err := mbToWide(b, uint32(cp)); err == nil {
		return s
	}
	return string(b)
}

// ---- 小工具 ----

func le32u(b []byte) uint32 {
	return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
}

func le32s(b []byte) int32 { return int32(le32u(b)) }

// cstring 读一条 NUL 结尾字符串（返回原始字节，不含 NUL）。
func cstring(data []byte, off int) ([]byte, bool) {
	if off < 0 || off >= len(data) {
		return nil, false
	}
	end := off
	for end < len(data) && data[end] != 0 && end-off < maxCString {
		end++
	}
	if end >= len(data) || data[end] != 0 {
		return nil, false
	}
	return data[off:end], true
}

func preview(b []byte, n int) []byte {
	if len(b) > n {
		return b[:n]
	}
	return b
}

func argsText(args []int32) string {
	if len(args) == 0 {
		return ""
	}
	var sb strings.Builder
	for i, v := range args {
		if i > 0 {
			sb.WriteByte(' ')
		}
		fmt.Fprintf(&sb, "%d", v)
	}
	return " " + sb.String()
}
