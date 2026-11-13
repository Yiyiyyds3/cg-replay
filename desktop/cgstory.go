// cgstory.go —— 「CG 剧情模式」：把画廊 CG 关联回它在主线里的那段演出。
//
// 现状核实（2026-11-13，本作 + 通用 KiriKiri 画廊）：
//   - 画廊索引 cgrai00.ks 的 `*cgNN` 段**只有图片**：
//     [image layer=base storage=… page=back visible=false] + [trans][wt] + [l]，
//     没有 [台詞]（说话人宏）、没有 [vo]（语音）⇒ 鉴赏模式必然是「有图无声无字」。
//   - 同一批 CG 图在主线场景脚本（71kaigan01.ks…）里以 [image storage=…] 出现，
//     周围是完整演出：背景/立绘/台词/语音/BGM/转场。
//
// 所以剧情模式的关联键 = **CG 页图的 storage 名**：
//  1. 从 library.CGEvent.Pages 取该 CG 的图名集合（扫描时已从 cgrai00.ks 收集）；
//  2. 逐个场景脚本扫 image/img/cg/bg 命令的 storage，命中页图即定位到 ast.Seq 下标；
//  3. 取覆盖该下标的章节（`*labelNN|显示名`，没有章节则退到普通标签段）作为演出段；
//  4. 构建台本后**必须**有台词 op 才算剧情模式，否则如实回退纯图鉴赏。
//
// 不做猜测：命中不到、构建后没台词、或引擎不是可解析的 KAG 文本时，一律回退，
// 并在 SessionMeta.Dialogue.Note 里写明「该 CG 无台词数据」，不假装有。
package main

import (
	"errors"
	"path/filepath"
	"strings"
	"sync"

	"ucgr/internal/adapter/kirikiri"
	"ucgr/internal/ir"
	"ucgr/internal/library"
)

// cgNoDialogueNote 无剧情数据时的如实说明（前端卡片/回放横幅直接展示）。
const cgNoDialogueNote = "该 CG 无台词数据：画廊脚本只有图片，主线里也没有找到引用同一张图的演出"

// errNoCGStory 没有关联到剧情演出（调用方按回退处理，不当故障）。
var errNoCGStory = errors.New("没有可用的 CG 剧情演出")

// cgStoryLink CG 在主线演出里的落点。
type cgStoryLink struct {
	Script     string // 场景脚本（相对 root，slash）
	Section    string // 命中的标签段名（章节名或普通标签）
	Chapter    string // 章节显示名（`*labelNN|标题` 的标题；无章节时为空）
	LabelPos   int    // 章节在脚本标签表里的下标；-1 = 用 Section 定位
	Page       string // 命中的 CG 页图 storage
	Seq        int    // 命中位置在 ast.Seq 的下标
	Candidates int    // 同一张图在剧本里出现的候选处数（多路线复用同一 CG）
}

// DialogueDTO 剧情模式/台词数据的对外说明（SessionMeta.Dialogue）。
//
// 前端据此显示「剧情 / 鉴赏」切换与「该 CG 无台词数据」提示。
type DialogueDTO struct {
	Available  bool   `json:"available"`   // true = 剧情模式可用（有台词）
	TextLines  int    `json:"text_lines"`  // 演出段里的台词条数（text op）
	VoiceLines int    `json:"voice_lines"` // 演出段里的语音条数（voice_play op）
	Chapter    string `json:"chapter,omitempty"`
	Script     string `json:"script,omitempty"`
	Page       string `json:"page,omitempty"`
	Note       string `json:"note,omitempty"`
	Candidates int    `json:"candidates,omitempty"`
}

// cgStoryCache 关联结果缓存（key = root|库生成时间|CG id）。
//
// 解析 79 个场景脚本约几十毫秒到几百毫秒；同一个 CG 反复打开（列表点来点去）
// 不该每次重扫。库重扫后 GeneratedAt 变化，旧缓存自然失效。
var cgStoryCache = struct {
	mu sync.Mutex
	m  map[string]*cgStoryLink
}{m: map[string]*cgStoryLink{}}

// resolveCGStory 找 CG 页图在主线场景里的出处。
//
// 返回 (nil, nil) = 明确「没有剧情演出」（不是错误）。真错误只留给出不来的解析，
// 调用方一律按回退处理，不阻断回放。
func resolveCGStory(root string, lib *library.Library, cg *library.CGEvent) (*cgStoryLink, error) {
	if lib == nil || cg == nil || len(cg.Pages) == 0 {
		return nil, nil
	}
	key := strings.Join([]string{root, lib.GeneratedAt, cg.ID}, "\x00")
	cgStoryCache.mu.Lock()
	if v, ok := cgStoryCache.m[key]; ok {
		cgStoryCache.mu.Unlock()
		return v, nil
	}
	cgStoryCache.mu.Unlock()

	link := scanCGStory(root, lib, cg)

	cgStoryCache.mu.Lock()
	cgStoryCache.m[key] = link
	cgStoryCache.mu.Unlock()
	return link, nil
}

// scanCGStory 逐场景脚本找页图引用并挑出带台词的那一段。
func scanCGStory(root string, lib *library.Library, cg *library.CGEvent) *cgStoryLink {
	want := map[string]bool{}
	for _, p := range cg.Pages {
		if k := normCGRef(p); k != "" {
			want[k] = true
		}
	}
	if len(want) == 0 {
		return nil
	}

	candidates := 0
	// 两轮：先精确名（最常见），再前缀名（主线常用 `hebi01a` 这类差分，
	// 而画廊页记 `hebi01`）。前缀轮只在精确轮一无所获时启用，避免误挂。
	for _, prefixOK := range []bool{false, true} {
		for i := range lib.Scenes {
			sc := &lib.Scenes[i]
			if strings.EqualFold(filepath.ToSlash(sc.Script), filepath.ToSlash(cg.Script)) {
				continue // 画廊索引脚本自己不算剧情
			}
			ast, err := kirikiri.ParseScriptFile(filepath.Join(root, filepath.FromSlash(sc.Script)))
			if err != nil {
				continue // 单个脚本解析失败不该让整次关联失败
			}
			// 同一张图可能在一份剧本里出现多次（分支/回顾/差分）。不能只看
			// 第一处：第一处若是无台词的纯图段，后面同文件的正式演出就被漏掉。
			for hitSeq, it := range ast.Seq {
				ref := cgItemImageRef(it)
				if ref == "" {
					continue
				}
				key := normCGRef(ref)
				if !want[key] && !(prefixOK && cgRefPrefixMatch(want, key)) {
					continue
				}
				candidates++
				link := &cgStoryLink{Script: sc.Script, Seq: hitSeq, Page: ref, LabelPos: -1}
				if ch, ok := chapterAt(ast, hitSeq); ok {
					link.Chapter = ch.Title
					link.LabelPos = ch.LabelPos
					link.Section = ch.Name
				} else if sec, ok := sectionAt(ast, hitSeq); ok {
					link.Section = sec
				} else {
					continue // 连标签段都定不下来：这不是可回放的演出单位
				}
				// 有台词才算剧情演出（画廊图片段也可能被主线原样复用但没有对白）。
				if ev, berr := buildCGStoryEvent(root, link); berr == nil && countShowOps(ev, ir.ShowTextRun) > 0 {
					link.Candidates = candidates
					return link
				}
			}
		}
	}
	return nil
}

// buildCGStoryEvent 按关联结果构建剧情演出台本。
//
// 优先整章（保留章节内的前情与语音），没有章节时退到命中的标签段。
func buildCGStoryEvent(root string, link *cgStoryLink) (*ir.Event, error) {
	if link == nil {
		return nil, errNoCGStory
	}
	sp := filepath.Join(root, filepath.FromSlash(link.Script))
	if link.LabelPos >= 0 {
		return kirikiri.BuildShowChapterFile(sp, link.LabelPos)
	}
	return kirikiri.BuildShowSegment(sp, link.Section)
}

// dialogForStory 把关联结果 + 构建好的台本翻成前端可读的台词数据。
func dialogForStory(link *cgStoryLink, ev *ir.Event) *DialogueDTO {
	d := &DialogueDTO{
		Chapter:    link.Chapter,
		Script:     link.Script,
		Page:       link.Page,
		Candidates: link.Candidates,
	}
	if ev != nil {
		d.TextLines = countShowOps(ev, ir.ShowTextRun)
		d.VoiceLines = countShowOps(ev, ir.ShowVoicePlay)
	}
	d.Available = d.TextLines > 0
	if !d.Available {
		d.Note = cgNoDialogueNote
	}
	return d
}

// countShowOps 数台本里某类指令的条数（台词/语音透传统计）。
func countShowOps(ev *ir.Event, kind string) int {
	if ev == nil {
		return 0
	}
	n := 0
	for _, op := range ev.Show {
		if op.Kind == kind {
			n++
		}
	}
	return n
}

// chapterAt 找覆盖 ast.Seq 下标 idx 的演出段落。
func chapterAt(ast *kirikiri.Ast, idx int) (kirikiri.Chapter, bool) {
	for _, ch := range kirikiri.Chapters(ast) {
		if idx >= ch.SeqStart && idx < ch.SeqEnd {
			return ch, true
		}
	}
	return kirikiri.Chapter{}, false
}

// sectionAt 找覆盖 idx 的标签段（章节标记之外的长段/分支标签）。
func sectionAt(ast *kirikiri.Ast, idx int) (string, bool) {
	secs := kirikiri.Sections(ast)
	for i, s := range secs {
		end := len(ast.Seq)
		if i+1 < len(secs) {
			end = secs[i+1].SeqPos
		}
		if idx >= s.SeqPos && idx < end {
			return s.Name, true
		}
	}
	return "", false
}

// cgItemImageRef 取脚本项里的图片引用（只认画图类命令）。
func cgItemImageRef(it kirikiri.SeqItem) string {
	if it.Kind != "cmd" || it.Cmd == nil {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(it.Cmd.Tag)) {
	case "image", "img", "cg", "bg", "fg", "ev", "so", "backlay":
	default:
		return ""
	}
	for _, k := range []string{"storage", "file", "src"} {
		if v := strings.TrimSpace(it.Cmd.Attrs[k]); v != "" {
			return v
		}
	}
	return ""
}

// normCGRef 规范化素材引用：小写、去目录、去扩展名（`cg/kuukou01.png` → `kuukou01`）。
func normCGRef(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "\\", "/")
	s = filepath.Base(s)
	if ext := filepath.Ext(s); ext != "" {
		s = strings.TrimSuffix(s, ext)
	}
	return s
}

// cgRefPrefixMatch 前缀匹配（页图 `hebi01` vs 主线差分 `hebi01a`）。
// 至少 4 个字符才允许，避免 `cg1` 这类短名把整库都匹配上。
func cgRefPrefixMatch(want map[string]bool, ref string) bool {
	if len(ref) < 4 {
		return false
	}
	for p := range want {
		if len(p) < 4 {
			continue
		}
		if strings.HasPrefix(ref, p) || strings.HasPrefix(p, ref) {
			return true
		}
	}
	return false
}
