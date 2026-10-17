module ucgr/desktop

go 1.27.2

// 桌面壳（Wails v3）：Go 后端 + 系统 WebView。
// 依赖就绪步骤（需网络）：
//   cd desktop && go mod tidy     # 拉取 github.com/wailsapp/wails/v3
//   wails build
// 在此之前本子模块不参与根模块构建，核心 CLI（../cmd/ucgr）不受影响。

require (
	github.com/wailsapp/wails/v3 v3.0.0-alpha.37
	ucgr v0.0.0
)

replace ucgr => ../
