// Package unpack 把 galgame 归档解成「只够重演」的最小集合。
//
// 目标（用户提出的方向）：拖一个 galgame 进来 → 自动识别引擎 → 解出
// **只够重演 CG 事件**的最小集合 → 写成现有加载器能直接吃的目录 → 登记入库。
//
// 引擎支持现状（阶段 1/2）：
//   - KiriKiri（XP3）：完整支持（多段拼接 + zlib 解压 + 依赖闭包 + 落盘）。
//   - Artemis（PFS + 明文 Lua .ast）/ BGI（ARC + .bs）：能解出最小集合，
//     但**事件库接入（登记进游戏库）仍只支持 KiriKiri**（library.Scan 是
//     KiriKiri 形状的）。
//   - CatSystem2（.int/.kif + .cst）：解包已接上（事件粒度 = 一份 .cst
//     一个事件，与 BuildIR 同口径）；事件库登记仍由下一条 PR 的 library
//     分派接入，本分支不改变登记行为。
//   - 加密条目（XP3 adlr / BSE 加密图）：**不解密**，如实报告跳过（要每作密钥）。
//
// 落盘布局刻意与现有加载器一致（raw/data/scenario + raw/data/image），
// 所以解包产物能直接被 library.Scan + replay.GameLoader 消费，不需要改加载侧。
package unpack

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"ucgr/internal/adapter"
)

// Scope 最小集合的取材范围。
type Scope string

const (
	// ScopeCG 只要重演 CG 事件所需（默认）：事件脚本 + 它们引用的图。
	ScopeCG Scope = "cg"
	// ScopeAll 全部事件所需：所有场景/段落脚本引用到的素材。
	ScopeAll Scope = "all"
)

// Plan 解包预览（拖进来先看「会解出什么」，不落盘）。
type Plan struct {
	EngineID      string   `json:"engine_id"`
	Root          string   `json:"root"`
	Kind          string   `json:"kind"`           // loose=已是解包目录 / archive=归档
	Archives      []string `json:"archives"`       // 发现的归档
	ScenarioFiles int      `json:"scenario_files"` // 剧本文件数
	Labels        int      `json:"labels"`         // 事件（标签）数
	Refs          int      `json:"refs"`           // 引用到的素材数
	Videos        int      `json:"videos"`         // 其中影片（OP/ED）条目数
	VideoBytes    int64    `json:"video_bytes"`    // 影片原始体积（估算）
	// EstBytes/EstFiles 这次解包预计产出多少字节/文件（闭包里的引用总量）
	EstBytes    int64    `json:"est_bytes"`
	EstFiles    int      `json:"est_files"`
	Encrypted   int      `json:"encrypted"` // 加密条目数（不解，要密钥）
	Notes       []string `json:"notes"`
	Unsupported bool     `json:"unsupported"` // 引擎暂不支持自动解包
	Reason      string   `json:"reason"`      // 不支持的原因（对用户明说）
}

// Result 解包结果。
type Result struct {
	Dest      string   `json:"dest"`
	EngineID  string   `json:"engine_id"`
	Events    int      `json:"events"`
	Files     int      `json:"files"`
	Bytes     int64    `json:"bytes"`
	Missing   []string `json:"missing"`   // 引用了但归档里找不到的素材
	Encrypted []string `json:"encrypted"` // 加密条目（跳过）
	Warnings  []string `json:"warnings"`
	// SkippedVideo 按选项跳过的影片数
	SkippedVideo int `json:"skipped_video,omitempty"`
	// Reused 已存在且大小一致、直接复用没有重写的文件数（增量解包）
	Reused int `json:"reused,omitempty"`
}

// Preview 预览一个游戏目录/归档，不落盘（解包前先给用户看「会解出什么」）。
func Preview(root string) (*Plan, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, fmt.Errorf("没有选择路径")
	}
	st, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("读不到 %s：%w", root, err)
	}
	if !st.IsDir() {
		engine := detectEngine(root)
		if engine == "" {
			return &Plan{
				Root:        root,
				Kind:        "file",
				Unsupported: true,
				Reason:      fmt.Sprintf("认不出 %s 的引擎（支持的归档后缀：%s）", filepath.Base(root), archiveExtsHint()),
			}, nil
		}
		return planEngine(engine, []string{root})
	}
	// 已是解包目录？
	if st, err := os.Stat(filepath.Join(root, "raw", "data", "scenario")); err == nil && st.IsDir() {
		return &Plan{
			Root:     root,
			Kind:     "loose",
			EngineID: "kirikiri",
			Notes:    []string{"这已经是解包目录，直接登记即可（无需解包）"},
		}, nil
	}
	engine := detectEngine(root)
	if engine == "" {
		return &Plan{
			Root:        root,
			Kind:        "unknown",
			Unsupported: true,
			Reason:      "这个目录里没找到可识别的归档（也没找到已解包结构 raw/data/scenario）",
		}, nil
	}
	archives, err := findArchives(root, engine)
	if err != nil {
		return nil, err
	}
	if len(archives) == 0 {
		return &Plan{
			Root:        root,
			Kind:        "unknown",
			EngineID:    engine,
			Unsupported: true,
			Reason:      fmt.Sprintf("认出是 %s 引擎，但目录里没有它的归档文件", engine),
		}, nil
	}
	return planEngine(engine, archives)
}

// Options 解包选项（解密方案等）。
type Options struct {
	Scope   Scope  // 取材范围
	Decrypt Scheme // 解密方案（空 = 不解密）
	Key     byte   // 固定密钥（SchemeXor 用）
	// SkipVideo 不提取影片（OP/ED）。单个影片就 100~300 MB，全解出来很占盘；
	// 关掉后视频事件仍在事件库里，只是播放时缺素材（零值 = 提取，向后兼容）。
	SkipVideo bool
	// Force 忽略「已存在且大小一致就复用」的增量优化，全部重写。
	// 用途：换了汉化/修正补丁但文件大小碰巧没变时，增量会误判为「没变」，
	// 用户看到一个按钮就能强制重来（比默认存哈希清单便宜）。
	Force bool
}

// Extract 解包到 dest（dest 会自动创建），写出最小集合 + 约定文件。
func Extract(root, dest string, scope Scope) (*Result, error) {
	return ExtractWith(root, dest, Options{Scope: scope})
}

// ExtractWith 按选项解包。
func ExtractWith(root, dest string, opts Options) (*Result, error) {
	root = strings.TrimSpace(root)
	dest = strings.TrimSpace(dest)
	if root == "" || dest == "" {
		return nil, fmt.Errorf("解包需要源路径与目标目录")
	}
	if opts.Scope == "" {
		opts.Scope = ScopeCG
	}
	st, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("读不到 %s：%w", root, err)
	}
	if st.IsDir() {
		if s, err := os.Stat(filepath.Join(root, "raw", "data", "scenario")); err == nil && s.IsDir() {
			return &Result{
				Dest:     root,
				EngineID: "kirikiri",
				Warnings: []string{"源目录已是解包结构，未做复制（直接登记即可）"},
			}, nil
		}
		engine := detectEngine(root)
		if engine == "" {
			return nil, fmt.Errorf("认不出这个目录的引擎")
		}
		archives, err := findArchives(root, engine)
		if err != nil {
			return nil, err
		}
		if len(archives) == 0 {
			return nil, fmt.Errorf("没找到 %s 引擎的归档文件", engine)
		}
		return extractEngine(engine, archives, dest, opts)
	}
	engine := detectEngine(root)
	if engine == "" {
		return nil, fmt.Errorf("认不出 %s 的引擎", filepath.Base(root))
	}
	return extractEngine(engine, []string{root}, dest, opts)
}

// planEngine 按引擎预览。
func planEngine(engine string, archives []string) (*Plan, error) {
	deps, ok := engineFor(engine)
	if !ok {
		return &Plan{
			Root:        strings.Join(archives, " "),
			Kind:        "archive",
			Unsupported: true,
			Reason:      fmt.Sprintf("引擎 %s 暂不支持自动解包", engine),
		}, nil
	}
	assets, err := openArchives(archives, deps.ParseArchive, deps.OpenReader)
	if err != nil {
		// 解析失败要说清「它其实是什么」——实测有 .xp3 里装着 ZIP / EVB。
		// 嗅探必须针对真正失败的那个归档，而不是 archives[0]。
		hintPath := failedArchivePath(err)
		if hintPath == "" && len(archives) > 0 {
			hintPath = archives[0]
		}
		hint := ""
		if hintPath != "" {
			hint = sniffHint(hintPath)
		}
		reason := err.Error()
		if hint != "" {
			reason += "；" + hint
		}
		return &Plan{
			Root:        strings.Join(archives, " "),
			Kind:        "archive",
			EngineID:    engine,
			Archives:    archives,
			Unsupported: true,
			Reason:      reason,
		}, nil
	}
	defer assets.close()

	p := &Plan{
		EngineID:  engine,
		Root:      strings.Join(archives, " "),
		Kind:      "archive",
		Archives:  archives,
		Encrypted: len(assets.adlr), // 只算 XP3 的 adlr（CS2 的 Blowfish 会自动解）
	}
	if deps.ScriptNote != "" {
		p.Notes = append(p.Notes, deps.ScriptNote)
	}
	refs := map[string]bool{}
	for path, e := range assets.entries {
		if !deps.IsScript(path) {
			continue
		}
		p.ScenarioFiles++
		data, err := assets.read(e)
		if err != nil {
			p.Notes = append(p.Notes, fmt.Sprintf("读 %s 失败：%v", e.base(), err))
			continue
		}
		info, err := deps.ParseScript(data, e.base())
		if err != nil {
			p.Notes = append(p.Notes, fmt.Sprintf("解析 %s 失败：%v", e.base(), err))
			continue
		}
		p.Labels += len(info.Labels)
		for _, r := range info.Refs {
			refs[r] = true
		}
		for _, r := range info.VideoRefs {
			refs[r] = true
		}
	}
	p.Refs = len(refs)
	// 顺便数出影片条目与体积：用户据此决定「要不要把它们一起解出来」。
	for r := range refs {
		e, ok := assets.lookup(r)
		if !ok {
			continue
		}
		for _, sg := range e.segs {
			p.EstBytes += sg.Size
		}
		p.EstFiles++
		if isVideoPath(e.path) {
			p.Videos++
			for _, sg := range e.segs {
				p.VideoBytes += sg.Size
			}
		}
	}
	if len(assets.skipped) > 0 {
		for _, sk := range assets.skipped {
			p.Notes = append(p.Notes, "跳过无法解析的归档 —— "+sk)
		}
	}
	p.Notes = append(p.Notes, assets.warnings...)
	if p.Encrypted > 0 {
		if engine == "kirikiri" {
			p.Notes = append(p.Notes, fmt.Sprintf(
				"有 %d 个条目带 adlr 校验和（**多半不是加密**，只是校验和；若解出的内容是乱码，可选解密方案重试）",
				p.Encrypted))
		} else {
			p.Notes = append(p.Notes, fmt.Sprintf(
				"有 %d 个条目带加密标记，当前不会自动解密；若解出的内容是乱码，需要每作密钥",
				p.Encrypted))
		}
	}
	if p.Unsupported {
		p.Notes = append(p.Notes, p.Reason)
	}
	return p, nil
}

// extractEngine 按引擎解包并写出最小集合。
func extractEngine(engine string, archives []string, dest string, opts Options) (*Result, error) {
	scope := opts.Scope
	deps, ok := engineFor(engine)
	if !ok {
		return nil, fmt.Errorf("引擎 %s 暂不支持自动解包", engine)
	}
	assets, err := openArchives(archives, deps.ParseArchive, deps.OpenReader)
	if err != nil {
		hintPath := failedArchivePath(err)
		if hintPath == "" && len(archives) > 0 {
			hintPath = archives[0]
		}
		hint := ""
		if hintPath != "" {
			hint = sniffHint(hintPath)
		}
		if hint != "" {
			return nil, fmt.Errorf("%w；%s", err, hint)
		}
		return nil, err
	}
	defer assets.close()

	// Encrypted 记「本次实际碰到、且带 adlr 校验和」的条目——它们可能被加密，
	// 也可能只是校验和（解出来是乱码时再选解密方案）。
	res := &Result{Dest: dest, EngineID: engine}
	// 先建目标目录：素材/剧本都可能一条都写不出（比如脚本里全是动态引用），
	// 那时约定文件仍要落盘，否则加载器读不到这份解包目录。
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return nil, fmt.Errorf("建解包目录 %s 失败：%w", dest, err)
	}
	var videoNames []string // 解出的影片（相对产物根），写进清单备查
	totalFiles, totalBytes := 0, int64(0)

	// 增量是否可信：只有「源归档与上次解包时一致」才允许复用已存在的文件。
	// 否则说明用户换了补丁/更新了游戏，即使产物大小碰巧一样也可能内容变了；
	// 解密方案/密钥变了同样不可复用（旧产物是另一套字节）。
	curMarks := markArchives(archives)
	prev := readManifest(dest)
	archivesUnchanged := prev != nil && sameMarks(prev.Archives, curMarks) &&
		prev.Decrypt == string(opts.Decrypt) && prev.Key == int(opts.Key)
	if prev != nil && !archivesUnchanged {
		res.Warnings = append(res.Warnings,
			"源归档与上次解包时不一致（换了补丁或更新过），本次全部重解")
	}
	for _, sk := range assets.skipped {
		res.Warnings = append(res.Warnings, "跳过无法解析的归档 —— "+sk)
	}
	res.Warnings = append(res.Warnings, assets.warnings...)
	if deps.ScriptNote != "" {
		res.Warnings = append(res.Warnings, deps.ScriptNote)
	}
	refSet := map[string]bool{}
	scripts, scriptWarns := pickScripts(assets, deps.IsScript)
	res.Warnings = append(res.Warnings, scriptWarns...)
	for _, e := range scripts {
		data, err := assets.read(e)
		if err != nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("读 %s 失败：%v", e.base(), err))
			continue
		}
		// 解密（解压之后，与 arc_unpacker 同口径）。adlr 只是校验和，
		// 没选方案时按明文处理——很多游戏带 adlr 但内容是明文。
		if opts.Decrypt != SchemeNone {
			plain, derr := ApplyDecrypt(opts.Decrypt, e.segs[0], data, opts.Key)
			if derr != nil {
				res.Warnings = append(res.Warnings, fmt.Sprintf("%s 解密失败：%v", e.base(), derr))
				continue
			}
			data = plain
		}
		info, err := deps.ParseScript(data, e.base())
		if err != nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("解析 %s 失败：%v", e.base(), err))
			continue
		}
		if info.Dynamic {
			res.Warnings = append(res.Warnings,
				fmt.Sprintf("%s 里有动态素材引用（运行时拼文件名），这部分无法静态收集", e.base()))
		}
		refs := info.Refs
		if scope == ScopeAll {
			refs = info.AllRefs
		}
		for _, r := range refs {
			refSet[r] = true
		}
		// 影片总是并入（OP/ED 不在 CG 画廊引用里）；SkipVideo 时反而不并。
		if !opts.SkipVideo {
			for _, r := range info.VideoRefs {
				refSet[r] = true
			}
		}
		// 剧本落盘
		p := filepath.Join(dest, filepath.FromSlash(deps.ScriptDir), e.base())
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return nil, err
		}
		totalFiles++
		totalBytes += int64(len(data))
		if !opts.Force && archivesUnchanged && sameSize(p, int64(len(data))) {
			// 已经解过且大小一致：内容来自同一份归档，不重写（增量）。
			res.Reused++
		} else {
			if err := os.WriteFile(p, data, 0o644); err != nil {
				return nil, err
			}
			res.Files++
			res.Bytes += int64(len(data))
		}
		res.Events += len(info.Labels)
		// 兼容台本：宏家族（Mai@KF.dll 一系）的 .wks 用自定义宏画图，现有台本
		// 构建器只认标准 KAG 标签——不翻一份就播不出画面。翻出来的 .ks 与
		// 加载器的扫描口径一致（场景脚本），原脚本同时保留。
		if deps.ScriptCompat != nil {
			compat, warns := deps.ScriptCompat(data, e.base(), func(ref string) bool {
				_, ok := assets.lookup(ref)
				return ok
			})
			res.Warnings = append(res.Warnings, warns...)
			if len(compat) > 0 {
				cp := filepath.Join(dest, filepath.FromSlash(deps.ScriptDir), e.stem()+".ks")
				if err := os.WriteFile(cp, compat, 0o644); err != nil {
					return nil, err
				}
				res.Files++
				res.Bytes += int64(len(compat))
			}
		}
	}
	// 素材落盘（只解被引用的）。
	for ref := range refSet {
		e, ok := assets.lookup(ref)
		if !ok {
			res.Missing = append(res.Missing, ref)
			continue
		}
		if opts.SkipVideo && isVideoPath(e.path) {
			res.SkippedVideo++
			continue
		}
		if e.encrypted && opts.Decrypt == SchemeNone && strings.EqualFold(e.segs[0].KeyHint, "adlr") {
			// 带 adlr 的条目：先按明文试（多数是校验和，不是加密）；
			// 真解不开时上面的解析失败告警会指出。
			res.Encrypted = append(res.Encrypted, e.path)
		}
		assetPath := filepath.Join(dest, filepath.FromSlash(deps.ImageDir), e.base())
		want := int64(0)
		for _, sg := range e.segs {
			want += sg.Size
		}
		// 增量：已存在且大小与归档记录一致 → 连归档都不用读（省 IO 与解密）。
		// 只在不解密时按此判定：解密会改变字节数，大小对不上就老老实实重解。
		if !opts.Force && archivesUnchanged && opts.Decrypt == SchemeNone && sameSize(assetPath, want) {
			res.Reused++
			totalFiles++
			totalBytes += want
			if isVideoPath(e.path) {
				rel, rerr := filepath.Rel(dest, assetPath)
				if rerr == nil {
					videoNames = append(videoNames, filepath.ToSlash(rel))
				}
			}
			continue
		}
		data, err := assets.read(e)
		if err != nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("读素材 %s 失败：%v", e.base(), err))
			continue
		}
		if opts.Decrypt != SchemeNone {
			plain, derr := ApplyDecrypt(opts.Decrypt, e.segs[0], data, opts.Key)
			if derr != nil {
				res.Warnings = append(res.Warnings, fmt.Sprintf("%s 解密失败：%v", e.base(), derr))
				continue
			}
			data = plain
		}
		if err := os.MkdirAll(filepath.Dir(assetPath), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(assetPath, data, 0o644); err != nil {
			return nil, err
		}
		res.Files++
		res.Bytes += int64(len(data))
		totalFiles++
		totalBytes += int64(len(data))
		if isVideoPath(e.path) {
			rel, rerr := filepath.Rel(dest, assetPath)
			if rerr == nil {
				videoNames = append(videoNames, filepath.ToSlash(rel))
			}
		}
	}
	// 记下这次依据的归档指纹，供下次判断「能不能复用」。
	if err := writeManifest(dest, archives, videoNames, totalFiles, totalBytes, opts); err != nil {
		res.Warnings = append(res.Warnings, "写归档指纹失败："+err.Error())
	}

	sort.Strings(res.Missing)

	// 写约定文件（加载器按它找剧本目录）。非 KiriKiri 引擎的画廊/标签规则
	// 与 KiriKiri 不同，置空——那些字段只在 KiriKiri 的事件切分里用。
	conv := adapter.Convention{
		EngineID:    engine,
		ScenarioDir: deps.ScriptDir,
		ScriptExt:   deps.ScriptExt,
	}
	if engine == "kirikiri" {
		conv = adapter.DefaultKiriKiri()
		conv.ScenarioDir = deps.ScriptDir
	}
	// 已有约定文件 = 用户手工调过目录/命名，重新解包不能冲掉：保留用户写过的
	// 字段，只把留空的补成引擎默认值。解析不了的文件一律不动并告警，避免把
	// 用户配置悄悄改成默认（写坏时静默覆写最难排查）。
	if existing, ok, err := adapter.LoadConventionFile(dest); err != nil {
		res.Warnings = append(res.Warnings,
			fmt.Sprintf("%s 读取/解析失败，已保留原文件不覆写：%v", adapter.ConventionFileName, err))
	} else {
		if ok {
			conv = existing.Merge(conv)
		}
		if err := adapter.SaveConventionFile(dest, conv); err != nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("写约定文件失败：%v", err))
		}
	}
	return res, nil
}

// pickScripts 取出全部脚本条目，并按「基名」去重成确定的一份。
//
// 真机语料里 scenario.xp3 与 append.xp3 会有同名脚本（append 是补丁层，
// 路径不带子目录；scenario 里是 00/SKAC_001r.wks 这种），内容几乎一致。
// 两份都落盘会互相覆盖（map 迭代顺序随机 → 解出的版本会飘），所以按基名
// 只留一份：append/patch 归档优先，其次路径短、再字典序，并给出告警。
func pickScripts(s *archiveSet, isScript func(string) bool) ([]archiveEntry, []string) {
	type cand struct {
		e     archiveEntry
		owner string
	}
	groups := map[string][]cand{}
	var keys []string
	for p, e := range s.entries {
		if !isScript(p) {
			continue
		}
		k := strings.ToLower(e.base())
		if _, ok := groups[k]; !ok {
			keys = append(keys, k)
		}
		groups[k] = append(groups[k], cand{e: e, owner: s.owner[p]})
	}
	sort.Strings(keys)
	out := make([]archiveEntry, 0, len(keys))
	var warns []string
	for _, k := range keys {
		cs := groups[k]
		sort.Slice(cs, func(i, j int) bool {
			pi, pj := archPriority(cs[i].owner), archPriority(cs[j].owner)
			if pi != pj {
				return pi < pj
			}
			if len(cs[i].e.path) != len(cs[j].e.path) {
				return len(cs[i].e.path) < len(cs[j].e.path)
			}
			return cs[i].e.path < cs[j].e.path
		})
		out = append(out, cs[0].e)
		if len(cs) > 1 {
			others := make([]string, 0, len(cs)-1)
			for _, c := range cs[1:] {
				others = append(others, filepath.Base(c.owner)+":"+c.e.path)
			}
			warns = append(warns, fmt.Sprintf("脚本 %s 在多个归档里有同名副本，已取 %s:%s（忽略 %s）",
				cs[0].e.base(), filepath.Base(cs[0].owner), cs[0].e.path, strings.Join(others, "、")))
		}
	}
	return out, warns
}

// archPriority 归档优先级：append/patch 是补丁层，同名文件以它为准。
func archPriority(arch string) int {
	b := strings.ToLower(filepath.Base(arch))
	if strings.HasPrefix(b, "append") || strings.HasPrefix(b, "patch") {
		return 0
	}
	return 1
}

// maxArchiveDepth 归档搜索的目录层数（含根）：根 + 两级子目录。
// 整合包常把 xp3 放在 <game>/data/ 之类的位置，只搜一层会漏；再往下收益低，
// 还容易扫到存档、备份目录。
const maxArchiveDepth = 3

// findArchives 在目录里找某引擎的归档（根 + 两级子目录，避免扫全盘）。
func findArchives(root, engine string) ([]string, error) {
	deps, ok := engineFor(engine)
	if !ok {
		return nil, nil
	}
	// 不用 filepath.WalkDir：Windows 目录联接（游戏库里常见）会被它当成
	// 单个文件（Lstat 不跟随），导致「目录明明在、归档却一个没找到」。
	// 手动递归 + 深度上限。
	var out []string
	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		ents, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, d := range ents {
			path := filepath.Join(dir, d.Name())
			// 注意：ReadDir 的条目对 Windows 目录联接报 IsDir()=false，Type 也不带
			// ModeSymlink——只有 Stat 会跟随重解析点。所以「不是目录」时要再 Stat
			// 兜底，否则联接子目录整片看不见（实测：语料根下 15 个联接全被漏掉）。
			// Stat 只在下探还有余量时做，末层不再付这个代价。
			isDir := d.IsDir()
			if !isDir && depth+1 < maxArchiveDepth {
				if st, err := os.Stat(path); err == nil && st.IsDir() {
					isDir = true
				}
			}
			if isDir {
				if depth+1 < maxArchiveDepth {
					walk(path, depth+1)
				}
				continue
			}
			ext := strings.ToLower(filepath.Ext(path))
			for _, ae := range deps.ArchiveExts {
				if ext == strings.ToLower(ae) {
					out = append(out, path)
					break
				}
			}
		}
	}
	walk(root, 0)
	sort.Strings(out)
	return out, nil
}

// archiveExtsHint 支持的归档后缀（给用户看的提示）。
func archiveExtsHint() string {
	var exts []string
	for _, d := range engineRegistry {
		exts = append(exts, d.ArchiveExts...)
	}
	sort.Strings(exts)
	return strings.Join(exts, " / ")
}

// videoExts 影片扩展名。解包层只关心「要不要提取」，
// 不需要依赖解码实现（internal/video 是 ffmpeg 提帧那一层）。
var videoExts = map[string]bool{
	".mpg": true, ".mpeg": true, ".wmv": true, ".mp4": true,
	".avi": true, ".webm": true, ".mov": true, ".mkv": true,
}

func isVideoPath(p string) bool {
	return videoExts[strings.ToLower(filepath.Ext(p))]
}

// CountVideos 数一个目录里已有的影片文件（用于「这个游戏之前提过没」的判断）。
//
// 以磁盘实况为准而不是记一个布尔标记：用户可能手动拷过影片、也可能删过，
// 只有实际文件才决定「能不能播」。
func CountVideos(root string) int {
	n := 0
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if isVideoPath(p) {
			n++
		}
		return nil
	})
	return n
}

// sameSize 目标文件是否已存在且大小一致（增量解包的判定口径）。
//
// 只比大小不比内容：这些文件来自同一份归档，大小一致即可放心复用；
// 换成哈希就要把文件读一遍，反而把省下的 IO 花回去。
func sameSize(path string, want int64) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir() && st.Size() == want
}
