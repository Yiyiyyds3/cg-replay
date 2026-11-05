// qnt.go —— AliceSoft QNT 图像解码。
//
// QNT 是 System 3/4 系的原生图像格式（Wren 1998）：zlib 压缩 + 2×2 平面
// 重排 + 减法预测。兰斯10 的 CG（.qnt）与 DCF 差分的底图/差分数据全是它。
//
// 结构（据 alice-tools libsys4 qnt.c，已用真实语料核对；兰斯10 实测为
// 旧布局，hdr_size=68）：
//
//	偏移 0..3   magic "QNT\0"
//	偏移 4..7   rsv0 —— 布局判别：==0 为旧布局（hdr 固定 48、字段自 @8 起），
//	            非 0 为新布局（hdr_size @8、字段自 @12 起；兰斯10 rsv0=2）
//	───── 旧布局（rsv0==0）────────── 新布局（rsv0!=0，兰斯10）─────
//	x0            @8                    @12
//	y0            @12                   @16
//	width         @16                   @20
//	height        @20                   @24
//	bpp           @24                   @28
//	rsv           @28                   @32
//	pixel_size    @32                   @36
//	alpha_size    @36                   @40
//
//	其后 hdr_size 起：pixel_size 字节 zlib（像素）+ alpha_size 字节 zlib（alpha）
//
// 像素解码（extract_pixel）：
//  1. zlib 解压到 ((w+1)&~1) * ((h+1)&~1) * 3 字节 —— 宽高都补成偶数，
//     按「平面（i=2,1,0）→ 2×2 块行 → 2×2 块列」排布，每块 4 字节 =
//     (y,x) (y+1,x) (y,x+1) (y+1,x+1)
//  2. 重排回 BGR 行优先图（raw 第 0 个平面写通道 2，即 raw 是 R,G,B 序）
//  3. 减法预测还原：首行 pic[x] = pic[x-1]-pic[x]；首列 pic[y] = 上方-当前；
//     其余 pic = ((上+左)>>1) - 当前（全部 8 位截断）
//
// alpha 解码（extract_alpha）与像素不同：不做 2×2 重排，只有行优先 +
// 奇数宽度行尾补 1 字节（ALDExplorer 只补宽不补高），预测同型。
// 无 alpha 数据时按不透明处理（libsys4 同款）。
package decode

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"
)

// inflateExact 把 zlib 流解压到恰好 want 字节（多退少补报错）。
// QNT 像素平面要求精确长度（libsys4 zlib_decompress_exact 同款）。
func inflateExact(src []byte, want int) ([]byte, error) {
	out, err := inflateAtLeast(src, want)
	if err != nil {
		return nil, err
	}
	if len(out) != want {
		return nil, fmt.Errorf("解压长度 %d != 目标 %d", len(out), want)
	}
	return out, nil
}

// inflateAtLeast 把 zlib 流解压到**至少** want 字节，多余部分忽略。
// QNT 的 alpha 平面用这个：libsys4 的 extract_alpha 走非精确解压
// （ALDExplorer 只补宽不补高，流尾可能带多余字节），要求精确会在
// 其它 System 4 游戏上误报溢出。
func inflateAtLeast(src []byte, want int) ([]byte, error) {
	if want < 0 {
		return nil, fmt.Errorf("非法解压目标长度 %d", want)
	}
	zr, err := zlib.NewReader(bytes.NewReader(src))
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	out := make([]byte, want)
	if _, err := io.ReadFull(zr, out); err != nil {
		return nil, fmt.Errorf("解压不足（要 %d 字节）：%w", want, err)
	}
	_, _ = io.Copy(io.Discard, zr) // 忽略尾部多余字节
	return out, nil
}

// decodeQNT 把 QNT 字节解成 BGRA 像素（Image.Colors 恒 4）。
func decodeQNT(data []byte) (*Image, error) {
	if len(data) < 40 {
		return nil, fmt.Errorf("QNT 头部截断（%d 字节）", len(data))
	}
	rsv0 := int32(binary.LittleEndian.Uint32(data[4:]))
	var hdrSize, w, h, bpp, pixelSize, alphaSize int
	if rsv0 == 0 {
		// 旧布局：hdr 固定 48，字段自 @8 起
		hdrSize = 48
		w, h = s32(data[16:]), s32(data[20:])
		bpp = s32(data[24:])
		pixelSize, alphaSize = s32(data[32:]), s32(data[36:])
	} else {
		// 新布局：hdr_size @8，字段自 @12 起（兰斯10 rsv0=2、hdr=68）
		hdrSize = int(int32(binary.LittleEndian.Uint32(data[8:])))
		w, h = s32(data[20:]), s32(data[24:])
		bpp = s32(data[28:])
		pixelSize, alphaSize = s32(data[36:]), s32(data[40:])
	}
	if w <= 0 || h <= 0 || w > 1<<16 || h > 1<<16 {
		return nil, fmt.Errorf("QNT 尺寸非法：%dx%d", w, h)
	}
	if bpp != 24 {
		return nil, fmt.Errorf("QNT bpp=%d 不受支持（应 24）", bpp)
	}
	if hdrSize < 0 || hdrSize > len(data) {
		return nil, fmt.Errorf("QNT hdr_size=%d 越界（文件 %d 字节）", hdrSize, len(data))
	}
	body := data[hdrSize:]
	if pixelSize < 0 || alphaSize < 0 || pixelSize+alphaSize > len(body) {
		return nil, fmt.Errorf("QNT 数据区越界：pixel %d + alpha %d > 剩余 %d",
			pixelSize, alphaSize, len(body))
	}

	img := &Image{Width: w, Height: h, Colors: 4, Pixels: make([]byte, w*h*4)}
	if pixelSize > 0 {
		pic := make([]byte, w*h*3)
		if err := qntPixel(pic, body[:pixelSize], w, h); err != nil {
			return nil, err
		}
		// BGR 行优先 → BGRA
		for p := 0; p < w*h; p++ {
			img.Pixels[p*4+0] = pic[p*3+0]
			img.Pixels[p*4+1] = pic[p*3+1]
			img.Pixels[p*4+2] = pic[p*3+2]
			img.Pixels[p*4+3] = 0xFF
		}
	}
	if alphaSize > 0 {
		alpha := make([]byte, w*h)
		if err := qntAlpha(alpha, body[pixelSize:pixelSize+alphaSize], w, h); err != nil {
			return nil, err
		}
		for p := 0; p < w*h; p++ {
			img.Pixels[p*4+3] = alpha[p]
		}
	}
	return img, nil
}

func s32(b []byte) int { return int(int32(binary.LittleEndian.Uint32(b))) }

// qntPixel 解出 w*h*3 的 BGR 行优先像素（含预测还原）。
func qntPixel(pic, comp []byte, w, h int) error {
	pw, ph := (w+1)&^1, (h+1)&^1
	raw, err := inflateExact(comp, pw*ph*3)
	if err != nil {
		return fmt.Errorf("QNT 像素解压失败：%w", err)
	}
	// 2×2 平面重排：raw 平面序 R,G,B → pic 通道 2,1,0（BGR）
	// （x/y 在 C 里是外层声明、跨循环使用；Go 需显式提到 for 外）
	j := 0
	for i := 2; i >= 0; i-- {
		var x, y int
		for y = 0; y < h-1; y += 2 {
			for x = 0; x < w-1; x += 2 {
				pic[(y*w+x)*3+i] = raw[j]
				pic[((y+1)*w+x)*3+i] = raw[j+1]
				pic[(y*w+x+1)*3+i] = raw[j+2]
				pic[((y+1)*w+x+1)*3+i] = raw[j+3]
				j += 4
			}
			if x != w { // 奇宽：尾列只有上/下两像素
				pic[(y*w+x)*3+i] = raw[j]
				pic[((y+1)*w+x)*3+i] = raw[j+1]
				j += 4
			}
		}
		if y != h { // 奇高：尾行
			for x = 0; x < w-1; x += 2 {
				pic[(y*w+x)*3+i] = raw[j]
				pic[(y*w+x+1)*3+i] = raw[j+2]
				j += 4
			}
			if x != w {
				pic[(y*w+x)*3+i] = raw[j]
				j += 4
			}
		}
	}
	// 减法预测还原（8 位截断）
	if w > 1 {
		for x := 1; x < w; x++ {
			for c := 0; c < 3; c++ {
				pic[x*3+c] = pic[(x-1)*3+c] - pic[x*3+c]
			}
		}
	}
	if h > 1 {
		for y := 1; y < h; y++ {
			for c := 0; c < 3; c++ {
				pic[y*w*3+c] = pic[((y-1)*w)*3+c] - pic[y*w*3+c]
			}
			for x := 1; x < w; x++ {
				for c := 0; c < 3; c++ {
					up := int(pic[((y-1)*w+x)*3+c])
					left := int(pic[(y*w+x-1)*3+c])
					// C 语义：加法提升为 int 再移位，赋值截断回 8 位
					pic[(y*w+x)*3+c] = byte((up+left)>>1 - int(pic[(y*w+x)*3+c]))
				}
			}
		}
	}
	return nil
}

// qntAlpha 解出 w*h 的 alpha 平面（行优先 + 奇宽补位，预测还原）。
func qntAlpha(alpha, comp []byte, w, h int) error {
	raw, err := inflateAtLeast(comp, ((w+1)&^1)*h)
	if err != nil {
		return fmt.Errorf("QNT alpha 解压失败：%w", err)
	}
	i := 0
	alpha[0] = raw[i]
	i++
	for x := 1; x < w; x++ {
		alpha[x] = alpha[x-1] - raw[i]
		i++
	}
	if w%2 == 1 {
		i++
	}
	for y := 1; y < h; y++ {
		alpha[y*w] = alpha[(y-1)*w] - raw[i]
		i++
		for x := 1; x < w; x++ {
			left, up := int(alpha[y*w+x-1]), int(alpha[(y-1)*w+x])
			alpha[y*w+x] = byte((left+up)>>1 - int(raw[i]))
			i++
		}
		if w%2 == 1 {
			i++
		}
	}
	return nil
}
