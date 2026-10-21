// kirikiri.go —— KiriKiri（XP3）解包：读条目 → 算依赖闭包 → 落最小集合。
//
// 关键设计：
//   - **多段条目**：XP3 里一个文件可能由多个段拼成（每个段各自 zlib），
//     适配器返回的是「每段一条 ArchiveEntry」，这里按 path 归并再顺序拼接。
//   - **加密条目**：XP3 的 adlr 过滤是**每作一把密钥**，阶段 1 不解密，
//     如实记进 Result.Encrypted 并跳过（不静默丢文件）。
//   - **落盘布局**与现有加载器一致（raw/data/scenario + raw/data/image），
//     所以产物能被 library.Scan + replay.GameLoader 直接消费。
package unpack

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"ucgr/internal/adapter"
	"ucgr/internal/adapter/kirikiri"
)

// xp3Entry 一个归档里的一个文件（多段已归并）。
type xp3Entry struct {
	path      string
	segs      []adapter.ArchiveEntry
	encrypted bool
}

// base 取路径的文件名。
func (e xp3Entry) base() string { return filepath.Base(filepath.FromSlash(e.path)) }

// stem 取去掉扩展名的文件名（storage 名常常不带扩展名）。
func (e xp3Entry) stem() string {
	b := e.base()
	return strings.TrimSuffix(b, filepath.Ext(b))
}

// assetSet 一批 XP3 归档的条目索引（持有文件句柄，避免每读一条就重扫归档）。
type assetSet struct {
	archives  []string
	files     map[string]*os.File
	entries   map[string]xp3Entry
	owner     map[string]string // 条目路径 → 所属归档
	encrypted []string
}

// openAssets 打开归档集合并索引全部条目。
func openAssets(archives []string) (*assetSet, error) {
	s := &assetSet{
		archives: archives,
		files:    map[string]*os.File{},
		entries:  map[string]xp3Entry{},
		owner:    map[string]string{},
	}
	a := &kirikiri.Adapter{}
	for _, arch := range archives {
		segs, err := a.ParseArchive(arch)
		if err != nil {
			s.close()
			return nil, fmt.Errorf("解析 XP3 %s 失败：%w", filepath.Base(arch), err)
		}
		order := []string{}
		grouped := map[string][]adapter.ArchiveEntry{}
		enc := map[string]bool{}
		for _, e := range segs {
			if _, ok := grouped[e.Path]; !ok {
				order = append(order, e.Path)
			}
			grouped[e.Path] = append(grouped[e.Path], e)
			if e.Encrypted {
				enc[e.Path] = true
			}
		}
		for _, p := range order {
			entry := xp3Entry{path: p, segs: grouped[p], encrypted: enc[p]}
			if _, exists := s.entries[p]; !exists {
				s.entries[p] = entry
				s.owner[p] = arch
				if entry.encrypted {
					s.encrypted = append(s.encrypted, p)
				}
			}
		}
	}
	sort.Strings(s.encrypted)
	return s, nil
}

// close 关掉所有归档句柄。
func (s *assetSet) close() {
	for _, f := range s.files {
		_ = f.Close()
	}
	s.files = map[string]*os.File{}
}

// read 读一个条目的完整内容（多段顺序拼接，每段各自 zlib 解压）。
func (s *assetSet) read(e xp3Entry) ([]byte, error) {
	arch, ok := s.owner[e.path]
	if !ok {
		return nil, fmt.Errorf("条目 %s 不知道属于哪个归档", e.path)
	}
	f, ok := s.files[arch]
	if !ok {
		var err error
		f, err = os.Open(arch)
		if err != nil {
			return nil, err
		}
		s.files[arch] = f
	}
	var out []byte
	for _, seg := range e.segs {
		b, err := kirikiri.ReadArchive(f, seg)
		if err != nil {
			return nil, err
		}
		out = append(out, b...)
	}
	return out, nil
}

// isScenario 剧本条目（路径含 scenario/ 且是 .ks）。
func isScenario(p string) bool {
	sl := strings.ToLower(filepath.ToSlash(p))
	return strings.Contains(sl, "scenario") && strings.EqualFold(filepath.Ext(p), ".ks")
}

// isGallery 画廊脚本（cgrai*.ks）——CG 事件的目录。
func isGallery(p string) bool {
	b := strings.ToLower(filepath.Base(p))
	return strings.HasPrefix(b, "cgrai") && strings.EqualFold(filepath.Ext(b), ".ks")
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
		return nil, fmt.Errorf("脚本解析结果类型异常")
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

// nameIndex 按文件名/无扩展名索引条目（storage 名常常不带扩展名）。
func (s *assetSet) nameIndex() (map[string]xp3Entry, map[string]xp3Entry) {
	byBase := map[string]xp3Entry{}
	byStem := map[string]xp3Entry{}
	for _, e := range s.entries {
		lb := strings.ToLower(e.base())
		if _, ok := byBase[lb]; !ok {
			byBase[lb] = e
		}
		ls := strings.ToLower(e.stem())
		if _, ok := byStem[ls]; !ok {
			byStem[ls] = e
		}
	}
	return byBase, byStem
}

// kirikiriPlan 预览 XP3 内容（不落盘）。
func kirikiriPlan(archives []string) (*Plan, error) {
	assets, err := openAssets(archives)
	if err != nil {
		return nil, err
	}
	defer assets.close()

	p := &Plan{
		EngineID:  "kirikiri",
		Root:      strings.Join(archives, " "),
		Kind:      "xp3",
		Archives:  archives,
		Encrypted: len(assets.encrypted),
	}
	for path := range assets.entries {
		if isScenario(path) {
			p.ScenarioFiles++
		}
	}
	refs := map[string]bool{}
	for path, e := range assets.entries {
		if !isGallery(path) {
			continue
		}
		data, err := assets.read(e)
		if err != nil {
			p.Notes = append(p.Notes, fmt.Sprintf("读 %s 失败：%v", e.base(), err))
			continue
		}
		ast, err := parseAst(data, e.base())
		if err != nil {
			p.Notes = append(p.Notes, fmt.Sprintf("解析 %s 失败：%v", e.base(), err))
			continue
		}
		for _, label := range cgLabelsOf(ast) {
			p.Labels++
			for _, r := range kirikiri.SegmentRefs(ast, label) {
				refs[r] = true
			}
		}
	}
	p.Refs = len(refs)
	if p.Encrypted > 0 {
		p.Notes = append(p.Notes, fmt.Sprintf("有 %d 个加密条目（需要该作密钥），这部分不会被解出", p.Encrypted))
	}
	return p, nil
}

// kirikiriExtract 解出最小集合。
func kirikiriExtract(archives []string, dest string, scope Scope) (*Result, error) {
	assets, err := openAssets(archives)
	if err != nil {
		return nil, err
	}
	defer assets.close()

	res := &Result{Dest: dest, EngineID: "kirikiri", Encrypted: assets.encrypted}

	// ① 收集剧本条目 + CG 标签 + 引用闭包。
	type scriptInfo struct {
		entry  xp3Entry
		labels []string
	}
	var scripts []scriptInfo
	refSet := map[string]bool{}
	for path, e := range assets.entries {
		if !isScenario(path) {
			continue
		}
		data, err := assets.read(e)
		if err != nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("读 %s 失败：%v", e.base(), err))
			continue
		}
		ast, err := parseAst(data, e.base())
		if err != nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("解析 %s 失败：%v", e.base(), err))
			continue
		}
		info := scriptInfo{entry: e}
		if isGallery(path) {
			info.labels = cgLabelsOf(ast)
			for _, l := range info.labels {
				for _, r := range kirikiri.SegmentRefs(ast, l) {
					refSet[r] = true
				}
			}
		}
		if scope == ScopeAll {
			for _, r := range kirikiri.ResourceRefs(ast) {
				refSet[r] = true
			}
		}
		if kirikiri.DynamicRefs(ast) {
			res.Warnings = append(res.Warnings,
				fmt.Sprintf("%s 里有动态素材引用（运行时拼文件名），这部分无法静态收集", e.base()))
		}
		scripts = append(scripts, info)
	}
	sort.Slice(scripts, func(i, j int) bool { return scripts[i].entry.path < scripts[j].entry.path })

	// ② 名字 → 条目索引。
	byBase, byStem := assets.nameIndex()
	lookup := func(ref string) (xp3Entry, bool) {
		ref = strings.TrimSpace(ref)
		lb := strings.ToLower(filepath.Base(filepath.FromSlash(ref)))
		if e, ok := byBase[lb]; ok {
			return e, true
		}
		ls := strings.TrimSuffix(lb, filepath.Ext(lb))
		if e, ok := byStem[ls]; ok {
			return e, true
		}
		return xp3Entry{}, false
	}

	// ③ 落盘：剧本 → raw/data/scenario，素材 → raw/data/image。
	scenarioDir := filepath.Join(dest, "raw", "data", "scenario")
	imageDir := filepath.Join(dest, "raw", "data", "image")
	for _, s := range scripts {
		data, err := assets.read(s.entry)
		if err != nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("读 %s 失败：%v", s.entry.base(), err))
			continue
		}
		p := filepath.Join(scenarioDir, s.entry.base())
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(p, data, 0o644); err != nil {
			return nil, err
		}
		res.Files++
		res.Bytes += int64(len(data))
		res.Events += len(s.labels)
	}
	for ref := range refSet {
		e, ok := lookup(ref)
		if !ok {
			res.Missing = append(res.Missing, ref)
			continue
		}
		if e.encrypted {
			continue // 已在 Encrypted 里报告
		}
		data, err := assets.read(e)
		if err != nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("读素材 %s 失败：%v", e.base(), err))
			continue
		}
		p := filepath.Join(imageDir, e.base())
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(p, data, 0o644); err != nil {
			return nil, err
		}
		res.Files++
		res.Bytes += int64(len(data))
	}
	sort.Strings(res.Missing)

	// ④ 写约定文件（加载器按它找剧本目录与素材目录）。
	conv := adapter.DefaultKiriKiri()
	if err := adapter.SaveConventionFile(dest, conv); err != nil {
		res.Warnings = append(res.Warnings, fmt.Sprintf("写约定文件失败：%v", err))
	}
	return res, nil
}
