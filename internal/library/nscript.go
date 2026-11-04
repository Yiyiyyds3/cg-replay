// nscript.go —— NScripter 事件库：把 nscript.dat 切成一串可回放事件。
//
// NScripter 的剧本是整作一份（nscript.dat / 0.txt），事件边界由适配器的
// builder 决定（换背景即切场景），这里直接复用 `nscript.BuildScriptEvents`
// ——保证「库里列的」与「IR 里的」一一对应。
//
// 事件 id 沿用适配器的 `<脚本名>#<序号>`；Label 记序号（回放据此重建）。
package library

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ucgr/internal/adapter"
	"ucgr/internal/adapter/nscript"
)

// ScanNScripter 扫 NScripter 解包目录，产出事件库。
//
// 剧本在根目录（nscript.dat / 0.txt / 00.txt）；找不到时向下一层探测
// （有的整理版把游戏本体套在一层目录里）。
func ScanNScripter(root string, conv adapter.Convention) (*Library, error) {
	script := findNScripterScript(root)
	if script == "" {
		return nil, fmt.Errorf("没有找到 NScripter 剧本（nscript.dat / 0.txt，%s）", root)
	}
	data, err := nscript.ReadScriptData(script)
	if err != nil {
		return nil, fmt.Errorf("读剧本 %s 失败：%w", filepath.Base(script), err)
	}
	evs, err := nscript.BuildScriptEvents(data, filepath.Base(script))
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
	stem := strings.TrimSuffix(filepath.Base(script), filepath.Ext(script))
	rel := relKey(root, script)
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
			ID:     id,
			Title:  title,
			Script: rel,
			Label:  fmt.Sprint(i + 1),
		})
	}
	if len(lib.Scenes) == 0 {
		return nil, fmt.Errorf("没有识别到可回放事件（%s 可解析但切不出事件）", filepath.Base(script))
	}
	lib.StageW, lib.StageH = detectStageSize(root)
	return lib, nil
}

// findNScripterScript 找 NScripter 剧本：先根目录，再往下探两层。
func findNScripterScript(root string) string {
	for depth := 0; depth <= 2; depth++ {
		p := probeNScript(root, depth)
		if p != "" {
			return p
		}
	}
	return ""
}

// probeNScript 在 depth 层内找 nscript.dat / 0.txt / 00.txt。
func probeNScript(root string, maxDepth int) string {
	found := ""
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || found != "" {
			return nil
		}
		if d.IsDir() {
			if rel, e := filepath.Rel(root, p); e == nil && rel != "." &&
				strings.Count(rel, string(filepath.Separator)) >= maxDepth {
				return fs.SkipDir
			}
			return nil
		}
		switch strings.ToLower(d.Name()) {
		case "nscript.dat", "0.txt", "00.txt":
			if nscript.LooksLikeNScripterDat(mustRead(p)) || strings.HasSuffix(strings.ToLower(d.Name()), ".txt") {
				found = p
				return fs.SkipAll
			}
		}
		return nil
	})
	return found
}

func mustRead(p string) []byte {
	data, _ := os.ReadFile(p)
	return data
}
