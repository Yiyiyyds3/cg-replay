// script.go —— Nitro+ NSS 明文脚本解析。
//
// NSS 是类 C 语法的明文脚本（scene 块 + if/goto + 引擎命令），CP932（日文）
// 或 GBK（汉化组转码，nss-1.npa）编码。事件相关的只有三类构造：
//   - <PRE @boxN> ... </PRE> 文本块（[textNNNN] 书签行、//【说话人】 注释行、
//     <voice ... src="voice/xx"> 语音标签、<RUBY text="..">字</RUBY> 注音、
//     <k> 换页标记）；
//   - CreateTextureEX("名", 层, 锚, 锚, "cg/bg/...jpg") 图像引用；
//   - OnBG(10, "B_背景名") 背景切换（对应 cg.npa bg/ 下同名 jpg）。
package nitro

import (
	"fmt"
	"regexp"
	"strings"

	"ucgr/internal/textcodec"
)

// SegKind 段落种类。
type SegKind uint8

const (
	SegText   SegKind = iota // PRE 文本块
	SegVoice                 // <voice src>
	SegImage                 // CreateTextureEX / OnBG
	SegBGM                   // SoundPlay("@m07")
)

// Seg 一个段落。
type Seg struct {
	Kind    SegKind
	Text    string // SegText：清洗后正文
	Speaker string // SegText：说话人（//【X】）
	Ref     string // SegVoice/SegImage/SegBGM 的资源引用
}

// Script 一个 .nss 的解析产物。
type Script struct {
	Name   string // 条目名（0103.nss）
	TextCP int    // 932=日文 / 936=汉化 GBK
	Segs   []Seg
}

var (
	reRuby    = regexp.MustCompile(`<RUBY text="[^"]*">([^<]*)</RUBY>`)
	reTag     = regexp.MustCompile(`</?[A-Za-z][^>\n]*>`)
	reSpeaker = regexp.MustCompile(`//?【([^】]{1,32})】`)
	reVoice   = regexp.MustCompile(`<voice[^>]*\bsrc="([^"]+)"`)
	reTexture = regexp.MustCompile(`CreateTextureEX?\s*\(\s*"[^"]*"\s*,[^"]*"([^"]+\.(?:jpg|png|bmp))"\s*\)`)
	reOnBG    = regexp.MustCompile(`OnBG\s*\(\s*[^,]+,\s*"([^"]+)"\s*\)`)
	reSound   = regexp.MustCompile(`SoundPlay\s*\(\s*"(@[A-Za-z0-9_]+)"`)
)

// ParseNSS 解析 .nss 明文脚本。
func ParseNSS(data []byte, name string) (*Script, error) {
	cp := detectTextCP(data)
	var text string
	if cp == 932 {
		text = textcodec.SJIS(data)
	} else {
		text = decodeGBK(data)
	}
	if text == "" {
		return nil, fmt.Errorf("nitro: 脚本解码为空: %s", name)
	}
	s := &Script{Name: name, TextCP: cp}
	lines := strings.Split(text, "\n")
	inPRE := false
	var buf []string
	var speaker string
	var blockVoice string // 块内 <voice src>（随文本段一起产出）
	flush := func() {
		if !inPRE {
			return
		}
		body := strings.TrimSpace(cleanText(strings.Join(buf, "\n")))
		if blockVoice != "" {
			s.Segs = append(s.Segs, Seg{Kind: SegVoice, Ref: blockVoice})
		}
		if body != "" {
			s.Segs = append(s.Segs, Seg{Kind: SegText, Text: body, Speaker: speaker})
		}
		buf = buf[:0]
		speaker = ""
		blockVoice = ""
		inPRE = false
	}
	for _, raw := range lines {
		line := strings.TrimRight(raw, "\r")
		trim := strings.TrimSpace(line)
		if inPRE {
			switch {
			case strings.HasPrefix(trim, "</PRE>"):
				flush()
			case strings.HasPrefix(trim, "[text"):
				// 书签行，跳过
			default:
				if m := reSpeaker.FindStringSubmatch(trim); m != nil {
					speaker = m[1]
					continue
				}
				if vm := reVoice.FindStringSubmatch(trim); vm != nil {
					blockVoice = vm[1]
					continue
				}
				buf = append(buf, line)
			}
			continue
		}
		if strings.HasPrefix(trim, "<PRE") {
			inPRE = true
			buf = buf[:0]
			speaker = ""
			continue
		}
		if m := reTexture.FindStringSubmatch(line); m != nil {
			s.Segs = append(s.Segs, Seg{Kind: SegImage, Ref: m[1]})
			continue
		}
		if m := reOnBG.FindStringSubmatch(line); m != nil {
			ref := "cg/bg/" + m[1] + ".jpg"
			s.Segs = append(s.Segs, Seg{Kind: SegImage, Ref: ref})
			continue
		}
		if m := reSound.FindStringSubmatch(line); m != nil {
			ref := "bgm/" + strings.TrimPrefix(m[1], "@") + ".ogg"
			s.Segs = append(s.Segs, Seg{Kind: SegBGM, Ref: ref})
			continue
		}
	}
	flush()
	return s, nil
}

// cleanText 清洗正文：<RUBY> 取内文、剔其余行内标签、归一空白。
func cleanText(s string) string {
	s = reRuby.ReplaceAllString(s, "$1")
	s = reTag.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "\u3000", "　") // 全角空格保留
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '\r':
			continue
		case '\n':
			if b.Len() > 0 && !strings.HasSuffix(b.String(), "\n") {
				b.WriteRune('\n')
			}
		default:
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

// detectTextCP 判码：CP932 解码后平假名占非 ASCII 字符的比例 > 0.1 → 日文；
// 否则按汉化 GBK 处理（GBK 字节用 CP932 误解出大量半角假名，平假名率≈0）。
func detectTextCP(data []byte) int {
	t := textcodec.SJIS(data)
	var kana, cjk int
	for _, r := range t {
		switch {
		case r >= 0x3040 && r <= 0x309F: // 平假名
			kana++
			cjk++
		case r >= 0x30A0 && r <= 0x30FF, r >= 0x4E00 && r <= 0x9FFF, r >= 0xFF66 && r <= 0xFF9D:
			cjk++
		}
	}
	if cjk > 0 && float64(kana)/float64(cjk) > 0.1 {
		return 932
	}
	return 936
}
