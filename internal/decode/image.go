package decode

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"

	"golang.org/x/image/webp"
)

// DecodeImage 解 PNG/WebP（第三方与标准库）或 TLG5/JPEG/HG3（本包，自动还原 XOR 掩码）。
//
// 放在 decode 包（而不是 replay）是为了让适配层也能用：rmmz 适配器需要
// 读图片判断尺寸/不透明占比，若它反向依赖 replay 会与 replay 的测试形成
// import 环。
func DecodeImage(data []byte) (image.Image, error) {
	// RPG Maker MV/MZ 加密素材：32 字节头（16 签名 + 16 密钥）+ 图片数据。
	if len(data) > 32 && bytes.HasPrefix(data, []byte("RPGMV")) {
		if img, err := decodeRPGMV(data); err == nil {
			return img, nil
		}
	}
	if len(data) > 8 && bytes.Equal(data[1:4], []byte("PNG")) {
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("PNG 解码失败: %w", err)
		}
		return img, nil
	}
	// Ren'Py 时代的素材大量用 WebP（RIFF….WEBP 魔数）。
	if len(data) > 16 && bytes.Equal(data[:4], []byte("RIFF")) &&
		bytes.Equal(data[8:12], []byte("WEBP")) {
		img, err := webp.Decode(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("WebP 解码失败: %w", err)
		}
		return img, nil
	}
	im, err := Decode(data)
	if err != nil {
		return nil, err
	}
	// HG3 的 24 位图只有 3 字节/像素（Colors==3），不能按 4 字节取。
	ps := 4
	if im.Colors == 3 {
		ps = 3
	}
	return &bgraImage{w: im.Width, h: im.Height, ps: ps, pix: im.Pixels}, nil
}

// pngPrefix 是 PNG 文件的固定前 16 字节（签名 + IHDR 长度 + 类型）。
// RPG Maker 加密会把这段换成自己的 32 字节头，解密后要补回来。
var pngPrefix = []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d, 'I', 'H', 'D', 'R'}

// decodeRPGMV 还原 .rpgmvp / .png_。
//
// 两种实测形态都试一遍：
//  1. 头部被整段替换、图片数据仍是明文（部分汉化/解密包）；
//  2. 图片数据与 16 字节密钥循环异或（官方加密）。
func decodeRPGMV(data []byte) (image.Image, error) {
	body, key := data[32:], data[16:32]
	plain := make([]byte, len(body))
	for i := range body {
		plain[i] = body[i] ^ key[i%len(key)]
	}
	xorPrefixed := make([]byte, 0, len(pngPrefix)+len(plain))
	xorPrefixed = append(xorPrefixed, pngPrefix...)
	xorPrefixed = append(xorPrefixed, plain...)
	prefixed := make([]byte, 0, len(pngPrefix)+len(body))
	prefixed = append(prefixed, pngPrefix...)
	prefixed = append(prefixed, body...)
	for _, cand := range [][]byte{prefixed, xorPrefixed, plain} {
		if img, err := png.Decode(bytes.NewReader(cand)); err == nil {
			return img, nil
		}
	}
	return nil, fmt.Errorf("RPGMV 图片解码失败")
}

// bgraImage 把 decode.Image 的 B、G、R[、A] 字节序适配成 image.Image。
// ps 为每像素字节数（3 = BGR 无 alpha，4 = BGRA）。
type bgraImage struct {
	w, h int
	ps   int
	pix  []byte
}

func (b *bgraImage) ColorModel() color.Model { return color.RGBAModel }
func (b *bgraImage) Bounds() image.Rectangle { return image.Rect(0, 0, b.w, b.h) }

func (b *bgraImage) At(x, y int) color.Color {
	if x < 0 || y < 0 || x >= b.w || y >= b.h {
		return color.RGBA{}
	}
	i := (y*b.w + x) * b.ps
	if i+b.ps > len(b.pix) {
		return color.RGBA{}
	}
	if b.ps == 3 {
		return color.RGBA{R: b.pix[i+2], G: b.pix[i+1], B: b.pix[i], A: 255}
	}
	return color.RGBA{R: b.pix[i+2], G: b.pix[i+1], B: b.pix[i], A: b.pix[i+3]}
}
