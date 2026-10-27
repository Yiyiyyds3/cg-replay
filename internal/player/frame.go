package player

import (
	"image"
	"image/color"
	"math"
)

// RenderFrame 合成一帧画面：图层 + 对话框底色。
//
// 文字不在这里画（GDI 提供更好的字形与抗锯齿），调用方拿到 Frame() 后
// 自行用 win32.TextDrawer 把 Texts 画到画布上。
func RenderFrame(p *Player, load Loader, w, h int) (*image.RGBA, []string, error) {
	f := p.Frame()
	rgba, drawn, err := Compose(f, load, w, h)
	if err != nil {
		return nil, nil, err
	}
	for _, box := range f.Texts {
		if box.Frame == "" || !box.Visible {
			continue
		}
		img, lerr := load(ResolveRef(p.Event(), box.Frame))
		if lerr != nil {
			continue
		}
		DrawBoxBackground(rgba, box, img, f.QuakeX, f.QuakeY)
	}
	return rgba, drawn, nil
}

// VideoProvider 按引用与相对时间取影片帧（平台层注入 ffmpeg 实现）。
type VideoProvider func(ref string, tMs int64) (image.Image, error)

// RenderFrameWithVideo 在 RenderFrame 之上叠影片：OP/ED 铺满画布。
//
// 视频不进 Compose —— player 包不依赖 ffmpeg，取帧由调用方注入。
func RenderFrameWithVideo(p *Player, load Loader, video VideoProvider, w, h int) (*image.RGBA, []string, error) {
	rgba, drawn, err := RenderFrame(p, load, w, h)
	if err != nil {
		return rgba, drawn, err
	}
	f := p.Frame()
	if f.Video == nil || video == nil || f.Video.Ref == "" {
		return rgba, drawn, nil
	}
	img, verr := video(f.Video.Ref, f.Video.Now)
	if verr != nil {
		return rgba, append(drawn, "✗ video←"+f.Video.Ref), nil
	}
	blitFit(rgba, img, f.QuakeX, f.QuakeY)
	return rgba, append(drawn, "video←"+f.Video.Ref), nil
}

// blitFit 等比铺满画布（留黑边），按最近邻缩放。
func blitFit(dst *image.RGBA, src image.Image, qx, qy int) {
	db := dst.Bounds()
	sb := src.Bounds()
	if sb.Dx() <= 0 || sb.Dy() <= 0 || db.Dx() <= 0 || db.Dy() <= 0 {
		return
	}
	scale := math.Min(float64(db.Dx())/float64(sb.Dx()), float64(db.Dy())/float64(sb.Dy()))
	dw, dh := int(float64(sb.Dx())*scale), int(float64(sb.Dy())*scale)
	ox := (db.Dx()-dw)/2 + qx
	oy := (db.Dy()-dh)/2 + qy
	for y := 0; y < dh; y++ {
		sy := sb.Min.Y + sb.Dy()*y/dh
		dy := oy + y
		if dy < db.Min.Y || dy >= db.Max.Y {
			continue
		}
		for x := 0; x < dw; x++ {
			sx := sb.Min.X + sb.Dx()*x/dw
			dx := ox + x
			if dx < db.Min.X || dx >= db.Max.X {
				continue
			}
			r, g, b, a := src.At(sx, sy).RGBA()
			if a == 0 {
				continue
			}
			dst.SetRGBA(dx, dy, color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), 0xFF})
		}
	}
}
