// decode.go —— 引擎内码解码。
//
// 官方中文版（Baldr Sky Steam）把文本和索引条目名都转成了 GBK；
// 日文原版是 CP932。GBK 是「几乎全码位合法」的编码，对日文文本
// 也能解出（错字）——所以不能只按「解码是否成功」判断，按脚本
// 投票：假名浓度高的是 CP932，否则 GBK。
package nexas

import "unicode/utf8"

// decodeName 索引条目名：GBK 优先（中文版），失败退 CP932。
func decodeName(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	if s, ok := decodeGBK(raw); ok {
		return s
	}
	return decodeCP932(raw)
}

// decodeNameCP 按投票出的内码解条目名。
func decodeNameCP(raw []byte, cp int) string {
	s, _ := decodeMB(raw, cp)
	return s
}

// namesVoteCP 整个 pac 的条目名内码投票。
//
// 判据：CP932 双字节合法性（纯 Go 检查，不依赖 Windows API——
// MB_ERR_INVALID_CHARS 对孤立尾字节太宽容，区分度不够）。
// 实测（Script.pac 索引名）：
//
//	Baldr Sky Steam（汉化组把名字转成 GBK）：GBK 汉字第二字节大量落在
//	  CP932 非法首字节区（FD-FE 等），well-formed 比例极低 → GBK；
//	BALDRSKY DIVEX（日文原版内码）：全部 well-formed → CP932。
//
// 反方向不成立：CP932 名字在 GBK 里几乎全部「合法」（错字但解得出），
// 所以只能用 CP932 的合法性单向判。
func namesVoteCP(rawNames [][]byte) int {
	bad := 0
	total := 0
	for _, n := range rawNames {
		if len(n) == 0 {
			continue
		}
		total++
		if !cp932WellFormed(n) {
			bad++
		}
	}
	if total > 0 && bad*4 > total { // cp932 非法 > 25% → GBK
		return 936
	}
	return 932
}

// cp932WellFormed 报字节流是不是合法 CP932 序列：
// 0x00-0x7F 单字节；0xA1-0xDF 半角假名单字节；0x81-0x9F/0xE0-0xFC
// 双字节首字节（尾字节 0x40-0xFC 除 0x7F）；其余非法。
func cp932WellFormed(b []byte) bool {
	for i := 0; i < len(b); {
		c := b[i]
		if c < 0x80 {
			i++
			continue
		}
		if c >= 0xA1 && c <= 0xDF {
			i++
			continue
		}
		if (c >= 0x81 && c <= 0x9F) || (c >= 0xE0 && c <= 0xFC) {
			if i+1 >= len(b) {
				return false
			}
			t := b[i+1]
			if t < 0x40 || t > 0xFC || t == 0x7F {
				return false
			}
			i += 2
			continue
		}
		return false // 0x80 / 0xA0 / 0xFD-0xFF 等非法首字节
	}
	return true
}

// decodeGBK / decodeCP932 走 Windows MultiByteToWideChar（936=GBK，
// 932=CP932）；其它平台退化为原样字节（目标平台是 Windows）。
func decodeGBK(b []byte) (string, bool) {
	return decodeMB(b, 936)
}

// decodeCP932 按日文内码解（失败时 U+FFFD 兜底）。
func decodeCP932(b []byte) string {
	s, _ := decodeMB(b, 932)
	return s
}

// decodeMB 多字节编码 → UTF-8。ok=false 表示 Windows API 拒绝了输入
// （非法字节序列）。
func decodeMB(b []byte, cp int) (string, bool) {
	if len(b) == 0 {
		return "", true
	}
	s, err := mbToWide(b, uint32(cp))
	if err != nil {
		return string(b), false
	}
	return s, true
}

// looksJapanese 报文本里的假名浓度（粗判日文）。
func looksJapanese(s string) bool {
	kana := 0
	total := 0
	for _, r := range s {
		if r == utf8.RuneError {
			continue
		}
		if r < 0x80 {
			continue // ASCII（标点、罗马字）不参与
		}
		total++
		if (r >= 0x3040 && r <= 0x30FF) || (r >= 0xFF66 && r <= 0xFF9D) {
			kana++
		}
	}
	return total > 0 && kana*4 > total // 假名占比 > 25%
}

// hiraganaRatio 平假名浓度（判别日文段的关键指标——实测分离度：
// 日文脚本 0.405，中文脚本 0.000。中文 GBK 字节被 CP932 误解出的
// 几乎全是半角假名/汉字，平假名极少）。
func hiraganaRatio(s string) float64 {
	total := 0
	hira := 0
	for _, r := range s {
		if r < 0x80 {
			continue
		}
		total++
		if r >= 0x3041 && r <= 0x309F {
			hira++
		}
	}
	if total == 0 {
		return 0
	}
	return float64(hira) / float64(total)
}

// decodeSegAuto 逐段选内码：CP932 解出平假名 → 日文段；否则 GBK
// （官方中文版）。GBK 失败时退 CP932。
//
// 实测（Baldr Sky Steam 中文版 Script.pac）：
//
//	00：ADV体験版.bin（日文残留）：CP932 平假名率均值 0.405 → 全部选对
//	01：レイン編01.bin（中文正文）：均值 0.000 → 全部选 GBK
func decodeSegAuto(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	t932, _ := decodeMB(b, 932)
	if hiraganaRatio(t932) > 0.12 {
		return t932
	}
	if tg, ok := decodeMB(b, 936); ok {
		return tg
	}
	return t932
}
