// afapool.go —— AliceSoft 归档图片池：GameLoader 的兜底数据源。
//
// 兰斯10 的图全在 Rance10CG*.afa / Rance10Flat*.afa（AJP/QNT/DCF，其中
// DCF 是差分 CG，底图可能在**同池另一个 afa** 里），所以这里逐归档
// LookupStem；DCF 解码用池自身做底图查找回调。
//
// 名字域：AFAEntry.Name 与事件资产名都已是 UTF-8（ain/AFA 解码时
// GBK→UTF-8），所以主查找直接同域；DCF 头里的底图名是 GBK 原始字节，
// lookupBase 里先 decodeGBK。保留 GBK 回退查找以防混域引用。
package replay

import (
	"bytes"
	"image"
	"image/png"
	"path/filepath"
	"sort"
	"strings"

	"ucgr/internal/adapter/alice"
	"ucgr/internal/decode"
)

// encodePNG 把解码结果编码成无损 PNG 字节（BGRA → RGBA）。
func encodePNG(img *decode.Image) ([]byte, error) {
	rgba := image.NewNRGBA(image.Rect(0, 0, img.Width, img.Height))
	for p := 0; p < img.Width*img.Height; p++ {
		rgba.Pix[p*4+0] = img.Pixels[p*4+2]
		rgba.Pix[p*4+1] = img.Pixels[p*4+1]
		rgba.Pix[p*4+2] = img.Pixels[p*4+0]
		rgba.Pix[p*4+3] = img.Pixels[p*4+3]
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, rgba); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// afaPool 一个游戏目录下的全部 AliceSoft 归档。
type afaPool struct {
	archives []*alice.AFA
}

// newAfaPool 惰性创建：glob 根目录的 *.afa（AliceSoft 的归档约定就在游戏
// 根）。解析失败的 afa 跳过。
func newAfaPool(root string) *afaPool {
	p := &afaPool{}
	matches, _ := filepath.Glob(filepath.Join(root, "*.afa"))
	sort.Strings(matches)
	for _, m := range matches {
		if a, err := alice.ParseAFA(m); err == nil {
			p.archives = append(p.archives, a)
		}
	}
	return p
}

// empty 报告池里有没有可用归档。
func (p *afaPool) empty() bool { return p == nil || len(p.archives) == 0 }

// image 按名取图：词干（去目录、去扩展名）→ 归档条目 → 读出。
// DCF 差分就地合成并转成无损 PNG 字节（返回值要能被上层 DecodeImage
// 消费；运行时兜底路径，不涉及入库资产改写）。
func (p *afaPool) image(name string) (data []byte, ok bool) {
	if p.empty() {
		return nil, false
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, false
	}
	if data, ok := p.lookup(name); ok {
		return data, true
	}
	// GBK 回退：万一来的是 GBK 原始字节引用
	if g, err := alice.EncodeGBK(name); err == nil {
		if data, ok := p.lookup(string(g)); ok {
			return data, true
		}
	}
	return nil, false
}

// lookup 逐归档词干查找并读出。
func (p *afaPool) lookup(name string) (data []byte, ok bool) {
	stem := strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
	if stem == "" {
		return nil, false
	}
	for _, a := range p.archives {
		e := a.LookupStem(stem)
		if e == nil {
			continue
		}
		out, err := a.ReadEntry(*e)
		if err != nil {
			continue // 单个条目坏了不拖垮查找
		}
		if bytes.HasPrefix(out, []byte("dcf ")) {
			img, err := decode.DecodeDCF(out, p.lookupBase)
			if err != nil {
				continue
			}
			pngBytes, err := encodePNG(img)
			if err != nil {
				continue
			}
			return pngBytes, true
		}
		return out, true
	}
	return nil, false
}

// lookupBase 供 DecodeDCF 递归用的底图查找。DCF 头里的底图名是 GBK
// 原始字节，先转 UTF-8 再走同域词干查找。
func (p *afaPool) lookupBase(stem string) ([]byte, error) {
	name := alice.DecodeGBKString(stem)
	if s := strings.TrimSuffix(name, filepath.Ext(name)); s != "" {
		name = s
	}
	for _, a := range p.archives {
		if e := a.LookupStem(name); e != nil {
			return a.ReadEntry(*e)
		}
	}
	return nil, errAfaMiss{name}
}

// errAfaMiss AFA 里找不到条目。
type errAfaMiss struct{ key string }

func (e errAfaMiss) Error() string { return "AFA 里没有条目 " + e.key }
