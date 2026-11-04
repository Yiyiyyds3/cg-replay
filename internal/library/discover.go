// discover.go —— 剧本目录自动探测。
//
// 为什么需要：同一引擎的不同游戏，解包产物在磁盘上的层级差别很大——
//
//	Artemis  <root>/文本/<卷>.pfs/script/*.ast
//	         <root>/原始归档解包/<卷>.pfs/script/*.ast
//	BGI      <root>/文本_脚本/data01099/*.bs
//	CatSystem2 <root>/原始归档解包/scene/*.cst
//
// 「一层 ReadDir」的写法只能覆盖其中一种，其余全部报「读不到剧本目录」。
// 这里统一按扩展名递归收集，约定里的 scenario_dir 只作为**收窄范围**的
// 可选提示（写了且目录存在就只在它下面找）。
package library

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// discoverMaxDepth 自动探测的最大层数。
//
// 实测最深的剧本：<root>/原始归档解包/Amakano3.pfs/script/x.ast = 4 层
// （从 root 算），留 6 层余量；再深就有把备份/工具目录里的同名剧本
// 一起扫进来的风险。
const discoverMaxDepth = 6

// scriptFiles 收集 root 下的剧本文件（绝对路径，按相对路径升序）。
//
// sub 非空且该目录存在时只在它下面找（用户用 ucgr-convention.json 指定了
// 剧本目录）；否则在 root 下最多 maxDepth 层内自动探测。
func scriptFiles(root, sub, ext string, maxDepth int) []string {
	base := root
	if sub != "" {
		if p := filepath.Join(root, filepath.FromSlash(sub)); isDir(p) {
			base = p
			maxDepth = 64 // 用户显式指定目录：整棵子树都算它的
		}
	}
	base = filepath.Clean(base)
	var out []string
	_ = filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // 读不到的子树跳过，不拖垮整个扫描
		}
		if d.IsDir() {
			if rel, e := filepath.Rel(base, p); e == nil && rel != "." {
				if strings.Count(rel, string(filepath.Separator)) >= maxDepth {
					return fs.SkipDir
				}
			}
			return nil
		}
		if strings.EqualFold(filepath.Ext(p), ext) {
			out = append(out, p)
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return relKey(root, out[i]) < relKey(root, out[j]) })
	return out
}

// relKey root 相对路径（'/' 分隔），用于稳定排序与生成事件 id。
func relKey(root, p string) string {
	if r, err := filepath.Rel(root, p); err == nil {
		return filepath.ToSlash(r)
	}
	return filepath.ToSlash(p)
}

// isDir 报告 p 是否存在的目录。
func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

// stemOf 文件名去扩展名。
func stemOf(p string) string {
	return strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
}

// uniqueStems 为一批剧本文件生成事件 id 前缀。
//
// 文件名在本批里唯一时就用文件名（保持既有事件 id 不变），重名时才退回
// 相对 root 的路径——解包树里多个 dataXXXXX / 卷目录下可能有同名剧本，
// id 撞了会导致「列表里两个条目点开播的是同一个」。
func uniqueStems(root string, files []string) func(string) string {
	count := make(map[string]int, len(files))
	for _, p := range files {
		count[stemOf(p)]++
	}
	return func(p string) string {
		s := stemOf(p)
		if count[s] > 1 {
			return strings.TrimSuffix(relKey(root, p), filepath.Ext(p))
		}
		return s
	}
}

// scriptDirHint 报错时说明「按约定找的还是自动探测的」，避免用户以为
// 是自己写的 scenario_dir 没生效。
func scriptDirHint(root, sub string) string {
	if sub == "" {
		return root + "（自动探测）"
	}
	return filepath.Join(root, filepath.FromSlash(sub))
}

// kirikiriScriptDirMaxDepth 自动探测 .ks 目录的最大层数。
//
// 实测：raw/data_zh/scenario（2 层）、_分类/文本/data/scenario（3 层）、
// 原始归档解包/data/scenario（2 层）。再深多半是解包工具的备份目录。
const kirikiriScriptDirMaxDepth = 4

// preferredKiriKiriDirNames KiriKiri 剧本目录的常见名。
// 名字命中时优先于「.ks 文件更多」——sysscn 里常有几十个系统脚本，
// 按数量挑会把系统脚本目录当成剧本目录。
var preferredKiriKiriDirNames = map[string]bool{
	"scenario": true, "scn": true, "script": true, "scripts": true,
}

// findKiriKiriScriptDir 在 root 下探测最像剧本目录的一层，找不到返回 ""。
func findKiriKiriScriptDir(root string) string {
	type cand struct {
		dir    string
		n      int
		prefer bool
	}
	var best cand
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		rel, e := filepath.Rel(root, p)
		if e != nil {
			return nil
		}
		if rel != "." && strings.Count(rel, string(filepath.Separator)) >= kirikiriScriptDirMaxDepth {
			return fs.SkipDir
		}
		ents, e := os.ReadDir(p)
		if e != nil {
			return nil
		}
		n := 0
		for _, x := range ents {
			if !x.IsDir() && strings.EqualFold(filepath.Ext(x.Name()), ".ks") {
				n++
			}
		}
		if n < 2 { // 只有一个 sysmenu.ks 的目录不是剧本目录
			return nil
		}
		prefer := preferredKiriKiriDirNames[strings.ToLower(d.Name())]
		if (prefer && !best.prefer) || (prefer == best.prefer && n > best.n) {
			best = cand{dir: p, n: n, prefer: prefer}
		}
		return nil
	})
	return best.dir
}

// scriptDirMinVotes 判定「像剧本目录」的最少文件数。
// 单个 .scn/.ast 出现在工具目录里不算，至少两个才认。
const scriptDirMinVotes = 2

// densestScriptDir 在 root 下挑「直接含该扩展名文件最多」的一层目录。
//
// 与 findKiriKiriScriptDir 的区别：这里不猜目录名，纯按密度选，用于
// 整理型解包（`解包整理/data_脚本与数据/scn`）等名字不固定的布局。
func densestScriptDir(root, ext string) string {
	files := scriptFiles(root, "", ext, discoverMaxDepth)
	if len(files) < scriptDirMinVotes {
		return ""
	}
	count := make(map[string]int, len(files))
	for _, p := range files {
		count[filepath.Dir(p)]++
	}
	best, bestN := "", 0
	for dir, n := range count {
		if n > bestN || (n == bestN && dir < best) {
			best, bestN = dir, n
		}
	}
	return best
}

// findFileByName 在 root 下按文件名（不区分大小写）找文件，返回层数最浅的一个。
func findFileByName(root, name string, maxDepth int) string {
	root = filepath.Clean(root)
	var best string
	bestDepth := maxDepth + 1
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, e := filepath.Rel(root, p)
		if e != nil {
			return nil
		}
		depth := 0
		if rel != "." {
			depth = strings.Count(rel, string(filepath.Separator)) + 1
		}
		if d.IsDir() {
			if rel != "." && depth > maxDepth {
				return fs.SkipDir
			}
			return nil
		}
		if strings.EqualFold(d.Name(), name) && depth < bestDepth {
			best, bestDepth = p, depth
		}
		return nil
	})
	return best
}
