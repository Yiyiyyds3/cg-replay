// bgi.go —— BGI 事件库：把 .bs 脚本切成一串可回放事件。
//
// 事件边界由适配器的 builder 决定（换背景/换事件时切），这里直接复用
// `bgi.BuildScriptEvents`——保证「库里列的」与「IR 里的」一一对应，
// 不会出现列了却播不出来。
//
// 事件 id 沿用适配器的 `<脚本名>#<序号>`；Label 记序号（回放据此重建）。
package library

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"ucgr/internal/adapter"
	"ucgr/internal/adapter/bgi"
)

// ScanBGI 扫 BGI 解包目录（.bs 编译脚本），产出事件库。
func ScanBGI(root string, conv adapter.Convention) (*Library, error) {
	dir := filepath.Join(root, filepath.FromSlash(conv.ScenarioDir))
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("读不到剧本目录 %s：%w", dir, err)
	}
	lib := &Library{
		Root:        root,
		Game:        filepath.Base(strings.TrimRight(root, `\/`)),
		GeneratedAt: time.Now().Format(time.RFC3339),
		CGs:         []CGEvent{},
		Scenes:      []SceneEvent{},
		Chapters:    []ChapterEvent{},
	}
	rel := func(p string) string {
		if r, err := filepath.Rel(root, p); err == nil {
			return filepath.ToSlash(r)
		}
		return filepath.ToSlash(p)
	}
	var names []string
	for _, e := range ents {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".bs") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	for _, name := range names {
		p := filepath.Join(dir, name)
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		evs, err := bgi.BuildScriptEvents(data, name)
		if err != nil {
			continue // 单个脚本坏掉不拖垮整个库
		}
		stem := strings.TrimSuffix(name, filepath.Ext(name))
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
				Script: rel(p),
				Label:  fmt.Sprint(i + 1),
			})
		}
	}
	if len(lib.Scenes) == 0 {
		return nil, fmt.Errorf("没有识别到可回放事件（%s 下没有可解析的 .bs）", conv.ScenarioDir)
	}
	return lib, nil
}
