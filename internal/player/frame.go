package player

import "image"

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
