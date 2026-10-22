// artemis.go —— Artemis（PFS + 明文 Lua .ast）解包。
//
// Artemis 的剧本是**明文 Lua 表**（适配器已确认，无需解密），引用提取也
// 已有现成实现（`(*Ast).ExtractRefs`，含 `:bg/` `:cg/` 之类路径别名的清理）。
//
// 事件切分：Artemis 没有 KiriKiri 那种「画廊槽位」的概念，剧本按**标签**
// 分段——这里以标签为事件单位，闭包按**整份剧本**的引用算（保守：宁多勿缺）。
package unpack

import (
	"path/filepath"
	"sort"
	"strings"

	"ucgr/internal/adapter"
	"ucgr/internal/adapter/artemis"
)

func init() {
	a := &artemis.Adapter{}
	registerEngine(engineDeps{
		ID:           "artemis",
		ArchiveExts:  []string{".pfs"},
		ParseArchive: a.ParseArchive,
		IsScript: func(p string) bool {
			return strings.EqualFold(filepath.Ext(p), ".ast")
		},
		ParseScript: func(data []byte, name string) (scriptInfo, error) {
			obj, err := a.ParseScript(data, &adapter.ScriptCtx{EntryPath: name})
			if err != nil {
				return scriptInfo{}, err
			}
			ast, ok := obj.(*artemis.Ast)
			if !ok {
				return scriptInfo{}, errTypeMismatch
			}
			labels := make([]string, 0, len(ast.Labels))
			for l := range ast.Labels {
				labels = append(labels, l)
			}
			sort.Strings(labels)
			refs := dedupSorted(ast.ExtractRefs())
			// Artemis 的引用是「包内路径」，没有画廊概念 → 闭包按整份剧本算。
			return scriptInfo{
				Labels:  labels,
				Refs:    refs,
				AllRefs: refs,
			}, nil
		},
		ScriptExt: ".ast",
		ScriptDir: "scenario",
		ImageDir:  "image",
	})
}
