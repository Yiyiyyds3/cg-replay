// unity.go —— Unity/ACT 游戏的「事件库」：一部 CG 就是一个事件。
//
// 素材由 tools/unity_cg_export.py 导出（cg/<CG名>/ + cg-index.json），
// 这里只按索引列事件，不解析任何脚本——Unity 的 CG 没有剧本。
package library

import (
	"fmt"
	"path/filepath"
	"time"

	"ucgr/internal/adapter/unitycg"
)

// ScanUnity 按 cg-index.json 列出可回放的 CG。
//
// frames / still 走 Go 侧逐帧合成；spine 由前端 Spine WebGL 组件渲染，
// 两者都能列出来。kind=unknown 仍排除，避免点开必然失败。
func ScanUnity(root string) (*Library, error) {
	idx, err := unitycg.LoadIndex(root)
	if err != nil {
		return nil, fmt.Errorf("这个目录不是导出的 CG 库（%w）", err)
	}
	lib := &Library{
		Root:        root,
		EngineID:    "unity",
		Game:        filepath.Base(root),
		GeneratedAt: time.Now().Format(time.RFC3339),
		CGs:         []CGEvent{},
		Scenes:      []SceneEvent{},
	}
	pending := 0
	kinds := map[string]int{}
	for _, cg := range idx.CGs {
		if !unitycg.Playable(cg) {
			pending++
			kinds[cg.Kind]++ // 如实记下是什么类型：binary-skel / unknown / 缺文件的不完整 spine
			continue
		}
		lib.Scenes = append(lib.Scenes, SceneEvent{
			ID:     cg.ID,
			Title:  cg.Title(),
			Script: filepath.ToSlash(filepath.Join("cg", cg.ID)),
			Label:  cg.Kind, // frames / still / spine：打开时据此拼台本
		})
	}
	if len(lib.Scenes) == 0 {
		if pending > 0 {
			detail := ""
			for k, n := range kinds {
				detail += fmt.Sprintf(" %s=%d", k, n)
			}
			return nil, fmt.Errorf("这 %d 部 CG 都不可播（类型分布:%s）；"+
				"unknown/不完整说明缺 skeleton、atlas 或贴图，请检查导出日志", pending, detail)
		}
		return nil, fmt.Errorf("索引里没有可回放的 CG")
	}
	return lib, nil
}

// findCG 按 id 找索引里的 CG 记录。
func findCG(idx *unitycg.Index, id string) (unitycg.CG, bool) {
	for _, cg := range idx.CGs {
		if cg.ID == id {
			return cg, true
		}
	}
	return unitycg.CG{}, false
}

// BuildUnityEvent 打开某个 CG 事件时构建演出台本。
func BuildUnityEvent(root, id string) (unitycg.CG, error) {
	idx, err := unitycg.LoadIndex(root)
	if err != nil {
		return unitycg.CG{}, err
	}
	cg, ok := findCG(idx, id)
	if !ok {
		return unitycg.CG{}, fmt.Errorf("索引里没有 CG %s", id)
	}
	return cg, nil
}
