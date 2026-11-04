// catsystem2.go —— CatSystem2 事件库：一个 .cst 剧本 = 一个可回放事件。
//
// CS2 的剧本天然按场景分文件（`原始归档解包/scene/ama01.cst`），
// 所以事件单位直接用文件，不再像 KiriKiri 那样找画廊槽位/章节标记。
//
// 事件 id 用文件名（去 .cst）；多个 scene 目录下有同名文件时才退回相对路径。
// 标题取剧本里的第一句对白——CS2 没有可用的「章节显示名」字段。
// 解析与演出构建都复用适配器（BuildShowScript），保证「库里列的」
// 与「点开能播的」是同一条路径；含影片指令的事件标 HasVideo。
package library

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"ucgr/internal/adapter"
	"ucgr/internal/adapter/catsystem2"
	"ucgr/internal/ir"
)

// cs2TitleMax 标题最多取多少字符（列表里一行放得下，不截断会撑爆界面）。
const cs2TitleMax = 40

// ScanCatSystem2 扫 CatSystem2 解包目录（.cst 剧本），产出事件库。
func ScanCatSystem2(root string, conv adapter.Convention) (*Library, error) {
	files := scriptFiles(root, conv.ScenarioDir, ".cst", discoverMaxDepth)
	if len(files) == 0 {
		return nil, fmt.Errorf("没有找到 .cst 剧本（%s）", scriptDirHint(root, conv.ScenarioDir))
	}
	lib := &Library{
		Root:        root,
		Game:        filepath.Base(strings.TrimRight(root, `\/`)),
		GeneratedAt: time.Now().Format(time.RFC3339),
		CGs:         []CGEvent{},
		Scenes:      []SceneEvent{},
		Chapters:    []ChapterEvent{},
	}
	stemOf := uniqueStems(root, files)
	failed := 0
	for _, p := range files {
		data, err := os.ReadFile(p)
		if err != nil {
			failed++
			continue
		}
		// 能解析且能构建出演出才算可回放（坏文件、空脚本都不列进库——
		// 否则会出现「库里列着但点开播不出来」）；同时拿到 ir.Event
		// 用于 HasVideo 判定与标题提取。
		ev, err := catsystem2.BuildShowScript(data, filepath.Base(p))
		if err != nil || len(ev.Show) == 0 {
			failed++ // 单个剧本坏掉不拖垮整个库
			continue
		}
		stem := stemOf(p)
		lib.Scenes = append(lib.Scenes, SceneEvent{
			ID:       stem,
			Title:    cs2Title(ev, stem),
			Script:   relKey(root, p),
			HasVideo: ir.HasVideo(ev),
		})
	}
	if failed > 0 {
		lib.Warnings = append(lib.Warnings, fmt.Sprintf(
			"%d/%d 个 .cst 解析失败已跳过（剧本可能损坏或不是 CatScene 容器）", failed, len(files)))
	}
	if len(lib.Scenes) == 0 {
		return nil, fmt.Errorf("没有识别到可回放事件（%s 下没有可解析的 .cst）", scriptDirHint(root, conv.ScenarioDir))
	}
	lib.StageW, lib.StageH = detectStageSize(root)
	return lib, nil
}

// cs2Title 取演出事件里的第一句对白当标题（没有就用文件名）。
func cs2Title(ev *ir.Event, fallback string) string {
	for _, t := range ev.Texts {
		s := strings.TrimSpace(t.Text)
		if s == "" {
			continue
		}
		return truncateRunes(s, cs2TitleMax)
	}
	return fallback
}

// truncateRunes 按字符（不是字节）截断，避免把 CP932/UTF-8 多字节字符劈开。
func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n]) + "…"
}
