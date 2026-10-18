// Package win32 是播放器的 Windows 平台层：GDI 画布（DIB）+ 窗口/消息循环。
//
// 分工：图层合成在 Go 里做（internal/player.Compose），本包只负责
//
//	①把 RGBA 位图拷进 32bpp DIB（CreateDIBSection，顶朝下）；
//	②用 GDI 画消息框框图与文字（CJK 直接走系统字体，不需要第三方库）；
//	③窗口：RegisterClassExW/CreateWindowExW + PeekMessage 循环（~60fps），
//	  WM_PAINT 里 StretchBlt 到窗口 DC（画布 1600×1200 通常大于屏幕，按比例缩）。
//
// 全部走 syscall 调 gdi32/user32，不引第三方依赖（本机无网络，见交接文档）。
package win32

import (
	"image"
	"syscall"
	"unsafe"

	"ucgr/internal/player"
)

var (
	gdi32 = syscall.NewLazyDLL("gdi32.dll")

	procCreateCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	procCreateDIBSection   = gdi32.NewProc("CreateDIBSection")
	procSelectObject       = gdi32.NewProc("SelectObject")
	procDeleteObject       = gdi32.NewProc("DeleteObject")
	procDeleteDC           = gdi32.NewProc("DeleteDC")
	procBitBlt             = gdi32.NewProc("BitBlt")
	procStretchBlt         = gdi32.NewProc("StretchBlt")
	procSetStretchBltMode  = gdi32.NewProc("SetStretchBltMode")
	procCreateFontW        = gdi32.NewProc("CreateFontW")
	procSetTextColor       = gdi32.NewProc("SetTextColor")
	procSetBkMode          = gdi32.NewProc("SetBkMode")
	// DrawTextW / GetTextExtentPoint32W 在 user32.dll（user32 句柄见 window.go）。
	procDrawTextW           = user32.NewProc("DrawTextW")
	procGetTextExtentPointW = user32.NewProc("GetTextExtentPoint32W")
	procCreateSolidBrush    = gdi32.NewProc("CreateSolidBrush")
	procAddFontResourceExW  = gdi32.NewProc("AddFontResourceExW")
	procRectangle           = gdi32.NewProc("Rectangle")
)

type bitmapInfoHeader struct {
	biSize          uint32
	biWidth         int32
	biHeight        int32
	biPlanes        uint16
	biBitCount      uint16
	biCompression   uint32
	biSizeImage     uint32
	biXPelsPerMeter int32
	biYPelsPerMeter int32
	biClrUsed       uint32
	biClrImportant  uint32
}

type bitmapInfo struct {
	header bitmapInfoHeader
	colors [1]uint32
}

// Canvas 32bpp 顶朝下的 DIB 画布（内存序是 B,G,R,A；与 image.RGBA 的 R,G,B,A 相反，
// 因此 BlitRGBA/Image 都要交换 R/B）。
// 另外：GDI 绘制不维护 DIB 的 alpha 字节，存 PNG 前要调 ForceOpaque。
type Canvas struct {
	W, H int
	dc   uintptr
	bmp  uintptr
	old  uintptr
	bits unsafe.Pointer
}

// NewCanvas 建 w×h 的内存画布。
func NewCanvas(w, h int) (*Canvas, error) {
	if w <= 0 || h <= 0 {
		w, h = 1, 1
	}
	dc, _, err := procCreateCompatibleDC.Call(0)
	if dc == 0 {
		return nil, err
	}
	bi := bitmapInfo{header: bitmapInfoHeader{
		biSize:        uint32(unsafe.Sizeof(bitmapInfoHeader{})),
		biWidth:       int32(w),
		biHeight:      -int32(h), // 负高度 = 顶朝下
		biPlanes:      1,
		biBitCount:    32,
		biCompression: 0, // BI_RGB
	}}
	var bits unsafe.Pointer
	bmp, _, err := procCreateDIBSection.Call(dc, uintptr(unsafe.Pointer(&bi)), 0, /*DIB_RGB_COLORS*/
		uintptr(unsafe.Pointer(&bits)), 0, 0)
	if bmp == 0 {
		procDeleteDC.Call(dc)
		return nil, err
	}
	old, _, _ := procSelectObject.Call(dc, bmp)
	return &Canvas{W: w, H: h, dc: dc, bmp: bmp, old: old, bits: bits}, nil
}

// DC 返回画布的设备上下文（GDI 画框图/文字用）。
func (c *Canvas) DC() uintptr { return c.dc }

// Free 释放 GDI 对象。
func (c *Canvas) Free() {
	if c.dc == 0 {
		return
	}
	if c.old != 0 {
		procSelectObject.Call(c.dc, c.old)
	}
	if c.bmp != 0 {
		procDeleteObject.Call(c.bmp)
	}
	procDeleteDC.Call(c.dc)
	c.dc, c.bmp, c.old, c.bits = 0, 0, 0, nil
}

// Pix 画布的原始像素（BGRA，top-down，行距 W*4）。
func (c *Canvas) Pix() []byte {
	if c.bits == nil {
		return nil
	}
	return unsafe.Slice((*byte)(c.bits), c.W*c.H*4)
}

// BlitRGBA 把一张 RGBA 图拷进画布（整块覆盖；尺寸不匹配时按左上角对齐并裁剪）。
//
// 注意：Windows 的 32bpp DIB 内存字节序是 B、G、R、A，而 Go 的 image.RGBA
// 是 R、G、B、A，所以这里必须交换 R/B 两个通道，否则画面上红蓝互换。
func (c *Canvas) BlitRGBA(img *image.RGBA) {
	pix := c.Pix()
	if pix == nil || img == nil {
		return
	}
	b := img.Bounds()
	for y := 0; y < c.H && y < b.Dy(); y++ {
		srow := img.PixOffset(b.Min.X, b.Min.Y+y)
		drow := y * c.W * 4
		n := c.W
		if n > b.Dx() {
			n = b.Dx()
		}
		for x := 0; x < n; x++ {
			s := srow + x*4
			d := drow + x*4
			pix[d+0] = img.Pix[s+2] // B
			pix[d+1] = img.Pix[s+1] // G
			pix[d+2] = img.Pix[s+0] // R
			pix[d+3] = img.Pix[s+3] // A
		}
	}
}

// ForceOpaque 把画布所有像素的 alpha 置为 255。
//
// GDI 的文字/图形绘制不维护 32bpp DIB 的 alpha 通道（画过的地方 alpha 常常是 0），
// 直接存 PNG 会得到一层透明字。存图前调一次即可。
func (c *Canvas) ForceOpaque() {
	pix := c.Pix()
	if pix == nil {
		return
	}
	for i := 3; i < len(pix); i += 4 {
		pix[i] = 0xFF
	}
}

// Image 把画布读成 RGBA（存 PNG 用；同样要交换 DIB 的 B/R 字节序）。
func (c *Canvas) Image() *image.RGBA {
	out := image.NewRGBA(image.Rect(0, 0, c.W, c.H))
	src := c.Pix()
	if src == nil {
		return out
	}
	for i := 0; i+3 < len(src); i += 4 {
		out.Pix[i+0] = src[i+2] // R
		out.Pix[i+1] = src[i+1] // G
		out.Pix[i+2] = src[i+0] // B
		out.Pix[i+3] = src[i+3] // A
	}
	return out
}

// ---------------------------------------------------------------
// 字体与文字
// ---------------------------------------------------------------

// RegisterFont 私有注册字体文件（AddFontResourceExW + FR_PRIVATE），返回是否成功。
func RegisterFont(path string) bool {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return false
	}
	const frPrivate = 0x10
	r, _, _ := procAddFontResourceExW.Call(uintptr(unsafe.Pointer(p)), frPrivate, 0)
	return r != 0
}

// TextStyle 文字样式（本作 Config.tjs 实参：白色 + 粗体 + 黑阴影，见 player.Options）。
type TextStyle struct {
	Face      string
	SizePx    float64
	Bold      bool
	Color     [3]uint8
	Shadow    [3]uint8
	HasShadow bool
	Align     uint32 // GDI DT_LEFT/DT_CENTER/DT_RIGHT
}

// DefaultTextStyle 本作默认文字样式。
func DefaultTextStyle(face string, sizePx float64) TextStyle {
	return TextStyle{
		Face: face, SizePx: sizePx, Bold: true,
		Color: [3]uint8{255, 255, 255}, Shadow: [3]uint8{0, 0, 0},
		HasShadow: true, Align: dtLeft,
	}
}

const (
	dtLeft       = 0x00000000
	dtCenter     = 0x00000001
	dtRight      = 0x00000002
	dtVCenter    = 0x00000004
	dtWordBreak  = 0x00000010
	dtSingleLine = 0x00000020
	dtNoPrefix   = 0x00000800
)

// TextDrawer 复用字体句柄（一次播放只建一次）。
type TextDrawer struct {
	fonts map[string]uintptr
	style TextStyle
}

// NewTextDrawer 建一个字体缓存（style 是默认文字样式）。
func NewTextDrawer(style TextStyle) *TextDrawer {
	return &TextDrawer{fonts: map[string]uintptr{}, style: style}
}

// Style 默认文字样式（可改）。
func (d *TextDrawer) TextStyle() *TextStyle { return &d.style }

// Free 释放字体句柄。
func (d *TextDrawer) Free() {
	for _, h := range d.fonts {
		procDeleteObject.Call(h)
	}
	d.fonts = map[string]uintptr{}
}

func (d *TextDrawer) font(st TextStyle) uintptr {
	key := st.Face + "|" + itoa(int(st.SizePx)) + "|" + boolKey(st.Bold)
	if h, ok := d.fonts[key]; ok {
		return h
	}
	face, err := syscall.UTF16PtrFromString(st.Face)
	if err != nil {
		return 0
	}
	weight := int32(400)
	if st.Bold {
		weight = 700
	}
	const (
		gb2312Charset    = 0x86
		cleartypeQuality = 5
		defaultPitch     = 0
	)
	h, _, _ := procCreateFontW.Call(
		uintptr(int32(st.SizePx)), 0, 0, 0, uintptr(weight),
		0, 0, 0, gb2312Charset, 0, 0, cleartypeQuality, defaultPitch,
		uintptr(unsafe.Pointer(face)))
	d.fonts[key] = h
	return h
}

// Draw 在 dc 上画一段文字（换行由 GDI 按字符处理，适合 CJK）；
// origin 是文本区左上角（像素），textW/textH 是文本区尺寸。
func (d *TextDrawer) Draw(dc uintptr, txt string, x, y, textW, textH int, st TextStyle) {
	if txt == "" || dc == 0 {
		return
	}
	font := d.font(st)
	if font == 0 {
		return
	}
	old, _, _ := procSelectObject.Call(dc, font)
	defer procSelectObject.Call(dc, old)

	procSetBkMode.Call(dc, 1) // TRANSPARENT
	u16, err := syscall.UTF16FromString(txt)
	if err != nil {
		return
	}
	flags := uintptr(dtWordBreak | dtNoPrefix | st.Align)
	if st.HasShadow {
		rc := rect{int32(x + 2), int32(y + 2), int32(x + 2 + textW), int32(y + 2 + textH)}
		procSetTextColor.Call(dc, colorRef(st.Shadow))
		procDrawTextW.Call(dc, uintptr(unsafe.Pointer(&u16[0])), uintptr(int32(len(u16)-1)),
			uintptr(unsafe.Pointer(&rc)), flags)
	}
	rc := rect{int32(x), int32(y), int32(x + textW), int32(y + textH)}
	procSetTextColor.Call(dc, colorRef(st.Color))
	procDrawTextW.Call(dc, uintptr(unsafe.Pointer(&u16[0])), uintptr(int32(len(u16)-1)),
		uintptr(unsafe.Pointer(&rc)), flags)
}

type rect struct {
	left, top, right, bottom int32
}

// colorRef COLORREF = 0x00BBGGRR。
func colorRef(c [3]uint8) uintptr {
	return uintptr(uint32(c[0]) | uint32(c[1])<<8 | uint32(c[2])<<16)
}

// FillRect 用纯色填充（消息框底色，frameOpacity 非 0 时）。
func FillRect(dc uintptr, x, y, w, h int, c [3]uint8, opacity uint8) {
	if dc == 0 || w <= 0 || h <= 0 || opacity == 0 {
		return
	}
	r := rect{int32(x), int32(y), int32(x + w), int32(y + h)}
	// 混合色：GDI 没有直接的不透明度填充，这里按比例混到黑色底上近似
	mix := func(v uint8) uint8 { return uint8(uint32(v) * uint32(opacity) / 255) }
	brush, _, _ := procCreateSolidBrush.Call(colorRef([3]uint8{mix(c[0]), mix(c[1]), mix(c[2])}))
	if brush == 0 {
		return
	}
	defer procDeleteObject.Call(brush)
	old, _, _ := procSelectObject.Call(dc, brush)
	defer procSelectObject.Call(dc, old)
	procRectangle.Call(dc, uintptr(r.left), uintptr(r.top), uintptr(r.right), uintptr(r.bottom))
}

// DrawFrameImage 把框图素材按原尺寸贴到 (x,y)（KAG 的 frame 图不缩放，见 Config.tjs 注释）。
func DrawFrameImage(dc uintptr, src *Canvas, x, y int) {
	if dc == 0 || src == nil {
		return
	}
	procBitBlt.Call(dc, uintptr(int32(x)), uintptr(int32(y)), uintptr(int32(src.W)), uintptr(int32(src.H)),
		src.DC(), 0, 0, ropSrcCopy)
}

// Stretch 把画布缩放贴到目标 dc（窗口按比例适配屏幕）。
func Stretch(dstDC uintptr, src *Canvas, x, y, w, h int) {
	if dstDC == 0 || src == nil {
		return
	}
	procSetStretchBltMode.Call(dstDC, 4) // HALFTONE
	procStretchBlt.Call(dstDC, uintptr(int32(x)), uintptr(int32(y)), uintptr(int32(w)), uintptr(int32(h)),
		src.DC(), 0, 0, uintptr(int32(src.W)), uintptr(int32(src.H)), ropSrcCopy)
}

const ropSrcCopy = 0x00CC0020

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

func boolKey(b bool) string {
	if b {
		return "b"
	}
	return "n"
}

// DrawBoxes 画所有消息层的文字（scale 为缩放；绘图坐标按 scale 放大）。
func (d *TextDrawer) DrawBoxes(dc uintptr, boxes []player.TextBox, scale float64) {
	if scale <= 0 {
		scale = 1
	}
	for _, box := range boxes {
		if !box.Visible || len(box.Lines) == 0 {
			continue
		}
		st := d.style
		if box.FontSize > 0 {
			st.SizePx = box.FontSize
		}
		x, y, w, h := BoxTextRect(box)
		d.Draw(dc, TextOfBox(box),
			int(float64(x)*scale), int(float64(y)*scale),
			int(float64(w)*scale), int(float64(h)*scale), st)
	}
}

// TextOfBox 把消息层的各行拼成一段（GDI 自己按宽度换行）。
func TextOfBox(box player.TextBox) string {
	out := ""
	for i, ln := range box.Lines {
		if i > 0 {
			out += "\r\n"
		}
		out += ln.Text
	}
	return out
}

// BoxTextRect 消息层的文字区（几何 + margin），未给几何时用兜底框。
func BoxTextRect(box player.TextBox) (x, y, w, h int) {
	if !box.HasGeom {
		return 32, 32, 1280 - 64, 960 - 64
	}
	x = int(box.X + box.MarginL)
	y = int(box.Y + box.MarginT)
	w = int(box.W - box.MarginL - box.MarginR)
	h = int(box.H - box.MarginT - box.MarginB)
	if w < 0 {
		w = 0
	}
	if h < 0 {
		h = 0
	}
	return
}
