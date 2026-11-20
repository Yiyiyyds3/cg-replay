# 17 · UCGR 使用手册（从打开到看见画面）

> 本文所有命令都在仓库根目录 `E:\Software_tools\DevSpace\Projects\Agent\Unified_CG_Replay_Library` 的 PowerShell 里跑。
> 事实来源：本次实机验证（[01_实机验证日志.md](../reports/使用指南/01_实机验证日志.md)）、[02_启动与GUI.md](../reports/使用指南/02_启动与GUI.md)、[00_功能地图.md](../reports/使用指南/00_功能地图.md)、[03_常见问题与排错.md](../reports/使用指南/03_常见问题与排错.md)。标「未实测」的就是这次没真跑过的，别当结论。

---

## 0. 三十秒版本

这软件干的事：**把已经解包好的 galgame 资源，按原作自己的分割方式（CG 鉴赏槽位 / 编号剧本）扫成事件库，再用自己的回放器逐帧重演演出**（图层 + 转场 + 对话框文字 + 语音），并能导成 GIF / MP4 / 静帧。

四个入口，按目的挑：

| 你想干什么 | 用哪个 | 一句话 |
| --- | --- | --- |
| 打开库、翻事件、点开就播、导出 GIF/MP4 | 桌面 GUI `desktop\build\bin\cgreplay-ui.exe` | 双击即用，不敲命令 |
| 只播单个游戏、轻量 | 单机回放器 `bin\cgreplay.exe` | 也双击，窗口更小 |
| 批量、脚本化、出帧出 GIF | CLI `bin\ucgr.exe` | 一条命令一件事 |
| 验收 pass 游戏画面对不对 | 抽检画廊 `out\review\index.html` | 浏览器里翻图、按键打分 |

---

## 1. 它到底能干什么

| 能力 | 入口 | 产物 | 本次实测 |
| --- | --- | --- | --- |
| 体积盘点（按扩展名分类） | `ucgr scan <目录>` | 终端报告，不写文件 | ✅ |
| 归档索引概况 | `ucgr probe <归档>` | 终端概况 | ⏸ 缺样本未测 |
| 归档解包（CatSystem2 KIF） | `ucgr unpack <归档> --out <目录> [--key 口令]` | 解包目录 | ⏸ 未测 |
| 引用提取 / 裁剪保留集 | `ucgr refs` / `ucgr prune` | 引用表 / 最小资源集 | ⏸ 未测 |
| 剧本压成 IR | `ucgr ir <解包目录> <脚本绝对路径> --out ir.json` | ir.json（V1~V8 校验） | ✅ |
| IR 回放（时间轴 / 出帧 / GIF） | `ucgr replay ir.json [--assets] [--render 目录] [--gif x.gif] [--at ms,…]` | 时间轴文本 / PNG 帧 / GIF | ✅（GIF 未测） |
| 窗口实时播放 | `ucgr play <ir.json> --assets <素材根>` | 窗口演出 | ⏸ 未测 |
| 扫原作事件表 | `ucgr events <解包目录> --out lib.json` | 事件库 JSON（schema 2） | ✅（仅 kirikiri） |
| 音频解码 | `ucgr audio <音频> [--out out.pcm]` | PCM / 格式打印 | ⏸ 未测 |
| 桌面库：多游戏登记 / 播放 / 导出 | `cgreplay-ui.exe` | GIF/MP4/截图/catalog.db | ✅ 启动通过 |

**它不干的事**（省得瞎试）：不负责给加密图「破解」——`ucgr decode` 当前只吃 TLG5 / JPEG / HG3，**`.rpgmvp` 和明文 PNG 都会报「非 TLG/JPEG/HG3 文件」**（实测）；Unity 与 RPG Maker MZ 也没接进 CLI 的 `events`。

---

## 2. 入口 A：桌面 GUI（先试这个）

### 2.1 启动

```powershell
cd E:\Software_tools\DevSpace\Projects\Agent\Unified_CG_Replay_Library
.\desktop\build\bin\cgreplay-ui.exe
```

**预期现象**（实测）：弹出 1440×860 窗口，标题「重返蓝鲸岛 · CG 演出回放」，**无黑框**，进程存活；数据目录自动建在 `%LOCALAPPDATA%\cgreplay-ui`（含 `catalog.db`、`libraries\<id>.json`、`covers\`）。

等价入口：`bin\ucgr-gui.exe`（18,311,168 字节那份才是好的，小的那份是 dev 模式坏包，白屏 + 标题 `[Error]`）。

> 别在 `desktop\` 里裸跑 `go build`：那样出来的是 dev 模式坏包。要重建照 §6 的参数。

### 2.2 五分钟流程

1. **加游戏**：点「添加游戏 / AddGame」，选**解包后的游戏目录**（不是 `.xp3` 压缩包）；也可把归档直接拖进窗口，会走 `UnpackPreview → UnpackImport` 解包预览再导入。
2. **等自动扫描**：登记后扫出事件库，落 `libraries\<id>.json`；库大要等，重扫按 Rescan。
3. **看事件列表**：左侧列表双击（或回车）即播。CG 事件来自原作 CG 鉴赏槽位（`*cgNN`），场景事件来自编号剧本。
4. **播放**：空格/回车推进，`P` 暂停，`Tab` 加速，`S` 跳过等待，`F12` 截图，`Esc` 退出。
5. **导出**：事件上选「导出 GIF / MP4」，异步任务中心跑、可取消。GIF = 10fps / 480×360 / 最长 15s；MP4 = 25fps / 960×720 / 最长 60s，走本机 ffmpeg，**无音轨**。
6. **收藏 / 评分 / 备注 / 角色 / 搜索 / 最近**：都落 `catalog.db`（SQLite），删了就等于清库。

### 2.3 GUI 三件事

- 白屏 / 标题 `[Error]` → 拿到 dev 包了，换 §2.1 的 exe 或按 §6 重建。
- 双击没反应 → 缺 WebView2 运行时（安装包 `desktop\build\bin\cgreplay-ui-amd64-installer.exe` 内带 `MicrosoftEdgeWebview2Setup.exe`）。
- 加了目录事件列表空 → 见 §8 第 3、4 条。

---

## 3. 入口 B：单机版 `bin\cgreplay.exe`

双击即用，Win32 原生窗口、单游戏。左侧事件列表双击/回车即播，「选择游戏目录…」选解包目录，有上一个/下一个/自动推进/F12 截图。配置在 `bin\cgreplay.conf`，事件清单 `bin\cgreplay-lib.json`（这两个文件本机已存在，说明跑过）；`--selftest` 自检出图，`bin\selftest.png` / `selftest-thumb.png` 就是它的产物。

适合：只想快速看某一款，懒得开大窗口。

---

## 4. 入口 C：CLI 全流程

### 4.1 构建 + 探活

```powershell
go build -o bin\ucgr.exe ./cmd\ucgr
.\bin\ucgr.exe engines      # 探活：8 个引擎 id，exit 0
```

实测输出：`artemis bgi catsystem2 kirikiri nexas nitro nscript scn`。
`ucgr --help` 会把用法印全，但**故意 exit 2**，别拿它当健康检查。

### 4.2 全流程（每步标实测结果）

```powershell
# 1) 盘点体积：不认引擎，任何目录都能跑
.\bin\ucgr.exe scan "D:\UCGR_Work\unitycg\1692614"                                  # ✅ exit 0

# 2) 解包（CatSystem2 KIF；其它引擎用各自工具或 GUI 拖放）
.\bin\ucgr.exe unpack "X:\path\game.int" --out "D:\UCGR_Work\unpacked\game"        # ⏸ 未实测

# 3) 扫事件表 —— 只对 kirikiri 类解包目录有效
.\bin\ucgr.exe events "D:\UCGR_Work\unpacked\SKAE" --out "reports\库\SKAE.json"    # ✅ 出 11 场景

# 4) 选一个脚本压成 IR（脚本必须【绝对路径】）
.\bin\ucgr.exe ir "D:\UCGR_Work\unpacked\SKAE" "D:\UCGR_Work\unpacked\SKAE\data\scenario\SKAE_001r.ks" --out "reports\库\ir_SKAE_001r.json"   # ✅ V1~V8 通过

# 5) 回放：只看时间轴（不弹窗）
.\bin\ucgr.exe replay "reports\库\ir_SKAE_001r.json"                                 # ✅ exit 0

# 6) 离线出帧（合成画面，不弹窗）
.\bin\ucgr.exe replay "reports\库\ir_SKAE_001r.json" --assets "D:\UCGR_Work\unpacked\SKAE" --render "reports\使用指南\demo\frames" --at 1000,2000   # ✅ 出 2 张 1280×720 PNG

# 7) 出 GIF
.\bin\ucgr.exe replay "reports\库\ir_SKAE_001r.json" --assets "D:\UCGR_Work\unpacked\SKAE" --gif "reports\库\SKAE.gif"   # ⏸ 未实测

# 窗口实时播放（空格推进 / P 暂停 / Tab 加速 / S 跳等待 / F12 截图 / Esc 退出）
.\bin\ucgr.exe play "reports\库\ir_SKAE_001r.json" --assets "D:\UCGR_Work\unpacked\SKAE"                                  # ⏸ 未实测
```

已实测走通的链路：**scan（可选）→ events → ir → replay（时间轴 / `--render` 出帧）→ 要动图再加 `--gif`**。

### 4.3 CLI 的三个硬坑（实测踩出来的）

| 坑 | 现象 | 正确做法 |
| --- | --- | --- |
| `events` 不认 unity / rmmz | `错误: 未知引擎 "unity"` / `没有找到可回放的剧本` | 这两类走桌面端或 library 层，CLI 别试 |
| `ir` 的脚本参数按当前目录解析 | `错误: 读不到脚本 data/scenario/xxx.ks` | 传**绝对路径** |
| `replay` 吃 events.json | `IR 校验未过` + `事件 0 个`，但 **exit 0** | 只喂 `ir` 生成的 ir.json；exit 0 ≠ 成功 |

---

## 5. 支持哪些引擎 / 数据类型

- **CLI 引擎适配器（8 个）**：artemis、bgi、catsystem2、kirikiri、nexas、nitro、nscript、scn。
- **rmmz（RPG Maker MZ）**：回放读源目录 `www\data` 的 JSON + `www\img`，没有独立导出物；加密图（`.rpgmvp` / `.png_`）当前看不了，要先解出明文。
- **Unity CG**：先用 `tools/unity_cg_export.py` 导出成 `D:\UCGR_Work\unitycg\<游戏名>\cg\` + `cg-index.json`，桌面端/library 层消费；CLI `events` 不支持。
- **spike**：只证明管线通，覆盖面很小。

判断一款游戏属于哪类：看 `reports\人工抽检\pass清单.json` 的 `engine` 字段，或找目录里的 `www\data`（rmmz）、`cg-index.json`（unity 导出）、`.ks/.scn`（kirikiri 系）。

---

## 6. 自己重建 GUI

```powershell
# 官方流程（wails 不在这台机 PATH 里）
$env:PATH = "D:\01_Dev\DevEnv\go\gopath\bin;" + $env:PATH
cd E:\Software_tools\DevSpace\Projects\Agent\Unified_CG_Replay_Library\desktop
wails build                       # 实测 12.761s 通过
# 不想让它改脏 wailsjs 绑定和 go.mod：
wails build -skipbindings -m -nosyncgomod

# 不装 wails 也能建，但参数必须带全，否则是 dev 坏包：
go build -tags "desktop,production" -ldflags "-H windowsgui -w -s" -o ..\bin\ucgr-gui.exe .
```

对照实测：`go build -o ..\bin\ucgr-gui.exe .` → PE 子系统 3（有黑框）、标题 `[Error]`；带上上面的 tags/ldflags → PE 子系统 2（无黑框）、标题「重返蓝鲸岛 · CG 演出回放」。

---

## 7. 目录与产物地图

```
仓库
├─ cmd/ucgr/            CLI（scan/probe/refs/prune/ir/replay/play/events/decode/unpack/engines/audio）
├─ cmd/cgreplay/        单机回放器 → bin/cgreplay.exe
├─ cmd/library-audit/   游戏库批量审计 → reports/audit/*.json
├─ desktop/             Wails GUI（React 前端）→ desktop/build/bin/cgreplay-ui.exe
├─ internal/            适配器 / IR / 裁剪 / 解码 / 回放 / 播放器 / library
├─ tools/               unity_cg_export.py、review-gallery、review-prescreen
├─ bin/                 可执行文件 + 运行期配置（cgreplay.conf / cgreplay-lib.json / covers/）
├─ out/review/          抽检画廊 index.html
├─ reports/             audit/、人工抽检/、使用指南/、各类验收报告
└─ docs/                01~17 设计与手册
```

数据三处落点：**解包目录**（源素材）、**库目录**（ir.json / 事件库）、**`%LOCALAPPDATA%\cgreplay-ui`**（catalog.db + libraries + covers，删掉=清库，可重建）。

---

## 8. 故障速查（详细版见 [03_常见问题与排错.md](../reports/使用指南/03_常见问题与排错.md)）

| 现象 | 先看这条 |
| --- | --- |
| 加密图全是「不可渲染」占位 | rmmz `.rpgmvp` 要先解明文；`ucgr decode` 目前不支持 |
| `events` 报「未知引擎 unity」 | 正常限制，Unity 走桌面端 / library |
| `events` 报「没有找到可回放的剧本」 | 目录不是 kirikiri 类，换引擎或换工具 |
| `ir` 报「读不到脚本」 | 改成脚本的绝对路径 |
| `replay` 出 0 个事件却 exit 0 | 喂错文件，要 ir.json 不是 events.json |
| 回放白屏 / 全黑 | `--assets` 素材根不对，或 IR 与素材不匹配 |
| 对话文字不显示 / 变宋体 | 字体没注册，见 03 §4.3 |
| 路径含空格 / 中文 / 日文出错 | 一律加双引号；超 260 见 03 §3.5 |
| `go build` 报 gcc / CGO | 见 03 §3.2 |
| 画廊只显示 200/715 张 | 单款抽样上限，等步长抽样，不是截断尾部 |

---

## 9. 顺带：抽检画廊（验画面，不是回放）

```powershell
go run ./tools/review-gallery            # 生成 → out\review\index.html（5.7s）
start out\review\index.html             # 浏览器里翻 53 款 pass
start reports\人工抽检\预筛可疑.md       # 机器预筛线索（rmmz 版：预筛可疑_rmmz.md）
```

里面：左侧 53 个锚点跳转、顶部四个筛选（导出产物 16 / rmmz 源目录 37 / 大款 20 / 预筛⚠ 41）、每款带 severity 线索条；按键 `1` 好 / `2` 小问题 / `3` 严重 / `4` 待查，右上「导出 CSV」→ 覆盖 `reports\人工抽检\打分表.csv`。一页判定清单见 [00_如何开始.md](../reports/人工抽检/00_如何开始.md)。

---

## 10. 一页速查卡

```powershell
# ── GUI ──
.\desktop\build\bin\cgreplay-ui.exe        # 多游戏库；拖入目录即导入；导出 GIF/MP4
.\bin\cgreplay.exe                          # 单机版；双击事件即播

# ── CLI ──
go build -o bin\ucgr.exe ./cmd\ucgr
.\bin\ucgr.exe engines                                              # 探活：8 个引擎
.\bin\ucgr.exe scan   "<目录>"                                       # 体积盘点
.\bin\ucgr.exe events "<解包目录>" --out lib.json                    # 事件库（kirikiri 系）
.\bin\ucgr.exe ir     "<解包目录>" "<脚本绝对路径>" --out ir.json     # IR
.\bin\ucgr.exe replay ir.json                                       # 时间轴
.\bin\ucgr.exe replay ir.json --assets "<素材根>" --render frames\ --at 1000,2000   # 出帧
.\bin\ucgr.exe replay ir.json --assets "<素材根>" --gif out.gif                     # 出 GIF
.\bin\ucgr.exe play   ir.json --assets "<素材根>"                                   # 窗口播

# ── 验收画廊 ──
go run ./tools/review-gallery; start out\review\index.html
```

**记三条就够**：① 想看 → 双击 `cgreplay-ui.exe`，加目录、双击事件；② 想批量 → `events → ir → replay`，脚本给绝对路径；③ 想验画面对不对 → 画廊按 1~4 打分，导出 CSV 回填打分表。
