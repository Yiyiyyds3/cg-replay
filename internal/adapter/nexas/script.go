// script.go —— .bin 编译脚本的文本段解析（zbspac 实测口径）。
//
// 结构：头是 u64 数组，第一个 u64 的值 n 决定文本区起点 = (n+1)*8
// （数组其余项是指向文本的指针/参数，回放不需要）。
// 文本区：GBK/CP932 段落，段与段之间 1 个以上 0x00；遇到 <0x20 或
// 0xFF 即进入尾部控制区。
//
// 段落语义（Baldr Sky 实测）：
//   - 说话人：台词段前的短段（甲 / ？？？ / 雅…）
//   - 台词：@v<语音号>「…」 或纯叙述；@n 换行、@k 停顿
//   - 标签：# 开头（跳转目标）
//   - 资源：*.png / *.fnt / *.spm（标题画面/UI/粒子特效）
//   - 指令：大写字母或数字开头的效果名（RE_Hit02 / Sword01…）
//   - 分支：*.bin 结尾的脚本引用（选项或跳转）
package nexas

import (
	"encoding/binary"
	"fmt"
	"strings"
	"unicode/utf8"
)

// SegKind 段落类型。
type SegKind int

const (
	SegText    SegKind = iota // 台词（含叙述）
	SegSpeaker                // 说话人
	SegLabel                  // # 标签
	SegAsset                  // 资源引用（*.png / *.fnt / *.spm）
	SegBranch                 // *.bin 脚本引用
	SegCmd                    // 效果指令（大写/数字开头）
)

// Seg 一个文本段。
type Seg struct {
	Kind  SegKind
	Text  string // UTF-8（已清 @ 控制码与注音读音）
	Voice string // @v 语音号（台词段可能有）
	Raw   string // 清码前的原文
}

// Script 一个 .bin 的解析结果。
type Script struct {
	Name   string // 条目名（GBK 显示名）
	TextCP int    // 932=日文版 / 936=中文版
	Segs   []Seg
}

// ParseBin 解析 .bin 脚本。
//
// 头结构有两种实测变体：
//   - Baldr Sky Steam（2019）：头是 u64 数组，第一项 n → 文本区 (n+1)*8；
//   - BALDRSKY DIVEX（2012）：头是 u32 指令块，第二项 n → 文本区 n*8。
//
// 文本区：GBK/CP932 C 字符串段落，段与段之间 0x00（Steam 版文本区纯净；
// DIVEX 段间夹杂二进制指令字节，用「跳过非法字节、连续 64 个非法才停」
// 的宽松扫描——两种变体都能正确收集段落，尾部控制区自动截停）。
func ParseBin(data []byte, name string) (*Script, error) {
	if len(data) < 16 {
		return nil, fmt.Errorf("nexas: %s 太短，不是 .bin 脚本", name)
	}
	textStart, ok := findTextStart(data)
	if !ok {
		return nil, fmt.Errorf("nexas: %s 头部指针越界，不是 .bin 脚本", name)
	}
	var raws [][]byte
	i := textStart
	for i < len(data) && data[i] == 0 {
		i++
	}
	miss := 0
	for i < len(data) && miss < 64 {
		b := data[i]
		if b < 0x20 || b == 0xFF {
			i++ // 控制字节：跳过（尾部控制区由此累计截停）
			miss++
			continue
		}
		end := i
		for end < len(data) && data[end] != 0 {
			end++
		}
		seg := data[i:end]
		if _, ok := validSeg(seg); ok {
			raws = append(raws, append([]byte(nil), seg...))
			miss = 0
		} else {
			miss++ // 解不出的杂段按非法字节计
		}
		i = end + 1
		for i < len(data) && data[i] == 0 {
			i++
		}
	}
	if len(raws) == 0 {
		return nil, fmt.Errorf("nexas: %s 没有解析出文本段", name)
	}
	cp := decodeSegVoteCP(raws)
	s := &Script{Name: name, TextCP: cp}
	for _, raw := range raws {
		text := decodeSegAuto(raw)
		s.Segs = append(s.Segs, classify(text))
	}
	return s, nil
}

// findTextStart 定位文本区起点（两种头变体，见 ParseBin 注释）。
func findTextStart(data []byte) (int, bool) {
	// Steam：u64[0] = n → (n+1)*8。
	if n := leU64(data); n > 0 && n < uint64(len(data)) {
		st := int(n+1) * 8
		if st > 0 && st < len(data) {
			return st, true
		}
	}
	// DIVEX：u32[1] = n → n*8。
	if len(data) >= 8 {
		n := binary.LittleEndian.Uint32(data[4:8])
		st := int(n) * 8
		if st > 0 && st < len(data) {
			return st, true
		}
	}
	return 0, false
}

// validSeg 报一段字节是不是合法文本段（GBK/CP932 至少一种解出且
// 乱码率低）。
func validSeg(seg []byte) (string, bool) {
	if len(seg) == 0 {
		return "", false
	}
	text := decodeSegAuto(seg)
	if text == "" {
		return "", false
	}
	bad := 0
	total := 0
	for _, r := range text {
		if r == utf8.RuneError {
			bad++
		}
		total++
	}
	if total == 0 || bad*5 > total { // 乱码 > 20% 不是文本
		return "", false
	}
	return text, true
}

// decodeSegVoteCP 只为 Meta 记录用：脚本主流内码（932=日文 / 936=中文）。
func decodeSegVoteCP(segs [][]byte) int {
	jp := 0
	for _, seg := range segs {
		if len(seg) < 8 {
			continue
		}
		if hiraganaRatio(decodeCP932(seg)) > 0.12 {
			jp++
		}
	}
	if jp > 0 && jp*5 > len(segs) { // 日文段占比 > 20%
		return 932
	}
	return 936
}

// classify 段落分类 + 清理控制码。
func classify(text string) Seg {
	if hasSuffixFold(text, ".bin") {
		return Seg{Kind: SegBranch, Text: text, Raw: text}
	}
	if len(text) > 1 && text[0] == '#' {
		return Seg{Kind: SegLabel, Text: text, Raw: text}
	}
	if hasSuffixFold(text, ".png") || hasSuffixFold(text, ".bmp") ||
		hasSuffixFold(text, ".fnt") || hasSuffixFold(text, ".spm") {
		return Seg{Kind: SegAsset, Text: text, Raw: text}
	}
	// 台词：@v<语音号>「…」。
	voice := ""
	t := text
	if len(t) > 4 && t[0] == '@' && (t[1] == 'v' || t[1] == 'V') {
		j := 2
		for j < len(t) && t[j] >= '0' && t[j] <= '9' {
			j++
		}
		if j > 2 && j <= 12 {
			voice = t[2:j]
			t = t[j:]
		}
	}
	// 指令/特效名（大写字母/数字开头、纯 ASCII）优先于正文长度判定——
	// RE_Hit02 这类 8 字符指令名不能被「较长叙述」规则吞掉。
	if isCmdName(text) {
		return Seg{Kind: SegCmd, Text: text, Raw: text}
	}
	clean := cleanText(t)
	if voice != "" && clean != "" {
		return Seg{Kind: SegText, Text: clean, Voice: voice, Raw: text}
	}
	if strings.Contains(clean, "「") || strings.Contains(clean, "『") {
		return Seg{Kind: SegText, Text: clean, Raw: text}
	}
	// 无引号无语音：较长的正文算叙述，短的非指令串算说话人。
	rn := []rune(clean)
	if len(rn) >= 8 {
		return Seg{Kind: SegText, Text: clean, Raw: text}
	}
	if !isCmdName(text) && clean != "" {
		return Seg{Kind: SegSpeaker, Text: clean, Raw: text}
	}
	if isCmdName(text) {
		return Seg{Kind: SegCmd, Text: text, Raw: text}
	}
	if clean != "" {
		return Seg{Kind: SegText, Text: clean, Raw: text}
	}
	return Seg{Kind: SegCmd, Text: text, Raw: text}
}

// cleanText 剥 @ 控制码与注音读音。
//
// 注音格式（实测）：@r須藤@すどう@@r雅@まさ@ —— @r 开正文、@ 开读音、
// @ 闭合回到正文；连续注音时闭合与新开共用 @@。三态状态机走一遍正好。
func cleanText(s string) string {
	runes := []rune(s)
	var out []rune
	const (
		modeNormal = iota // 普通正文
		modeRubyBody      // @r 后的注音正文（收集）
		modeRubyPhon      // @ 后的读音（丢弃，@ 闭合）
	)
	mode := modeNormal
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r != '@' {
			if mode != modeRubyPhon {
				out = append(out, r)
			}
			continue
		}
		if i+1 >= len(runes) {
			continue // 段尾孤立 @
		}
		c := runes[i+1]
		switch {
		case c == 'n' || c == 'N':
			if mode != modeRubyPhon {
				out = append(out, ' ')
			}
			i++
		case c == 'k' || c == 'K' || c == 'd' || c == 'D' || c == 'w' || c == 'W':
			i++
		case c == 'r' || c == 'R':
			// 注音开始：@r 后的正文照常收集（rubyBody 与 normal 行为
			// 一致——正文收集，遇到下一个 @ 才转入读音）。
			mode = modeRubyBody
			i++
		default:
			if mode == modeRubyBody {
				// 注音正文的闭合 @：进入读音（丢弃）。
				mode = modeRubyPhon
			} else if mode == modeRubyPhon {
				// 读音闭合 @：回到普通正文。
				mode = modeNormal
			} else if isKanaRune(c) {
				// 普通正文里裸 @假名（无 @r 开头）：当读音丢弃，保底。
				mode = modeRubyPhon
			}
			// 孤立 @ 丢弃。
		}
	}
	for len(out) > 0 && (out[0] == ' ' || out[0] == '　') {
		out = out[1:]
	}
	for len(out) > 0 && (out[len(out)-1] == ' ' || out[len(out)-1] == '　') {
		out = out[:len(out)-1]
	}
	return string(out)
}

// isKanaRune 假名判断（注音读音几乎全是假名）。
func isKanaRune(r rune) bool {
	return (r >= 0x3041 && r <= 0x309F) || // 平假名
		(r >= 0x30A1 && r <= 0x30FF) || // 片假名
		(r >= 0xFF66 && r <= 0xFF9D) // 半角片假名
}

// isCmdName 判断是不是指令/特效名形态（大写字母/数字开头、纯 ASCII）。
func isCmdName(s string) bool {
	if s == "" {
		return false
	}
	first := s[0]
	if !((first >= 'A' && first <= 'Z') || (first >= '0' && first <= '9')) {
		return false
	}
	for _, r := range s {
		if r > 0x7F {
			return false
		}
	}
	return true
}

func isDigits(s string) bool {
	if len(s) == 0 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func hasSuffixFold(s, suf string) bool {
	if len(s) < len(suf) {
		return false
	}
	return lowerASCII(s[len(s)-len(suf):]) == lowerASCII(suf)
}

func lowerASCII(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 32
		}
	}
	return string(b)
}

func leU64(b []byte) uint64 {
	return uint64(b[0]) | uint64(b[1])<<8 | uint64(b[2])<<16 | uint64(b[3])<<24 |
		uint64(b[4])<<32 | uint64(b[5])<<40 | uint64(b[6])<<48 | uint64(b[7])<<56
}
