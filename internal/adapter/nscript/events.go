// events.go —— NScripter 脚本的事件级入口（事件库与 GUI 回放共用）。
//
// nscript.dat 是整作一份脚本，适配器的 builder 按「换背景」切成事件，
// 与 BuildIR 同源同粒度；Show 台本由 builder 直接产出（无需 Timeline 转换）。
package nscript

import (
	"fmt"
	"path/filepath"

	"ucgr/internal/adapter"
	"ucgr/internal/ir"
)

// BuildScriptEvents 解析脚本并返回它切出的事件列表。
func BuildScriptEvents(data []byte, name string) ([]ir.Event, error) {
	a := &Adapter{}
	obj, err := a.ParseScript(data, &adapter.ScriptCtx{EntryPath: name})
	if err != nil {
		return nil, err
	}
	out, err := a.BuildIR(obj, &adapter.BuildCtx{GameID: "nscript"})
	if err != nil {
		return nil, err
	}
	return out.Events, nil
}

// BuildShowEvent 构建脚本里第 n 个事件（1 起）的演出。
func BuildShowEvent(data []byte, name string, n int) (*ir.Event, error) {
	evs, err := BuildScriptEvents(data, name)
	if err != nil {
		return nil, err
	}
	if n < 1 || n > len(evs) {
		return nil, fmt.Errorf("nscript: %s 里没有第 %d 个事件（共 %d 个）", name, n, len(evs))
	}
	return &evs[n-1], nil
}

// BuildShowEventFileStage 读文件并构建第 n 个事件的演出（按舞台尺寸摆对话框）。
//
// 台词层名是 "text"：对话框几何必须落在同一层上，否则文字从整屏左上角铺。
func BuildShowEventFileStage(path string, n, stageW, stageH int) (*ir.Event, error) {
	data, err := ReadScriptData(path)
	if err != nil {
		return nil, err
	}
	ev, err := BuildShowEvent(data, filepath.Base(path), n)
	if err != nil {
		return nil, err
	}
	if len(ev.Show) > 0 {
		ev.Show = append(ir.MessageBoxOps(textLayer, stageW, stageH), ev.Show...)
	}
	return ev, nil
}
