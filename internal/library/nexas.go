// nexas.go —— NeXAS 事件库：把 Script.pac 里的 .bin 编译脚本切成场景事件。
//
// 一个 .bin = 一个场景（Baldr Sky 一个章节一个脚本，300-900 段）；
// 事件粒度由适配器的 builder 决定，这里直接复用 `nexas.BuildScriptEvents`
// ——保证「库里列的」与「IR 里的」一一对应。
//
// 事件 id 沿用适配器的 `<脚本名>#<序号>`；Label 记 .bin 条目名（回放据此
// 从 Script.pac 重建）。
package library

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"ucgr/internal/adapter"
	"ucgr/internal/adapter/nexas"
)

// ScanNeXAS 扫 NeXAS 游戏目录（Baldr Sky 系列），产出事件库。
func ScanNeXAS(root string, conv adapter.Convention) (*Library, error) {
	scriptPac := findScriptPac(root)
	if scriptPac == "" {
		return nil, fmt.Errorf("没有找到 NeXAS 剧本归档（Script.pac，%s）", root)
	}
	evs, err := nexas.BuildPacEvents(scriptPac)
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
	rel := relKey(root, scriptPac)
	for i, se := range evs {
		id := se.Event.EventID
		if id == "" {
			id = fmt.Sprintf("script#%03d", i+1)
		}
		title := strings.TrimSpace(se.Event.Title)
		if title == "" {
			title = fmt.Sprintf("script #%03d", i+1)
		}
		lib.Scenes = append(lib.Scenes, SceneEvent{
			ID:     id,
			Title:  title,
			Script: rel,
			Label:  se.Label,
		})
	}
	if len(lib.Scenes) == 0 {
		return nil, fmt.Errorf("没有识别到可回放事件（%s 可解析但切不出事件）", filepath.Base(scriptPac))
	}
	// 舞台尺寸：Baldr Sky 是 1024×768（立绘高 ~775 贴底）；约定可覆盖。
	lib.StageW, lib.StageH = 1024, 768
	if conv.StageW > 0 && conv.StageH > 0 {
		lib.StageW, lib.StageH = conv.StageW, conv.StageH
	}
	return lib, nil
}

// findScriptPac 找 NeXAS 剧本归档：优先根目录 Script.pac；没有则试探
// 各 .pac 里是否含 .bin 条目（有的游戏可能改名）。
func findScriptPac(root string) string {
	for _, name := range []string{"Script.pac", "script.pac"} {
		p := filepath.Join(root, name)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	// 逐 pac 试探（找含 .bin 条目最多的）。
	matches, _ := filepath.Glob(filepath.Join(root, "*.pac"))
	sort.Strings(matches)
	for _, m := range matches {
		a, err := nexas.ParsePac(m)
		if err != nil {
			continue
		}
		for _, e := range a.Entries {
			if strings.HasSuffix(strings.ToLower(e.Name), ".bin") {
				return m
			}
		}
	}
	return ""
}
