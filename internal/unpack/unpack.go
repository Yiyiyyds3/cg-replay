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
//   - CatSystem2（.int/.kif + .cst）：完整支持（2026-10-26 接上；此前误判为
//     「无脚本解析」）。事件粒度 = 一份 .cst 一个事件（与 BuildIR 同口径）。
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
	Encrypted     int      `json:"encrypted"`      // 加密条目数（不解，要密钥）
	Notes         []string `json:"notes"`
	Unsupported   bool     `json:"unsupported"` // 引擎暂不支持自动解包
	Reason        string   `json:"reason"`      // 不支持的原因（对用户明说）
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
	assets, err := openArchives(archives, deps.ParseArchive)
	if err != nil {
		// 解析失败要说清「它其实是什么」——实测有 .xp3 里装着 ZIP / EVB。
		hint := ""
		if len(archives) > 0 {
			hint = sniffHint(archives[0])
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
		Encrypted: len(assets.encrypted),
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
		if !ok || !isVideoPath(e.path) {
			continue
		}
		p.Videos++
		for _, sg := range e.segs {
			p.VideoBytes += sg.Size
		}
	}
	if p.Encrypted > 0 {
		p.Notes = append(p.Notes, fmt.Sprintf(
			"有 %d 个条目带 adlr 校验和（**多半不是加密**，只是校验和；若解出的内容是乱码，可选解密方案重试）",
			p.Encrypted))
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
	assets, err := openArchives(archives, deps.ParseArchive)
	if err != nil {
		hint := ""
		if len(archives) > 0 {
			hint = sniffHint(archives[0])
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
	refSet := map[string]bool{}
	for path, e := range assets.entries {
		if !deps.IsScript(path) {
			continue
		}
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
		if !opts.Force && sameSize(p, int64(len(data))) {
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
		if e.encrypted && opts.Decrypt == SchemeNone {
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
		if !opts.Force && opts.Decrypt == SchemeNone && sameSize(assetPath, want) {
			res.Reused++
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
	if err := adapter.SaveConventionFile(dest, conv); err != nil {
		res.Warnings = append(res.Warnings, fmt.Sprintf("写约定文件失败：%v", err))
	}
	return res, nil
}

// findArchives 在目录里找某引擎的归档（顶层 + 两层子目录，避免扫全盘）。
func findArchives(root, engine string) ([]string, error) {
	deps, ok := engineFor(engine)
	if !ok {
		return nil, nil
	}
	var out []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // 单个目录读不了就跳过
		}
		if d.IsDir() {
			rel, _ := filepath.Rel(root, path)
			if rel != "." && strings.Count(filepath.ToSlash(rel), "/") >= 2 {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		for _, ae := range deps.ArchiveExts {
			if ext == strings.ToLower(ae) {
				out = append(out, path)
				break
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("扫描归档失败：%w", err)
	}
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
