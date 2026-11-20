# 02 · 启动与 GUI（统一 CG 演出回放库）

> 本文所有结论均来自本机实跑，失败与成功一律附命令原文与原始输出。
> 采集时间：2026-11-16 · 机器：Windows 11 专业版 24H2 (Build 26100) · 仓库分支 `main`
> 仓库：`E:\Software_tools\DevSpace\Projects\Agent\Unified_CG_Replay_Library`

---

## 0. 先看这段（30 秒结论）

**有图形界面，而且现在就有一份能双击运行的成品。**

| 问题 | 答案 |
| :--- | :--- |
| 有没有 GUI？ | **有**。Wails v2 + React 18 + 系统 WebView2，窗口标题「重返蓝鲸岛 · CG 演出回放」 |
| 现在能不能直接跑？ | **能**。`desktop\build\bin\cgreplay-ui.exe`（18,333,696 字节，2026-11-13 构建）实测启动成功、无黑框、标题正确 |
| 能不能重新构建 GUI？ | **能**。`wails build` 全流程实跑通过（12.761s）；`wails` CLI 在机器上但**不在 PATH** |
| 纯 CLI 能用吗？ | **能，且完整**。`bin\ucgr.exe` 的子命令全部实测可用 |
| 唯一的坑是什么？ | 在 `desktop\` 里**裸跑 `go build`** 产出的是 dev 模式：**带黑框 + 窗口标题变成 `[Error]` + 白屏**。必须加 `-tags production -H windowsgui` |

> ⚠️ 任务原始假设「GUI 可能不可构建」**不成立**：GUI 工程完备，依赖齐全，缺的只有「正确的构建参数」和「把 wails 加进 PATH」。

---

## 1. 项目结构上的一个必须知道的事实

GUI **不在仓库根目录**，而在子目录 `desktop\`，且是**独立的 Go 模块**：

```
Unified_CG_Replay_Library/
├── go.mod                    # module ucgr（核心，无 main.go）
├── cmd/ucgr/main.go          # CLI 主入口
├── cmd/cgreplay/main.go      # 第二 CLI 入口
├── cmd/library-audit/        # 工具 CLI
├── cmd/spike-a/              # 试验 CLI
├── internal/                 # 核心能力（含 platform/win32 GDI 窗口与字体）
├── desktop/                  # ← Wails 壳，独立模块 ucgr/desktop
│   ├── go.mod                # replace ucgr => ../（复用根模块）
│   ├── wails.json            # Wails 配置（不在仓库根！）
│   ├── main.go               # GUI 入口：wails.Run
│   ├── frontend/             # React 18 + Vite + Tailwind
│   │   ├── package.json
│   │   ├── node_modules/     # ✅ 已安装
│   │   ├── dist/             # ✅ 已构建
│   │   └── wailsjs/          # 生成的绑定（已提交入库）
│   └── build/
│       ├── appicon.png
│       ├── windows/{icon.ico, info.json, wails.exe.manifest, installer/}
│       └── bin/cgreplay-ui.exe   # ✅ 现成可跑成品
└── bin/                      # 历史构建产物
```

`.gitignore` 里 `build/`（第 115 行）与 `bin/`、`*.exe`（第 78–80 行）被忽略，`frontend/wailsjs/` 则**故意保留提交**（第 112–114 行注释：「约定提交（团队协作需要）」）。这解释了为什么 `wailsjs/` 在仓库里而 `build/` 不在。

**后果（本机实测）**：在仓库根执行 `go build -o bin\ucgr-gui.exe .` 会失败——

```
$ go build -o bin\ucgr-gui.exe .
no Go files in E:\Software_tools\DevSpace\Projects\Agent\Unified_CG_Replay_Library
[exit=1]
```

根目录没有 `main.go`，GUI 入口在 `desktop\main.go`。**必须先 `cd desktop`。**

---

## 2. 本机可用的启动方式

### 2.1 方式 A：直接跑现成 GUI（最快，推荐先试这个）

```powershell
cd E:\Software_tools\DevSpace\Projects\Agent\Unified_CG_Replay_Library
.\desktop\build\bin\cgreplay-ui.exe
```

**预期现象**（实测 10 秒观察）：

- 弹出 1440×860 窗口，标题 **「重返蓝鲸岛 · CG 演出回放」**
- **无控制台黑框**（PE 子系统 = 2 WINDOWS_GUI）
- 数据目录 `%LOCALAPPDATA%\cgreplay-ui` 已自动存在
- 进程存活，`MainWindowHandle` 非 0

实测记录：

```
ALIVE=True  PID=46856  MainWindowTitle=[重返蓝鲸岛 · CG 演出回放]  MainWindowHandle=4000916
```

### 2.2 方式 B：跑 `bin\ucgr-gui.exe`

**注意**：`bin\ucgr-gui.exe` 若来自「在 desktop 里裸跑 go build」，则是坏的。本鲸鱼已用正确参数重建过它（见 2.4），重建后实测正常。

```powershell
.\bin\ucgr-gui.exe
```

### 2.3 方式 C：`wails build`（官方流程）

```powershell
# wails 不在 PATH，必须用全路径，或先临时加 PATH
$env:PATH = "D:\01_Dev\DevEnv\go\gopath\bin;" + $env:PATH

cd E:\Software_tools\DevSpace\Projects\Agent\Unified_CG_Replay_Library\desktop
wails build
```

产物：`desktop\build\bin\cgreplay-ui.exe`

**实测输出**（用 `-o` 换名以免覆盖现有成品）：

```
$ wails build -m -nosyncgomod -o cgreplay-ui-probe
Wails CLI v2.17.0
Build Mode         | production
Skip Frontend      | false
Output File        | cgreplay-ui-probe

# Building target: windows/amd64
  • Generating bindings: Done.
  • Installing frontend dependencies: Done.
  • Compiling frontend: Done.
  • Generating application assets: Done.
  • Compiling application: Done.
Built 'E:\...\desktop\build\bin\cgreplay-ui-probe' in 12.761s.
[exit=0]
```

> ⚠️ **实测踩坑**：`wails build` 会重新生成 `desktop\frontend\wailsjs\go\models.ts`（本次 diff：**+110 / −62 行**），把已入库的绑定文件改脏。本鲸鱼已 `git checkout --` 还原，`git status` 确认干净。
> 只想要可执行文件、不想动绑定：加 `-skipbindings`。
> 不想让它跑 `go mod tidy` 改 `go.mod`：加 `-m -nosyncgomod`。

### 2.4 方式 D：纯 `go build`（不加 wails CLI，但要带对参数）

`desktop\main.go` 第 9–12 行注释写的是「无需 wails CLI，直接 go build 即可」，但它给的命令 `go build -o ../bin/cgreplay-ui.exe .` **少了两个关键参数**，裸跑会得到 dev 模式坏包。正确写法：

```powershell
cd E:\Software_tools\DevSpace\Projects\Agent\Unified_CG_Replay_Library\desktop
go build -tags "desktop,production" -ldflags "-H windowsgui -w -s" -o ..\bin\ucgr-gui.exe .
```

**实测结果**：

| 命令 | exit | 产物大小 | PE 子系统 | 启动后窗口标题 |
| :--- | :-- | :--- | :--- | :--- |
| `go build -o ..\bin\ucgr-gui.exe .` | 0 | 13,915,648 | 3 = CUI（**有黑框**） | **`[Error]`**（白屏） |
| `go build -tags "desktop,production" -ldflags "-H windowsgui -w -s" ...` | 0 | 18,311,168 | **2 = GUI（无黑框）** | **`重返蓝鲸岛 · CG 演出回放`** ✅ |

两个参数各管一件事：

- `-tags production`：让 Wails 用 `//go:embed all:frontend/dist` 里嵌入的前端资源。**缺了它 = dev 模式**，程序会去连 `http://localhost:34115` 上的 Vite 开发服务器，连不上就显示 `[Error]` 页。
- `-ldflags "-H windowsgui"`：把 PE 头子系统从 CUI(3) 改成 GUI(2)，**黑框消失**。

### 2.5 方式 E：纯 CLI（GUI 坏了也能完整用）

```powershell
cd E:\Software_tools\DevSpace\Projects\Agent\Unified_CG_Replay_Library
go build -o bin\ucgr.exe ./cmd\ucgr         # 已验证 exit=0，7,778,304 字节
go build -o bin\cgreplay.exe ./cmd\cgreplay # 已验证 exit=0，6,928,896 字节
```

完整命令表来自 `bin\ucgr.exe`（无参实跑输出，**exit code = 2**）：

| 子命令 | 作用 | 实测 |
| :--- | :--- | :--- |
| `scan <目录>` | S0 体积盘点（按扩展名分类） | ✅ exit 0 |
| `probe <归档文件>` | 解析归档索引概况 | 未跑（缺样本） |
| `refs <解包目录> <脚本…>` | 提取引用并匹配资源池 | 未跑 |
| `prune <解包目录> <脚本…> [--out 目录]` | 裁剪保留集 | 未跑 |
| `ir <解包目录> <脚本> [--out file.json]` | 构建 IR | 未跑 |
| `replay <ir.json> [--assets] [--render] [--gif] [--at] [--frames] [--size] [--event]` | 回放 IR（默认打印时间轴） | ✅ exit 0 |
| `play <ir.json> --assets …` / `play --script <脚本>` | 开窗口实时播放 | ✅ 参数错误分支 exit 1 |
| `events <解包目录> [--out 库.json]` | 扫出原作事件表 | ✅ 错误分支 exit 1 |
| `decode <图片> [--out] [--raw]` | TLG5/JPEG→PNG | 未跑 |
| `unpack <归档> [--out] [--key] [--limit]` | 解包归档（CatSystem2 KIF） | 未跑 |
| `engines` | 列出已注册引擎 | ✅ exit 0 |
| `audio <音频> [--out] [--play-ms]` | 解码音频 | 未跑 |

`bin\ucgr.exe engines` 实测输出（8 个引擎）：

```
artemis
bgi
catsystem2
kirikiri
nexas
nitro
nscript
scn
```

`bin\ucgr.exe scan .\internal\ir` 实测输出：

```
盘点目录: .\internal\ir
总文件数: 3    总体积: 18.98 KiB    错误: 0
分类统计（按体积降序）:
  其它/系统                   18.98 KiB        3 个  100.0%
```

`bin\ucgr.exe replay reports\使用指南\demo\ir_SKAE_001r.json` 实测输出（节选，共 153 条台词）：

```
游戏 raw · 引擎 kirikiri · 保真度 generic · 事件 1 个

=== 事件 SKAE_001r（scene）图层 1 台词 153 ===
  [图层 image  background z=1 data/image/misc/eh_a09_01.png]
  --- 时间轴 ---
     400ms 【?】「啊……嗯，呵呵……。今天的健酱也很有精神呢～？」
     400ms 【?】「噗嘻嘻……我唯一拿得出手的优点也就是精力旺盛了嘛。」
     400ms 🖼 图层 image ← EH_A09_01
```

**纯 CLI 出图（不需要窗口）** —— 这是 GUI 不可用时的完整替代路径：

```powershell
# 离线渲染，不弹窗
.\bin\ucgr.exe replay <ir.json> --assets <素材根> --render <输出目录> --frames 3

# 或按目标时刻出帧（更适合截图存档）
.\bin\ucgr.exe play <ir.json> --assets <素材根> --shot <目录> --at 1000,3000
```

实测（用 `reports\使用指南\demo\ir_SKAE_001r.json` + `bin\cgreplay.conf` 里的素材根）：

```
=== 事件 SKAE_001r（scene）合成：3 帧，素材根 C:\Users\Administrator\AppData\Roaming\Trae CN\...\重返蓝鲸岛解包 ===
  t=     0ms  可见 0 层
⚠️ 2 帧因素材缺失跳过：...\eh_a09_05.png: The system cannot find the path specified.; ...\storage: The system cannot find the file specified.
PNG 帧已写入 C:\Users\Administrator\AppData\Local\Temp\ucgr-shot-probe（1 张）
[exit=0]
```

> 素材缺失是这台机器上那个解包目录的实际状态，不是程序问题。程序**如实报缺、不静默**。

---

## 3. GUI 状态：可用（工程完备）

### 3.1 逐项体检

| 检查项 | 结论 | 实测证据 |
| :--- | :--- | :--- |
| `wails.json` 存在？ | ✅ 在 | `desktop\wails.json`，19 行，`name=cgreplay-ui`、`outputDirectory=./build/bin`、`productVersion=0.4.0` |
| `frontend/package.json` 存在？ | ✅ 在 | `desktop\frontend\package.json`，React 18.3.1 / Vite 5.4.10 / TypeScript 5.6.3 / Tailwind 3.4.14 / spine-webgl 4.2.120 |
| frontend 依赖装了？ | ✅ 装了 | `desktop\frontend\node_modules\` 存在，含 `vite`、`react`、`tailwindcss`、`@esbuild/win32-x64` 等 150+ 包 |
| 前端构建产物在？ | ✅ 在 | `desktop\frontend\dist\index.html`（2,246 字节）+ `dist\assets\` |
| `wailsjs` 绑定在？ | ✅ 在 | `go\main\App.js`(4,336) / `go\main\App.d.ts`(2,932) / `go\models.ts`(24,050) / `runtime\runtime.js`(7,205) |
| `build\` 目录齐？ | ✅ 齐 | `build\appicon.png`、`build\windows\icon.ico`、`info.json`、`wails.exe.manifest`、`installer\{project.nsi, wails_tools.nsh}` |
| Wails CLI 在？ | ⚠️ 在但**不在 PATH** | `D:\01_Dev\DevEnv\go\gopath\bin\wails.exe`（36,823,552 字节），`wails version` → **`v2.17.0`**；裸调 `wails` → `术语 'wails' 不会被识别为 cmdlet、函数、脚本文件或可执行程序的名称。` |
| `wails doctor` 体检 | ✅ SUCCESS | `SUCCESS  Your system is ready for Wails development!` |
| WebView2 运行时 | ✅ 装了 | `157.0.4341.46`（注册表 `HKLM\SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-...}`） |
| Node / npm | ✅ 装了 | node `v24.19.0`、npm `11.17.0` |
| CGO 编译器（gcc） | ✅ 装了 | `gcc.exe (x86_64-posix-seh-rev1, Built by MinGW-Builds project) 16.2.0`；`CGO_ENABLED=1` |
| 根模块整体编译 | ✅ 通过 | `go build ./...` → exit 0；`go vet ./...` → exit 0 |
| `wails build` 全流程 | ✅ 通过 | 12.761s，全 5 个阶段 Done |

### 3.2 GUI 到底能干什么（代码实证）

`desktop\main.go`（129 行）关键片段：

```go
//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()
	err := wails.Run(&options.App{
		Title:     "重返蓝鲸岛 · CG 演出回放",
		Width:     1440,  Height:    860,
		MinWidth:  1024,  MinHeight: 640,
		AssetServer: &assetserver.Options{Assets: assets},
		OnStartup:   app.startup,
		DragAndDrop: &options.DragAndDrop{EnableFileDrop: true},
		Bind:        []interface{}{app},
	})
	if err != nil { log.Fatal(err) }
}
```

已注册的绑定方法（`App`）：

- `Version()` — 返回 `"ucgr replay-ui 0.4.0 · wails v2 · go core ready"`，前端启动时用它自检桥通路
- `ScanDirectory(dir)` — 对目录跑 S0 体积盘点
- `ReportClientError(msg)` — 前端异常写 `errors.log`
- 支持**把 galgame 目录直接拖进窗口**（`OnFileDrop` → 触发解包预览）

窗口配置：`WebviewIsTransparent: false`、`WindowIsTranslucent: false`、`DisableWindowIcon: false`。

### 3.3 「缺什么」清单 —— 实际只缺两样，都不是缺软件

| 缺的东西 | 现状 | 补齐命令 |
| :--- | :--- | :--- |
| `wails` 在 PATH 里 | 可执行文件在 `D:\01_Dev\DevEnv\go\gopath\bin\wails.exe`，但 PATH 未含该目录 | 临时（当前会话）：`$env:PATH = "D:\01_Dev\DevEnv\go\gopath\bin;" + $env:PATH`<br>永久：`[Environment]::SetEnvironmentVariable("PATH", [Environment]::GetEnvironmentVariable("PATH","User") + ";D:\01_Dev\DevEnv\go\gopath\bin", "User")` |
| 正确构建参数 | `desktop\main.go` 第 12 行注释给的 `go build -o ../bin/cgreplay-ui.exe .` 会产出 dev 模式坏包 | `go build -tags "desktop,production" -ldflags "-H windowsgui -w -s" -o ..\bin\ucgr-gui.exe .` |

**不需要装的**（`wails doctor` 判定为 Optional 且未安装，不影响构建与运行）：

- `upx`（压缩，省体积用）
- `nsis`（打 `.exe` 安装包用；已有历史安装包 `cgreplay-ui-amd64-installer.exe`，说明当时装过或用了别的机器）

**不需要补的**：WebView2 ✅、Node ✅、npm ✅、gcc ✅、Go ✅、字体 ✅、磁盘空间 ✅。

---

## 4. 首次运行前置条件清单

### 4.1 工具链（本机实测值，可直接对照）

| 项 | 本机实际 | 要求 | 判定 |
| :--- | :--- | :--- | :--- |
| Go | `go1.27.3 windows/amd64` | `go.mod` 声明 **1.27.2**（根）/ **1.27.2**（desktop） | ✅ 满足（1.27.3 ≥ 1.27.2） |
| `CGO_ENABLED` | `1` | 必须 1（Wails 与 GDI 走 cgo/系统 API） | ✅ |
| GOPATH | `D:\01_Dev\DevEnv\go\gopath` | 任意可写目录 | ✅ |
| GOMODCACHE | `D:\01_Dev\DevEnv\go\gopath\pkg\mod` | 需有 `modernc.org/sqlite`、`github.com/wailsapp/wails/v2` 缓存 | ✅ 已缓存（编译 12.7s 无下载） |
| gcc / MinGW | `16.2.0 (x86_64-posix-seh-rev1)` | cgo 需要 | ✅ |
| Node.js | `v24.19.0` | Vite 5 需 ≥ 18 | ✅ |
| npm | `11.17.0` | `wails.json` 里 `frontend:install` 就是 `npm install` | ✅ |
| pnpm | `10.34.5` | **不需要**（本项目用 npm，`wails.json` 未提 pnpm） | ✅ 有无都行 |
| Wails CLI | `v2.17.0` @ `D:\01_Dev\DevEnv\go\gopath\bin\wails.exe` | 与 `desktop\go.mod` 的 `wails/v2 v2.17.0` **版本一致** | ✅ 但需加 PATH |
| WebView2 Runtime | `157.0.4341.46` | GUI 运行必需 | ✅ |
| 系统 | Windows 11 专业版 24H2 (26100) | **Windows only** —— `docs\10_本地环境配置.md:9` 明确「CLI 与 GUI 目前只能在 Windows 上构建，因为它们依赖 `internal/platform/win32`」 | ✅ |
| 内存 | 16 GB | — | ✅ |
| 磁盘 E: | 剩余 34,644,811,776 字节（≈ 32.3 GiB） | 前端构建 + 产物约 200 MB | ✅ |

### 4.2 字体依赖（**关键，别忽略**）

GUI 和 CLI 走**两条完全不同的字体路径**：

**GUI（WebView2 + React）**：字体由 CSS 决定，渲染交给 WebView2，不读系统 GDI 字体表。只要 WebView2 装了就有字。

**CLI 播放器（GDI 直绘）**：字体解析链在 `cmd\ucgr\main.go:1204-1220`：

```go
// 字体：本作自带思源宋体 SC（OTF/CFF，且被单字节 XOR 混淆）。
face := fontName
ff := fontFile
if ff == "" {
    ff = filepath.Join(assets, "raw", "data", "SourceHanSerifSC-Regular.otf")
}
if n, ok := win32.RegisterFontFile(ff); ok {
    if face == "" { face = n }
    fmt.Printf("字体：私有注册 %s（家族 %s，使用 %s）\n", filepath.Base(ff), n, face)
} else {
    if face == "" { face = "SimSun" }        // ← 兜底
    fmt.Printf("字体：%s 加载失败，使用系统字体 %s\n", filepath.Base(ff), face)
}
td := win32.NewTextDrawer(win32.DefaultTextStyle(face, opt.FontSize))
```

落字靠 GDI `CreateFontW`（`internal\platform\win32\canvas.go:280`），**不是** `golang.org/x/image/font`（全仓库 grep 该依赖为 0 命中）。

字体前置条件：

| 字体 | 用途 | 本机 | 结论 |
| :--- | :--- | :--- | :--- |
| `SourceHanSerifSC-Regular.otf` | 游戏自带，从 `<素材根>\raw\data\` 私有注册（会先还原 XOR 混淆） | 取决于解包目录 | 缺了不致命，降级到 SimSun |
| `SimSun` = `C:\Windows\Fonts\simsun.ttc` | CLI 兜底字体 | ✅ `True` | 中文可显示 |
| `Microsoft YaHei` = `msyh.ttc` / `msyhbd.ttc` / `msyhl.ttc` | 可用 `--font Microsoft YaHei` 指定 | ✅ 三个都在（19.7 / 16.9 / 12.2 MB） | 可选用 |

**已知坑**：直接 `play --script` 时若素材根里没有那个 OTF，会在控制台打一行「字体：... 加载失败，使用系统字体 SimSun」，然后**照常出字**（不是报错，别当成失败）。想用别的字体就 `--font "Microsoft YaHei"` 或 `--font-file <某个.ttf/otf>`。

### 4.3 目录与写权限

| 位置 | 作用 | 本机状态 |
| :--- | :--- | :--- |
| `%LOCALAPPDATA%\cgreplay-ui\` | **主数据目录**：`conf/games.json/libraries/covers/exports/catalog.db/errors.log` 全在这里 | ✅ 已存在：`C:\Users\Administrator\AppData\Local\cgreplay-ui` |
| `%APPDATA%\cgreplay-ui\` | 回退目录（`LOCALAPPDATA` 为空时） | 未使用 |
| exe 同目录 | 再回退（拿不到用户目录时） | — |
| 素材根（解包目录） | 只读即可 | `bin\cgreplay.conf` 指向 `C:\Users\Administrator\AppData\Roaming\Trae CN\...\重返蓝鲸岛解包`，**实测存在 = True** |
| `<仓库>\bin\` | CLI 产物、演示 json/png | 可写，gitignored |

`desktop\app.go:129-144` 的实现（`dataDir()`）说明**为什么不用管理员权限**：

> 「优先 `%LOCALAPPDATA%\cgreplay-ui`，回退 `%APPDATA%\cgreplay-ui`，再回退 exe 同目录……装进 Program Files 后非管理员也能正常写。」

另外 `desktop\main.go:74` 在启动时做一次性迁移：把 v0.2 及以前落在 exe 同目录的数据搬进 `dataDir()`，**只搬不删、目标存在不覆盖**。

**结论**：不需要管理员权限，不需要预先建目录（程序自己 `MkdirAll`）。

---

## 5. 排错表（现象 → 原因 → 命令）

| 现象 | 原因 | 命令 / 处置 |
| :--- | :--- | :--- |
| **窗口一片白，标题栏显示 `[Error]`** | 在 `desktop\` 里裸跑 `go build`，缺 `-tags production` → 走 dev 模式去连 `localhost:34115` 的 Vite 服务器，没服务器就报错页 | 重编：`cd desktop; go build -tags "desktop,production" -ldflags "-H windowsgui -w -s" -o ..\bin\ucgr-gui.exe .`<br>或直接用现成成品 `.\desktop\build\bin\cgreplay-ui.exe` |
| **双击/运行 GUI 时多弹一个黑色控制台窗口** | PE 子系统是 CUI(3)，缺 `-ldflags "-H windowsgui"` | 同上重编。自查方法：跑本文 §6 的 PE 自检脚本，返回 2 就没黑框 |
| **`go build -o bin\ucgr-gui.exe .` 报 `no Go files in ...`** | 仓库根没有 `main.go`，GUI 入口在 `desktop\main.go`（独立模块） | `cd desktop` 再构建；或在根目录改用 `go build -o bin\ucgr.exe ./cmd\ucgr` |
| **`wails: 术语 'wails' 不会被识别为 cmdlet...`** | wails CLI 不在 PATH（文件在 `D:\01_Dev\DevEnv\go\gopath\bin\wails.exe`） | `$env:PATH = "D:\01_Dev\DevEnv\go\gopath\bin;" + $env:PATH`，或直接用全路径：`& "D:\01_Dev\DevEnv\go\gopath\bin\wails.exe" build` |
| **`wails build` 之后 `git status` 多出 `M desktop/frontend/wailsjs/go/models.ts`** | wails 会重新生成前端绑定，把已入库的 `wailsjs/` 改脏（本机实测 +110/−62 行） | 还原：`git checkout -- desktop/frontend/wailsjs/go/models.ts`；下次构建加 `-skipbindings` 避免 |
| **`wails build` 改动了 `go.mod` / `go.sum`** | wails 默认会跑 `go mod tidy` 并同步 go.mod | 构建时加 `-m -nosyncgomod` |
| **`wails build` 报 `nsis` / `upx` 相关错误** | 这两个是 Optional 依赖，打安装包 / 压缩时才要 | 加 `-nopackage` 跳过安装包；或 `-nopackage -nosyncgomod` 只出 exe |
| **前端编译失败（`tsc -b` 或 `vite build` 报错）** | `node_modules` 缺包或版本漂移 | `cd desktop\frontend; npm install; npm run build`。装不动就用 `wails build -s` 跳过前端构建、复用现成 `dist/` |
| **`npm install` 卡住/超时** | 网络 | 改用已装好的现状：`desktop\frontend\node_modules\` 已完整，加 `wails build -s` 直接跳过安装与构建 |
| **窗口能开但界面空白、`errors.log` 有内容** | 前端运行时异常（`ReportClientError` 写盘） | `Get-Content "$env:LOCALAPPDATA\cgreplay-ui\errors.log" -Tail 30` |
| **CLI 出图全是空的/缺图** | 素材根路径不对，或该目录没解包全 | 输出里会打 `⚠️ N 帧因素材缺失跳过：...`，照报的路径补素材；或 `.\bin\ucgr.exe events <解包目录>` 先确认能认出引擎 |
| **`ucgr play` 报 `错误: 用法: ucgr play <ir.json> --assets <素材根> ...`（exit 1）** | 参数不全。`play` 必须给 `<ir.json>` 或 `--script`，且要 `--assets` | 照提示补：`.\bin\ucgr.exe play <ir.json> --assets <素材根>` |
| **`ucgr events` 报 `没有找到可回放的剧本（.ks/.scn/.ast/.bs/.cst/...），也没认出打包签名`（exit 1）** | 该目录没有可识别剧本，或还没解包 | 这是**如实报错**不是 bug。指向真正的解包目录，或先 `ucgr unpack` |
| **`ucgr` 不带参数 / `ucgr help` 打印用法但 exit code = 2** | 设计如此（用法提示即非正常退出） | 脚本里别用 `exit code != 0` 判「命令不存在」，`engines` 等成功命令是 0 |
| **CLI 播放器中文显示成方块/乱码** | 兜底字体缺失，或 `--font` 指向了不存在的家族名 | 已确认 `simsun.ttc` 在本机存在；指定：`--font "Microsoft YaHei"`（`msyh.ttc` 已确认存在） |
| **CLI 播放器打「字体：... 加载失败，使用系统字体 SimSun」** | 素材根里没有 `raw\data\SourceHanSerifSC-Regular.otf` | **不是错误**，中文照常出。要消除提示就 `--font-file <某个字体文件>` |
| **GUI 打开后看不到自己的游戏库** | 配置文件 `conf` 里的 `root` 指错/失效 | 看 `%LOCALAPPDATA%\cgreplay-ui\conf`（老数据也可能在 `bin\cgreplay.conf`），字段形如 `{"root":"...","library":"...","auto":false}`；把 `root` 改成真实解包目录，或直接在 GUI 里重新选目录 / 把目录拖进窗口 |
| **`go build ./...` 通过但没生成任何 exe** | 正常。根模块没有 `package main`，`./...` 只做编译检查不落盘 | 要产物必须显式 `-o`，如 `go build -o bin\ucgr.exe ./cmd\ucgr` |
| **杀软报毒 / 隔离 `ucgr-gui.exe`** | Go 编译的无签名 GUI 程序，常见误报 | 加白名单；或用已有的 `desktop\build\bin\cgreplay-ui.exe` |

---

## 6. 附：PE 子系统自检脚本（判断某 exe 会不会弹黑框）

实测有效的判据 —— **子系统 2 = 无黑框，3 = 有黑框**：

```powershell
function Get-PESubsystem($path){
  $fs = [System.IO.File]::OpenRead((Resolve-Path $path)); $br = New-Object System.IO.BinaryReader($fs)
  $fs.Position = 0x3C; $peOff = $br.ReadInt32(); $fs.Position = $peOff; $sig = $br.ReadUInt32()
  if($sig -ne 0x00004550){ $fs.Close(); return "NOT_PE" }
  $fs.Position = $peOff + 0x5C; $sub = $br.ReadUInt16(); $fs.Close()
  if($sub -eq 2){ "2 = WINDOWS_GUI (无控制台黑框)" } elseif($sub -eq 3){ "3 = WINDOWS_CUI (带控制台黑框)" } else { "$sub" }
}
Get-PESubsystem ".\desktop\build\bin\cgreplay-ui.exe"
```

本机全部相关 exe 的实测结果：

| exe | 子系统 | 黑框 | 说明 |
| :--- | :--- | :--- | :--- |
| `desktop\build\bin\cgreplay-ui.exe` | **2 GUI** | 无 | ✅ 正式产物，可直接双击 |
| `bin\ucgr-gui.exe`（重建后） | **2 GUI** | 无 | ✅ 用 §2.4 正确参数重建 |
| `desktop\build\bin\cgreplay-ui-dev.exe` | 3 CUI | 有 | dev 版，带调试控制台，正常设计 |
| `bin\ucgr-gui.exe`（裸 `go build` 版） | 3 CUI | 有 | ❌ 已用正确参数覆盖重建 |
| `bin\cgreplay-ui.exe`（2026-10-21 旧包） | 3 CUI | 有 | ⚠️ 旧 dev 产物，别用 |
| `bin\ucgr.exe` / `bin\cgreplay.exe` | 3 CUI | 有 | ✅ CLI 本来就需要终端 |

---

## 7. 一句话总结

**有 GUI，而且能跑。** 想马上看到界面就双击 `desktop\build\bin\cgreplay-ui.exe`；想自己构建就 `cd desktop` 再 `wails build`（先加 PATH）或 `go build -tags "desktop,production" -ldflags "-H windowsgui"`。GUI 万一坏了，纯 CLI `bin\ucgr.exe` 的 13 个子命令 + 离线 `--render`/`--shot` 出图足以完整覆盖「解包 → 建 IR → 回放 → 导出」全链路，不依赖窗口、不依赖 WebView2。
