// kirikiri.go —— KiriKiri（XP3）解包：读条目 → 算依赖闭包 → 落最小集合。
//
// 关键设计：
//   - **多段条目**：XP3 里一个文件可能由多个段拼成（每个段各自 zlib），
//     通用 archiveSet 已按 path 归并，这里顺序拼接即可。
//   - **加密条目**：XP3 的 adlr 过滤是**每作一把密钥**，阶段 1 不解密，
//     如实记进 Result.Encrypted 并跳过（不静默丢文件）。
//   - **落盘布局**与现有加载器一致（raw/data/scenario + raw/data/image），
//     所以产物能被 library.Scan + replay.GameLoader 直接消费。
package unpack

import (
	"errors"
	"path/filepath"
	"sort"
	"strings"

	"ucgr/internal/adapter"
	"ucgr/internal/adapter/kirikiri"
)

// errTypeMismatch 适配器返回的 AST 类型不对（引擎实现有问题，要报错不静默）。
var errTypeMismatch = errors.New("脚本解析结果类型异常")

func init() {
	a := &kirikiri.Adapter{}
	registerEngine(engineDeps{
		ID:           "kirikiri",
		ArchiveExts:  []string{".xp3"},
		ParseArchive: a.ParseArchive,
		IsScript: func(p string) bool {
			sl := strings.ToLower(filepath.ToSlash(p))
			return strings.Contains(sl, "scenario") && strings.EqualFold(filepath.Ext(p), ".ks")
		},
		ParseScript: func(data []byte, name string) (scriptInfo, error) {
			ast, err := parseAst(data, name)
			if err != nil {
				return scriptInfo{}, err
			}
			info := scriptInfo{AllRefs: kirikiri.ResourceRefs(ast), Dynamic: kirikiri.DynamicRefs(ast)}
			// 事件切分 = 画廊里的槽位（sysbutton 的 graphic/target 配对），
			// 与 library.Scan 同口径。
			for _, label := range cgLabelsOf(ast) {
				info.Labels = append(info.Labels, label)
				info.Refs = append(info.Refs, kirikiri.SegmentRefs(ast, label)...)
			}
			info.Refs = dedupSorted(info.Refs)
			return info, nil
		},
		ScriptExt: ".ks",
		ScriptDir: filepath.Join("raw", "data", "scenario"),
		ImageDir:  filepath.Join("raw", "data", "image"),
	})
}

// parseAst 解析一份脚本为 AST（失败返回错误，调用方记录并跳过）。
func parseAst(data []byte, name string) (*kirikiri.Ast, error) {
	a := &kirikiri.Adapter{}
	obj, err := a.ParseScript(data, &adapter.ScriptCtx{EntryPath: name})
	if err != nil {
		return nil, err
	}
	ast, ok := obj.(*kirikiri.Ast)
	if !ok {
		return nil, errTypeMismatch
	}
	return ast, nil
}

// cgLabelsOf 取画廊脚本里的 CG 事件标签（sysbutton 的 graphic/target 配对，
// 与 library.Scan 同口径：只有两者都像 CG 槽位才算一个事件）。
func cgLabelsOf(ast *kirikiri.Ast) []string {
	seen := map[string]bool{}
	var out []string
	for _, sec := range kirikiri.Sections(ast) {
		for _, it := range kirikiri.Segment(ast, sec.Name) {
			if it.Kind != "cmd" || it.Cmd == nil {
				continue
			}
			if !strings.EqualFold(it.Cmd.Tag, "sysbutton") {
				continue
			}
			thumb := strings.TrimSpace(it.Cmd.Attrs["graphic"])
			target := strings.TrimSpace(it.Cmd.Attrs["target"])
			if thumb == "" || target == "" {
				continue
			}
			label := strings.TrimPrefix(target, "*")
			if label == "" || seen[label] {
				continue
			}
			seen[label] = true
			out = append(out, label)
		}
	}
	sort.Strings(out)
	return out
}

// isGallery 画廊脚本（cgrai*.ks）——CG 事件的目录。
func isGallery(p string) bool {
	b := strings.ToLower(filepath.Base(p))
	return strings.HasPrefix(b, "cgrai") && strings.EqualFold(filepath.Ext(b), ".ks")
}

// dedupSorted 去重并排序（引用集用）。
func dedupSorted(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}
