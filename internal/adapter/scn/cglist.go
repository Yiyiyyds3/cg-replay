package scn

// cglist.go —— 9nine 的 CG 鉴赏表。
//
// main/cglist.csv 是 CP932、Tab 分隔：
//
//	#CGモード一覧
//	thum_ev101,	ev101a,ev101b,ev101c
//
// 第一列是缩略图 storage 名，后面是该 CG 的图片序列（鉴赏时逐张切换）。
// 标题表 cgtitle.csv 大多数条目为空，空时回退到缩略图名。

import (
	"os"
	"path/filepath"
	"strings"

	"ucgr/internal/textcodec"
)

// decodeCGList 解码 CG 表文本（CP932 原文 / 单字节 XOR 变体）。
//
// 部分解包工具会把**文本资源**整体逐字节异或，键随游戏不同（本机实测：
// 新章 0x10、春色 0x13、雪色 0x45，九次九日/天色为明文）。而且键**不保证
// 全文件一致**：春色的 cglist.csv 在第 589 字节（ev311 行中间）从 0x13
// 切成 0x45（用磁盘上真实存在的 ev311a..i 图片名核对过）。
//
// 所以接受标准不是「开头像 CG 表」，而是「整份解出来每个字节都像 cglist
// 内容」；单键做不到时退到分段解码，每段单独选键。
func decodeCGList(data []byte) (string, byte) {
	// 快路径：原文或某个单键能**整份**解成合法字符（明文与正常混淆变体）。
	if cgBytesPlausible(data) {
		if t := textcodec.SJIS(data); looksLikeCGList(t) {
			return t, 0
		}
	}
	un := make([]byte, len(data))
	for k := 1; k < 256; k++ {
		for i, b := range data {
			un[i] = b ^ byte(k)
		}
		if cgBytesPlausible(un) {
			if t := textcodec.SJIS(un); looksLikeCGList(t) {
				return t, byte(k)
			}
		}
	}
	// 慢路径：键在文件中途切换（春色 0x13→0x45，切换点在 ev311 行中间）。
	// 分段解码与「分数最高的单键候选」比分数——分段能接上切换点，但解包
	// 产物真有零星损坏时单键反而更完整，谁分高用谁。
	best, bestK, bestScore := "", byte(0), -1.0
	if t := textcodec.SJIS(data); looksLikeCGList(t) {
		best, bestK, bestScore = t, 0, cgListScore(t)
	}
	for k := 1; k < 256; k++ {
		for i, b := range data {
			un[i] = b ^ byte(k)
		}
		if t := textcodec.SJIS(un); looksLikeCGList(t) {
			if s := cgListScore(t); s > bestScore {
				best, bestK, bestScore = t, byte(k), s
			}
		}
	}
	if t, ok := decodeCGListSegmented(data); ok {
		if s := cgListScore(t); s > bestScore {
			best, bestK = t, 0
		}
	}
	if best == "" {
		best = textcodec.SJIS(data)
	}
	return best, bestK
}

// cgListScore 给解码结果打分：token 化后「像条目」的比例。
//
// token 合法 = 纯 ASCII 的 storage 名样式（字母数字 _ ? #，带数字），
// 或纯 CP932 文本（标题表的日文标题）。ASCII 与双字节混排的基本是乱码。
// 表头 token（`#CGモード一覧` 这类混排）所有候选一样失分，不影响比较。
func cgListScore(t string) float64 {
	total, good := 0, 0
	for _, line := range strings.Split(t, "\n") {
		for _, tok := range strings.FieldsFunc(line, func(r rune) bool { return r == ',' || r == '\t' }) {
			tok = strings.TrimSpace(tok)
			if tok == "" {
				continue
			}
			total++
			if cgTokenOK(tok) {
				good++
			}
		}
	}
	if total == 0 {
		return 0
	}
	return float64(good) / float64(total)
}

func cgTokenOK(tok string) bool {
	ascii, dbcs := true, true
	hasDigit := false
	for _, r := range tok {
		if r >= 0x80 {
			ascii = false
			if r == 0xFFFD { // CP932 解码失败
				dbcs = false
			}
			continue
		}
		dbcs = false
		switch {
		case r >= '0' && r <= '9':
			hasDigit = true
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
		case r == '_' || r == '?' || r == '#':
		default:
			return false // 空格、引号、> ; 等：数据行不该有
		}
	}
	if ascii {
		return hasDigit
	}
	return dbcs
}

// cgCharOK 判断「解出来的字节是否像 cglist 内容」。
//
// 数据行只有字母/数字/分隔符（`?` 是合法语法：`sd_z01a?sd_z01_base1`
// 表示差分叠底图）；表头行带 CP932 汉字（双字节由调用方配对处理）和
// `.`/`:` 这类标点。刻意比「可打印」严：乱码常见的 > [ " ; ` 一律不认——
// 这是单键验收和分段选键能找对切换点的关键。
func cgCharOK(b byte) bool {
	switch {
	case b == '\r' || b == '\n' || b == ',' || b == '\t' || b == '#' || b == '_' ||
		b == ' ' || b == '?' || b == '.' || b == ':':
		return true
	case b >= '0' && b <= '9', b >= 'a' && b <= 'z', b >= 'A' && b <= 'Z':
		return true
	case b >= 0x81 && b <= 0x9f, b >= 0xe0 && b <= 0xef: // CP932 首字节
		return true
	}
	return false
}

// cgBytesPlausible 整份字节流按 CG 表字符集校验（CP932 双字节配对）。
// 有一个字节不像就 false——单键「开头对后面乱」的文件在这里被拦下。
func cgBytesPlausible(data []byte) bool {
	inDBCS := false
	for _, b := range data {
		if inDBCS {
			if b < 0x40 || b > 0xfc || b == 0x7f {
				return false
			}
			inDBCS = false
			continue
		}
		if !cgCharOK(b) {
			return false
		}
		if b >= 0x81 {
			inDBCS = true
		}
	}
	return true
}

// decodeCGListSegmented 分段选键解码：在位置 pos 处穷举 0..255，取「能连续
// 解出合法字符」最长的键，消费这一段后从断点继续。
//
// 贪心边界会「多吃」几个字节：切换点附近两把键都能解出合法字符（春色实测
// 多吃 3 字节，`ev311h` 变 `ev3ggh`）。所以贪心之后再做一次**边界精化**：
// 同一行的图片名共享前缀（ev311a..ev311i 都以 ev311 开头），在候选切换点
// 里选能让行内 token 前缀最一致的那个。
//
// 只在单键解码失败时启用；解完仍要过 looksLikeCGList 整体校验。
func decodeCGListSegmented(data []byte) (string, bool) {
	segs := make([]cgSegment, 0, 4)
	pos, prevK := 0, 0
	for pos < len(data) {
		bestK, bestLen := 0, 0
		for k := 0; k < 256; k++ {
			n := runLen(data, pos, k)
			if n > bestLen || (n == bestLen && n > 0 && k == prevK) {
				bestK, bestLen = k, n
			}
		}
		if bestLen == 0 {
			return "", false
		}
		segs = append(segs, cgSegment{pos, bestK, bestLen})
		pos += bestLen
		prevK = bestK
	}
	refineSegmentBounds(data, segs)

	out := make([]byte, 0, len(data))
	for _, s := range segs {
		for i := s.start; i < s.start+s.length; i++ {
			out = append(out, data[i]^byte(s.key))
		}
	}
	t := textcodec.SJIS(out)
	if !looksLikeCGList(t) {
		return "", false
	}
	return t, true
}

// cgSegment 一段同键区间。
type cgSegment struct {
	start, key, length int
}

// refineSegmentBounds 原地精化相邻两段的边界（两段键不同才需要）。
//
// 候选切换点 q：从贪心边界向前走，只要「后段键从 q 起能一路解到后段末尾」
// 就算候选（最多回看 64 字节——接缝不会有几十字节宽）。打分：按候选点
// 解码所在行，数「以行内图片名前缀开头的 token」个数，最高者胜；平手不动。
func refineSegmentBounds(data []byte, segs []cgSegment) {
	for i := 0; i+1 < len(segs); i++ {
		b := segs[i].start + segs[i].length
		k1, k2 := segs[i].key, segs[i+1].key
		if k1 == k2 {
			continue
		}
		end2 := segs[i+1].start + segs[i+1].length

		// 行首：b 之前最后一个 \r\n 之后（前段键覆盖到 b，用它找行首）。
		rowStart := 0
		inDB := false
		for j := 0; j < b; j++ {
			c := data[j] ^ byte(k1)
			if inDB {
				inDB = false
				continue
			}
			if c >= 0x81 {
				inDB = true
				continue
			}
			if c == '\n' {
				rowStart = j + 1
			}
		}
		// 行内图片名前缀：第一个 thum_ token 去掉 thum_ 与结尾字母。
		base := ""
		for _, tok := range strings.Split(string(decodeRange(data, rowStart, b, k1)), ",") {
			tok = strings.TrimSpace(tok)
			if strings.HasPrefix(tok, "thum_") && len(tok) > 5 {
				x := tok[5:]
				for len(x) > 0 && x[len(x)-1] >= 'a' && x[len(x)-1] <= 'z' {
					x = x[:len(x)-1]
				}
				base = x
				break
			}
		}
		if base == "" {
			continue
		}

		bestQ, bestScore := b, -1
		for q := b; q >= rowStart+1 && q >= b-64; q-- {
			if runLen(data, q, k2) < end2-q {
				if q != b {
					break // 更早处都不可行就停
				}
				continue
			}
			score := 0
			win := decodeRange(data, rowStart, end2, k1)
			for j := q; j < end2 && j-rowStart < len(win); j++ {
				win[j-rowStart] = data[j] ^ byte(k2) // [q,end2) 用 k2 重解
			}
			for _, tok := range strings.FieldsFunc(string(win), func(r rune) bool {
				return r == ',' || r == '\t' || r == '\n' || r == '\r'
			}) {
				tok = strings.TrimSpace(tok)
				if tok == "" {
					continue
				}
				if strings.HasPrefix(tok, base) || strings.HasPrefix(tok, "thum_"+base) {
					score++
				}
			}
			if score > bestScore {
				bestQ, bestScore = q, score
			}
		}
		if bestQ != b {
			segs[i].length = bestQ - segs[i].start
			segs[i+1].start = bestQ
			segs[i+1].length = end2 - bestQ
		}
	}
}

// decodeRange 按 key 解码 [from,to)。
func decodeRange(data []byte, from, to, key int) []byte {
	out := make([]byte, 0, max(0, to-from))
	for i := from; i < to && i < len(data); i++ {
		out = append(out, data[i]^byte(key))
	}
	return out
}

// runLen 从 pos 起按键 k 解码时，连续合法的字符数（CP932 双字节配对）。
func runLen(data []byte, pos, k int) int {
	i := pos
	inDBCS := false
	for i < len(data) {
		b := data[i] ^ byte(k)
		if inDBCS {
			if b < 0x40 || b > 0xfc || b == 0x7f {
				break
			}
			inDBCS = false
		} else if !cgCharOK(b) {
			break
		} else if b >= 0x81 {
			inDBCS = true
		}
		i++
	}
	return i - pos
}

// looksLikeCGList 判断文本是否是 CG 鉴赏表（表头或首列缩略图名）。
func looksLikeCGList(s string) bool {
	s = strings.TrimPrefix(s, "\ufeff")
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(strings.TrimRight(line, "\r"))
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#CG") {
			return true
		}
		if strings.HasPrefix(strings.ToLower(line), "thum_") {
			return true
		}
		// 前几行都不是表头/条目就放弃（避免把任意二进制当 CG 表）。
		if !strings.HasPrefix(line, "#") {
			return false
		}
	}
	return false
}

// CGEntry 一个 CG 鉴赏条目。
type CGEntry struct {
	Index  int
	Thumb  string   // thum_ev101
	Images []string // ev101a, ev101b, ...
	Title  string
}

// ParseCGList 读 cglist.csv（自动解码 CP932），并合并同目录 cgtitle.csv。
func ParseCGList(path string) ([]CGEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	text, _ := decodeCGList(data)
	text = strings.TrimPrefix(text, "\ufeff")
	titles := parseCGTitles(filepath.Join(filepath.Dir(path), "cgtitle.csv"))
	var out []CGEntry
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		thumb, rest := splitThumb(line)
		if thumb == "" {
			continue
		}
		var images []string
		for _, f := range strings.FieldsFunc(rest, func(r rune) bool { return r == ',' || r == '\t' }) {
			f = strings.Trim(strings.TrimSpace(f), `"`)
			if f != "" {
				images = append(images, f)
			}
		}
		title := titles[thumb]
		if title == "" {
			title = thumb
		}
		out = append(out, CGEntry{
			Index:  len(out),
			Thumb:  thumb,
			Images: images,
			Title:  title,
		})
	}
	return out, nil
}

// splitThumb 拆出第一列（缩略图）与剩余列。
//
// 9nine 原格式是「首列逗号 + 表头 Tab」的混合：`thum_ev101,\tev101a,ev101b`，
// 所以取两种分隔符里最先出现的一个，不能只认 Tab（否则首列会带尾逗号）。
// 纯 Tab / 纯逗号（汉化整理版）同样适用。
func splitThumb(line string) (string, string) {
	i := strings.IndexAny(line, "\t,")
	if i < 0 {
		return strings.Trim(line, `"`), ""
	}
	return strings.Trim(strings.TrimSpace(line[:i]), `"`), line[i+1:]
}

// parseCGTitles 读 cgtitle.csv：第一列缩略图，第二列标题（可为空）。
func parseCGTitles(path string) map[string]string {
	out := map[string]string{}
	data, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	text, _ := decodeCGList(data)
	text = strings.TrimPrefix(text, "\ufeff")
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, ",", 3)
		if len(parts) < 2 {
			continue
		}
		thumb := strings.Trim(strings.TrimSpace(parts[0]), `"`)
		title := strings.Trim(strings.TrimSpace(parts[1]), `"`)
		if thumb != "" && title != "" {
			out[thumb] = title
		}
	}
	return out
}

// FindCG 按缩略图名（thum_ev101）或纯数字（101）找条目。
func FindCG(entries []CGEntry, id string) *CGEntry {
	id = strings.TrimSpace(id)
	for i := range entries {
		if strings.EqualFold(entries[i].Thumb, id) {
			return &entries[i]
		}
	}
	if digits := tailDigits(id); digits != "" {
		for i := range entries {
			if tailDigits(entries[i].Thumb) == digits {
				return &entries[i]
			}
		}
	}
	return nil
}

func tailDigits(s string) string {
	i := len(s)
	for i > 0 && s[i-1] >= '0' && s[i-1] <= '9' {
		i--
	}
	return s[i:]
}
