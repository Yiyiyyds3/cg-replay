//go:build windows

package win32

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"unicode/utf16"
)

// 本作自带的思源宋体 SC 是 OTF/CFF，且整个文件被单字节 XOR 混淆。
// GDI（AddFontResourceExW + CreateFontW）其实完全支持 OTF/CFF，当初失败
// 只是因为文件被混淆。这里负责：还原混淆 → 写临时文件 → 私有注册 → 返回家族名。

var sfntMagic = [][4]byte{
	{'O', 'T', 'T', 'O'},     // OpenType/CFF
	{0x00, 0x01, 0x00, 0x00}, // TrueType
	{'t', 'r', 'u', 'e'},     // TrueType (Apple)
	{'t', 't', 'c', 'f'},     // TrueType Collection
}

// looksLikeSFNT 判断字节流是否是未混淆的 sfnt 字体。
func looksLikeSFNT(b []byte) bool {
	if len(b) < 4 {
		return false
	}
	for _, m := range sfntMagic {
		if b[0] == m[0] && b[1] == m[1] && b[2] == m[2] && b[3] == m[3] {
			return true
		}
	}
	return false
}

// deobfuscateFont 用单字节 XOR 还原被混淆的字体；changed 表示是否发生了还原。
func deobfuscateFont(b []byte) (out []byte, changed bool) {
	if len(b) < 4 || looksLikeSFNT(b) {
		return b, false
	}
	for _, m := range sfntMagic {
		k := b[0] ^ m[0]
		if b[1]^k == m[1] && b[2]^k == m[2] && b[3]^k == m[3] {
			dec := make([]byte, len(b))
			for i, c := range b {
				dec[i] = c ^ k
			}
			if looksLikeSFNT(dec) {
				return dec, true
			}
		}
	}
	return b, false
}

// fontFamilyName 从 sfnt 的 name 表读出家族名（优先排版家族名/Windows 平台）。
func fontFamilyName(b []byte) string {
	if len(b) < 12 {
		return ""
	}
	if string(b[0:4]) == "ttcf" {
		if len(b) < 16 {
			return ""
		}
		off := int(binary.BigEndian.Uint32(b[12:16]))
		if off <= 0 || off >= len(b) {
			return ""
		}
		return fontFamilyName(b[off:])
	}
	numTables := int(binary.BigEndian.Uint16(b[4:6]))
	nameOff := -1
	for i := 0; i < numTables; i++ {
		rec := 12 + i*16
		if rec+16 > len(b) {
			break
		}
		if string(b[rec:rec+4]) == "name" {
			nameOff = int(binary.BigEndian.Uint32(b[rec+8 : rec+12]))
			break
		}
	}
	if nameOff < 0 || nameOff+6 > len(b) {
		return ""
	}
	count := int(binary.BigEndian.Uint16(b[nameOff+2 : nameOff+4]))
	strBase := nameOff + int(binary.BigEndian.Uint16(b[nameOff+4:nameOff+6]))
	best, bestScore := "", -1
	for i := 0; i < count; i++ {
		r := nameOff + 6 + i*12
		if r+12 > len(b) {
			break
		}
		pid := binary.BigEndian.Uint16(b[r : r+2])
		eid := binary.BigEndian.Uint16(b[r+2 : r+4])
		lid := binary.BigEndian.Uint16(b[r+4 : r+6])
		nid := binary.BigEndian.Uint16(b[r+6 : r+8])
		ln := int(binary.BigEndian.Uint16(b[r+8 : r+10]))
		off := int(binary.BigEndian.Uint16(b[r+10 : r+12]))
		if nid != 1 && nid != 16 {
			continue
		}
		if strBase+off+ln > len(b) || ln == 0 {
			continue
		}
		s := decodeNameString(b[strBase+off:strBase+off+ln], pid)
		if s == "" {
			continue
		}
		score := 0
		if nid == 16 {
			score += 100 // 优先排版家族名
		}
		if pid == 3 {
			score += 20
		}
		if eid == 1 {
			score += 10
		}
		if lid == 0x0409 {
			score += 5
		}
		if score > bestScore {
			best, bestScore = s, score
		}
	}
	return best
}

// decodeNameString 解码 name 表字符串（Windows/Unicode 用 UTF-16BE，其余按 Latin-1）。
func decodeNameString(b []byte, pid uint16) string {
	if pid == 3 || pid == 0 {
		if len(b)%2 != 0 {
			return ""
		}
		u := make([]uint16, len(b)/2)
		for i := range u {
			u[i] = binary.BigEndian.Uint16(b[i*2:])
		}
		return string(utf16.Decode(u))
	}
	r := make([]rune, len(b))
	for i, c := range b {
		r[i] = rune(c)
	}
	return string(r)
}

// RegisterFontFile 读取字体文件（必要时先还原 XOR 混淆）并私有注册到 GDI，
// 返回可用于 CreateFontW 的家族名。文件缺失或无法识别时 ok=false。
func RegisterFontFile(path string) (face string, ok bool) {
	raw, err := os.ReadFile(path)
	if err != nil || len(raw) < 4 {
		return "", false
	}
	b, changed := deobfuscateFont(raw)
	if !looksLikeSFNT(b) {
		return "", false
	}
	face = fontFamilyName(b)
	p := path
	if changed {
		// GDI 私有注册要从真实文件加载，混淆字体需写一份还原后的临时文件。
		tmp := filepath.Join(os.TempDir(), "ucgr-font-"+filepath.Base(path))
		if err := os.WriteFile(tmp, b, 0o644); err != nil {
			return face, false
		}
		p = tmp
	}
	if !RegisterFont(p) {
		return face, false
	}
	return face, true
}
