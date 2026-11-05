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
	"ucgr/internal/adapter/nexas"
	"ucgr/internal/adapter/nitro"
	"ucgr/internal/adapter/nscript"
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
			// Label 为空的条目是「整份剧本一个事件」（新口径）；
			// 老库里的 block 级条目仍带序号，继续支持。
			w, h := stageOf(conv, lib)
			if strings.TrimSpace(e.Label) == "" {
				ev, err := buildArtemisScript(abs(e.Script), w, h)
				return ev, e.Title, err
			}
			ev, err := buildArtemisShow(abs(e.Script), e.Label, w, h)
			return ev, e.Title, err
		case strings.EqualFold(engine, "bgi"):
			w, h := stageOf(conv, lib)
			ev, err := buildBGIShow(abs(e.Script), e.Label, w, h)
			return ev, e.Title, err
		case strings.EqualFold(engine, "catsystem2"):
			// 舞台尺寸要传进 BuildIR：对话框位置按画布高度算，用错画布
			// 会让框跑到画面外。优先扫描时从素材推断的尺寸。
			w, h := stageOf(conv, lib)
			ev, err := buildCS2Show(abs(e.Script), w, h)
			return ev, e.Title, err
		case strings.EqualFold(engine, "nscript"):
			// NScripter：Label = 事件序号（1 起）。舞台推断不出来时
			// 按 NScripter 标准舞台 640×480。
			w, h := stageOf(conv, lib)
			if w <= 0 || h <= 0 {
				w, h = 640, 480
			}
			n, err := strconv.Atoi(strings.TrimSpace(e.Label))
			if err != nil {
				return nil, e.Title, fmt.Errorf("nscript 事件 %s 缺序号（%q）", id, e.Label)
			}
			ev, err := nscript.BuildShowEventFileStage(abs(e.Script), n, w, h)
			return ev, e.Title, err
		case strings.EqualFold(engine, "nexas"):
			// NeXAS：Label = .bin 条目名。舞台推不出来按 Baldr Sky 1024×768。
			w, h := stageOf(conv, lib)
			if w <= 0 || h <= 0 {
				w, h = 1024, 768
			}
			ev, err := nexas.BuildPacScriptEvent(abs(e.Script), e.Label, w, h)
			return ev, e.Title, err
		case strings.EqualFold(engine, "nitro"):
			// Nitro+：Label = .nss 条目名。舞台推不出来按鬼哭街重制版 1024×768。
			w, h := stageOf(conv, lib)
			if w <= 0 || h <= 0 {
				w, h = 1024, 768
			}
			ev, err := nitro.BuildNssEventStage(abs(e.Script), e.Label, w, h)
			return ev, e.Title, err
		default:
			ev, err := kirikiri.BuildShowSegment(abs(e.Script), e.Label)
			return ev, e.Title, err
		}
	default:
		return nil, "", fmt.Errorf("未知事件类型 %s", kind)
	}
}

// stageOf 取该游戏的舞台尺寸：约定优先，其次扫描时从素材推断的值。
// 都拿不到返回 (0, 0)，由各适配器用自己的默认值。
func stageOf(conv adapter.Convention, lib *library.Library) (int, int) {
	if conv.StageW > 0 && conv.StageH > 0 {
		return conv.StageW, conv.StageH
	}
	if lib != nil && lib.StageW > 0 && lib.StageH > 0 {
		return lib.StageW, lib.StageH
	}
	return 0, 0
}

// engineOf 引擎 id 归一化（空 = KiriKiri 默认）。
func engineOf(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return "kirikiri"
	}
	return s
}
