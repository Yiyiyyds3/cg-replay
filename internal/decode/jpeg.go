package decode

// jpeg.go —— JPEG 解码（含「整文件单字节 XOR 掩码」变种）。
//
// 背景：部分 KiriKiri 系游戏的图片资产带一层**整文件单字节 XOR 掩码**——
// 重返蓝鲸岛全树 1185 个 .jpg 的掩码是 0xA4（JFIF 头 FF D8 FF E0 存成
// 5B 7C 5B 44、文件尾 FF D9 存成 5B 7D），海市蜃楼之馆同款；同作 962 个
// .png 不带掩码。GDI+ 直读这种文件会报 Out of memory，必须先还原。
//
// 掩码在**全文件**上一致（已核对首尾），所以「由首字节反推掩码 k，再用
// SOI/EOI 两头确认」即可判定，不需要猜、也不会误伤明文图片。
//
// 注意：JPEG 本身是有损格式，这里走标准库解码（YCbCr → RGB 有舍入）。
// 「禁止有损转码」指的是**入库资产不被改写**，不是要求 JPEG 逐字节可逆。

import (
	"bytes"
	"fmt"
	"image/jpeg"
)

// JPEG 结构标记：SOI（FF D8 FF）与 EOI（FF D9）。
var (
	jpegSOI = []byte{0xFF, 0xD8, 0xFF}
	jpegEOI = []byte{0xFF, 0xD9}
)

// UnmaskJPEG 判定输入是不是 JPEG，并抹掉可能的「整文件单字节 XOR 掩码」。
//
// 返回可用于解码的字节、掩码值、是否命中。**明文 JPEG 也算命中**
// （ok=true、k=0、字节不变），这样 Decode 只看 ok 就能分派到 JPEG 解码。
// 判定方式：由首字节反推掩码 k，再要求 SOI（FF D8 FF）与 EOI（FF D9）两头
// 都对上；任一处不符即判否并原样返回（未命中时 k 恒为 0）。
func UnmaskJPEG(data []byte) ([]byte, byte, bool) {
	n := len(data)
	if n < 4 {
		return data, 0, false
	}
	k := data[0] ^ jpegSOI[0] // 由首字节反推；明文 JPEG 得 k == 0
	if data[1]^k != jpegSOI[1] || data[2]^k != jpegSOI[2] ||
		data[n-2]^k != jpegEOI[0] || data[n-1]^k != jpegEOI[1] {
		return data, 0, false
	}
	if k == 0 {
		return data, 0, true // 明文：无需改写
	}
	out := make([]byte, n)
	for i, b := range data {
		out[i] = b ^ k
	}
	return out, k, true
}

// decodeJPEG 把（已还原的）JPEG 字节解成 BGRA 像素。
// mask 仅用于错误信息与 Image.Mask（0 表示明文）。
func decodeJPEG(b []byte, mask byte) (*Image, error) {
	src, err := jpeg.Decode(bytes.NewReader(b))
	if err != nil {
		if mask != 0 {
			return nil, fmt.Errorf("JPEG 解码失败（已还原掩码 0x%02X）：%w", mask, err)
		}
		return nil, fmt.Errorf("JPEG 解码失败：%w", err)
	}
	bnd := src.Bounds()
	w, h := bnd.Dx(), bnd.Dy()
	pix := make([]byte, w*h*4)
	i := 0
	for y := bnd.Min.Y; y < bnd.Max.Y; y++ {
		for x := bnd.Min.X; x < bnd.Max.X; x++ {
			r, g, bl, a := src.At(x, y).RGBA() // 16 位非预乘
			pix[i] = byte(bl >> 8)
			pix[i+1] = byte(g >> 8)
			pix[i+2] = byte(r >> 8)
			pix[i+3] = byte(a >> 8)
			i += 4
		}
	}
	return &Image{Width: w, Height: h, Colors: 4, Pixels: pix, Mask: mask}, nil
}
