// events.go —— BGI 剧本的事件级入口（事件库与 GUI 回放都要用）。
//
// BGI 的 .bs 脚本在适配器里被 builder 按「换背景/换事件」切成若干事件
// （EventID 形如 `<脚本名>#<序号>`），builder **同时产 Show 台本**——
// 与 Artemis 不同，BGI 不需要 Timeline→Show 转换，直接能播。
package bgi

import (
	"fmt"
	"os"
	"path/filepath"

	"ucgr/internal/adapter"
	"ucgr/internal/ir"
)

// BuildScriptEvents 解析 .bs 并返回它切出的事件列表（与 BuildIR 同源、同粒度）。
func BuildScriptEvents(data []byte, name string) ([]ir.Event, error) {
	a := &Adapter{}
	obj, err := a.ParseScript(data, &adapter.ScriptCtx{EntryPath: name})
	if err != nil {
		return nil, err
	}
	out, err := a.BuildIR(obj, &adapter.BuildCtx{GameID: "bgi"})
	if err != nil {
		return nil, err
	}
	return out.Events, nil
}

// BuildShowEvent 构建脚本里第 n 个事件（1 起）的演出。
//
// n 越界要报错——事件库里列出来的每个 id 都要能播出对应的事件。
func BuildShowEvent(data []byte, name string, n int) (*ir.Event, error) {
	evs, err := BuildScriptEvents(data, name)
	if err != nil {
		return nil, err
	}
	if n < 1 || n > len(evs) {
		return nil, fmt.Errorf("bgi: %s 里没有第 %d 个事件（共 %d 个）", name, n, len(evs))
	}
	return &evs[n-1], nil
}

// BuildShowEventFile 读文件并构建第 n 个事件。
func BuildShowEventFile(path string, n int) (*ir.Event, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return BuildShowEvent(data, filepath.Base(path), n)
}
