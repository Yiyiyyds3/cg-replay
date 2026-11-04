// nitro.go —— Nitro+ / SDActi 事件库：把 nss.npa（汉化版为 nss-1.npa 覆盖包）
// 里的 .nss 明文脚本切成场景事件。
//
// 一个 .nss = 一个场景事件（与 nitro.BuildIR 的事件粒度一致，保证「库里
// 列的」与「IR 里的」一一对应）。事件 id 沿用适配器的 `<脚本名>#001`；
// Label 记 .nss 条目名（回放据此从 NPA 重建）。
package library

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ucgr/internal/adapter"
	"ucgr/internal/adapter/nitro"
)

// ScanNitro 扫 Nitro+（NPA 归档 + NSS 明文脚本）游戏目录，产出事件库。
func ScanNitro(root string, conv adapter.Convention) (*Library, error) {
	scriptNpa := findScriptNpa(root)
	if scriptNpa == "" {
		return nil, fmt.Errorf("没有找到 Nitro+ 剧本归档（nss.npa / 含 .nss 条目的 .npa，%s）", root)
	}
	evs, err := nitro.BuildNpaEvents(scriptNpa)
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
	rel := relKey(root, scriptNpa)
	for i, se := range evs {
		id := se.Event.EventID
		if id == "" {
			id = fmt.Sprintf("script#%03d", i+1)
		}
		title := strings.TrimSpace(se.Event.Title)
		if title == "" {
			title = strings.TrimSuffix(se.Label, ".nss")
		}
		lib.Scenes = append(lib.Scenes, SceneEvent{
			ID:     id,
			Title:  title,
			Script: rel,
			Label:  se.Label,
		})
	}
	if len(lib.Scenes) == 0 {
		return nil, fmt.Errorf("没有识别到可回放事件（%s 可解析但切不出事件）", filepath.Base(scriptNpa))
	}
	// 舞台尺寸：鬼哭街重制版是 1024×768；约定可覆盖。
	lib.StageW, lib.StageH = 1024, 768
	if conv.StageW > 0 && conv.StageH > 0 {
		lib.StageW, lib.StageH = conv.StageW, conv.StageH
	}
	return lib, nil
}

// findScriptNpa 找 Nitro+ 剧本归档：优先根目录 nss.npa / nss-1.npa（汉化
// 覆盖包）；没有则试探各 .npa 里是否含 .nss 条目（有的游戏可能改名）。
func findScriptNpa(root string) string {
	for _, name := range []string{"nss.npa", "nss-1.npa"} {
		p := filepath.Join(root, name)
		if isFile(p) {
			return p
		}
	}
	matches, _ := filepath.Glob(filepath.Join(root, "*.npa"))
	for _, m := range matches {
		a, err := nitro.ParseNpa(m)
		if err != nil {
			continue
		}
		for _, e := range a.Entries {
			if strings.HasSuffix(strings.ToLower(e.Name), ".nss") {
				return m
			}
		}
	}
	return ""
}

func isFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}
