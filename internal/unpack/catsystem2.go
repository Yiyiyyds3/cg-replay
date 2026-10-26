// catsystem2.go —— CatSystem2（.int/.kif 归档 + .cst 剧本）解包。
//
// 修正（2026-10-26）：此前这里写着「CatSystem2 没有脚本解析」并返回
// errNoScriptParser —— **那是错的**。适配器从移植时（`2831e19`）就实现了
// `ParseScript`（CatScene 魔数 + zlib + 条目流）、`(*Ast).ExtractRefs` 与
// `BuildIR`。本轮把它接进解包流程，四个引擎至此全部可解。
//
// 事件粒度：CS2 的 `BuildIR` 是「一份 .cst = 一个事件」（脚本里没有可靠的
// 场景切换标记），解包侧沿用同一口径 —— 只影响「预计事件数」的展示，不影响
// 依赖闭包（闭包按整份脚本算）。
package unpack

import (
	"path/filepath"
	"strings"

	"ucgr/internal/adapter"
	"ucgr/internal/adapter/catsystem2"
)

func init() {
	a := &catsystem2.Adapter{}
	registerEngine(engineDeps{
		ID: "catsystem2",
		// 归档是 KIF（.int 内部包 / .kif 明文包）；.cst 剧本通常在归档里。
		ArchiveExts:  []string{".int", ".kif"},
		ParseArchive: a.ParseArchive,
		IsScript: func(p string) bool {
			return strings.EqualFold(filepath.Ext(p), ".cst")
		},
		ParseScript: func(data []byte, name string) (scriptInfo, error) {
			obj, err := a.ParseScript(data, &adapter.ScriptCtx{EntryPath: name})
			if err != nil {
				return scriptInfo{}, err
			}
			ast, ok := obj.(*catsystem2.Ast)
			if !ok {
				return scriptInfo{}, errTypeMismatch
			}
			refs := dedupSorted(ast.ExtractRefs())
			// 一脚本一事件（与 BuildIR 口径一致）。
			return scriptInfo{
				Labels:  []string{"script"},
				Refs:    refs,
				AllRefs: refs,
			}, nil
		},
		ScriptExt: ".cst",
		// CS2 解包树里剧本在 scene/ 下，素材在 image/ 下。
		ScriptDir: "scene",
		ImageDir:  "image",
	})
}
