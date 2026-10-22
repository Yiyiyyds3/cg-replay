// catsystem2.go —— CatSystem2（KIF 归档 / .cst）：**暂不支持自动解包**。
//
// 原因（要说清楚，不能只说失败）：CatSystem2 的适配器实现了归档列举
// （ParseArchive）与 CST 解压（DecompressCST），但**没有脚本解析**——
// 而「只够重演的最小集合」必须靠解析演出脚本算出资源引用闭包，没有脚本
// 解析就没有闭包，只能解出整个游戏（那就不叫最小集合了）。
//
// 因此这里只注册引擎身份与归档后缀，让 Preview 能认出「这是 CatSystem2」，
// 并给出可走的退路：手动解包后用「添加游戏」登记（事件库接入见 §4 说明）。
package unpack

import (
	"path/filepath"
	"strings"

	"ucgr/internal/adapter/catsystem2"
)

func init() {
	a := &catsystem2.Adapter{}
	registerEngine(engineDeps{
		ID:           "catsystem2",
		ArchiveExts:  []string{".cst"},
		ParseArchive: a.ParseArchive,
		IsScript: func(p string) bool {
			// CatSystem2 的剧本是 .ns2/mit 一类，适配器尚未解析。
			ext := strings.ToLower(filepath.Ext(p))
			return ext == ".ns2" || ext == ".mit"
		},
		ParseScript: func(data []byte, name string) (scriptInfo, error) {
			return scriptInfo{}, errNoScriptParser
		},
		ScriptDir: "scenario",
		ImageDir:  "image",
	})
}
