// alice.go —— AliceSoft System 4 事件库：把 Rance10.ain 的场景函数
// 切成场景事件。
//
// 一个场景函数 = 一个场景事件（与 alice.BuildIR 的事件粒度一致）。
// 事件 id 沿用函数名（含 CJK，如「萨娜琪亚／参战」）；Script 记 .ain
// 相对路径，Label 记函数名（回放据此反查函数）。
package library

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"ucgr/internal/adapter"
	"ucgr/internal/adapter/alice"
)

// ScanAlice 扫 System 4（AliceSoft）游戏目录，产出事件库。
func ScanAlice(root string, conv adapter.Convention) (*Library, error) {
	ainPath := findAliceAIN(root)
	if ainPath == "" {
		return nil, fmt.Errorf("没有找到 System 4 剧本（*.ain，%s）", root)
	}
	ain, err := alice.ParseAINFile(ainPath)
	if err != nil {
		return nil, err
	}
	scenes := ain.SceneFuncs()
	if len(scenes) == 0 {
		return nil, fmt.Errorf("%s 里没有可回放的场景函数", filepath.Base(ainPath))
	}
	lib := &Library{
		Root:        root,
		Game:        filepath.Base(strings.TrimRight(root, `\/`)),
		GeneratedAt: time.Now().Format(time.RFC3339),
		CGs:         []CGEvent{},
		Scenes:      []SceneEvent{},
		Chapters:    []ChapterEvent{},
	}
	rel := relKey(root, ainPath)
	for _, fnIdx := range scenes {
		name := ain.Funcs[fnIdx].Name
		if strings.TrimSpace(name) == "" {
			continue
		}
		lib.Scenes = append(lib.Scenes, SceneEvent{
			ID:     name,
			Title:  name,
			Script: rel,
			Label:  name,
		})
	}
	if len(lib.Scenes) == 0 {
		return nil, fmt.Errorf("没有识别到可回放事件（%s）", filepath.Base(ainPath))
	}
	// 舞台尺寸：兰斯10 原生 1920×1080（CG 全是 1920×1080）；约定可覆盖。
	lib.StageW, lib.StageH = 1920, 1080
	if conv.StageW > 0 && conv.StageH > 0 {
		lib.StageW, lib.StageH = conv.StageW, conv.StageH
	}
	return lib, nil
}

// findAliceAIN 找 System 4 剧本：根目录（含一层 Data/）下的 *.ain。
func findAliceAIN(root string) string {
	dirs := []string{root, filepath.Join(root, "Data")}
	for _, d := range dirs {
		matches, _ := filepath.Glob(filepath.Join(d, "*.ain"))
		if len(matches) > 0 {
			return matches[0]
		}
	}
	return ""
}
