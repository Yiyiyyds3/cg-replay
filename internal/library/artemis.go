// artemis.go —— Artemis 事件库：把 .ast 剧本里的 block 切成一串可回放事件。
//
// Artemis 的一个 `.ast` 里含多个 block（剧情段），事件单位 = block
// （与 `artemis.BuildIR` 的粒度一致）。每个 block 出一个「场景」事件：
// id = `<文件名>#<block 序号>`，Label 记 block 序号（回放时据此重建演出）。
//
// 与 KiriKiri 的差别：KiriKiri 有「画廊槽位」这种明确的 CG 事件表，
// Artemis 没有——所有 block 都是剧情段，所以这里只出场景事件。
package library

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"ucgr/internal/adapter"
	"ucgr/internal/adapter/artemis"
)

// ScanArtemis 扫 Artemis 解包目录，产出事件库（每个 block 一个场景事件）。
func ScanArtemis(root string, conv adapter.Convention) (*Library, error) {
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
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".ast") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	for _, name := range names {
		p := filepath.Join(dir, name)
		ast, err := artemis.ParseAstFile(p)
		if err != nil {
			continue // 单个剧本坏掉不拖垮整个库
		}
		stem := strings.TrimSuffix(name, filepath.Ext(name))
		movieFlags := artemis.BlockMovieFlags(ast)
		for bi, blk := range artemis.BlockTitles(ast) {
			title := blk.Title
			if title == "" {
				title = fmt.Sprintf("%s#%d", stem, blk.Index)
			}
			hasVideo := bi < len(movieFlags) && movieFlags[bi]
			lib.Scenes = append(lib.Scenes, SceneEvent{
				ID:       fmt.Sprintf("%s#%d", stem, blk.Index),
				Title:    title,
				Script:   rel(p),
				Label:    fmt.Sprint(blk.Index),
				HasVideo: hasVideo,
			})
		}
	}
	if len(lib.Scenes) == 0 {
		return nil, fmt.Errorf("没有识别到可回放事件（%s 下没有可解析的 .ast）", conv.ScenarioDir)
	}
	return lib, nil
}
