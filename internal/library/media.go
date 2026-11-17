// media.go —— 「视频/图片为主」游戏的「事件库直连」：一张图/一个影片一个事件。
//
// 与 unity/renpy 同路：ScanMedia 直接产出事件库，BuildShow 由 desktop/CLI
// 按 engine_id 分派到 adapter/media。
package library

import (
	"fmt"
	"path/filepath"
	"time"

	"ucgr/internal/adapter/media"
)

// ScanMedia 扫「视频/图片为主」的游戏目录。
func ScanMedia(root string) (*Library, error) {
	res, err := media.Scan(root)
	if err != nil {
		return nil, err
	}
	lib := &Library{
		Root:        root,
		EngineID:    media.EngineID,
		Game:        filepath.Base(root),
		GeneratedAt: time.Now().Format(time.RFC3339),
		CGs:         []CGEvent{},
		Scenes:      []SceneEvent{},
		Chapters:    []ChapterEvent{},
		Warnings:    res.Warnings,
	}
	for _, ref := range res.Refs {
		lib.Scenes = append(lib.Scenes, SceneEvent{
			ID:       ref.ID,
			Title:    ref.Title,
			Script:   ref.Path,
			Label:    ref.Kind, // still / video：打开时据此拼台本
			HasVideo: ref.Kind == "video",
		})
	}
	if len(lib.Scenes) == 0 {
		return nil, fmt.Errorf("媒体扫描没有产出事件")
	}
	return lib, nil
}
