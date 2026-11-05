// ajp.go —— AliceSoft AJP 图像解码。
//
// AJP = JPEG 本体 + 独立 alpha 掩码的组合容器（System 4 系）。
// 兰斯10 实测分布（CG.afa 前 400 个 .ajp）：372 个带 zlib 掩码、28 个无掩码，
// 没有 webp / pms8 掩码，所以这里只实现 zlib 分支，其余给出明确错误。
//
// 结构（据 alice-tools libsys4 ajp.c，已用真实语料核对）：
//
//	偏移 0..3   magic "AJP\0"
//	偏移 4..11  未用
//	偏移 12..15 width        （小端）
//	偏移 16..19 height
//	偏移 20..23 jpeg_off     （文件内绝对偏移）
//	偏移 24..27 jpeg_size
//	偏移 28..31 mask_off
//	偏移 32..35 mask_size
//
// 加密：JPEG 块与 mask 块的**开头各 16 字节**与固定密钥 XOR（注意不是整文件，
// 块内 16 字节之后就是明文 JPEG / zlib 流）：
//
//	5d 91 ae 87 4a 56 41 cd 83 ec 4c 92 b5 cb 16 34
//
// mask：解密后首字节 0x78 即 zlib 流，解压恰为 width*height 字节，
// 每字节就是该像素的 alpha（实测值域 0..255，系统图大量透明区为 0）。
// 无 mask 时视为完全不透明。
package decode

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

var ajpKey = [16]byte{
	0x5d, 0x91, 0xae, 0x87, 0x4a, 0x56, 0x41, 0xcd,
	0x83, 0xec, 0x4c, 0x92, 0xb5, 0xcb, 0x16, 0x34,
}

// decodeAJP 把 AJP 字节解成 BGRA 像素（Image.Colors 恒 4）。
func decodeAJP(data []byte) (*Image, error) {
	if len(data) < 36 {
		return nil, fmt.Errorf("AJP 头部截断（%d 字节）", len(data))
	}
	w := int(binary.LittleEndian.Uint32(data[12:]))
	h := int(binary.LittleEndian.Uint32(data[16:]))
	joff := int(binary.LittleEndian.Uint32(data[20:]))
	jsize := int(binary.LittleEndian.Uint32(data[24:]))
	moff := int(binary.LittleEndian.Uint32(data[28:]))
	msize := int(binary.LittleEndian.Uint32(data[32:]))
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("AJP 尺寸非法：%dx%d", w, h)
	}
	if joff+jsize > len(data) || moff+msize > len(data) {
		return nil, fmt.Errorf("AJP 数据块越界：jpeg[%d+%d] mask[%d+%d] 文件 %d 字节",
			joff, jsize, moff, msize, len(data))
	}

	jpeg := make([]byte, jsize)
	copy(jpeg, data[joff:joff+jsize])
	xorAJPTail16(jpeg)
	if _, _, ok := UnmaskJPEG(jpeg); !ok {
		return nil, fmt.Errorf("AJP 内嵌 JPEG 校验失败（SOI/EOI 不符）")
	}
	img, err := decodeJPEG(jpeg, 0)
	if err != nil {
		return nil, fmt.Errorf("AJP 内嵌 JPEG 解码失败：%w", err)
	}
	if img.Width != w || img.Height != h {
		return nil, fmt.Errorf("AJP 头尺寸 %dx%d 与 JPEG 尺寸 %dx%d 不符", w, h, img.Width, img.Height)
	}

	// mask → alpha；没有 mask 或解不出来时保持 JPEG 的不透明结果。
	if msize > 0 {
		mask := make([]byte, msize)
		copy(mask, data[moff:moff+msize])
		xorAJPTail16(mask)
		alpha, err := decodeAJPMask(mask, w*h)
		if err != nil {
			return nil, err
		}
		for i, a := range alpha {
			img.Pixels[i*4+3] = a
		}
	}
	return img, nil
}

// xorAJPTail16 就地对块首 16 字节与固定密钥异或（AJP 的全部「加密」）。
func xorAJPTail16(block []byte) {
	n := 16
	if len(block) < n {
		n = len(block)
	}
	for i := 0; i < n; i++ {
		block[i] ^= ajpKey[i]
	}
}

// decodeAJPMask 解出 width*height 字节的 alpha 平面。
// zlib 流（首字节 0x78）是兰斯10 唯一出现的形态；pms8/webp 理论上存在，
// 真遇到时明确报错而不是输出错误的不透明图。
func decodeAJPMask(mask []byte, npix int) ([]byte, error) {
	switch {
	case len(mask) >= 4 && bytes.Equal(mask[:4], []byte("PMS8")):
		return nil, fmt.Errorf("AJP mask 为 PMS8 格式，暂未实现")
	case len(mask) >= 4 && bytes.Equal(mask[:4], []byte("WEBP")):
		return nil, fmt.Errorf("AJP mask 为 WebP 格式，暂未实现")
	case len(mask) >= 1 && mask[0] == 0x78:
		alpha, err := inflateExact(mask, npix)
		if err != nil {
			return nil, fmt.Errorf("AJP mask 解压失败：%w", err)
		}
		return alpha, nil
	default:
		return nil, fmt.Errorf("AJP mask 形态不明（首字节 %02x %02x %02x %02x）",
			mask[0], mask[1], mask[2], mask[3])
	}
}
