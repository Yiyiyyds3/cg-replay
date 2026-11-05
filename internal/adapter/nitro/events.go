// events.go —— NPA 脚本的事件级入口（事件库与 GUI 回放共用）。
//
// 一个 .nss = 一个场景事件。事件库扫 nss.npa（汉化版游戏优先 nss-1.npa
// 覆盖包——游戏运行时先读它，但文本判码自动分流，这里两个都收）。
package nitro

import (
	"fmt"
	"sort"
	"strings"

	"ucgr/internal/adapter"
	"ucgr/internal/ir"
)

// NssScript 一个 .nss 脚本的解析产物。
type NssScript struct {
	Label string // .nss 条目名
	Event ir.Event
}

// BuildNpaEvents 解析剧本 NPA 里全部 .nss，每个产出一个事件。
//
// 单个脚本坏了不拖垮整体（跳过），全坏才报错。
func BuildNpaEvents(path string) ([]NssScript, error) {
	a, err := ParseNpa(path)
	if err != nil {
		return nil, err
	}
	var out []NssScript
	var firstErr error
	for _, e := range a.Entries {
		if !hasSuffixFold(e.Name, ".nss") {
			continue
		}
		data, err := a.ReadEntry(e)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("%s: %w", e.Name, err)
			}
			continue
		}
		ev, err := buildEvent(a, e.Name, data, 1024, 768)
		if err != nil {
			continue
		}
		out = append(out, NssScript{Label: e.Name, Event: *ev})
	}
	if len(out) == 0 {
		if firstErr != nil {
			return nil, fmt.Errorf("剧本归档里没有可解析的 .nss 脚本：%v", firstErr)
		}
		return nil, fmt.Errorf("剧本归档里没有 .nss 脚本")
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Label < out[j].Label })
	return out, nil
}

// buildEvent 单脚本 → 单事件（带 MessageBox 前置几何）。
func buildEvent(a *Archive, label string, data []byte, stageW, stageH int) (*ir.Event, error) {
	s, err := ParseNSS(data, label)
	if err != nil {
		return nil, err
	}
	if len(s.Segs) == 0 {
		return nil, fmt.Errorf("nitro: %s 没有内容段", label)
	}
	ad := &Adapter{}
	irObj, err := ad.BuildIR(s, &adapter.BuildCtx{GameID: "nitro"})
	if err != nil {
		return nil, err
	}
	if len(irObj.Events) == 0 {
		return nil, fmt.Errorf("nitro: %s 切不出事件", label)
	}
	ev := irObj.Events[0]
	// 前置对话框几何（鬼哭街重制版 1024×768 舞台底部 210px 高）。
	ev.Show = append(ir.MessageBoxOps("msg", stageW, stageH), ev.Show...)
	return &ev, nil
}

// BuildNssEvent 回放入口：按条目名从剧本 NPA 重建单事件（1024×768 标准舞台）。
func BuildNssEvent(scriptNpa, label string) (*ir.Event, error) {
	return BuildNssEventStage(scriptNpa, label, 1024, 768)
}

// BuildNssEventStage 回放入口（可指定舞台尺寸，对话框几何按舞台算）。
func BuildNssEventStage(scriptNpa, label string, stageW, stageH int) (*ir.Event, error) {
	a, err := ParseNpa(scriptNpa)
	if err != nil {
		return nil, err
	}
	var e *Entry
	for i := range a.Entries {
		if strings.EqualFold(a.Entries[i].Name, label) {
			e = &a.Entries[i]
			break
		}
	}
	if e == nil {
		return nil, fmt.Errorf("nitro: 剧本归档里没有条目 %s", label)
	}
	data, err := a.ReadEntry(*e)
	if err != nil {
		return nil, err
	}
	return buildEvent(a, e.Name, data, stageW, stageH)
}

func hasSuffixFold(s, suf string) bool {
	return strings.HasSuffix(strings.ToLower(s), strings.ToLower(suf))
}
