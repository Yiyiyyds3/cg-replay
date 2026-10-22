// engine.go —— 引擎抽象：解包层只做编排，容器解析与脚本规则各引擎自己提供。
//
// 一个引擎要提供四件事：
//  1. 列归档条目（复用适配器的 ParseArchive）
//  2. 判断条目是不是演出剧本（后缀 + 路径）
//  3. 解析剧本 → 事件标签 + 资源引用 + 是否有动态引用
//  4. 落盘约定（剧本目录 / 素材目录）
//
// 阶段 1 只做了 KiriKiri；Artemis 与 BGI 走同一套框架，CatSystem2 因
// **脚本格式尚未接入解析**（无从算依赖闭包）明确报「暂不支持」。
package unpack

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"ucgr/internal/adapter"
)

// scriptInfo 一个演出剧本的解析结果。
type scriptInfo struct {
	Labels  []string // 事件标签（按原作自己的切分）
	Refs    []string // 事件用到的静态资源引用（ScopeCG 用）
	AllRefs []string // 整份剧本的静态资源引用（ScopeAll 用）
	Dynamic bool     // 有动态引用（运行时拼文件名）→ 闭包必然不全
}

// engineDeps 一个引擎的解包能力。
type engineDeps struct {
	ID           string
	ArchiveExts  []string                                           // 归档后缀（如 .xp3）
	ParseArchive func(path string) ([]adapter.ArchiveEntry, error)  // 列条目
	IsScript     func(path string) bool                             // 条目是不是剧本
	ParseScript  func(data []byte, name string) (scriptInfo, error) // 解析剧本
	// ScriptDir 落盘时剧本放哪（相对解包根）；ImageDir 素材放哪。
	ScriptDir string
	ImageDir  string
	// ScriptExt 剧本后缀（如 .ks / .ast / .bs）——写进约定文件。
	ScriptExt string
}

// engineRegistry 引擎 id → 能力表（按引擎注册，核心不感知差异）。
var engineRegistry = map[string]engineDeps{}

// registerEngine 注册引擎能力（重复注册 panic——防止静默覆盖）。
func registerEngine(d engineDeps) {
	if _, dup := engineRegistry[d.ID]; dup {
		panic("unpack: 引擎重复注册: " + d.ID)
	}
	engineRegistry[d.ID] = d
}

// engineFor 按 id 取引擎能力。
func engineFor(id string) (engineDeps, bool) {
	d, ok := engineRegistry[id]
	return d, ok
}

// engineIDs 已支持的引擎 id（排序）。
func engineIDs() []string {
	out := make([]string, 0, len(engineRegistry))
	for id := range engineRegistry {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// detectEngine 猜一个路径的引擎：先按归档后缀，再让各引擎自己认。
//
// 目录要额外扫一遍归档后缀：游戏往往只把 .xp3/.pfs 放在目录里，剧本在
// 归档内部——各引擎的 Identify 看的是**松散**剧本文件，认不出这种布局。
//
// 认不出时返回空串（调用方要给出「为什么认不出」，不能只说失败）。
func detectEngine(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	for _, d := range engineRegistry {
		for _, ae := range d.ArchiveExts {
			if ext == strings.ToLower(ae) {
				return d.ID
			}
		}
	}
	// 目录：扫各引擎的归档后缀。
	if st, err := os.Stat(path); err == nil && st.IsDir() {
		for _, id := range engineIDs() {
			archives, err := findArchives(path, id)
			if err == nil && len(archives) > 0 {
				return id
			}
		}
	}
	// 目录：让各引擎的 Identify 认（它们会找自家特征文件）。
	for _, d := range engineRegistry {
		a, ok := adapter.Get(d.ID)
		if !ok {
			continue
		}
		if id, err := a.Identify(path); err == nil && id != "" {
			return id
		}
	}
	return ""
}
