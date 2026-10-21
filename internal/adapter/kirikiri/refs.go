// refs.go —— 从演出脚本里静态收集资源引用（解包器算依赖闭包用）。
//
// 「只够重演」的最小集合 = 演出脚本 + 脚本引用到的素材文件。要算出后者，
// 就得知道脚本里点了哪些图/语音/音效——这里把 build_ir 里那套引用规则
// （firstResourceRef / voiceRef）导出来给解包器复用，避免规则写两份。
package kirikiri

import (
	"sort"
	"strings"
)

// ResourceRefs 收集整份脚本里引用到的全部静态资源名（去重、按名排序）。
//
// 动态引用（&/$ 变量拼接、语音 base+行号）静态不可解，会被跳过——
// 解包器据此知道「闭包不完整」，可以退化成解全量或提示用户。
func ResourceRefs(ast *Ast) []string {
	if ast == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	add := func(ref string) {
		ref = strings.TrimSpace(ref)
		if ref == "" || seen[ref] || isDynamic(ref) {
			return
		}
		seen[ref] = true
		out = append(out, ref)
	}
	for i := range ast.Commands {
		c := &ast.Commands[i]
		add(firstResourceRef(c))
		add(voiceRef(c))
	}
	sort.Strings(out)
	return out
}

// SegmentRefs 收集某个标签段（一个事件）里引用到的静态资源名。
func SegmentRefs(ast *Ast, label string) []string {
	if ast == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	add := func(ref string) {
		ref = strings.TrimSpace(ref)
		if ref == "" || seen[ref] || isDynamic(ref) {
			return
		}
		seen[ref] = true
		out = append(out, ref)
	}
	for _, it := range Segment(ast, label) {
		if it.Kind != "cmd" || it.Cmd == nil {
			continue
		}
		add(firstResourceRef(it.Cmd))
		add(voiceRef(it.Cmd))
	}
	sort.Strings(out)
	return out
}

// DynamicRefs 报告脚本里是否存在**动态**资源引用（静态解不出文件名）。
// 有动态引用时依赖闭包必然不完整，解包器要如实告知（FR-12 不静默）。
func DynamicRefs(ast *Ast) bool {
	if ast == nil {
		return false
	}
	for i := range ast.Commands {
		c := &ast.Commands[i]
		if isDynamic(firstResourceRef(c)) || isDynamic(voiceRef(c)) {
			return true
		}
	}
	return false
}
