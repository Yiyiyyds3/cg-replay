// eventbuild.go —— 库条目 → IR 台本的统一入口。
//
// OpenEvent（回放）与 CatalogIndexText（台词索引）必须走同一条构建路径，
// 否则会出现「列表里有、点了播不出」或「索引台词与播放台词不一致」。
// 这里按引擎 + 事件类型分派，壳层其余代码不再直接调各适配器。
package main

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"ucgr/internal/adapter"
	"ucgr/internal/adapter/kirikiri"
	"ucgr/internal/adapter/scn"
	"ucgr/internal/ir"
	"ucgr/internal/library"
)

// scnOptions 把约定翻成 scn 适配器的构建参数。
func scnOptions(conv adapter.Convention) scn.Options {
	opt := scn.DefaultOptions()
	if conv.StageW > 0 {
		opt.StageW = conv.StageW
	}
	if conv.StageH > 0 {
		opt.StageH = conv.StageH
	}
	opt.ImageDirs = append([]string(nil), conv.ImageDirs...)
	opt.AudioDirs = append([]string(nil), conv.AudioDirs...)
	return opt.Normalize()
}

// buildEvent 按引擎 + 事件类型构建台本（root/engine 显式传入，方便 CLI 复用）。
func buildEvent(root, engine string, conv adapter.Convention, lib *library.Library, kind, id string) (*ir.Event, string, error) {
	if lib == nil {
		return nil, "", fmt.Errorf("事件库为空")
	}
	abs := func(rel string) string { return filepath.Join(root, filepath.FromSlash(rel)) }

	switch kind {
	case "chapter":
		ch := lib.FindChapter(id)
		if ch == nil {
			return nil, "", fmt.Errorf("找不到演出段落 %s", id)
		}
		ev, err := library.BuildChapterEvent(root, ch)
		return ev, ch.Title, err

	case "cg":
		e := lib.FindCG(id)
		if e == nil {
			return nil, "", fmt.Errorf("找不到 CG 事件 %s", id)
		}
		if strings.EqualFold(engine, "scn") {
			ev, err := scn.BuildShowCG(root, abs(e.Script), e.Thumb, scnOptions(conv))
			return ev, e.Title, err
		}
		ev, err := kirikiri.BuildShowSegment(abs(e.Script), e.Label)
		return ev, e.Title, err

	case "scene":
		e := lib.FindScene(id)
		if e == nil {
			return nil, "", fmt.Errorf("找不到场景 %s", id)
		}
		switch {
		case strings.EqualFold(engine, "scn"):
			idx, err := strconv.Atoi(strings.TrimSpace(e.Label))
			if err != nil {
				return nil, e.Title, fmt.Errorf("scn 事件 %s 缺 scene 序号（%q）", id, e.Label)
			}
			ev, err := scn.BuildShowScene(root, abs(e.Script), idx, scnOptions(conv))
			return ev, e.Title, err
		case strings.EqualFold(engine, "artemis"):
			ev, err := buildArtemisShow(abs(e.Script), e.Label)
			return ev, e.Title, err
		case strings.EqualFold(engine, "bgi"):
			ev, err := buildBGIShow(abs(e.Script), e.Label)
			return ev, e.Title, err
		default:
			ev, err := kirikiri.BuildShowSegment(abs(e.Script), e.Label)
			return ev, e.Title, err
		}
	default:
		return nil, "", fmt.Errorf("未知事件类型 %s", kind)
	}
}

// engineOf 引擎 id 归一化（空 = KiriKiri 默认）。
func engineOf(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return "kirikiri"
	}
	return s
}
