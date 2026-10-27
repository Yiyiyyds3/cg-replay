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
	"ucgr/internal/ir"
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
		// 能解析**且**能构建出演出才算可回放（坏文件、空脚本都不列进库——
		// 否则会出现「库里列着但点开播不出来」）。
		ev, err := catsystem2.BuildShowScript(data, name)
		if err != nil || len(ev.Show) == 0 {
			continue
		}
		stem := strings.TrimSuffix(name, filepath.Ext(name))
		hasVideo := false
		for _, op := range ev.Show {
			if op.Kind == ir.ShowVideoPlay {
				hasVideo = true
				break
			}
		}
		lib.Scenes = append(lib.Scenes, SceneEvent{
			ID:       stem,
			Title:    stem,
			Script:   rel(p),
			HasVideo: hasVideo,
		})
	}
	if len(lib.Scenes) == 0 {
		return nil, fmt.Errorf("没有识别到可回放事件（%s 下没有可解析的 .cst）", conv.ScenarioDir)
	}
	return lib, nil
}
