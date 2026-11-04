// scn.go —— 编译型剧本（PSB / .scn）的事件库。
//
// 9nine 没有可读的 KAG 脚本：剧情编译成 PSB 容器，一个 .scn 里含多个
// scene（label 形如 a01_1_c），CG 鉴赏表另在 main/cglist.csv。
// 事件切分因此对齐原作自己的单位：
//   - 场景事件 = .scn 里一个「有台词的 scene」
//   - CG 事件  = cglist.csv 的一行（缩略图 + 图片序列）
//
// id 口径与适配器一致（`<剧本名>#<sceneIndex>`、`<缩略图名>`），
// 回放页据此重建台本；不做二次编号，避免「库里列的」与「能播的」错位。
package library

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"ucgr/internal/adapter"
	"ucgr/internal/adapter/scn"
)

// cgListCandidates CG 鉴赏表的兜底探测顺序（原样 / 整理 / 扁平三种解包布局）。
var cgListCandidates = []string{
	"raw/data/main/cglist.csv",
	"原始归档解包/data_文本/main/cglist.csv",
	"_分类/文本/data_main__cglist.csv",
	"解包整理/data_脚本与数据/main/cglist.csv",
}

// scnScenarioCandidates 剧本目录的兜底探测顺序（同上三种布局 + 整理布局）。
var scnScenarioCandidates = []string{
	"raw/data/scn",
	"原始归档解包/data_脚本/scn",
	"_分类/脚本",
	"解包整理/data_脚本与数据/scn",
}

// ResolveScenarioDir 定位剧本目录：约定路径优先，其次按引擎的常见布局探测。
//
// 三种 9nine 解包布局的剧本目录各不相同（raw/data/scn、原始归档解包/
// data_脚本/scn、_分类/脚本），约定只写一个默认值时靠这里兜底，
// 用户不必为每部游戏手写 ucgr-convention.json 才能扫出事件。
func ResolveScenarioDir(root string, conv adapter.Convention) (string, error) {
	if p := conv.ScenarioDir; p != "" {
		abs := filepath.Join(root, filepath.FromSlash(p))
		if st, err := os.Stat(abs); err == nil && st.IsDir() {
			return abs, nil
		}
	}
	for _, c := range scenarioDirCandidates(conv.EngineID) {
		abs := filepath.Join(root, filepath.FromSlash(c))
		if st, err := os.Stat(abs); err == nil && st.IsDir() {
			return abs, nil
		}
	}
	// 兜底：按扩展名自动探测。解包整理目录名五花八门（data_脚本与数据、
	// 脚本_文本、scn_backup…），枚举不完，就按「哪层 .scn 最多」挑。
	if strings.EqualFold(conv.EngineID, "scn") {
		if dir := densestScriptDir(root, ".scn"); dir != "" {
			return dir, nil
		}
	}
	if conv.ScenarioDir != "" {
		return "", fmt.Errorf("读不到剧本目录 %s（相对 %s）", conv.ScenarioDir, root)
	}
	return "", fmt.Errorf("读不到剧本目录（引擎 %s）", conv.EngineID)
}

func scenarioDirCandidates(engineID string) []string {
	switch strings.ToLower(strings.TrimSpace(engineID)) {
	case "scn":
		return scnScenarioCandidates
	case "kirikiri", "":
		return kirikiriScenarioDirs
	}
	return nil
}

// ScanSCN 扫一个 PSB 剧本目录，产出事件库。
func ScanSCN(root string, conv adapter.Convention) (*Library, error) {
	dir, err := ResolveScenarioDir(root, conv)
	if err != nil {
		return nil, err
	}
	files, err := scn.SortedSCNFiles(dir)
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

	// ① 场景事件：一个 .scn 里的每个有台词 scene。
	var failed []string
	dropped := 0
	for _, p := range files {
		data, rerr := os.ReadFile(p)
		if rerr != nil {
			failed = append(failed, filepath.Base(p))
			continue
		}
		sc, err := scn.ParseScript(data, p)
		if err != nil {
			failed = append(failed, filepath.Base(p))
			continue // 单个剧本坏掉不拖垮整个库
		}
		dropped += sc.Dropped
		stem := strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
		for _, s := range sc.SceneInfos() {
			title := s.Title
			if strings.TrimSpace(title) == "" {
				title = s.Label
			}
			if strings.TrimSpace(title) == "" {
				title = fmt.Sprintf("%s #%d", stem, s.Index)
			}
			lib.Scenes = append(lib.Scenes, SceneEvent{
				ID:     fmt.Sprintf("%s#%d", stem, s.Index),
				Title:  title,
				Script: rel(p),
				Label:  strconv.Itoa(s.Index),
			})
		}
	}

	// ② CG 事件：cglist.csv（缩略图 + 图片序列）。
	cgList := findCGList(root, conv.CGList)
	if cgList == "" {
		lib.Warnings = append(lib.Warnings, "没有找到 CG 鉴赏表（cglist.csv），CG 事件为空")
	}
	if cgList != "" {
		entries, err := scn.ParseCGList(cgList)
		if err == nil {
			for i, e := range entries {
				idx, err := strconv.Atoi(tailDigits(e.Thumb))
				if err != nil {
					idx = i + 1
				}
				pages := make([]string, 0, len(e.Images))
				for _, img := range e.Images {
					page, _ := scn.ResolveImageComposite(root, img)
					pages = append(pages, page)
				}
				title := e.Title
				if title == "" || title == e.Thumb {
					if len(pages) > 0 {
						title = fmt.Sprintf("%s · %s", e.Thumb, filepath.Base(pages[0]))
					} else {
						title = e.Thumb
					}
				}
				lib.CGs = append(lib.CGs, CGEvent{
					ID:     e.Thumb,
					Index:  idx,
					Title:  title,
					Thumb:  e.Thumb,
					Script: rel(cgList),
					Label:  e.Thumb, // scn 没有 kag 标签，Label 直接记缩略图名
					Pages:  pages,
				})
			}
		}
	}

	if len(failed) > 0 {
		// 只报数量 + 前几个文件名：完整清单会撑爆库清单，用户按名字能自己核对。
		sample := failed
		if len(sample) > 5 {
			sample = sample[:5]
		}
		suffix := ""
		if len(failed) > len(sample) {
			suffix = " 等"
		}
		lib.Warnings = append(lib.Warnings, fmt.Sprintf(
			"%d/%d 个 .scn 解析失败（解包产物不完整或混淆变体），已跳过：%s%s",
			len(failed), len(files), strings.Join(sample, "、"), suffix))
	}
	if dropped > 0 {
		// 局部损坏的剧本仍能播，但台词会断；如实报数，别让用户以为剧本就这么多。
		lib.Warnings = append(lib.Warnings, fmt.Sprintf(
			"%d 个值树节点不可读已跳过（解包产物局部损坏，这些位置的台词/演出会缺失）", dropped))
	}

	if len(lib.Scenes) == 0 && len(lib.CGs) == 0 {
		return nil, fmt.Errorf("没有识别到可回放事件（%s 下没有可解析的 .scn）", dir)
	}
	return lib, nil
}

// findCGList 定位鉴赏表：约定优先，其次按已知解包布局探测。
func findCGList(root, configured string) string {
	try := func(p string) string {
		if strings.TrimSpace(p) == "" {
			return ""
		}
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
		return ""
	}
	if configured != "" {
		if p := try(filepath.Join(root, filepath.FromSlash(configured))); p != "" {
			return p
		}
	}
	for _, c := range cgListCandidates {
		if p := try(filepath.Join(root, filepath.FromSlash(c))); p != "" {
			return p
		}
	}
	// 认不出的整理布局：按文件名找，取层数最浅的一个。
	return findFileByName(root, "cglist.csv", 4)
}
