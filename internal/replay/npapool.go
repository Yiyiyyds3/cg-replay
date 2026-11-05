// npapool.go —— Nitro+ 归档图片池：GameLoader 的兜底数据源。
//
// 鬼哭街重制版的图全在 cg.npa / cg-1.npa 里（JPG/PNG 直存，仅头部段加密），
// 散装目录只有 exe 和 dx 视频。GameLoader 在文件系统、NSA 池、NeXAS 池都
// 找不到时落到这里：按名查条目、解密，再走 DecodeImage。
package replay

import (
	"path/filepath"
	"sort"
	"strings"

	"ucgr/internal/adapter/nitro"
)

// npaPool 一个游戏目录下的全部 Nitro+ 归档。
type npaPool struct {
	archives []*nitro.Archive
}

// newNpaPool 惰性创建：glob 根目录的 *.npa（不递归，Nitro+ 的归档约定
// 就在游戏根）。解析失败的 npa 跳过。
func newNpaPool(root string) *npaPool {
	p := &npaPool{}
	matches, _ := filepath.Glob(filepath.Join(root, "*.npa"))
	sort.Strings(matches)
	for _, m := range matches {
		if a, err := nitro.ParseNpa(m); err == nil {
			p.archives = append(p.archives, a)
		}
	}
	return p
}

// empty 报告池里有没有可用归档。
func (p *npaPool) empty() bool { return p == nil || len(p.archives) == 0 }

// image 按名取图（大小写不敏感；脚本引用带 cg/ 前缀与扩展名，立绘指令
// 引用裸名，OGG 语音不走这里）。
func (p *npaPool) image(name string) (data []byte, ok bool) {
	if p.empty() {
		return nil, false
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, false
	}
	for _, a := range p.archives {
		e := a.Lookup(name)
		if e == nil {
			continue
		}
		// 只认图类条目（png/jpg），别把 ogg/nss 当图解。
		lower := strings.ToLower(e.Name)
		if !strings.HasSuffix(lower, ".png") && !strings.HasSuffix(lower, ".jpg") &&
			!strings.HasSuffix(lower, ".jpeg") {
			continue
		}
		out, err := a.ReadEntry(*e)
		if err != nil {
			continue // 单个条目坏了不拖垮查找
		}
		return out, true
	}
	return nil, false
}
