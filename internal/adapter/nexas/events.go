// events.go —— NeXAS 脚本的事件级入口（事件库与 GUI 回放共用）。
//
// Script.pac 里每个 .bin 编译脚本 = 一个场景事件；Show 台本由 builder
// 直接产出。回放侧按条目名（Label）从 pac 里重建。
package nexas

import (
	"fmt"
	"path/filepath"

	"ucgr/internal/adapter"
	"ucgr/internal/ir"
)

// PacScript 一个 .bin 脚本的解析产物。
type PacScript struct {
	Label string // .bin 条目名（GBK 显示名）
	Event ir.Event
}

// BuildPacEvents 解析 Script.pac 里全部 .bin 脚本，每个产出一个事件。
//
// 单个脚本坏了不拖垮整体（跳过），全坏才报错。
func BuildPacEvents(path string) ([]PacScript, error) {
	a, err := ParsePac(path)
	if err != nil {
		return nil, err
	}
	var out []PacScript
	var firstErr error
	for _, e := range a.Entries {
		if !hasSuffixFold(e.Name, ".bin") {
			continue
		}
		data, err := a.ReadEntry(e)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("%s: %w", e.Name, err)
			}
			continue
		}
		ad := &Adapter{}
		obj, err := ad.ParseScript(data, &adapter.ScriptCtx{EntryPath: e.Name})
		if err != nil {
			continue
		}
		irObj, err := ad.BuildIR(obj, &adapter.BuildCtx{GameID: "nexas"})
		if err != nil {
			continue
		}
		for _, ev := range irObj.Events {
			out = append(out, PacScript{Label: e.Name, Event: ev})
		}
	}
	if len(out) == 0 {
		if firstErr != nil {
			return nil, fmt.Errorf("Script.pac 里没有可解析的 .bin 脚本：%v", firstErr)
		}
		return nil, fmt.Errorf("Script.pac 里没有 .bin 脚本")
	}
	return out, nil
}

// BuildPacScriptEvent 重建指定 .bin 条目的演出（回放入口）。
//
// 台词层名是 "text"：对话框几何必须前置，否则文字从整屏左上角铺。
func BuildPacScriptEvent(pacPath, label string, stageW, stageH int) (*ir.Event, error) {
	a, err := ParsePac(pacPath)
	if err != nil {
		return nil, err
	}
	e, ok := a.Lookup(label)
	if !ok {
		return nil, fmt.Errorf("nexas: %s 里没有条目 %s", filepath.Base(pacPath), label)
	}
	data, err := a.ReadEntry(e)
	if err != nil {
		return nil, err
	}
	ad := &Adapter{}
	obj, err := ad.ParseScript(data, &adapter.ScriptCtx{EntryPath: e.Name})
	if err != nil {
		return nil, err
	}
	irObj, err := ad.BuildIR(obj, &adapter.BuildCtx{GameID: "nexas"})
	if err != nil {
		return nil, err
	}
	ev := irObj.Events[0]
	if len(ev.Show) > 0 {
		ev.Show = append(ir.MessageBoxOps(textLayer, stageW, stageH), ev.Show...)
	}
	return &ev, nil
}
