// catsystem2.go —— CatSystem2 事件库：一份 .cst 剧本 = 一个事件。
//
// 与 Artemis 的差别：CS2 的 `.cst` 是「一个场景一份脚本」，适配器的 BuildIR
// 也只产一个事件（`EventID: "script"`），脚本里没有可靠的场景切换标记，
// 所以事件库按**文件**切分，id 取文件名（去掉扩展名）。
package library

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"ucgr/internal/adapter"
	"ucgr/internal/adapter/catsystem2"
)

// ScanCS2 扫 CatSystem2 解包目录（scene/*.cst），一份脚本一个场景事件。
func ScanCS2(root string, conv adapter.Convention) (*Library, error) {
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
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".cst") {
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
		// 能解析才算可回放（坏文件不列进事件库）。
		if _, err := catsystem2.BuildShowScript(data, name); err != nil {
			continue
		}
		stem := strings.TrimSuffix(name, filepath.Ext(name))
		lib.Scenes = append(lib.Scenes, SceneEvent{
			ID:     stem,
			Title:  stem,
			Script: rel(p),
		})
	}
	if len(lib.Scenes) == 0 {
		return nil, fmt.Errorf("没有识别到可回放事件（%s 下没有可解析的 .cst）", conv.ScenarioDir)
	}
	return lib, nil
}
