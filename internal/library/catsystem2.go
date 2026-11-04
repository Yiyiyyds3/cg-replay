// catsystem2.go —— CatSystem2 事件库：一个 .cst 剧本 = 一个可回放事件。
//
// CS2 的剧本天然按场景分文件（`原始归档解包/scene/ama01.cst`），
// 所以事件单位直接用文件，不再像 KiriKiri 那样找画廊槽位/章节标记。
//
// 事件 id 用文件名（去 .cst）；多个 scene 目录下有同名文件时才退回相对路径。
// 标题取剧本里的第一句对白——CS2 没有可用的「章节显示名」字段。
// 解析与演出构建都复用适配器（ParseScript / BuildIR），保证「库里列的」
// 与「点开能播的」是同一条路径。
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
	a := &catsystem2.Adapter{}
	stemOf := uniqueStems(root, files)
	failed := 0
	for _, p := range files {
		data, err := os.ReadFile(p)
		if err != nil {
			failed++
			continue
		}
		ast, err := a.ParseScript(data, &adapter.ScriptCtx{EntryPath: p})
		if err != nil {
			failed++ // 单个剧本坏掉不拖垮整个库
			continue
		}
		cs2, ok := ast.(*catsystem2.Ast)
		if !ok {
			failed++
			continue
		}
		stem := stemOf(p)
		lib.Scenes = append(lib.Scenes, SceneEvent{
			ID:     stem,
			Title:  cs2Title(cs2, stem),
			Script: relKey(root, p),
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

// cs2Title 取剧本里的第一句对白当标题（没有就用文件名）。
func cs2Title(ast *catsystem2.Ast, fallback string) string {
	for _, s := range ast.Texts {
		s = strings.TrimSpace(s)
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
