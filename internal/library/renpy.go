// renpy.go —— Ren'Py 的「事件库直连」：一个含图片演出的 label 就是一个事件。
//
// 与 rmmz/unity 同路：不走 docs/04 的五方法契约，ScanRenPy 直接产出事件库，
// BuildShow 由 desktop 层按 engine_id 分派到 adapter/renpy。
package library

import (
	"fmt"
	"path/filepath"
	"time"

	"ucgr/internal/adapter"
	"ucgr/internal/adapter/renpy"
)

// ScanRenPy 扫 Ren'Py 游戏目录。
//
// game 目录的标准位置是 root/game，但整合包常套壳（兑现 就是两层目录下
// 才有 Game），这里以 renpy.FindGameDir 实际探测为准。
func ScanRenPy(root string, conv adapter.Convention) (*Library, error) {
	gameDir, ok := renpy.FindGameDir(root)
	if !ok {
		return nil, fmt.Errorf("没找到 Ren'Py 的 game 目录（根目录/子目录里都要有 .rpy 或 images）")
	}
	res, err := renpy.Scan(root, gameDir)
	if err != nil {
		return nil, err
	}
	lib := &Library{
		Root:        root,
		EngineID:    renpy.EngineID,
		Game:        filepath.Base(root),
		GeneratedAt: time.Now().Format(time.RFC3339),
		CGs:         []CGEvent{},
		Scenes:      []SceneEvent{},
		Chapters:    []ChapterEvent{},
		Warnings:    res.Warnings,
	}
	for _, ref := range res.Refs {
		lib.Scenes = append(lib.Scenes, SceneEvent{
			ID:     ref.Key,
			Title:  ref.Title,
			Script: joinGameScript(gameDir, ref.File),
			Label:  ref.Key,
		})
	}
	if len(lib.Scenes) == 0 {
		return nil, fmt.Errorf("没有识别到图片演出（game=%s，脚本 %d 个，警告 %d 条）",
			gameDir, res.Scripts, len(res.Warnings))
	}
	return lib, nil
}

func joinGameScript(gameDir, file string) string {
	if gameDir == "" || gameDir == "." {
		return file
	}
	return gameDir + "/" + file
}
