// 桌面壳入口（Wails v2 稳定版：Go 后端 + 系统 WebView2）。
//
// 依据 UI 设计文稿方案 B v0.2（2026-10-20 技术定稿）：
//   - 壳层保持薄：画面合成全部走根模块 internal/player（diff=0 红线，见文稿 §红线）。
//   - 本 M1 里程碑只做骨架打通：窗口 + React 前端 + 绑定桥验证。
//   - 架构原则（上游 FINAL 1017 §10.1，docs/06）：内核与界面分离——
//     摄入/去重/索引/合成是独立 Go core（仓库根模块），本壳只是它的一个前端。
// 构建方式（无需 wails CLI，直接 go build 即可）：
//   cd desktop/frontend && npm install && npm run build
//   cd desktop && go build -o ../bin/cgreplay-ui.exe .
package main

import (
	"context"
	"embed"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"

	"ucgr/internal/scan"
)

//go:embed all:frontend/dist
var assets embed.FS

// App Wails 绑定层：把 core 能力暴露给前端（v2 通过 Bind 注册）。
type App struct {
	ctx  context.Context
	root string // 游戏根目录（conf 迁移在 M4 接入）
}

// NewApp 构造绑定层。
func NewApp() *App { return &App{} }

// startup 保存运行上下文（后续弹窗/全屏/剪贴板都从 ctx 取窗口）。
func (a *App) startup(ctx context.Context) { a.ctx = ctx }

// Version 绑定桥自检：前端启动时调用，验证 window.go 通路。
func (a *App) Version() string {
	return "ucgr replay-ui 0.2.0-m2 · wails v2 · go core ready"
}

// ReportClientError 前端未捕获异常上报（落到 exe 同目录 errors.log）。
// 诊断用；保留常驻——黑屏比红横幅更糟（FR-12 不静默的兜底）。
func (a *App) ReportClientError(msg string) error {
	f, err := os.OpenFile(filepath.Join(exeDir(), "errors.log"),
		os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(time.Now().Format("15:04:05") + " " + msg + "\n")
	return err
}

// ScanDirectory 对目录执行 S0 体积盘点（设置页/选目录用，示例绑定保留）。
func (a *App) ScanDirectory(dir string) (string, error) {
	res, err := scan.ScanDir(dir, 0)
	if err != nil {
		return "", err
	}
	return scan.FormatReport(res), nil
}

func main() {
	app := NewApp()
	err := wails.Run(&options.App{
		Title:     "重返蓝鲸岛 · CG 演出回放",
		Width:     1440,
		Height:    860,
		MinWidth:  1024,
		MinHeight: 640,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup: app.startup,
		Bind:      []interface{}{app},
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			DisableWindowIcon:    false,
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}
