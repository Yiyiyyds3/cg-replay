// Wails 桌面壳入口（v3：Go 后端 + 系统 WebView）。
//
// 本文件为骨架：完整 UI 见 docs/06_客户端功能与交互设计.md。
// 构建需要 Wails 工具链与依赖（需网络）：
//   cd desktop && go mod tidy     # 拉取 github.com/wailsapp/wails/v3
//   wails build
//
// 架构原则（上游 FINAL 1017 §10.1）：内核与界面分离——
// 摄入/去重/索引/归档跟踪是独立 Go core（仓库根模块），本壳只是它的一个前端。
// 本子模块通过 go.mod 的 replace 指向根模块，复用 internal/ 全部能力。
package main

import (
	"embed"

	"github.com/wailsapp/wails/v3/pkg/application"

	"ucgr/internal/scan"
)

//go:embed all:frontend/dist
var assets embed.FS

// App Wails 绑定层：把 core 能力暴露给前端。
type App struct {
	LibraryRoot string
}

// ScanDirectory 对目录执行 S0 体积盘点（示例绑定）。
func (a *App) ScanDirectory(dir string) string {
	res, err := scan.ScanDir(dir, 0)
	if err != nil {
		return "错误: " + err.Error()
	}
	return scan.FormatReport(res)
}

func main() {
	app := application.New(application.Options{
		Name:        "统一 CG 演出回放库",
		Description: "CG 演出重温 · 空间裁剪",
		Services: []application.Service{
			application.NewService(&App{}),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
	})
	app.NewWebviewWindow(application.WebviewWindowOptions{
		Title:  "统一 CG 演出回放库",
		Width:  1280,
		Height: 800,
	})
	if err := app.Run(); err != nil {
		panic(err)
	}
}
