// bgi.go —— BGI 事件库：把 .bs 脚本切成一串可回放事件。
//
// 事件边界由适配器的 builder 决定（换背景/换事件时切），这里直接复用
// `bgi.BuildScriptEvents`——保证「库里列的」与「IR 里的」一一对应，
// 不会出现列了却播不出来。
//
// 事件 id 沿用适配器的 `<脚本名>#<序号>`；Label 记序号（回放据此重建）。
// 剧本目录由 scriptFiles 自动探测（BGI 的 .bs 分在多个 dataXXXXX 目录下）。
package library

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ucgr/internal/adapter"
	"ucgr/internal/adapter/bgi"
	"ucgr/internal/ir"
)

// ScanBGI 扫 BGI 解包目录（.bs 编译脚本），产出事件库。
func ScanBGI(root string, conv adapter.Convention) (*Library, error) {
	files := scriptFiles(root, conv.ScenarioDir, ".bs", discoverMaxDepth)
	if len(files) == 0 {
		return nil, fmt.Errorf("没有找到 .bs 剧本（%s）", scriptDirHint(root, conv.ScenarioDir))
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
		name := filepath.Base(p)
		data, err := os.ReadFile(p)
		if err != nil {
			failed++
			continue
		}
		evs, err := bgi.BuildScriptEvents(data, name)
		if err != nil {
			failed++ // 单个脚本坏掉不拖垮整个库
			continue
		}
		stem := stemOf(p)
		for i, ev := range evs {
			id := ev.EventID
			if id == "" {
				id = fmt.Sprintf("%s#%03d", stem, i+1)
			}
			title := strings.TrimSpace(ev.Title)
			if title == "" {
				title = fmt.Sprintf("%s #%03d", stem, i+1)
			}
				lib.Scenes = append(lib.Scenes, SceneEvent{
				ID:       id,
				Title:    title,
				Script:   relKey(root, p),
				Label:    fmt.Sprint(i + 1),
				HasVideo: ir.HasVideo(&ev),
			})
		}
	}
	if failed > 0 {
		lib.Warnings = append(lib.Warnings, fmt.Sprintf(
			"%d/%d 个 .bs 解析失败已跳过（脚本可能损坏或不是 BGI 编译脚本）", failed, len(files)))
	}
	if len(lib.Scenes) == 0 {
		return nil, fmt.Errorf("没有识别到可回放事件（%s 下没有可解析的 .bs）", scriptDirHint(root, conv.ScenarioDir))
	}
	lib.StageW, lib.StageH = detectStageSize(root)
	return lib, nil
}
