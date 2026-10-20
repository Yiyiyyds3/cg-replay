// Package replay 把 IR 时间轴合成为「可播放演出」的帧。
//
// 对应 docs/05_流水线设计.md §0.5 的 S3「单事件回放」：最简方式回放
// （命令行合成图）——输入 = 裁剪资源 + IR，输出 = 可播放演出。
//
// 只吃 IR，不碰脚本：这正是 03 文档「回放器只吃 IR」的验证点。
// 素材侧解码统一走 internal/decode（TLG5 / JPEG / 整文件单字节 XOR 掩码 JPEG），
// PNG 走标准库 —— 所以「重返蓝鲸岛」那种 0xA4 掩码 JPEG 不需要任何外部工具。
package replay

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/color/palette"
	"image/draw"
	"image/gif"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"ucgr/internal/decode"
	"ucgr/internal/ir"
)

// LayerState 某一时刻某个图层的可见状态。
type LayerState struct {
	LayerID string
	Ref     string // 资源相对路径（空 = 该层当前没有画面）
	Visible bool
	X, Y    float64
	HasPos  bool // 是否明确给过 left/top（KAG 允许省略 → 继承原位置）
	Opacity float64
	Z       int
}

// StatesAt 推出 tMs 时刻的图层状态：事件初态（ev.Layers）+ 时间轴前缀指令。
//
// 「事件初态」口径与 build_ir 一致：Layer.Asset/Transform/Visible 记的是事件初态，
// 之后的变化只在时间轴上 —— 所以这里从初态出发、把 t_ms ≤ tMs 的指令按序吃掉。
func StatesAt(ev *ir.Event, tMs int64) []LayerState {
	st := map[string]*LayerState{}
	var order []string
	get := func(id string) *LayerState {
		if s, ok := st[id]; ok {
			return s
		}
		s := &LayerState{LayerID: id, Opacity: 1}
		st[id] = s
		order = append(order, id)
		return s
	}

	for i := range ev.Layers {
		l := &ev.Layers[i]
		s := get(l.LayerID)
		s.Visible = l.Visible
		s.Z = l.Z
		if l.Asset != nil {
			s.Ref = l.Asset.OrigPath
		}
		if l.Transform != nil {
			if l.Transform.X != nil {
				s.X = *l.Transform.X
				s.HasPos = true
			}
			if l.Transform.Y != nil {
				s.Y = *l.Transform.Y
				s.HasPos = true
			}
			if l.Transform.Opacity != nil {
				s.Opacity = *l.Transform.Opacity
			}
		}
	}

	for i := range ev.Timeline {
		op := &ev.Timeline[i]
		if op.TMs > tMs {
			break // V3 保证时间轴非降序
		}
		s, ok := st[op.Target]
		if !ok {
			continue // 只回放已登记的层（文本框层等不在 layers 里，不画）
		}
		p, _ := op.Params.(map[string]interface{})
		switch op.Op {
		case "image_show":
			s.Visible = true
			if r, ok := p["ref"].(string); ok && r != "" {
				s.Ref = resolveRef(ev, r)
			}
			if v, ok := numParam(p, "left"); ok {
				s.X, s.HasPos = v, true
			}
			if v, ok := numParam(p, "top"); ok {
				s.Y, s.HasPos = v, true
			}
			if v, ok := numParam(p, "opacity"); ok {
				s.Opacity = v
			}
			if v, ok := numParam(p, "z"); ok {
				s.Z = int(v)
			}
		case "image_hide":
			s.Visible = false
		}
	}

	out := make([]LayerState, 0, len(order))
	for _, id := range order {
		out = append(out, *st[id])
	}
	return out
}

// resolveRef 把 KAG 的 storage 名对到事件资产表里的相对路径。
//
// KAG 里 [image storage=kaigan01_bg] 往往不带扩展名，引擎自己去找 .jpg/.png/…
// （重返蓝鲸岛的 71kaigan01.ks 就是这个形态）。IR 的时间轴上记的是**脚本原文**，
// 所以回放时按「精确 → 去扩展名 → 文件名（去扩展名）」三级匹配 Assets。
// 匹配不上就原样返回，交给 Loader 碰运气 —— 不编造路径。
func resolveRef(ev *ir.Event, ref string) string {
	if ev == nil || ref == "" {
		return ref
	}
	slash := func(s string) string { return strings.ToLower(strings.ReplaceAll(s, "\\", "/")) }
	stripExt := func(s string) string { return strings.TrimSuffix(s, filepath.Ext(s)) }
	base := func(s string) string {
		if i := strings.LastIndex(s, "/"); i >= 0 {
			return s[i+1:]
		}
		return s
	}
	norm := slash(ref)

	paths := make([]string, 0, len(ev.Assets))
	for _, a := range ev.Assets {
		if a.OrigPath != "" {
			paths = append(paths, a.OrigPath)
		}
	}
	for _, p := range paths { // 1. 精确
		if slash(p) == norm {
			return p
		}
	}
	for _, p := range paths { // 2. 去扩展名相等
		if stripExt(slash(p)) == norm {
			return p
		}
	}
	for _, p := range paths { // 3. 文件名（去扩展名）相等
		if stripExt(base(slash(p))) == stripExt(norm) {
			return p
		}
	}
	return ref
}

// numParam 从 IR params 里取数值：JSON 反序列化后可能是 float64，
// 但适配器也可能原样塞字符串（KAG 属性值），两种都认。
func numParam(p map[string]interface{}, key string) (float64, bool) {
	v, ok := p[key]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case string:
		f, err := strconv.ParseFloat(n, 64)
		if err != nil {
			return 0, false
		}
		return f, true
	}
	return 0, false
}

// Loader 把 IR 里的相对路径读成像素。
type Loader func(rel string) (image.Image, error)

// DirLoader 从素材根目录按相对路径读图（带缓存）。素材根可以是原始解包目录，
// 也可以是 S2 的裁剪产物目录 —— 后者能顺带证明「裁剪没伤到演出」。
func DirLoader(root string) Loader {
	cache := map[string]image.Image{}
	return func(rel string) (image.Image, error) {
		if img, ok := cache[rel]; ok {
			return img, nil
		}
		path := filepath.Join(root, filepath.FromSlash(rel))
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		img, err := DecodeImage(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", rel, err)
		}
		cache[rel] = img
		return img, nil
	}
}

// DecodeImage 解 PNG（标准库）或 TLG5/JPEG（internal/decode，自动还原 XOR 掩码）。
func DecodeImage(data []byte) (image.Image, error) {
	if len(data) > 8 && bytes.Equal(data[1:4], []byte("PNG")) {
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("PNG 解码失败: %w", err)
		}
		return img, nil
	}
	im, err := decode.Decode(data)
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
		return color.RGBA{R: b.pix[i+2], G: b.pix[i+1], B: b.pix[i], A: 0xFF}
	}
	return color.RGBA{R: b.pix[i+2], G: b.pix[i+1], B: b.pix[i], A: b.pix[i+3]}
}

// Compose 按 z 升序把可见层叠到画布上。
//
// canvasW/canvasH 传 0 时按「z 最小的可见层」的尺寸自动定画布
// （KAG 里那就是背景；比写死 1600×1200 稳）。
// 返回合成图与「实际画上去的层」清单（供人工比对核对）。
func Compose(sts []LayerState, load Loader, canvasW, canvasH int) (*image.RGBA, []string, error) {
	ordered := make([]LayerState, len(sts))
	copy(ordered, sts)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Z < ordered[j].Z })

	var drawn []string
	var dst *image.RGBA
	if canvasW <= 0 || canvasH <= 0 {
		// 优先用 z 最小的可见层定画布。若事件开头还没有任何可见层
		// （CS2 常见的 fade-in：bg 在 4800ms 才 image_show），退而用
		// 已登记但尚未显示的层定尺寸；否则会得到 1×1 黑帧。
		useLayer := func(visibleOnly bool) bool {
			for _, s := range ordered {
				if s.Ref == "" || (visibleOnly && !s.Visible) {
					continue
				}
				img, err := load(s.Ref)
				if err != nil {
					if visibleOnly {
						return false
					}
					continue
				}
				canvasW, canvasH = img.Bounds().Dx(), img.Bounds().Dy()
				return true
			}
			return false
		}
		if !useLayer(true) {
			useLayer(false)
		}
	}
	if canvasW <= 0 || canvasH <= 0 {
		canvasW, canvasH = 1, 1
	}
	dst = image.NewRGBA(image.Rect(0, 0, canvasW, canvasH))

	for _, s := range ordered {
		if !s.Visible || s.Ref == "" {
			continue
		}
		img, err := load(s.Ref)
		if err != nil {
			return nil, drawn, err
		}
		blend(dst, img, int(s.X), int(s.Y), s.Opacity)
		drawn = append(drawn, fmt.Sprintf("%s←%s", s.LayerID, s.Ref))
	}
	return dst, drawn, nil
}

// blend 把 src 以 alpha-over 叠到 dst 的 (x0,y0) 处，opacity 为整层不透明度（1=原样）。
func blend(dst *image.RGBA, src image.Image, x0, y0 int, opacity float64) {
	if opacity <= 0 {
		return
	}
	if opacity > 1 {
		opacity = 1
	}
	db := dst.Bounds()
	// 先把源矩形裁到画布内再逐像素，别拿 image.Uniform 这种「无限大」的源去空转。
	// 目标区 = 源平移 (x0,y0) 后与画布的交集，反解回源坐标。
	xFrom := db.Min.X - x0
	if xFrom < src.Bounds().Min.X {
		xFrom = src.Bounds().Min.X
	}
	xTo := db.Max.X - x0
	if xTo > src.Bounds().Max.X {
		xTo = src.Bounds().Max.X
	}
	yFrom := db.Min.Y - y0
	if yFrom < src.Bounds().Min.Y {
		yFrom = src.Bounds().Min.Y
	}
	yTo := db.Max.Y - y0
	if yTo > src.Bounds().Max.Y {
		yTo = src.Bounds().Max.Y
	}
	for y := yFrom; y < yTo; y++ {
		dy := y0 + (y - src.Bounds().Min.Y)
		for x := xFrom; x < xTo; x++ {
			dx := x0 + (x - src.Bounds().Min.X)
			r, g, bl, a := src.At(x, y).RGBA() // 16 位非预乘
			sa := float64(a) / 65535 * opacity
			if sa <= 0 {
				continue
			}
			i := dst.PixOffset(dx, dy)
			if sa >= 1 {
				dst.Pix[i+0] = byte(r >> 8)
				dst.Pix[i+1] = byte(g >> 8)
				dst.Pix[i+2] = byte(bl >> 8)
				dst.Pix[i+3] = 0xFF
				continue
			}
			inv := 1 - sa
			dst.Pix[i+0] = byte(float64(r>>8)*sa + float64(dst.Pix[i+0])*inv)
			dst.Pix[i+1] = byte(float64(g>>8)*sa + float64(dst.Pix[i+1])*inv)
			dst.Pix[i+2] = byte(float64(bl>>8)*sa + float64(dst.Pix[i+2])*inv)
			dst.Pix[i+3] = byte(float64(0xFF)*sa + float64(dst.Pix[i+3])*inv)
		}
	}
}

// SampleTimes 采样时刻（毫秒）：0 … 事件最后一刻，均分 n 帧。
// 事件长度取 timeline/texts 里的最大 t_ms（IR 的 end_ms 多数适配器不填）。
func SampleTimes(ev *ir.Event, n int) []int64 {
	if n < 1 {
		n = 1
	}
	var end int64
	for _, op := range ev.Timeline {
		if op.TMs > end {
			end = op.TMs
		}
	}
	for _, t := range ev.Texts {
		if t.TMs > end {
			end = t.TMs
		}
	}
	if end <= 0 {
		return []int64{0}
	}
	times := make([]int64, 0, n)
	for i := 0; i < n; i++ {
		times = append(times, end*int64(i)/int64(n-1))
		if n == 1 {
			break
		}
	}
	return times
}

// WriteGIF 把帧序列写成动图（「可播放演出」的最简载体：
// 命令行合成图 + 动图，不需要客户端窗口）。
//
// GIF 只有 256 色，这里用 Plan9 调色板 + Floyd–Steinberg 抖动 ——
// 只作「人工比对」的观察用，不代表像素级正确性（正确性看 PNG 帧）。
func WriteGIF(frames []*image.RGBA, path string, delayCs int) error {
	if delayCs <= 0 {
		delayCs = 40
	}
	anim := &gif.GIF{LoopCount: 0}
	for _, f := range frames {
		pal := image.NewPaletted(f.Bounds(), color.Palette(palette.Plan9))
		draw.FloydSteinberg.Draw(pal, f.Bounds(), f, image.Point{})
		anim.Image = append(anim.Image, pal)
		anim.Delay = append(anim.Delay, delayCs)
	}
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()
	return gif.EncodeAll(out, anim)
}
