// wolf.go —— Wolf RPG Editor 事件库：把 CommonEvent.dat 的剧情 CE
// 切成场景事件。
//
// 一个含 Message 的 CommonEvent = 一个场景事件（ID 形如 CE260，Title 用
// CE 名字「善第2章」；回放按 Label 里的编号反查）。系统 CE（无 Message，
// 如「○アイテム増減」）不入库。
package library

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"ucgr/internal/adapter"
	"ucgr/internal/adapter/wolf"
)

// findWolfGame 找 Wolf RPG 游戏根：本层或 Data 上层含 Game.exe + CommonEvent.dat。
func findWolfGame(root string) string {
	if wolf.LooksLikeWolfDir(root) {
		return root
	}
	return ""
}

// ScanWolf 扫 Wolf RPG 游戏目录，产出事件库。
func ScanWolf(root string, conv adapter.Convention) (*Library, error) {
	datPath := filepath.Join(root, filepath.FromSlash("data/BasicData/CommonEvent.dat"))
	ce, err := wolf.ParseCommonEventFile(datPath)
	if err != nil {
		return nil, err
	}
	lib := &Library{
		Root:        root,
		Game:        filepath.Base(strings.TrimRight(root, `\/`)),
		GeneratedAt: time.Now().Format(time.RFC3339),
		CGs:         []CGEvent{},
		Scenes:      []SceneEvent{},
		Chapters:    []ChapterEvent{},
	}
	rel := relKey(root, datPath)
	seen := map[string]bool{}
	for _, e := range ce.Events {
		id := fmt.Sprintf("CE%03d", e.ID)
		if seen[id] {
			continue
		}
		// 只列剧情 CE（含 Message）；系统 CE（物品增减等）没有回放价值。
		hasMsg := false
		for _, c := range e.Commands {
			if c.Op == 101 {
				hasMsg = true
				break
			}
		}
		if !hasMsg {
			continue
		}
		seen[id] = true
		lib.Scenes = append(lib.Scenes, SceneEvent{
			ID:     id,
			Title:  firstNonEmptyTitle(e.Name, id),
			Script: rel,
			Label:  strconv.Itoa(e.ID),
		})
	}
	if len(lib.Scenes) == 0 {
		return nil, fmt.Errorf("没有识别到可回放事件（%s）", filepath.Base(datPath))
	}
	// 舞台尺寸：Wolf RPG Editor 默认 816×624；约定可覆盖。
	lib.StageW, lib.StageH = 816, 624
	if conv.StageW > 0 && conv.StageH > 0 {
		lib.StageW, lib.StageH = conv.StageW, conv.StageH
	}
	return lib, nil
}

func firstNonEmptyTitle(name, id string) string {
	if strings.TrimSpace(name) != "" {
		return name
	}
	return "CE" + id
}
