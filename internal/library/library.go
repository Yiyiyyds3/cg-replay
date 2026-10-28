// Package library 扫出「原作自己的事件表」——本作（重返蓝鲸岛）把 CG 鉴赏写成
// cgrai.ks…cgrai6.ks（6 页 × 16 格，cg001…cg096）+ cgrai00.ks（每个 *cgNN 一段演出），
// 主线则是编号剧本 01kuukou.ks…76utyu.ks。
//
// 这里只做「按原作的分割方式切事件」：每个 CG 一个独立事件，每个主线剧本一个场景事件。
package library

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"ucgr/internal/adapter"
	"ucgr/internal/adapter/kirikiri"
	"ucgr/internal/ir"
)

// CGEvent 一个 CG 事件的回放单位（= cgrai00.ks 里的一个 *cgNN 段）。
type CGEvent struct {
	ID     string   `json:"id"`     // 画廊槽位名，如 cg017
	Index  int      `json:"index"`  // 17
	Title  string   `json:"title"`  // 展示名（优先首张图名）
	Thumb  string   `json:"thumb"`  // 画廊里的缩略图 storage（cg017）
	Script string   `json:"script"` // 相对 root 的脚本路径
	Label  string   `json:"label"`  // 标签名（cg17）
	Pages  []string `json:"pages"`  // 依次显示的图（storage 名）
}

// SceneEvent 一个主线场景事件（= 一个编号剧本）。
type SceneEvent struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Script string `json:"script"` // 相对 root 的脚本路径
	// Label 引擎内的定位串（KiriKiri 不用；Artemis = block 序号）。
	Label string `json:"label,omitempty"`
	// HasVideo 该事件里含影片指令（OP/ED）——前端给 ▶ 角标，别让人误以为是坏图。
	HasVideo bool `json:"has_video,omitempty"`
}

// ChapterEvent 一个「演出段落」= 原作自己的章节标记 `*labelNN|显示名`。
//
// 场景脚本动辄一整块自动播放一小时（71kaigan01.ks），按章节切开才能逐段回放。
// 章节标记会重号（分支路线同号异名），故用 LabelPos（在脚本标签表里的下标）寻址。
type ChapterEvent struct {
	ID       string `json:"id"`        // 唯一 id：<场景id>#<labelPos>
	Index    int    `json:"index"`     // 章节号（label94 → 94）
	Title    string `json:"title"`     // `|` 之后的显示名
	Script   string `json:"script"`    // 相对 root 的脚本路径
	Scene    string `json:"scene"`     // 所属场景 id
	LabelPos int    `json:"label_pos"` // 章节标记在脚本标签表里的下标
	Name     string `json:"name"`      // 标签名（label94）
	// 章跨文件：本章演完还要接下一场景脚本的「章前段」
	// （首个章节标记之前的内容）。空 = 无接续。
	ContScript   string `json:"cont_script,omitempty"`    // 下一场景脚本（相对 root）
	ContLabelPos int    `json:"cont_label_pos,omitempty"` // 下一场景首章的 LabelPos（定位章前段截止处）
}

// Library 事件库清单。
type Library struct {
	Root        string         `json:"root"`
	Game        string         `json:"game,omitempty"`
	GeneratedAt string         `json:"generated_at"`
	CGs         []CGEvent      `json:"cgs"`
	Scenes      []SceneEvent   `json:"scenes"`
	Chapters    []ChapterEvent `json:"chapters,omitempty"`
}

var (
	thumbRe   = regexp.MustCompile(`^cg\d+$`)
	labelRe   = regexp.MustCompile(`^cg\d+$`)
	sceneRe   = regexp.MustCompile(`^\d{2}[A-Za-z_0-9]*\.ks$`)
	galleryRe = regexp.MustCompile(`^cgrai[0-9]*\.ks$`)
)

// conventionRe 把约定里的正则编译出来（空表达式 = 该项不启用）。
func conventionRe(expr string) *regexp.Regexp {
	if expr == "" {
		return nil
	}
	re, err := regexp.Compile(expr)
	if err != nil {
		// 约定由代码生成或用户提供，非法表达式不该让整个扫描崩掉：
		// 退回「不启用该项」，扫描照常进行（缺画廊就没有 CG 事件）。
		return nil
	}
	return re
}

// ScenarioDir 解包目录下的剧本目录（KiriKiri 默认约定）。
func ScenarioDir(root string) string {
	return filepath.Join(root, adapter.DefaultKiriKiri().ScenarioDir)
}

// Scan 扫一个解包目录，产出事件库（KiriKiri 默认约定）。
//
// 换一款同引擎游戏、目录/命名不同时，用 ScanWith + 自定义约定，
// 或在游戏根目录放 `ucgr-convention.json` 覆盖。
func Scan(root string) (*Library, error) {
	return ScanWith(root, adapter.DefaultKiriKiri())
}

// ScanWith 按给定约定扫一个解包目录，产出事件库。
//
// 约定决定：剧本目录、画廊文件模式、画廊索引脚本、CG 标签正则、场景文件正则。
// 解析器仍是引擎专属的（KiriKiri 的 AST），约定只管「文件在哪、叫什么」。
func ScanWith(root string, conv adapter.Convention) (*Library, error) {
	if conv.ScenarioDir == "" {
		conv = conv.Merge(adapter.DefaultKiriKiri())
	}
	// 事件切分是引擎专属的：Artemis 的事件单位是剧本里的 block，
	// KiriKiri 是画廊槽位 + 场景 + 章节。约定里的 engine_id 决定走哪条。
	if strings.EqualFold(conv.EngineID, "artemis") {
		return ScanArtemis(root, conv)
	}
	if strings.EqualFold(conv.EngineID, "bgi") {
		return ScanBGI(root, conv)
	}
	if strings.EqualFold(conv.EngineID, "catsystem2") {
		return ScanCS2(root, conv)
	}
	thumbRe := conventionRe(conv.CGLabelRe)
	labelRe := conventionRe(conv.CGLabelRe)
	galleryRe := conventionRe(conv.GalleryRe)
	sceneRe := conventionRe(conv.SceneRe)
	dir := filepath.Join(root, filepath.FromSlash(conv.ScenarioDir))
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("读不到剧本目录 %s：%w", dir, err)
	}
	lib := &Library{
		Root:        root,
		Game:        filepath.Base(strings.TrimRight(root, `\/`)),
		GeneratedAt: time.Now().Format(time.RFC3339),
		CGs:         []CGEvent{},
		Scenes:      []SceneEvent{},
		Chapters:    []ChapterEvent{},
	}

	// ① 画廊页（cgrai.ks、cgrai2..6.ks）给出槽位顺序 + 缩略图 + 跳转标签。
	type slot struct{ thumb, label string }
	var slots []slot
	var galleryFiles []string
	for _, e := range ents {
		name := e.Name()
		if e.IsDir() || !galleryRe.MatchString(name) || strings.EqualFold(name, "cgrai00.ks") {
			continue
		}
		galleryFiles = append(galleryFiles, name)
	}
	sort.Strings(galleryFiles)
	for _, name := range galleryFiles {
		ast, err := kirikiri.ParseScriptFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		for _, sec := range kirikiri.Sections(ast) {
			for _, it := range kirikiri.Segment(ast, sec.Name) {
				if it.Kind != "cmd" || it.Cmd == nil || !strings.EqualFold(it.Cmd.Tag, "sysbutton") {
					continue
				}
				thumb := strings.TrimSpace(it.Cmd.Attrs["graphic"])
				target := strings.TrimSpace(it.Cmd.Attrs["target"])
				if !thumbRe.MatchString(thumb) {
					continue
				}
				label := strings.TrimPrefix(target, "*")
				if !labelRe.MatchString(label) {
					continue
				}
				slots = append(slots, slot{thumb: thumb, label: label})
			}
		}
	}

	// ② cgrai00.ks 里每个 *cgNN 段的图片序列。
	cgScript := filepath.Join(dir, conv.GalleryIndex)
	rel := func(p string) string {
		if r, err := filepath.Rel(root, p); err == nil {
			return filepath.ToSlash(r)
		}
		return filepath.ToSlash(p)
	}
	pages := map[string][]string{}
	if ast, err := kirikiri.ParseScriptFile(cgScript); err == nil {
		for _, sec := range kirikiri.Sections(ast) {
			if !labelRe.MatchString(sec.Name) {
				continue
			}
			if imgs := kirikiri.SegmentImages(ast, sec.Name); len(imgs) > 0 {
				pages[sec.Name] = imgs
			}
		}
	}

	for _, s := range slots {
		ev := CGEvent{
			ID:     s.thumb,
			Index:  kirikiri.LabelIndex(s.thumb),
			Thumb:  s.thumb,
			Label:  s.label,
			Script: rel(cgScript),
			Pages:  pages[s.label],
		}
		ev.Title = s.thumb
		if len(ev.Pages) > 0 {
			ev.Title = fmt.Sprintf("%s · %s", s.thumb, ev.Pages[0])
		}
		lib.CGs = append(lib.CGs, ev)
	}
	sort.SliceStable(lib.CGs, func(i, j int) bool { return lib.CGs[i].Index < lib.CGs[j].Index })

	// ③ 主线剧本 = 场景事件。本作按编号命名（01kuukou.ks…76utyu.ks）；
	// 其他 KiriKiri 标题不一定编号（如 g01.ks、日语文件名）。没有编号
	// 场景时回退到「带章节显示名的故事脚本」：系统/引擎脚本没有
	// `*labelNN|显示名` 章节标记，且通常远小于 8 KiB，用这两个条件排除。
	var names []string
	for _, e := range ents {
		if e.IsDir() || !sceneRe.MatchString(e.Name()) {
			continue
		}
		names = append(names, e.Name())
	}
	if len(names) == 0 {
		names = fallbackStoryScenes(dir)
	}
	sort.Strings(names)
	for _, name := range names {
		id := strings.TrimSuffix(name, filepath.Ext(name))
		// 影片场景（OP/ED）：KiriKiri 的 .ks 是明文脚本，认一下自己的语法即可。
		hasVideo := false
		if data, err := os.ReadFile(filepath.Join(dir, name)); err == nil {
			hasVideo = kirikiri.ScriptHasMovie(data)
		}
		lib.Scenes = append(lib.Scenes, SceneEvent{
			ID:       id,
			Title:    id,
			Script:   rel(filepath.Join(dir, name)),
			HasVideo: hasVideo,
		})
	}

	// ③′ 影片脚本（OP/ED）：KiriKiri 把它们放在 raw/data/sysscn/ 之类的系统目录，
	// 不在剧本目录，所以上面扫不到。这里只收**含影片指令**的脚本——那个目录里
	// 几十个 .ks 全是系统 UI 脚本（brows/save/option…），收进来是噪声。
	for _, md := range conv.MovieDirs {
		mdir := filepath.Join(root, filepath.FromSlash(md))
		ents, err := os.ReadDir(mdir)
		if err != nil {
			continue // 没有这个目录很正常（不是每个游戏都有）
		}
		ext := conv.ScriptExt
		if ext == "" {
			ext = ".ks"
		}
		for _, e := range ents {
			if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ext) {
				continue
			}
			p := filepath.Join(mdir, e.Name())
			data, err := os.ReadFile(p)
			if err != nil || !kirikiri.ScriptHasMovie(data) {
				continue
			}
			id := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
			if lib.hasSceneID(id) {
				// 与剧本目录里的同名脚本撞了：加目录前缀区分
				id = filepath.Base(mdir) + "_" + id
			}
			lib.Scenes = append(lib.Scenes, SceneEvent{
				ID:       id,
				Title:    id,
				Script:   rel(p),
				HasVideo: true,
			})
		}
	}

	// ④ 演出段落：场景脚本里原作自己的章节标记（*labelNN|显示名）。
	// 一个长脚本常有几十个标签但只有十来个章节标记，按章节切才是「演出段落」。
	//
	// 章跨文件：下一个场景脚本的首个章节标记之前还有一段演出（本作的章尾
	// 常常写在下一文件开头），它没有自己的章节标记，只能挂到「上一章」的
	// 尾巴上。章节号（labelNN）全作递增，用它可以精确找出前序（文件编号
	// 不可靠：99tikadesu.ks 的 label60 排在 38tikatu 之后、但不是 71 的前序）。
	type sceneChapters struct {
		start, end    int // lib.Chapters 里的区间
		hasPrefix     bool
		first, last   int // 首/末章章节号
		firstLabelPos int
	}
	sceneInfo := map[string]*sceneChapters{}
	for _, s := range lib.Scenes {
		ast, err := kirikiri.ParseScriptFile(filepath.Join(root, filepath.FromSlash(s.Script)))
		if err != nil {
			continue
		}
		chs := kirikiri.Chapters(ast)
		if len(chs) == 0 {
			continue
		}
		info := &sceneChapters{
			start:         len(lib.Chapters),
			hasPrefix:     chs[0].SeqStart > 0,
			first:         chs[0].Index,
			last:          chs[len(chs)-1].Index,
			firstLabelPos: chs[0].LabelPos,
		}
		for _, ch := range chs {
			lib.Chapters = append(lib.Chapters, ChapterEvent{
				ID:       fmt.Sprintf("%s#%d", s.ID, ch.LabelPos),
				Index:    ch.Index,
				Title:    ch.Title,
				Script:   s.Script,
				Scene:    s.ID,
				LabelPos: ch.LabelPos,
				Name:     ch.Name,
			})
		}
		info.end = len(lib.Chapters)
		sceneInfo[s.ID] = info
	}

	// ⑤ 章前段归属：按章节号找「末章 = 本章前首章 - 1」的场景，把后者的
	// 章前段挂到前者的最后一个章节事件上（段落总数不变，只补演尾）。
	// 找不到前序（首场景 01kuukou 的宏定义块等）就保持不挂 —— 它们多是
	// 初始化指令，不进演出也有等价语义。
	for _, s := range lib.Scenes {
		info := sceneInfo[s.ID]
		if info == nil || !info.hasPrefix {
			continue
		}
		var prev *ChapterEvent
		for _, p := range lib.Scenes {
			pi := sceneInfo[p.ID]
			if pi == nil || pi.last != info.first-1 {
				continue
			}
			cand := &lib.Chapters[pi.end-1]
			if cand.ContScript == "" { // 多个同号分支时取先出现者
				prev = cand
				break
			}
		}
		if prev == nil {
			continue
		}
		prev.ContScript = s.Script
		prev.ContLabelPos = info.firstLabelPos
	}
	return lib, nil
}

// fallbackStoryScenes 在解包目录没有编号剧本时，挑出真正的故事脚本。
// 条件：≥8 KiB 且有 `*labelNN|显示名` 章节标记（引擎脚本通常两者都不满足）。
func fallbackStoryScenes(dir string) []string {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	const minSize = 8 << 10
	var out []string
	for _, e := range ents {
		name := e.Name()
		if e.IsDir() || !strings.EqualFold(filepath.Ext(name), ".ks") {
			continue
		}
		if galleryRe.MatchString(name) || strings.EqualFold(name, "cgrai00.ks") {
			continue
		}
		info, err := e.Info()
		if err != nil || info.Size() < minSize {
			continue
		}
		ast, err := kirikiri.ParseScriptFile(filepath.Join(dir, name))
		if err != nil || len(kirikiri.Chapters(ast)) == 0 {
			continue
		}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// Save 写库清单（JSON）。
func (l *Library) Save(path string) error {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// Load 读库清单。
func Load(path string) (*Library, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var lib Library
	if err := json.Unmarshal(data, &lib); err != nil {
		return nil, fmt.Errorf("解析库清单 %s 失败：%w", path, err)
	}
	return &lib, nil
}

// FindCG 按 id（cg017 / cg17 / 17）找 CG 事件。
func (l *Library) FindCG(id string) *CGEvent {
	key := strings.ToLower(strings.TrimSpace(id))
	digits := tailDigits(key)
	for i := range l.CGs {
		c := &l.CGs[i]
		if strings.ToLower(c.ID) == key || strings.ToLower(c.Label) == key {
			return c
		}
		if digits != "" && strconv.Itoa(c.Index) == digits {
			return c
		}
	}
	return nil
}

// FindScene 按 id（72toire / 71kaigan01）找场景事件。
func (l *Library) FindScene(id string) *SceneEvent {
	key := strings.ToLower(strings.TrimSpace(id))
	for i := range l.Scenes {
		if strings.ToLower(l.Scenes[i].ID) == key {
			return &l.Scenes[i]
		}
	}
	return nil
}

// FindChapter 按 id（71kaigan01#27 / 或章节标题）找演出段落。
func (l *Library) FindChapter(id string) *ChapterEvent {
	key := strings.ToLower(strings.TrimSpace(id))
	for i := range l.Chapters {
		if strings.ToLower(l.Chapters[i].ID) == key {
			return &l.Chapters[i]
		}
	}
	// 退一步：按章节标题精确匹配（用户可能直接输「水畔」）。
	for i := range l.Chapters {
		if strings.ToLower(l.Chapters[i].Title) == key {
			return &l.Chapters[i]
		}
	}
	return nil
}

// BuildChapterEvent 构建章节 IR（含章跨文件补演的章前段）。
//
// 本段 = 本章区间；若 ContScript 非空，再接上下一场景脚本的章前段，
// 其 TMs 从本段的末尾时刻起算，保证进度条单调。两个前端（cgreplay /
// desktop 回放页）都走这里，不要直接调 BuildShowChapterFile。
func BuildChapterEvent(root string, ch *ChapterEvent) (*ir.Event, error) {
	sp := filepath.Join(root, filepath.FromSlash(ch.Script))
	ev, err := kirikiri.BuildShowChapterFile(sp, ch.LabelPos)
	if err != nil {
		return nil, err
	}
	if ch.ContScript == "" {
		return ev, nil
	}
	pre, err := kirikiri.BuildShowPrefixFile(filepath.Join(root, filepath.FromSlash(ch.ContScript)))
	if err != nil {
		return nil, fmt.Errorf("章节 %s 的跨文件章前段构建失败：%w", ch.ID, err)
	}
	if len(pre.Show) > 0 {
		off := int64(0)
		if n := len(ev.Show); n > 0 {
			off = ev.Show[n-1].TMs
		}
		for i := range pre.Show {
			pre.Show[i].TMs += off
		}
		ev.Show = append(ev.Show, pre.Show...)
	}
	return ev, nil
}

func tailDigits(s string) string {
	i := len(s)
	for i > 0 && s[i-1] >= '0' && s[i-1] <= '9' {
		i--
	}
	return s[i:]
}

// hasSceneID 事件库里是否已有这个 ID（收影片脚本时避免与剧本目录撞名）。
func (l *Library) hasSceneID(id string) bool {
	for _, s := range l.Scenes {
		if s.ID == id {
			return true
		}
	}
	return false
}
