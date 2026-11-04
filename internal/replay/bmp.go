// bmp.go —— 最小 BMP 解码（24/32bpp 无压缩 + 8bpp 调色板）。
//
// NScripter 系游戏的图大量是 BMP：散装目录里的（寒蝉/海猫 bmp\）、
// NSA 归档里的 LZSS 解压产物、SPB 解出来的（固定 24bpp）。
// Go 标准库没有 BMP，x/image 不引入（保持零第三方依赖）。
package replay

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
)

// decodeBMP 解 24/32bpp 无压缩与 8bpp 调色板 BMP（BI_RGB）。
// 其余变体（RLE、16bpp、PNG 内嵌）不支持——本仓库语料用不到。
func decodeBMP(data []byte) (image.Image, error) {
	if len(data) < 54 || data[0] != 'B' || data[1] != 'M' {
		return nil, fmt.Errorf("不是 BMP")
	}
	pixelOff := int(binary.LittleEndian.Uint32(data[10:14]))
	headerSize := int(binary.LittleEndian.Uint32(data[14:18]))
	if headerSize < 12 || pixelOff > len(data) {
		return nil, fmt.Errorf("BMP 头非法")
	}
	width := int(int32(binary.LittleEndian.Uint32(data[18:22])))
	height := int(int32(binary.LittleEndian.Uint32(data[22:26])))
	bpp := int(binary.LittleEndian.Uint16(data[28:30]))
	compression := binary.LittleEndian.Uint32(data[30:34])
	flip := true
	if height < 0 { // top-down 位图
		flip = false
		height = -height
	}
	if width <= 0 || height <= 0 || width > 32768 || height > 32768 {
		return nil, fmt.Errorf("BMP 尺寸异常 %dx%d", width, height)
	}
	if compression != 0 {
		return nil, fmt.Errorf("BMP 压缩类型 %d 不支持", compression)
	}

	switch bpp {
	case 24, 32:
		ps := bpp / 8
		rowSize := (width*ps + 3) / 4 * 4
		img := image.NewRGBA(image.Rect(0, 0, width, height))
		for y := 0; y < height; y++ {
			srcY := y
			if flip {
				srcY = height - 1 - y
			}
			row := pixelOff + srcY*rowSize
			if row+width*ps > len(data) {
				break
			}
			for x := 0; x < width; x++ {
				o := row + x*ps
				b := data[o]
				g := data[o+1]
				r := data[o+2]
				a := uint8(255)
				if ps == 4 {
					a = data[o+3]
				}
				img.SetRGBA(x, y, color.RGBA{r, g, b, a})
			}
		}
		return img, nil
	case 8:
		// 调色板紧跟信息头（40 字节时在 data[54:]，BI_RGB_HEADER 12 字节时在 data[38:]）。
		palOff := 14 + headerSize
		palN := 1 << 8
		if palOff+palN*4 > len(data) {
			palN = (len(data) - palOff) / 4
		}
		if palN <= 0 {
			return nil, fmt.Errorf("BMP 调色板缺失")
		}
		pal := make(color.Palette, palN)
		for i := 0; i < palN; i++ {
			o := palOff + i*4
			pal[i] = color.RGBA{data[o+2], data[o+1], data[o], 255}
		}
		rowSize := (width + 3) / 4 * 4
		img := image.NewPaletted(image.Rect(0, 0, width, height), pal)
		for y := 0; y < height; y++ {
			srcY := y
			if flip {
				srcY = height - 1 - y
			}
			row := pixelOff + srcY*rowSize
			if row+width > len(data) {
				break
			}
			copy(img.Pix[y*img.Stride:y*img.Stride+width], data[row:row+width])
		}
		return img, nil
	}
	return nil, fmt.Errorf("BMP %d bpp 不支持", bpp)
}
