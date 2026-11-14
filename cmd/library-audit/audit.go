package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"ucgr/internal/adapter"
	"ucgr/internal/cover"
	"ucgr/internal/ir"
	"ucgr/internal/library"
	"ucgr/internal/replay"
	"ucgr/internal/unpack"

	// 引擎适配器的 Identify 只有被 import 才会注册进 adapter 注册表。
	// library.DetectEngine 的通用竞聘（IdentifyPath）依赖这张表，所以审计入口要
	// 把这些包都拉进来——否则 DetectEngine 只能认出 kirikiri/scn，
	// 其余引擎一律返回 ""（与 cmd/ucgr 的注册范围保持一致）。
	_ "ucgr/internal/adapter/artemis"
	_ "ucgr/internal/adapter/bgi"
	_ "ucgr/internal/adapter/catsystem2"
	_ "ucgr/internal/adapter/kirikiri"
	_ "ucgr/internal/adapter/nexas"
	_ "ucgr/internal/adapter/nitro"
	_ "ucgr/internal/adapter/nscript"
	_ "ucgr/internal/adapter/scn"
)

// Verdict 单款审计结论（-out/<分组>_<序号>.json 的完整字段）。
type Verdict struct {
	Group string `json:"group"`
	Game  string `json:"game"`
	Path  string `json:"path"`
	// RealPath 实体目录：游戏库的「按评价/<分组>/<游戏>」大量是 Junction
	// （指向 20260424/... 下的实体目录），Path 是索引视图，这里记解析后的真实目录。
	RealPath       string `json:"real_path,omitempty"`
	EngineGuess    string `json:"engine_guess"`
	EngineDetected string `json:"engine_detected"`
	// EngineResolved 实际用来构造 Convention 的引擎 id：DetectEngine 优先，
	// 空则用 TSV 的 engine_guess 兜底映射。空 = 两边都没认出来。
	EngineResolved string `json:"engine_resolved,omitempty"`

	TopEntries int `json:"top_entries"`
	Events     int `json:"events"`
	CG         int `json:"cg"`
	Scenes     int `json:"scenes"`
	Chapters   int `json:"chapters"`
	TextLines  int `json:"text_lines"`
	Voices     int `json:"voices"`
	// IRProbed 实际构建 IR 的事件数（text_lines/voices 的取样分母，0 = 未探测）。
	IRProbed int `json:"ir_probed_events"`

	CanUnpack    bool     `json:"can_unpack"`
	CategoryDirs []string `json:"category_dirs"`
	// AssetDirs 素材目录抽样（名字像素材且含 ≥5 张图的目录，最多 12 条）。
	// category_dirs 为空的款靠它说明「素材其实在别处」（如 RPG Maker 的 www/img/pictures）。
	AssetDirs     []string `json:"asset_dirs,omitempty"`
	CoverValid    bool     `json:"cover_valid"`
	CoverSource   string   `json:"cover_source,omitempty"`
	CoverNonBlack float64  `json:"cover_non_black,omitempty"`
	UnpackNotes   []string `json:"unpack_notes,omitempty"`
	Warnings      []string `json:"warnings,omitempty"`
	Errors        []string `json:"errors"`

	Verdict    string `json:"verdict"`
	Reason     string `json:"reason"`
	DurationMs int64  `json:"duration_ms,omitempty"`
}

// mainEngines main 分支上 library.ScanWith 真正有分派的引擎（见 library.go:143-162）。
//
// 不在这张表里的（Ren'Py 分支态、Unreal / Godot / Cocos2d-x / GameMaker /
// WolfRPG / Electron(asar) / 未知 / 私有容器）一律 verdict=skip，
// reason=engine_unsupported_in_main，并写明「main 无该引擎 ScanXxx 分派」。
var mainEngines = map[string]bool{
	"kirikiri":   true, // scanWith 默认分支
	"artemis":    true,
	"bgi":        true,
	"catsystem2": true,
	"scn":        true,
	"nscript":    true,
	"unity":      true,
	"rmmz":       true,
	"nexas":      true,
	"nitro":      true,
}

// mainEnginesHuman 报错里给人看的支持清单（顺序稳定）。
var mainEnginesHuman = []string{
	"kirikiri", "artemis", "bgi", "catsystem2", "scn", "nscript", "unity", "rmmz", "nexas", "nitro",
}

// guessEngineMap TSV 的 engine_guess 文本 → 引擎 id。
//
// TSV 的推断是人/机器混合口径（"NW.js / RPG Maker(MV/MZ)"、"已适配(ucgr)"、
// "未知(仅 game/ 目录)"…），这里只做人话到 id 的映射；认不出来的留空，
// 交给「无入口 → skip」处理，绝不瞎猜一个引擎去扫（错判比不判更坏）。
var guessEngineMap = map[string]string{
	"unity":                    "unity",
	"kirikiri":                 "kirikiri",
	"nscripter":                "nscript",
	"nscripter/onscripter":     "nscript",
	"nscript":                  "nscript",
	"rmmz":                     "rmmz",
	"rpg maker":                "rmmz",
	"nw.js / rpg maker(mv/mz)": "rmmz",
	"已适配(ucgr)":                "rmmz",
	"scn":                      "scn",
	"artemis":                  "artemis",
	"bgi":                      "bgi",
	"catsystem2":               "catsystem2",
	"nexas":                    "nexas",
	"nitro":                    "nitro",
	// 以下没有 main 分派，映射出来只为了把「卡点类型」写清楚。
	"ren'py":        "renpy",
	"renpy":         "renpy",
	"unreal engine": "unreal",
	"unreal":        "unreal",
	"godot":         "godot",
	"cocos2d-x":     "cocos",
	"gamemaker":     "gamemaker",
	"wolfrpg":       "wolfrpg",
	"electron":      "electron",
}

// normalizeEngineGuess 把清单里的引擎推断文本映射成引擎 id（认不出返回 ""）。
func normalizeEngineGuess(guess string) string {
	k := strings.ToLower(strings.TrimSpace(guess))
	if k == "" {
		return ""
	}
	if id, ok := guessEngineMap[k]; ok {
		return id
	}
	// 「未知」/「未知(仅 game/ 目录)」/「其它(自研/DX库)」等：不猜。
	return ""
}

// unsupportedMessage 引擎无 main 分派时的原始说明（进 errors，便于建 Issue 时贴）。
func unsupportedMessage(engine string) string {
	name := engine
	if name == "" {
		name = "未知（DetectEngine 与 engine_guess 都没认出来）"
	}
	return fmt.Sprintf(
		"main 无该引擎 ScanXxx 分派：engine=%s；library.ScanWith 当前只分派 %s —— 该款不进入扫描，判 skip（docs/14 ⑥：不死磕）",
		name, strings.Join(mainEnginesHuman, "/"))
}

// conventionForEngine 取引擎的默认约定。
//
// 特例：unity 在 main 上没有默认约定常量（ScanUnity(root) 不需要约定，
// 它的 CG 索引与目录由 internal/adapter/unitycg 内部探测），所以这里给一份
// 只有 EngineID 的空约定——ScanWith 只按 EngineID 分派到 ScanUnity。
func conventionForEngine(engine string) (adapter.Convention, bool) {
	if conv, ok := adapter.ConventionByEngine(engine); ok {
		return conv, true
	}
	if engine == "unity" {
		return adapter.Convention{EngineID: "unity"}, true
	}
	return adapter.Convention{}, false
}

// auditor 审计器。三个外部能力都可注入（单测不碰真实游戏目录）。
type auditor struct {
	timeout  time.Duration
	detect   func(root string) string
	scanWith func(root string, conv adapter.Convention) (*library.Library, error)
	preview  func(root string) (*unpack.Plan, error)
	probe    func(engine, root string, lib *library.Library) (textLines, voices, probed int, notes, errs []string)
}

// defaultAuditor 接真实实现。
func defaultAuditor(timeout time.Duration) *auditor {
	if timeout <= 0 {
		timeout = 15 * time.Minute
	}
	return &auditor{
		timeout:  timeout,
		detect:   library.DetectEngine,
		scanWith: library.ScanWith,
		preview:  unpack.Preview,
		probe:    defaultProbeIR,
	}
}

// auditWithTimeout 单款 goroutine + select 超时（docs/14 ⑥「单款 15 分钟不结论即 skip」）。
func auditWithTimeout(a *auditor, row TSVRow, absPath string) Verdict {
	ch := make(chan Verdict, 1) // 缓冲 1：超时后 goroutine 不永久阻塞
	go func() { ch <- a.audit(row, absPath) }()
	timeout := a.timeout
	if timeout <= 0 {
		timeout = 15 * time.Minute
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case v := <-ch:
		return v
	case <-timer.C:
		return Verdict{
			Group:        row.Group,
			Game:         row.Game,
			Path:         absPath,
			EngineGuess:  row.EngineGuess,
			CategoryDirs: []string{},
			Errors:       []string{fmt.Sprintf("单款审计超过 %s 未出结论（goroutine 超时被弃，判 skip）", timeout)},
			Verdict:      "skip",
			Reason:       "timeout",
			DurationMs:   timeout.Milliseconds(),
		}
	}
}

// audit 单款审计（顶层兜底 recover：任何 panic 都变成一条 errors + fail，不漏）。
func (a *auditor) audit(row TSVRow, absPath string) (v Verdict) {
	started := time.Now()
	v = Verdict{
		Group:        row.Group,
		Game:         row.Game,
		Path:         absPath,
		EngineGuess:  row.EngineGuess,
		CategoryDirs: []string{},
		Errors:       []string{},
	}
	defer func() {
		if r := recover(); r != nil {
			v.Errors = append(v.Errors, fmt.Sprintf("panic: %v", r))
			v.Verdict = "fail"
			v.Reason = "panic"
		}
		v.DurationMs = time.Since(started).Milliseconds()
	}()
	a.body(&v)
	return
}

// body 主体（只读；任何分支都要给出 verdict + reason）。
func (a *auditor) body(v *Verdict) {
	// ① 路径存在性 + 顶层条目数。
	st, err := os.Stat(v.Path)
	if err != nil {
		v.Errors = append(v.Errors, err.Error())
		v.Verdict, v.Reason = "fail", "path_missing"
		return
	}
	if !st.IsDir() {
		v.Errors = append(v.Errors, fmt.Sprintf("%s 不是目录", v.Path))
		v.Verdict, v.Reason = "fail", "path_not_dir"
		return
	}
	ents, err := os.ReadDir(v.Path)
	if err != nil {
		v.Errors = append(v.Errors, err.Error())
		v.Verdict, v.Reason = "fail", "path_unreadable"
		return
	}
	v.TopEntries = len(ents)
	if real := resolveJunction(v.Path); !strings.EqualFold(filepath.Clean(real), filepath.Clean(v.Path)) {
		v.RealPath = real
	}

	// ② 素材分类目录 + 封面（都只读，且都是「文件级」证据，不解码全图库）。
	v.CategoryDirs = scanCategoryDirs(v.Path)
	v.AssetDirs = scanAssetDirs(v.Path)
	valid, src, nonBlack, cerrs := probeCover(v.Path)
	v.CoverValid, v.CoverSource, v.CoverNonBlack = valid, src, nonBlack
	v.Errors = append(v.Errors, cerrs...)

	// ③ 引擎：DetectEngine 记账，DetectEngine 空才用 engine_guess 兜底。
	v.EngineDetected = a.detect(v.Path)
	engine := strings.ToLower(strings.TrimSpace(v.EngineDetected))
	if engine == "" {
		engine = normalizeEngineGuess(v.EngineGuess)
	}
	v.EngineResolved = engine

	// ④ main 没有该引擎的 ScanXxx 分派 → skip，且不再花时间（只多做一次只读预览）。
	if engine == "" || !mainEngines[engine] {
		v.Verdict, v.Reason = "skip", "engine_unsupported_in_main"
		v.Errors = append(v.Errors, unsupportedMessage(engine))
		a.recordPreview(v)
		return
	}

	// ⑤ 构造约定（引擎默认 + 游戏根目录 ucgr-convention.json 覆盖）。
	conv, ok := conventionForEngine(engine)
	if !ok {
		v.Verdict, v.Reason = "skip", "engine_unsupported_in_main"
		v.Errors = append(v.Errors, fmt.Sprintf("adapter.ConventionByEngine(%q) 没有默认约定", engine))
		a.recordPreview(v)
		return
	}
	conv.EngineID = engine
	if f, exists, ferr := adapter.LoadConventionFile(v.Path); ferr != nil {
		v.Errors = append(v.Errors, ferr.Error())
	} else if exists {
		conv = f.Merge(conv)
		conv.EngineID = engine
	}

	// ⑥ 扫描（panic 单独抓，文本原样进 errors）。
	lib, scanErr, panicTxt := a.scan(v.Path, conv)
	if panicTxt != "" {
		v.Errors = append(v.Errors, "panic: "+panicTxt)
		a.recordPreview(v)
		if v.CanUnpack {
			v.Verdict, v.Reason = "partial", "scan_panic_can_unpack"
		} else {
			v.Verdict, v.Reason = "fail", "scan_panic"
		}
		return
	}
	if scanErr != nil {
		v.Errors = append(v.Errors, scanErr.Error())
		a.recordPreview(v)
		if v.CanUnpack {
			v.Verdict, v.Reason = "partial", "scan_error_can_unpack"
		} else {
			v.Verdict, v.Reason = "fail", "scan_error"
		}
		return
	}
	if lib == nil {
		v.Errors = append(v.Errors, "library.ScanWith 返回 (nil, nil)：既没库也没错误")
		a.recordPreview(v)
		v.Verdict, v.Reason = "fail", "scan_nil"
		return
	}

	// ⑦ 事件数 / 台词 / 语音。
	v.CG, v.Scenes, v.Chapters = len(lib.CGs), len(lib.Scenes), len(lib.Chapters)
	v.Events = v.CG + v.Scenes
	if v.Events == 0 && v.Chapters > 0 {
		// 只有章节事件时章节就是回放单位（部分引擎只切章节）。
		v.Events = v.Chapters
	}
	v.Warnings = append(v.Warnings, lib.Warnings...)
	if a.probe != nil {
		tl, vo, probed, notes, perrs := a.probe(engine, v.Path, lib)
		v.TextLines, v.Voices, v.IRProbed = tl, vo, probed
		v.UnpackNotes = append(v.UnpackNotes, notes...)
		v.Errors = append(v.Errors, perrs...)
	}

	// ⑧ 零事件：再问一次「能不能解包」，能解 = partial，不能 = fail。
	if v.Events == 0 {
		a.recordPreview(v)
		if v.CanUnpack {
			v.Verdict, v.Reason = "partial", "no_events_can_unpack"
		} else {
			v.Verdict, v.Reason = "fail", "no_events"
		}
		return
	}

	v.Verdict, v.Reason = "pass", "events>0"
}

// scan 调 library.ScanWith，panic 转文本（不吞、不把 panic 当正常错误）。
func (a *auditor) scan(root string, conv adapter.Convention) (lib *library.Library, err error, panicTxt string) {
	defer func() {
		if r := recover(); r != nil {
			lib, err, panicTxt = nil, nil, fmt.Sprintf("%v", r)
		}
	}()
	lib, err = a.scanWith(root, conv)
	return lib, err, ""
}

// recordPreview 只读解包预览：记 can_unpack 与 Plan.Notes/Reason，不落盘。
func (a *auditor) recordPreview(v *Verdict) {
	if a.preview == nil {
		return
	}
	plan, panicTxt, err := a.previewSafe(v.Path)
	if panicTxt != "" {
		v.Errors = append(v.Errors, "unpack.Preview panic: "+panicTxt)
		return
	}
	if err != nil {
		v.Errors = append(v.Errors, err.Error())
		return
	}
	if plan == nil {
		return
	}
	v.CanUnpack = !plan.Unsupported && (plan.Kind == "archive" || len(plan.Archives) > 0)
	v.UnpackNotes = append(v.UnpackNotes,
		fmt.Sprintf("unpack: kind=%s engine=%s archives=%d scenario_files=%d labels=%d refs=%d encrypted=%d unsupported=%t",
			plan.Kind, plan.EngineID, len(plan.Archives), plan.ScenarioFiles, plan.Labels, plan.Refs, plan.Encrypted, plan.Unsupported))
	if plan.Reason != "" {
		v.UnpackNotes = append(v.UnpackNotes, "unpack.reason: "+plan.Reason)
	}
	v.UnpackNotes = append(v.UnpackNotes, plan.Notes...)
}

// previewSafe 调 unpack.Preview 并抓 panic。
func (a *auditor) previewSafe(root string) (plan *unpack.Plan, panicTxt string, err error) {
	defer func() {
		if r := recover(); r != nil {
			plan, err, panicTxt = nil, nil, fmt.Sprintf("%v", r)
		}
	}()
	plan, err = a.preview(root)
	return plan, "", err
}

// irProbeMax 单款 IR 探测的事件上限（取前 N 个章节）。text_lines/voices 是
// 「代码路径证据」而不是全量统计：审计层只证明「这台本构建得出来、指令种类对」，
// 全量统计属于回放层的事（第二层抽样真机）。
const irProbeMax = 8

// defaultProbeIR 建 IR 数台词/语音。只对 kirikiri 实现（library.BuildChapterEvent
// 是 KiriKiri 专属的章节 IR 构建入口）；其它引擎记 0 并显式写明「未探测」，
// 以免把「没查」误读成「没有台词」。
func defaultProbeIR(engine, root string, lib *library.Library) (textLines, voices, probed int, notes, errs []string) {
	if engine != "kirikiri" {
		notes = append(notes,
			fmt.Sprintf("IR 探测只对 kirikiri 实现（本款 engine=%s）：text_lines/voices 记 0 = 未探测，不代表「无台词/无语音」", engine))
		return 0, 0, 0, notes, nil
	}
	if len(lib.Chapters) == 0 && len(lib.Scenes) == 0 {
		notes = append(notes, "库内既无章节也无场景事件，跳过 IR 探测")
		return 0, 0, 0, notes, nil
	}
	if len(lib.Chapters) > 0 {
		for i := 0; i < len(lib.Chapters) && probed < irProbeMax; i++ {
			ch := &lib.Chapters[i]
			ev, perr, ptxt := buildChapterSafe(root, ch)
			probed++
			if ptxt != "" {
				errs = append(errs, fmt.Sprintf("章节 %s IR 构建 panic: %s", ch.ID, ptxt))
				continue
			}
			if perr != nil {
				errs = append(errs, fmt.Sprintf("章节 %s IR 构建失败: %v", ch.ID, perr))
				continue
			}
			t, vc := countEventText(ev)
			textLines += t
			voices += vc
		}
		notes = append(notes, fmt.Sprintf("IR 探测取前 %d 个章节：%d 个成功入账", len(lib.Chapters), probed))
		return textLines, voices, probed, notes, errs
	}
	// 没有章节事件：退一步把场景整份剧本当「从头演」的章节建 IR。
	for i := 0; i < len(lib.Scenes) && probed < irProbeMax; i++ {
		ch := &library.ChapterEvent{ID: lib.Scenes[i].ID, Script: lib.Scenes[i].Script, LabelPos: 0}
		ev, perr, ptxt := buildChapterSafe(root, ch)
		probed++
		if ptxt != "" {
			errs = append(errs, fmt.Sprintf("场景 %s IR 构建 panic: %s", ch.ID, ptxt))
			continue
		}
		if perr != nil {
			errs = append(errs, fmt.Sprintf("场景 %s IR 构建失败: %v", ch.ID, perr))
			continue
		}
		t, vc := countEventText(ev)
		textLines += t
		voices += vc
	}
	notes = append(notes, fmt.Sprintf("无章节事件，按场景整份剧本探测（上限 %d 个）", irProbeMax))
	return textLines, voices, probed, notes, errs
}

// buildChapterSafe 建章节 IR 并抓 panic。
func buildChapterSafe(root string, ch *library.ChapterEvent) (ev *ir.Event, err error, panicTxt string) {
	defer func() {
		if r := recover(); r != nil {
			ev, err, panicTxt = nil, nil, fmt.Sprintf("%v", r)
		}
	}()
	ev, err = library.BuildChapterEvent(root, ch)
	return ev, err, ""
}

// countEventText 数一个 IR 事件的台词行与 voice_play 指令。
//
// 台词优先用 ev.Texts；Texts 为空时退回数 show 里的 text 指令
// （部分构建路径只产出台本不产出 Texts 汇总）。
func countEventText(ev *ir.Event) (textLines, voices int) {
	if ev == nil {
		return 0, 0
	}
	textLines = len(ev.Texts)
	if textLines == 0 {
		for _, op := range ev.Show {
			if op.Kind == ir.ShowTextRun && op.Text != nil && strings.TrimSpace(op.Text.Text) != "" {
				textLines++
			}
		}
	}
	for _, op := range ev.Show {
		if op.Kind == ir.ShowVoicePlay {
			voices++
		}
	}
	return textLines, voices
}

// resolveJunction 跟随 Junction / 重解析点，返回实体目录。
//
// 游戏库的 `按评价/<分组>/<游戏>` 绝大多数是 Junction（ReparsePoint），指向
// `20260424/...` 下的实体目录。实测两点：
//   - filepath.EvalSymlinks 解析不了它们（Windows 上 junction 不算 symlink，
//     Lstat 的 Mode 里也没有 ModeSymlink），所以这里改用 os.Readlink；
//   - filepath.WalkDir 也跟着失效：它按 Lstat 把一个 junction 根当成非目录，
//     一个子项都不遍历（实测 dirs=0）。所以目录遍历一律走 walkFollow。
//
// 只读：只是用另一个路径形式访问同一个目录，不写、不改、不移动任何东西。
func resolveJunction(root string) string {
	cur := root
	for i := 0; i < 4; i++ {
		tgt, err := os.Readlink(cur)
		if err != nil || tgt == "" || strings.EqualFold(tgt, cur) {
			return cur
		}
		cur = tgt
	}
	return cur
}

// walkFollow 用 os.ReadDir 递归遍历（会跟随 junction），depth=0 是 root 的直接子项。
//
// fn 返回 false 表示「这个子项不用再往深处走」（目录就不再递归）。
func walkFollow(root string, maxDepth int, fn func(abs, rel string, depth int, ent os.DirEntry) bool) {
	var visit func(dir, rel string, depth int)
	visit = func(dir, rel string, depth int) {
		ents, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, ent := range ents {
			abs := filepath.Join(dir, ent.Name())
			childRel := ent.Name()
			if rel != "" {
				childRel = rel + "/" + ent.Name()
			}
			if fn(abs, childRel, depth, ent) && ent.IsDir() && depth+1 <= maxDepth {
				visit(abs, childRel, depth+1)
			}
		}
	}
	visit(root, "", 0)
}

// categoryAliases 素材分类目录名 → docs/14 ⑤-2 的 5 个标准类。
var categoryAliases = map[string]string{
	"cg":         "cg",
	"evcg":       "cg",
	"eventcg":    "cg",
	"bg":         "bg",
	"back":       "bg",
	"background": "bg",
	"stand":      "stand",
	"chara":      "stand",
	"tati":       "stand",
	"立ち絵":        "stand",
	"movie":      "movie",
	"mov":        "movie",
	"video":      "movie",
	"ui":         "ui",
	"sys":        "ui",
}

// scanCategoryDirs 找 game 树里（最多 3 层）符合 5 类的素材目录，返回标准类名（去重排序）。
func scanCategoryDirs(root string) []string {
	found := map[string]bool{}
	walkFollow(resolveJunction(root), 3, func(abs, rel string, depth int, ent os.DirEntry) bool {
		if !ent.IsDir() {
			return false
		}
		if cat, ok := categoryAliases[strings.ToLower(ent.Name())]; ok {
			found[cat] = true
		}
		return true
	})
	out := make([]string, 0, len(found))
	for k := range found {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// coverHints 封面候选的文件名关键字（各引擎/汉化组的常见命名）。
var coverHints = []string{"cover", "package", "banner", "thumb", "hyoushi", "hyoshi", "jacket", "表紙", "ジャケット", "title"}

// imageExts 审计层认的图片后缀（解码交给 replay.DecodeImage）。
var imageExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".bmp": true, ".webp": true,
	".tlg": true, ".hg3": true, ".g00": true, ".gif": true,
}

// probeCover 文件级封面判定：找一张封面候选图 → 解码 → internal/cover.Analyze
// 判「非近黑」。这是**单张封面图**的证据，不等于回放首帧非黑屏
// （docs/14 ⑤-3 的完整证据要第二层真机抽样）。
func probeCover(root string) (valid bool, source string, nonBlack float64, errs []string) {
	rel, p := findCoverFile(root)
	if p == "" {
		return false, "", 0, nil
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return false, rel, 0, []string{fmt.Sprintf("读封面失败 %s: %v", rel, err)}
	}
	img, err := replay.DecodeImage(data)
	if err != nil {
		return false, rel, 0, []string{fmt.Sprintf("封面解码失败 %s: %v", rel, err)}
	}
	m := cover.Analyze(img)
	return m.Valid(), rel, m.NonBlack, nil
}

// findCoverFile 在 root 下（最多 3 层）找封面候选图，返回 (相对路径, 绝对路径)；
// 找不到时绝对路径为 ""。
func findCoverFile(root string) (string, string) {
	best, bestRel := "", ""
	bestDepth := 1 << 30
	walkFollow(resolveJunction(root), 3, func(abs, rel string, depth int, ent os.DirEntry) bool {
		if ent.IsDir() {
			return true
		}
		ext := strings.ToLower(filepath.Ext(abs))
		if !imageExts[ext] {
			return false
		}
		name := strings.ToLower(strings.TrimSuffix(ent.Name(), filepath.Ext(ent.Name())))
		if coverNameMatches(name) && depth < bestDepth {
			best, bestRel, bestDepth = abs, filepath.ToSlash(rel), depth
		}
		return false
	})
	return bestRel, best
}

// coverNameMatches 文件名（小写、去后缀）是否像封面。
//
// 只认「词首 / 分隔符之后」的关键字，词尾匹配还要求前一个字符是分隔符：
// 早期版本用 strings.Contains，把 RPG Maker MV 的 `Recovery1.png`（re-cover-y）
// 当成封面；改成裸 HasSuffix 后 `discover.png` 又会中招。宁可判缺，
// 不许把非封面判成有封面——cover_valid 是真假判断题，误判成 true 会污染验收结论。
func coverNameMatches(name string) bool {
	for _, h := range coverHints {
		switch {
		case name == h,
			strings.HasPrefix(name, h),
			strings.Contains(name, "_"+h),
			strings.Contains(name, "-"+h),
			strings.Contains(name, "."+h),
			strings.Contains(name, " "+h):
			return true
		}
		// 词尾匹配：`bg_cover` 算，`discover` 不算。
		if strings.HasSuffix(name, h) {
			head := name[:len(name)-len(h)]
			if strings.HasSuffix(head, "_") || strings.HasSuffix(head, "-") ||
				strings.HasSuffix(head, ".") || strings.HasSuffix(head, " ") {
				return true
			}
		}
	}
	return false
}

// assetDirsMax / assetDirsLimit 素材目录抽样的规模上限（避免在超大游戏树上磨时间）。
const (
	assetDirsMax   = 400 // 最多看这么多个目录
	assetDirsLimit = 12  // 最多报这么多条
)

// scanAssetDirs 抽样「像素材目录」的目录（深度 ≤ 3、目录内直接含 ≥5 张图）。
//
// 用途只有一个：category_dirs 认不出 cg/bg/stand/movie/ui 五类时，
// 让报告能说清「素材其实按别的命名组织」（如 RPG Maker MV 的
// www/img/pictures、www/img/animations），而不是留下一个无从解释的空数组。
func scanAssetDirs(root string) []string {
	out := []string{}
	visited := 0
	walkFollow(resolveJunction(root), 3, func(abs, rel string, depth int, ent os.DirEntry) bool {
		if !ent.IsDir() {
			return false
		}
		visited++
		if visited > assetDirsMax || len(out) >= assetDirsLimit {
			return false
		}
		ents, rerr := os.ReadDir(abs)
		if rerr != nil {
			return true
		}
		imgs := 0
		for _, e := range ents {
			if e.IsDir() {
				continue
			}
			if imageExts[strings.ToLower(filepath.Ext(e.Name()))] {
				imgs++
			}
		}
		if imgs >= 5 {
			out = append(out, fmt.Sprintf("%s(%d张图)", filepath.ToSlash(rel), imgs))
		}
		return true
	})
	return out
}
