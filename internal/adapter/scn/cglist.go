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
// 新章 0x10、春色 0x13、雪色 0x45，九次九日/天色为明文）。键不是猜的：
// 只有解出来确实是 CG 表（#CG 表头或 thum_ 条目）才接受。
func decodeCGList(data []byte) (string, byte) {
	if t := textcodec.SJIS(data); looksLikeCGList(t) {
		return t, 0
	}
	un := make([]byte, len(data))
	for k := 1; k < 256; k++ {
		for i, b := range data {
			un[i] = b ^ byte(k)
		}
		if t := textcodec.SJIS(un); looksLikeCGList(t) {
			return t, byte(k)
		}
	}
	return textcodec.SJIS(data), 0
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
