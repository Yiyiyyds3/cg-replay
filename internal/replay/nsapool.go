// nsapool.go —— NScripter 归档图片池：GameLoader 的兜底数据源。
//
// 海猫鸣泣之时（出题篇）的图全部在 arc.nsa / arcN.nsa 里，散装目录只有
// 音频。GameLoader 在「文件系统找不到」时落到这里：按名查 NSA 条目、
// 解压（无压缩 / SPB / LZSS / NBZ），再走 DecodeImage。
package replay

import (
	"path/filepath"
	"sort"
	"strings"

	"ucgr/internal/adapter/nscript"
)

// nsaPool 一个游戏目录下的全部 NScripter 归档。
type nsaPool struct {
	archives []*nscript.NsaArchive
}

// newNsaPool 惰性创建：glob 根目录的 arc*.nsa / *.sar（不递归，
// NScripter 的归档约定就在游戏根）。
func newNsaPool(root string) *nsaPool {
	p := &nsaPool{}
	matches, _ := filepath.Glob(filepath.Join(root, "arc*.nsa"))
	sars, _ := filepath.Glob(filepath.Join(root, "*.sar"))
	matches = append(matches, sars...)
	sort.Strings(matches)
	for _, m := range matches {
		if a, err := nscript.ParseNsa(m); err == nil {
			p.archives = append(p.archives, a)
		}
	}
	return p
}

// empty 报告池里有没有可用归档。
func (p *nsaPool) empty() bool { return p == nil || len(p.archives) == 0 }

// image 按名取图（大小写不敏感，\ 与 / 等价，可省扩展名）。
func (p *nsaPool) image(name string) (data []byte, ok bool) {
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
		out, err := a.ReadEntry(e)
		if err != nil {
			continue // 单个条目坏了不拖垮查找
		}
		return out, true
	}
	return nil, false
}
