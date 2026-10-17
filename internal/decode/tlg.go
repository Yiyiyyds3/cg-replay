// Package decode 按需解码库内资产。
//
// 设计前提（上游 FINAL 1017「原样入库 + 按需解码」、NFR-3 无损）：
// 库内存原始格式（TLG / HG3 / SGCG / OGG…），只有显示时才解码，
// 禁止有损转码。所以本包只做「原始字节 → 像素」，不改变入库资产本身。
//
// 当前支持：KiriKiri TLG5（.tlg，本机 3991 个样本全是它）；
// JPEG（明文，或「整文件单字节 XOR 掩码」变种 —— 重返蓝鲸岛/海市蜃楼之馆实测 0xA4，
// 见 jpeg.go；GDI+ 直读掩码 JPEG 会报 Out of memory，所以必须自己解）。
// 未支持：TLG6 / TLG0 / HG3 —— 一律返回明确错误，不静默失败（对应 P1-7「缺资源告警」）。
//
// TLG5 结构（据 GARbro `ArcFormats/KiriKiri/ImageTLG.cs` 的 ReadV5 实现，已用本机样本核对）：
//
//	偏移  0..10  magic "TLG5.0\x00raw\x1a"
//	偏移  11     colors（3 或 4）
//	偏移  12..15 width   （小端）
//	偏移  16..19 height  （小端）
//	偏移  20..23 blockHeight（小端，每块行数）
//	偏移  24..   blockCount 个小端 dword 的「每块尺寸索引」——仅为原播放器寻址方便，
//
//	             解码用不到，直接跳过（blockCount = (height-1)/blockHeight + 1）
//	其后        逐块 × 逐通道：1 字节 mark + 4 字节小端 size + size 字节数据
//	             mark == 0 → 该通道数据是 TLG 私有 LZSS 流；否则为原样字节
package decode

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

var (
	tlgMagic5 = []byte("TLG5.0\x00raw\x1a")
	tlgMagic6 = []byte("TLG6.0\x00raw\x1a")
	tlgMagic0 = []byte("TLG0.0\x00sds\x1a")
)

// Image 是解码结果：每像素 4 字节，字节序 B、G、R、A（与 TLG 内部次序一致）。
type Image struct {
	Width  int
	Height int
	Colors int // 3 或 4
	Pixels []byte
	// Mask 记录的还原掩码（0 = 明文）。JPEG 掩码变种用得上：重新入库或
	// 与「原样入库」比对时要知道当初 XOR 了多少。
	Mask byte // 0 = 明文
}

// Decode 按魔数分派解码。
func Decode(data []byte) (*Image, error) {
	if len(data) < 11 {
		return nil, fmt.Errorf("文件过短（%d 字节），不是 TLG/JPEG", len(data))
	}
	switch {
	case bytes.HasPrefix(data, tlgMagic5):
		return decodeTLG5(data)
	case bytes.HasPrefix(data, tlgMagic6):
		return nil, fmt.Errorf("TLG6 暂未实现（需 Golomb 熵解码 + MED/AVG 滤镜）")
	case bytes.HasPrefix(data, tlgMagic0):
		return nil, fmt.Errorf("TLG0 暂未实现")
	}
	// JPEG：明文直接解；掩码变种（整文件单字节 XOR）先还原再解。
	if b, mask, ok := UnmaskJPEG(data); ok {
		return decodeJPEG(b, mask)
	}
	return nil, fmt.Errorf("非 TLG/JPEG 文件（魔数 %q）", string(data[:11]))
}

func decodeTLG5(data []byte) (*Image, error) {
	const (
		offColors = 11
		offWidth  = 12
		offHeight = 16
		offBlock  = 20
		offData   = 24
	)
	if len(data) < offData {
		return nil, fmt.Errorf("TLG5 头部截断（%d 字节）", len(data))
	}
	colors := int(data[offColors])
	if colors != 3 && colors != 4 {
		return nil, fmt.Errorf("TLG5 colors=%d 不受支持（应为 3 或 4）", colors)
	}
	width := int(binary.LittleEndian.Uint32(data[offWidth:]))
	height := int(binary.LittleEndian.Uint32(data[offHeight:]))
	blockHeight := int(binary.LittleEndian.Uint32(data[offBlock:]))
	if width <= 0 || height <= 0 || blockHeight <= 0 {
		return nil, fmt.Errorf("TLG5 尺寸非法：%dx%d，块高 %d", width, height, blockHeight)
	}
	// 跳过「每块尺寸索引」表（仅寻址用）。
	blockCount := (height-1)/blockHeight + 1
	p := offData + blockCount*4
	if p > len(data) {
		return nil, fmt.Errorf("TLG5 尺寸索引越界（块数 %d）", blockCount)
	}

	stride := width * 4
	img := &Image{Width: width, Height: height, Colors: colors, Pixels: make([]byte, height*stride)}
	text := make([]byte, 4096) // LZSS 滑动窗口，跨通道、跨块共享
	planes := make([][]byte, colors)
	for i := range planes {
		planes[i] = make([]byte, blockHeight*width+10)
	}

	z := 0         // LZSS 窗口写指针，跨通道、跨块延续
	prevline := -1 // 上一行的字节偏移；-1 表示图像首行
	for yb := 0; yb < height; yb += blockHeight {
		for c := 0; c < colors; c++ {
			if p+5 > len(data) {
				return nil, fmt.Errorf("TLG5 数据截断（块 y=%d 通道 %d）", yb, c)
			}
			mark := data[p]
			size := int(binary.LittleEndian.Uint32(data[p+1:]))
			p += 5
			if size < 0 || p+size > len(data) {
				return nil, fmt.Errorf("TLG5 通道数据越界（块 y=%d 通道 %d size=%d）", yb, c, size)
			}
			chunk := data[p : p+size]
			p += size
			if mark == 0 {
				z = lzssSlide(planes[c], chunk, text, z)
			} else {
				copy(planes[c], chunk)
			}
		}
		ylim := yb + blockHeight
		if ylim > height {
			ylim = height
		}
		outpos := 0
		for y := yb; y < ylim; y++ {
			cur := y * stride
			if prevline >= 0 {
				composeLine(img.Pixels, cur, prevline, planes, outpos, width, colors)
			} else {
				composeFirst(img.Pixels, cur, planes, outpos, width, colors)
			}
			outpos += width
			prevline = cur
		}
	}
	return img, nil
}

// composeFirst 还原图像首行：块内增量先按 g 做色差修正，再横向累加。
func composeFirst(px []byte, cur int, planes [][]byte, outpos, width, colors int) {
	pb, pg, pr, pa := byte(0), byte(0), byte(0), byte(0)
	for x := 0; x < width; x++ {
		g := planes[1][outpos+x]
		pb += planes[0][outpos+x] + g
		pg += g
		pr += planes[2][outpos+x] + g
		px[cur] = pb
		px[cur+1] = pg
		px[cur+2] = pr
		if colors == 4 {
			pa += planes[3][outpos+x]
			px[cur+3] = pa
		} else {
			px[cur+3] = 0xff
		}
		cur += 4
	}
}

// composeLine 还原非首行：纵向累加（叠加到上一行同 x 的像素上）。
func composeLine(px []byte, cur, upper int, planes [][]byte, outpos, width, colors int) {
	pc0, pc1, pc2, pc3 := byte(0), byte(0), byte(0), byte(0)
	for x := 0; x < width; x++ {
		c1 := planes[1][outpos+x]
		pc0 += planes[0][outpos+x] + c1
		pc1 += c1
		pc2 += planes[2][outpos+x] + c1
		px[cur] = pc0 + px[upper]
		px[cur+1] = pc1 + px[upper+1]
		px[cur+2] = pc2 + px[upper+2]
		if colors == 4 {
			pc3 += planes[3][outpos+x]
			px[cur+3] = pc3 + px[upper+3]
		} else {
			px[cur+3] = 0xff
		}
		cur += 4
		upper += 4
	}
}

// lzssSlide 解 TLG5 私有 LZSS 流（4KB 滑动窗口，跨调用共享 text 与写指针 r）。
// 返回更新后的窗口写指针，调用方须传回下一次调用（与原实现一致）。
func lzssSlide(out, in, text []byte, r int) int {
	var flags uint16
	o := 0
	for i := 0; i < len(in); {
		flags >>= 1
		if flags&256 == 0 {
			flags = uint16(in[i]) | 0xff00
			i++
			if i > len(in) {
				break
			}
		}
		if flags&1 != 0 { // 匹配串
			if i+1 >= len(in) {
				break
			}
			mpos := int(in[i]) | int(in[i+1]&0x0f)<<8
			mlen := int(in[i+1]&0xf0)>>4 + 3
			i += 2
			if mlen == 18 {
				if i >= len(in) {
					break
				}
				mlen += int(in[i])
				i++
			}
			for m := 0; m < mlen && o < len(out); m++ {
				c := text[mpos&4095]
				out[o] = c
				o++
				text[r&4095] = c
				r++
				mpos++
			}
		} else { // 字面量
			if i >= len(in) {
				break
			}
			c := in[i]
			i++
			if o < len(out) {
				out[o] = c
				o++
			}
			text[r&4095] = c
			r++
		}
	}
	return r
}
