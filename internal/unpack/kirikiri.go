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
	"fmt"
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
			// .wks 是 Mai@KF.dll 一系的剧本（内容仍是 KAG，但用自定义宏画图，
			// 且路径不带 scenario/ 前缀，不能按 .ks 的规则筛）。
			sl := strings.ToLower(filepath.ToSlash(p))
			if strings.EqualFold(filepath.Ext(p), ".wks") {
				return true
			}
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
		ScriptExt:    ".ks",
		ScriptDir:    filepath.Join("raw", "data", "scenario"),
		ImageDir:     filepath.Join("raw", "data", "image"),
		ScriptCompat: buildWKSCompat,
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

// cgLabelsOf 取画廊脚本里的 CG 事件标签。
//
// 两条口径：
//  1. 标准 KiriKiri 画廊：sysbutton 的 graphic/target 配对（与 library.Scan 同口径）。
//  2. 宏家族（Mai@KF.dll 一系，.wks）：没有 sysbutton 画廊，CG 事件写成脚本里的
//     `*eh_xxx` 标签——那就是原作自己的事件切分，按它切即可。
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
	if len(out) == 0 {
		for _, lb := range ast.Labels {
			name := strings.TrimSpace(lb.Name)
			if !strings.HasPrefix(strings.ToLower(name), "eh") || seen[name] {
				continue
			}
			seen[name] = true
			out = append(out, name)
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

// buildWKSCompat 把宏家族脚本（macImageDraw / TransName… 这类自定义宏，
// Mai@KF.dll 一系）翻成标准 KAG 标签的 .ks，供现有台本构建器直接播。
//
// 只保留「能演」的部分：台词、静态 CG 图、等待、BGM/SE、转场时长。
// & / $ 开头的引用是 TJS 运行时变量（背景、立绘名表都在 exe 里），静态不可解，
// 如实跳过并在告警里报数量；自定义宏的控制流、翻译叠加层（TransName/TransHitret
// 的英/繁/简三语）不进台本。
func buildWKSCompat(data []byte, name string, resolve func(string) bool) ([]byte, []string) {
	s := string(data)
	if !strings.Contains(s, "[macImageDraw") && !strings.Contains(s, "[macTrans") {
		return nil, nil // 不是宏家族脚本，原样交给现有构建器
	}
	lines := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	var b strings.Builder
	fmt.Fprintf(&b, "*label01|%s\n", strings.TrimSuffix(name, filepath.Ext(name)))
	inTrans := false
	keptImg, dyn := 0, 0
	for i, raw := range lines {
		t := strings.TrimSpace(raw)
		low := strings.ToLower(t)
		if t == "" || strings.HasPrefix(t, ";") || strings.HasPrefix(t, "//") {
			continue
		}
		if strings.HasPrefix(low, "[transname") {
			inTrans = true
			continue
		}
		if strings.HasPrefix(low, "[transhitret") {
			inTrans = false
			continue
		}
		if inTrans {
			continue
		}
		switch {
		case low == "[hitret]":
			// 原作宏：等点击 → 清消息层。
			b.WriteString("[l]\n[er]\n")
		case strings.HasPrefix(low, "[talk"):
			b.WriteString(t + "\n")
		case strings.HasPrefix(low, "[macwait"):
			if v := kagAttr(t, "time"); v != "" {
				fmt.Fprintf(&b, "[wait time=%s]\n", v)
			}
		case strings.HasPrefix(low, "[macplaybgm"):
			if v := kagAttr(t, "file"); v != "" && resolve(v) {
				fmt.Fprintf(&b, "[playbgm storage=%s]\n", v)
			}
		case strings.HasPrefix(low, "[macplayse"):
			if v := kagAttr(t, "file"); v != "" && resolve(v) {
				fmt.Fprintf(&b, "[playse storage=%s]\n", v)
			}
		case strings.HasPrefix(low, "[macimagedraw") || strings.HasPrefix(low, "[imagedraw"):
			v := kagAttr(t, "file")
			if v == "" || strings.HasPrefix(v, "&") || strings.HasPrefix(v, "$") {
				dyn++
				continue
			}
			if !resolve(v) {
				continue
			}
			fmt.Fprintf(&b, "[image storage=%s]\n", v)
			keptImg++
			// 原脚本多在下一条宏里转场/等待；没有的话补一个，别让 CG 一闪而过。
			if i+1 >= len(lines) || !strings.HasPrefix(strings.ToLower(strings.TrimSpace(lines[i+1])), "[macwait") {
				b.WriteString("[wait time=1200]\n")
			}
		case strings.HasPrefix(low, "[mactrans"):
			if v := kagAttr(t, "time"); v != "" {
				fmt.Fprintf(&b, "[trans time=%s]\n", v)
			}
		case strings.HasPrefix(t, "[") || strings.HasPrefix(t, "@"):
			// 其余自定义宏 / 控制流不进台本（与 build_show 的「未识别即跳过」一致）。
		default:
			b.WriteString(t + "\n")
		}
	}
	if keptImg == 0 {
		return nil, []string{fmt.Sprintf("%s：没有可静态解析的 CG 图（%d 条 & 变量引用不可解），未生成兼容台本", name, dyn)}
	}
	return []byte(b.String()), []string{fmt.Sprintf("%s：已生成兼容台本（CG 图 %d 张，跳过 %d 条 & 变量引用）", name, keptImg, dyn)}
}

// kagAttr 取一个 KAG 标签里的属性值（支持带引号与不带引号）。
func kagAttr(t, key string) string {
	i := strings.Index(t, key+"=")
	if i < 0 {
		return ""
	}
	v := t[i+len(key)+1:]
	if v == "" {
		return ""
	}
	if v[0] == '"' || v[0] == '\'' {
		if j := strings.IndexByte(v[1:], v[0]); j >= 0 {
			return v[1 : 1+j]
		}
		return v[1:]
	}
	if j := strings.IndexAny(v, " \t]"); j >= 0 {
		v = v[:j]
	}
	return v
}
