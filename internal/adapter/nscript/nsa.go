// nsa.go —— NScripter 归档（NSA / SAR）读取。
//
// 格式（ONScripter SarReader.readArchive / DirectReader.cpp 实测核对）：
//   - u16 BE 文件数、u32 BE base_offset（整体大端）；
//   - 条目：名字（\0 结尾）+ u8 压缩类型（仅 NSA）+ u32 BE 相对偏移 +
//     u32 BE 段长 + u32 BE 原始长（仅 NSA）；
//   - 压缩类型：0 = 无压缩、1 = SPB、2 = LZSS、3 = NBZ（bzip2）。
//
// 只做索引与条目解压，不做整体解包——回放按名取图就够了。
package nscript

import (
	"bytes"
	"compress/bzip2"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"strings"
)

// NSA 条目压缩类型（DirectReader.cpp 实测：SPB 头是宽高两 u16 BE，
// NBZ 头是 u32 BE 原始长 + bzip2 流）。
const (
	compNone = 0
	compSPB  = 1
	compLZSS = 2
	compNBZ  = 3
)

// NsaEntry 归档索引里的一条。
type NsaEntry struct {
	Name    string // 归档内路径（ '\' 原样保留，查找时归一化）
	Comp    uint8
	Offset  uint32 // 段数据偏移（绝对：base + 相对值已在解析时加好）
	Length  uint32 // 段长
	OrigLen uint32 // 解压后长度（无压缩时 = Length）
}

// NsaArchive 一个已解析索引的归档。
type NsaArchive struct {
	Path    string
	Entries map[string]NsaEntry   // 键 = 小写名字，分隔符归一为 '/'
	base    map[string][]NsaEntry // 键 = 小写 basename（去扩展名），同名多路径都留
	order   []string
}

// ParseNsa 解析归档索引。认不出（读越界、条目偏移链乱套）返回错误。
func ParseNsa(path string) (*NsaArchive, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseNsaData(data, path)
}

// ParseNsaData 从内存解析索引（测试用）。
func ParseNsaData(data []byte, path string) (*NsaArchive, error) {
	if len(data) < 6 {
		return nil, fmt.Errorf("nsa: %s 太短（%d 字节）", path, len(data))
	}
	num := int(binary.BigEndian.Uint16(data[0:2]))
	base := binary.BigEndian.Uint32(data[2:6])
	a := &NsaArchive{
		Path:    path,
		Entries: map[string]NsaEntry{},
		base:    map[string][]NsaEntry{},
	}
	off := 6
	for i := 0; i < num; i++ {
		nameEnd := bytes.IndexByte(data[off:], 0)
		if nameEnd < 0 {
			return nil, fmt.Errorf("nsa: %s 条目 %d 名字没有 \\0 结尾", path, i)
		}
		name := string(data[off : off+nameEnd])
		off += nameEnd + 1
		if off+13 > len(data) {
			return nil, fmt.Errorf("nsa: %s 条目 %d 头部越界", path, i)
		}
		comp := data[off]
		rel := binary.BigEndian.Uint32(data[off+1 : off+5])
		length := binary.BigEndian.Uint32(data[off+5 : off+9])
		orig := binary.BigEndian.Uint32(data[off+9 : off+13])
		off += 13
		if comp > compNBZ {
			return nil, fmt.Errorf("nsa: %s 条目 %d 压缩类型 %d 越界（不是 NSA 或已加密）", path, i, comp)
		}
		e := NsaEntry{
			Name:    name,
			Comp:    comp,
			Offset:  base + rel,
			Length:  length,
			OrigLen: orig,
		}
		if comp == compNone {
			e.OrigLen = length
		}
		key := nsaKey(name)
		if _, dup := a.Entries[key]; !dup {
			a.Entries[key] = e
			a.order = append(a.order, key)
		}
		// basename 索引：脚本引用有时只给裸名（省目录与扩展名）。
		stem := key
		if i := strings.LastIndex(stem, "/"); i >= 0 {
			stem = stem[i+1:]
		}
		if i := strings.LastIndex(stem, "."); i > 0 {
			stem = stem[:i]
		}
		if stem != "" {
			a.base[stem] = append(a.base[stem], e)
		}
	}
	// 偏移链 sanity：无压缩条目读头不能落在文件外（BE/LE 认错时会大面积越界）。
	if len(a.Entries) > 0 {
		bad := 0
		for _, k := range a.order {
			e := a.Entries[k]
			if e.Comp == compNone && (e.Offset+e.Length > uint32(len(data))) {
				bad++
			}
		}
		if bad*2 > len(a.order) {
			return nil, fmt.Errorf("nsa: %s 过半无压缩条目越界（%d/%d），格式可能认错了", path, bad, len(a.order))
		}
	}
	return a, nil
}

// nsaKey 归一化查找键：小写 + '\' → '/'。
func nsaKey(name string) string {
	return strings.ReplaceAll(strings.ToLower(name), "\\", "/")
}

// Lookup 按名字找条目（大小写不敏感，/ 与 \ 等价，可省略扩展名）。
func (a *NsaArchive) Lookup(name string) (NsaEntry, bool) {
	if e, ok := a.Entries[nsaKey(name)]; ok {
		return e, true
	}
	// 脚本引用常省略扩展名：补 .bmp / .png / .jpg 再试。
	for _, ext := range []string{".bmp", ".png", ".jpg", ".jpeg"} {
		if e, ok := a.Entries[nsaKey(name+ext)]; ok {
			return e, true
		}
	}
	// 还没有：按 basename（去扩展名）匹配——引用只给裸名时命的就是它。
	key := nsaKey(name)
	if i := strings.LastIndex(key, "/"); i >= 0 {
		key = key[i+1:]
	}
	if i := strings.LastIndex(key, "."); i > 0 {
		key = key[:i]
	}
	if cands := a.base[key]; len(cands) > 0 {
		return cands[0], true
	}
	return NsaEntry{}, false
}

// ReadEntry 读取并解压一个条目（返回解压后的完整字节）。
func (a *NsaArchive) ReadEntry(e NsaEntry) ([]byte, error) {
	f, err := os.Open(a.Path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	raw := make([]byte, e.Length)
	if _, err := f.ReadAt(raw, int64(e.Offset)); err != nil && e.Length > 0 {
		return nil, fmt.Errorf("nsa: 读 %s 段失败: %w", e.Name, err)
	}
	switch e.Comp {
	case compNone:
		return raw, nil
	case compSPB:
		return DecodeSPB(raw)
	case compLZSS:
		return DecodeLZSS(raw, int(e.OrigLen)), nil
	case compNBZ:
		// NBZ = u32 BE 原始长度 + bzip2 流（decodeNBZ：readLong 后 BZ2_bzReadOpen）。
		if len(raw) <= 4 {
			return nil, fmt.Errorf("nsa: NBZ %s 数据太短", e.Name)
		}
		out := make([]byte, e.OrigLen)
		r := bzip2.NewReader(bytes.NewReader(raw[4:]))
		if _, err := io.ReadFull(r, out); err != nil {
			return nil, fmt.Errorf("nsa: NBZ 解压 %s 失败: %w", e.Name, err)
		}
		return out, nil
	}
	return nil, fmt.Errorf("nsa: %s 压缩类型 %d 未实现", e.Name, e.Comp)
}

// ---- SPB：NScripter 自家位图压缩（DirectReader::decodeSPB）----

// DecodeSPB 解 SPB → BMP（24bpp，带 54 字节文件头）。
//
// 格式：u16 BE 宽、u16 BE 高，之后每通道（B/G/R）一段 RLE+增量流，
// 解出的像素按蛇形（偶数行左→右、奇数行右→左）填进 BMP 正文。
func DecodeSPB(raw []byte) ([]byte, error) {
	if len(raw) < 4 {
		return nil, fmt.Errorf("nsa: SPB 数据太短")
	}
	width := int(binary.BigEndian.Uint16(raw[0:2]))
	height := int(binary.BigEndian.Uint16(raw[2:4]))
	if width <= 0 || height <= 0 || width > 16384 || height > 16384 {
		return nil, fmt.Errorf("nsa: SPB 尺寸异常 %dx%d", width, height)
	}
	widthPad := (4 - width*3%4) % 4
	totalSize := (width*3+widthPad)*height + 54

	br := &bitReader{data: raw[4:]}
	buf := make([]byte, totalSize)
	copy(buf, bmpHeader(totalSize, width, height))

	channels := make([]byte, width*height+4)
	pix := buf[54:]
	for ch := 0; ch < 3; ch++ {
		count := 0
		c := br.bits(8)
		channels[count] = byte(c)
		count++
		for count < width*height {
			n := br.bits(3)
			switch {
			case n == 0:
				for j := 0; j < 4 && count < len(channels); j++ {
					channels[count] = byte(c)
					count++
				}
				continue
			case n == 7:
				emit(br, &c, channels, &count, br.bits(1)+1)
			default:
				emit(br, &c, channels, &count, n+2)
			}
		}
		// 蛇形填充：从最后一行末尾起，奇数行反向。
		p := (width*3+widthPad)*(height-1) + ch
		s := 0
		for j := 0; j < height; j++ {
			if j&1 == 1 {
				for k := 0; k < width; k++ {
					if p >= 0 && p < len(pix) {
						pix[p] = channels[s]
					}
					s++
					p -= 3
				}
				p -= width*3 + widthPad - 3
			} else {
				for k := 0; k < width; k++ {
					if p >= 0 && p < len(pix) {
						pix[p] = channels[s]
					}
					s++
					p += 3
				}
				p -= width*3 + widthPad + 3
			}
		}
	}
	return buf, nil
}

// emit 按 SPB 增量语义往 channels 里写 m 个像素值（DirectReader.cpp 原样）。
func emit(br *bitReader, c *int, channels []byte, count *int, m int) {
	for j := 0; j < 4; j++ {
		if *count >= len(channels) {
			return
		}
		if m == 8 {
			*c = br.bits(8)
		} else {
			k := br.bits(m)
			if k&1 == 1 {
				*c += (k >> 1) + 1
			} else {
				*c -= k >> 1
			}
		}
		channels[*count] = byte(*c)
		*count++
	}
}

// bmpHeader 拼一个 24bpp 无压缩 BMP 的 54 字节文件头。
func bmpHeader(totalSize, width, height int) []byte {
	h := make([]byte, 54)
	h[0], h[1] = 'B', 'M'
	h[2] = byte(totalSize)
	h[3] = byte(totalSize >> 8)
	h[4] = byte(totalSize >> 16)
	h[5] = byte(totalSize >> 24)
	h[10] = 54
	h[14] = 40
	h[18] = byte(width)
	h[19] = byte(width >> 8)
	h[22] = byte(height)
	h[23] = byte(height >> 8)
	h[26] = 1
	h[28] = 24
	h[34] = byte(totalSize - 54)
	h[35] = byte((totalSize - 54) >> 8)
	h[36] = byte((totalSize - 54) >> 16)
	h[37] = byte((totalSize - 54) >> 24)
	return h
}

// ---- LZSS：NScripter 变体（EI=8, EJ=4, N=256, F=17）----

// DecodeLZSS 解 LZSS（DirectReader::decodeLZSS 原样移植）。
func DecodeLZSS(raw []byte, origLen int) []byte {
	out := make([]byte, 0, origLen)
	const (
		ei = 8
		ej = 4
		n  = 1 << ei // 256
		f  = (1 << ej) + 1
	)
	br := &bitReader{data: raw}
	var ring [n]byte
	r := n - f
	for len(out) < origLen {
		if br.bits(1) == 1 {
			c := br.bits(8)
			if c < 0 {
				break
			}
			out = append(out, byte(c))
			ring[r] = byte(c)
			r = (r + 1) & (n - 1)
		} else {
			i := br.bits(ei)
			j := br.bits(ej)
			if i < 0 || j < 0 {
				break
			}
			for k := 0; k <= j+1 && len(out) < origLen; k++ {
				c := ring[(i+k)&(n-1)]
				out = append(out, c)
				ring[r] = c
				r = (r + 1) & (n - 1)
			}
		}
	}
	return out
}

// bitReader MSB 在前的位读取器（getbit 移植）。
type bitReader struct {
	data  []byte
	pos   int
	mask  byte
	eof   bool
	cache byte
}

func (b *bitReader) bits(n int) int {
	x := 0
	for i := 0; i < n; i++ {
		if b.mask == 0 {
			if b.pos >= len(b.data) {
				b.eof = true
				return -1
			}
			b.cache = b.data[b.pos]
			b.pos++
			b.mask = 128
		}
		x <<= 1
		if b.cache&b.mask != 0 {
			x++
		}
		b.mask >>= 1
	}
	return x
}
