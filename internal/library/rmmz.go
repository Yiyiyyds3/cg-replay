// rmmz.go —— RPG Maker MV/MZ 的「事件库」：一段连贯的图片演出就是一个事件。
//
// 素材目录就是游戏自己的 www/img（或 img），不搬运、不改写；
// 台词与图片指令由 adapter/rmmz 在打开事件时翻译成 IR 台本。
package library

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ucgr/internal/adapter"
	"ucgr/internal/adapter/rmmz"
)

func hasSystemJSON(root, dataDir string) bool {
	st, err := os.Stat(filepath.Join(root, filepath.FromSlash(dataDir), "System.json"))
	return err == nil && !st.IsDir()
}

// ScanRMMZ 扫 RPG Maker MV/MZ 游戏目录。
func ScanRMMZ(root string, conv adapter.Convention) (*Library, error) {
	dataDir := filepath.ToSlash(strings.TrimSpace(conv.ScenarioDir))
	// 约定里写死的目录可能和实际布局不符（MV 是 www/data，MZ 是 data，
	// 整合包是 内嵌/data）；这里以目录实际结构为准。
	if dataDir == "" || !hasSystemJSON(root, dataDir) {
		detected, ok := rmmz.FindDataDir(root)
		if !ok {
			return nil, fmt.Errorf("没找到 RPG Maker 的 data 目录（试过 www/data、data、内嵌/data）")
		}
		dataDir = detected
	}
	res, err := rmmz.Scan(root, dataDir)
	if err != nil {
		return nil, err
	}
	lib := &Library{
		Root:        root,
		EngineID:    rmmz.EngineID,
		Game:        filepath.Base(root),
		GeneratedAt: time.Now().Format(time.RFC3339),
		CGs:         []CGEvent{},
		Scenes:      []SceneEvent{},
		Chapters:    []ChapterEvent{},
	}
	for _, ref := range res.Refs {
		lib.Scenes = append(lib.Scenes, SceneEvent{
			ID:     ref.Key,
			Title:  ref.Title,
			Script: filepath.ToSlash(filepath.Join(dataDir, ref.File)),
			Label:  ref.Key,
		})
	}
	if len(lib.Scenes) == 0 {
		return nil, fmt.Errorf("没有识别到图片演出（扫了 %s，图片目录 %q，警告 %d 条）",
			dataDir, res.ImageDir, len(res.Warnings))
	}
	return lib, nil
}
