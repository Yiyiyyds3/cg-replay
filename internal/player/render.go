package player

// render.go —— 把 Frame 的层状态合成为一张 RGBA（文字由平台层用 GDI 画，
// 见 internal/platform/win32；本文件只做图像合成，便于无窗口单测）。
//
// KAG 图层 z 序（与 adapter 的 layerIndexOf 同口径）：base(0) < layer0(1) <
// layer1(2) < … < message0(message 序号+1000)，屏幕坐标越靠后画的越在上层。
//
// 裁剪（clipleft/cliptop/clipwidth/clipheight）：本作只在 [img] 宏用到 5 处。
// 这里取「源图裁剪」读法：从图层素材的 (ClipL, ClipT) 起取 ClipW×ClipH，
// 贴到图层位置 (X, Y)。【待人工比对】另一读法是「屏幕窗口」（贴整图再裁屏幕
// 矩形），两者在 clipleft==left 时结果相近。

import (
	"image"
	"math"
	"sort"

	"ucgr/internal/ir"
)

// LayerZ KAG 默认层序（用于绘制排序；数值越小越先画 = 越靠下层）。
func LayerZ(id string) int {
	switch {
	case id == "base" || id == "stage":
		return 0
	case len(id) > 5 && id[:5] == "layer":
		if n, ok := atoiTail(id[5:]); ok {
			return 1 + n
		}
	case len(id) > 7 && id[:7] == "message":
		if n, ok := atoiTail(id[7:]); ok {
			return 1000 + n
		}
	}
	return 500
}

func atoiTail(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
		n = n*10 + int(s[i]-'0')
	}
	return n, true
}

// Cached 包一层素材缓存（同一张图只解码一次）。
func Cached(load Loader) Loader {
	cache := map[string]image.Image{}
	return func(rel string) (image.Image, error) {
		if img, ok := cache[rel]; ok {
			return img, nil
		}
		img, err := load(rel)
		if err != nil {
			return nil, err
		}
		cache[rel] = img
		return img, nil
	}
}

// Compose 把 f 的图层合成到 w×h 的 RGBA 上（w/h <= 0 时按最底层可见层尺寸自动定）。
// 返回合成图与实际画上去的层清单（"<layer>←<ref>"）。缺素材的层跳过并登记。
func Compose(f Frame, load Loader, w, h int) (*image.RGBA, []string, error) {
	sts := make([]LayerState, 0, len(f.Layers))
	for _, st := range f.Layers {
		if st.Visible && st.Resolved != "" {
			sts = append(sts, st)
		}
	}
	// 稳定 z 序（同 z 保持台本登记顺序）
	sort.SliceStable(sts, func(i, j int) bool { return LayerZ(sts[i].Layer) < LayerZ(sts[j].Layer) })

	if w <= 0 || h <= 0 {
		if len(sts) == 0 {
			return image.NewRGBA(image.Rect(0, 0, 1, 1)), nil, nil
		}
		w, h = 1, 1
	}

	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	var drawn []string
	for _, st := range sts {
		img, err := load(st.Resolved)
		if err != nil {
			drawn = append(drawn, "✗ "+st.Layer+"←"+st.Resolved)
			continue
		}
		if w <= 1 && h <= 1 { // 自动画布
			b := img.Bounds()
			dst = image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
			w, h = b.Dx(), b.Dy()
		}
		if st.HasClip {
			img = cropImage(img, st.ClipL, st.ClipT, st.ClipW, st.ClipH)
		}
		blendLayer(dst, img, int(math.Round(st.X)), int(math.Round(st.Y)),
			int(math.Round(float64(f.QuakeX))), int(math.Round(float64(f.QuakeY))), st.Opacity)
		drawn = append(drawn, st.Layer+"←"+st.Resolved)
	}
	return dst, drawn, nil
}

// cropImage 取素材的一块矩形（越界部分按素材边界裁）。
func cropImage(img image.Image, x, y, w, h float64) image.Image {
	b := img.Bounds()
	x0, y0 := b.Min.X+int(math.Round(x)), b.Min.Y+int(math.Round(y))
	x1, y1 := x0+int(math.Round(w)), y0+int(math.Round(h))
	if x0 < b.Min.X {
		x0 = b.Min.X
	}
	if y0 < b.Min.Y {
		y0 = b.Min.Y
	}
	if x1 > b.Max.X {
		x1 = b.Max.X
	}
	if y1 > b.Max.Y {
		y1 = b.Max.Y
	}
	if x1 <= x0 || y1 <= y0 {
		return image.NewRGBA(image.Rect(0, 0, 1, 1))
	}
	type subImager interface {
		SubImage(r image.Rectangle) image.Image
	}
	if si, ok := img.(subImager); ok {
		return si.SubImage(image.Rect(x0, y0, x1, y1))
	}
	out := image.NewRGBA(image.Rect(0, 0, x1-x0, y1-y0))
	for yy := y0; yy < y1; yy++ {
		for xx := x0; xx < x1; xx++ {
			out.Set(xx-x0, yy-y0, img.At(xx, yy))
		}
	}
	return out
}

// blendLayer alpha-over 贴图（dst 不透明底；整体不透明度 opacity 0~1；越界裁剪）。
func blendLayer(dst *image.RGBA, src image.Image, x, y, dx, dy int, opacity float64) {
	if opacity <= 0 {
		return
	}
	if opacity > 1 {
		opacity = 1
	}
	x -= dx
	y -= dy
	sb, db := src.Bounds(), dst.Bounds()
	sw, sh := sb.Dx(), sb.Dy()
	// 先把贴图矩形裁到画布内（源为 SubImage/大图时都能早退）
	x0, y0 := x, y
	x1, y1 := x+sw, y+sh
	if x0 < db.Min.X {
		x0 = db.Min.X
	}
	if y0 < db.Min.Y {
		y0 = db.Min.Y
	}
	if x1 > db.Max.X {
		x1 = db.Max.X
	}
	if y1 > db.Max.Y {
		y1 = db.Max.Y
	}
	if x1 <= x0 || y1 <= y0 {
		return
	}
	for yy := y0; yy < y1; yy++ {
		for xx := x0; xx < x1; xx++ {
			sr, sg, sb8, sa := src.At(sb.Min.X+xx-x, sb.Min.Y+yy-y).RGBA()
			if sa == 0 {
				continue
			}
			a := float64(sa>>8) / 255 * opacity
			if a <= 0 {
				continue
			}
			i := dst.PixOffset(xx, yy)
			dr, dg, dbb := float64(dst.Pix[i]), float64(dst.Pix[i+1]), float64(dst.Pix[i+2])
			dst.Pix[i] = uint8(clampF(dr + (float64(sr>>8)-dr)*a))
			dst.Pix[i+1] = uint8(clampF(dg + (float64(sg>>8)-dg)*a))
			dst.Pix[i+2] = uint8(clampF(dbb + (float64(sb8>>8)-dbb)*a))
			dst.Pix[i+3] = 255
		}
	}
}

func clampF(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return v
}

// DrawBoxBackground 在合成图上画消息层的底色 / 框图（文字由平台层用 GDI 画）。
//
// KAG 规则（Config.tjs 注释）：指定了 frame 图时用框图（原尺寸贴，不缩放），
// 此时 color/opacity 无效；没指定 frame 图时用 frameColor + frameOpacity 填矩形。
func DrawBoxBackground(dst *image.RGBA, box TextBox, frameImg image.Image, dx, dy int) {
	if !box.Visible {
		return
	}
	if box.Frame != "" && frameImg != nil {
		blendLayer(dst, frameImg, int(math.Round(box.X)), int(math.Round(box.Y)), dx, dy, 1)
		return
	}
	if box.FrameOpacity == 0 || !box.HasGeom {
		return
	}
	x0 := int(math.Round(box.X)) - dx
	y0 := int(math.Round(box.Y)) - dy
	w := int(math.Round(box.W))
	h := int(math.Round(box.H))
	alpha := float64(box.FrameOpacity) / 255
	db := dst.Bounds()
	for y := y0; y < y0+h; y++ {
		if y < db.Min.Y || y >= db.Max.Y {
			continue
		}
		for x := x0; x < x0+w; x++ {
			if x < db.Min.X || x >= db.Max.X {
				continue
			}
			i := dst.PixOffset(x, y)
			dst.Pix[i] = uint8(clampF(float64(dst.Pix[i])*(1-alpha) + float64(box.FrameColor[0])*alpha))
			dst.Pix[i+1] = uint8(clampF(float64(dst.Pix[i+1])*(1-alpha) + float64(box.FrameColor[1])*alpha))
			dst.Pix[i+2] = uint8(clampF(float64(dst.Pix[i+2])*(1-alpha) + float64(box.FrameColor[2])*alpha))
			dst.Pix[i+3] = 255
		}
	}
}

// ComposeEventLayer 便捷：给定了素材表就直接合成（测试用）。
func ComposeEventLayer(ev *ir.Event, f Frame, load Loader, w, h int) (*image.RGBA, []string, error) {
	return Compose(f, load, w, h)
}
