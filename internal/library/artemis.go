// artemis.go —— Artemis 事件库：把 .ast 剧本里的 block 切成一串可回放事件。
//
// Artemis 的一个 `.ast` 里含多个 block（剧情段），事件单位 = block
// （与 `artemis.BuildIR` 的粒度一致）。每个 block 出一个「场景」事件：
// id = `<脚本相对路径>#<block 序号>`，Label 记 block 序号（回放时据此重建演出）。
//
// 与 KiriKiri 的差别：KiriKiri 有「画廊槽位」这种明确的 CG 事件表，
// Artemis 没有——所有 block 都是剧情段，所以这里只出场景事件。
//
// 剧本目录由 scriptFiles 自动探测（.ast 在 <卷>.pfs/script 下，卷名随游戏变）。
package library

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"ucgr/internal/adapter"
	"ucgr/internal/adapter/artemis"
)

// ScanArtemis 扫 Artemis 解包目录，产出事件库（一份 .ast = 一个场景事件）。
//
// 为什么不是「一 block 一事件」：Artemis 的 block 粒度极细（实测 65 KiB
// 剧本 179 个 block），按 block 切会让樱之刻产出十万级条目。事件单位取
// 整份剧本，与 scn / BGI 的场景事件粒度一致；块内顺序在回放时按文件序拼接。
func ScanArtemis(root string, conv adapter.Convention) (*Library, error) {
	files := scriptFiles(root, conv.ScenarioDir, ".ast", discoverMaxDepth)
	if len(files) == 0 {
		return nil, fmt.Errorf("没有找到 .ast 剧本（%s）", scriptDirHint(root, conv.ScenarioDir))
	}
	lib := &Library{
		Root:        root,
		Game:        filepath.Base(strings.TrimRight(root, `\/`)),
		GeneratedAt: time.Now().Format(time.RFC3339),
		CGs:         []CGEvent{},
		Scenes:      []SceneEvent{},
		Chapters:    []ChapterEvent{},
	}
	failed := 0
	stemOf := uniqueStems(root, files)
	for _, p := range files {
		ast, err := artemis.ParseAstFile(p)
		if err != nil {
			failed++ // 单个剧本坏掉不拖垮整个库
			continue
		}
		stem := stemOf(p)
		title := artemis.ScriptTitle(ast)
		if title == "" {
			title = stem
		}
		lib.Scenes = append(lib.Scenes, SceneEvent{
			ID:     stem,
			Title:  title,
			Script: relKey(root, p),
		})
	}
	if failed > 0 {
		lib.Warnings = append(lib.Warnings, fmt.Sprintf(
			"%d/%d 个 .ast 解析失败已跳过（剧本可能损坏）", failed, len(files)))
	}
	if len(lib.Scenes) == 0 {
		return nil, fmt.Errorf("没有识别到可回放事件（%s 下没有可解析的 .ast）", scriptDirHint(root, conv.ScenarioDir))
	}
	lib.StageW, lib.StageH = detectStageSize(root)
	return lib, nil
}
