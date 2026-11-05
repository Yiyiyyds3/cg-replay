// pacpool.go —— NeXAS 归档图片池：GameLoader 的兜底数据源。
//
// Baldr Sky 的图全在 Bmp.pac / Stand.pac / System.pac 里（标准 PNG/BMP，
// zlib 打包），散装目录只有 exe 和字体。GameLoader 在文件系统与 NSA
// 池都找不到时落到这里：按名查条目、解压，再走 DecodeImage。
package replay

import (
	"path/filepath"
	"sort"
	"strings"

	"ucgr/internal/adapter/nexas"
)

// pacPool 一个游戏目录下的全部 NeXAS 归档。
type pacPool struct {
	archives []*nexas.Archive
}

// newPacPool 惰性创建：glob 根目录的 *.pac（不递归，NeXAS 的归档约定
// 就在游戏根）。解析失败的 pac 跳过。
func newPacPool(root string) *pacPool {
	p := &pacPool{}
	matches, _ := filepath.Glob(filepath.Join(root, "*.pac"))
	sort.Strings(matches)
	for _, m := range matches {
		if a, err := nexas.ParsePac(m); err == nil {
			p.archives = append(p.archives, a)
		}
	}
	return p
}

// empty 报告池里有没有可用归档。
func (p *pacPool) empty() bool { return p == nil || len(p.archives) == 0 }

// image 按名取图（大小写不敏感，可省扩展名；脚本引用 *.png，立绘指令
// 引用裸名，Voice 是 ogg 不走这里）。
func (p *pacPool) image(name string) (data []byte, ok bool) {
	if p.empty() {
		return nil, false
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, false
	}
	for _, a := range p.archives {
		e, hit := a.Lookup(name)
		if !hit {
			continue
		}
		// 只认图类条目（png/bmp），别把 spm/dat 当图解。
		lower := strings.ToLower(e.Name)
		if !strings.HasSuffix(lower, ".png") && !strings.HasSuffix(lower, ".bmp") {
			continue
		}
		out, err := a.ReadEntry(e)
		if err != nil {
			continue // 单个条目坏了不拖垮查找
		}
		return out, true
	}
	return nil, false
}
