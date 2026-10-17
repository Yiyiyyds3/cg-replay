# 统一 CG 演出回放库（Unified CG Replay Library）

> 实现仓库 · 架构冻结版 v1.1 · 2026-10-17
> **技术栈：Wails v3（Go 后端 + 系统 WebView）+ Go 核心 CLI**（2026-10-17 由 Tauri+Rust 切换，AC-2026-1017-01）

## 这是什么

把已解包的 galgame 资源**裁剪**成"只够重演 CG 事件"的最小集合，装进**一套自研回放应用**，从而在**完整重温 CG 演出**的同时**大幅降低硬盘占用**。

本仓库包含：**七份设计文档 + 三份机器可读 schema + Go 核心实现（骨架已跑通）+ 实测报告存档**。上游《FINAL_统一CG演出回放库_架构冻结与执行提示词1017.md》第 3 节的七轮 drill 结论**已冻结**，本文档集只做细化，不推翻决策。

## 目录结构

```
Unified_CG_Replay_Library/
├── README.md                          ← 本文件（索引）
├── go.mod / go.sum                    # Go 核心模块（module: ucgr）
├── cmd/
│   └── ucgr/
│       └── main.go                    # 核心 CLI 入口
├── internal/
│   ├── ir/ir.go                       # IR 类型 + V1~V8 校验（docs/03 的 Go 化）
│   ├── scan/scan.go                   # S0 体积盘点（goroutine 并发遍历）
│   ├── prune/prune.go                 # 引用裁剪器（匹配/保守保留/BGM 全留/复制）
│   └── adapter/
│       ├── adapter.go                 # EngineAdapter 五方法接口 + 注册表
│       └── kirikiri/kirikiri.go       # KiriKiri 适配器（XP3 解析 + 脚本解码 + 引用提取）
├── desktop/                           # Wails v3 桌面壳（独立子模块）
│   ├── go.mod                         # replace 指向根模块；就绪后 go mod tidy + wails build
│   ├── main.go / wails.json
│   └── frontend/                      # Vite 前端骨架
├── docs/
│   ├── 01_需求规格说明书.md            # §13-1：含第 3 节全部冻结决策（DR-1 ~ DR-38）
│   ├── 02_数据模型与包格式规范.md      # §13-2：包 / 中心库 / 冷归档，字段级定义 + SQL DDL
│   ├── 03_IR规范.md                    # §13-3：图层 / 时间轴 / 转场 / 文本 / 语音 / 分支，字段级
│   ├── 04_引擎适配器接口规范.md        # §13-4：五方法契约 + 转场注册表 + 回退规则
│   ├── 05_流水线设计.md                # §13-5：M0 / P0 / P1 / P2 任务拆分与验收 + §7.5 剪枝
│   ├── 06_客户端功能与交互设计.md      # §13-6：检索 / 回放 / 调试 / 导出 / 收藏 / 后台任务
│   └── 07_里程碑与验收标准.md          # §13-7：门禁、指标、风险台账、禁忌复核
├── schema/
│   ├── ir.schema.json                 # IR 的 JSON Schema（与 03 文档同源）
│   ├── package.schema.json            # package.json 的 JSON Schema（与 02 文档同源）
│   └── refs.schema.json               # refs.json 的 JSON Schema（与 02 文档同源）
├── reports/                           # 实测报告存档（Python 原型阶段的结论）
│   ├── 引擎分布复核报告_2026-10-15.md  # 47 款引擎分布（删旧目录后重测）
│   ├── M0_清点报告_v2a.md              # 四类体积（本体 2.37 / 归档 171.68 / 解包 313.01 GB）
│   ├── P0_原型报告_KiriKiri引用提取.md # KiriKiri 11 款：引用率 29.5%
│   ├── P2_原型报告_CatSystem2引用提取.md # CS2 8 款：保留率 54.9%
│   └── P2_原型报告_Artemis引用提取.md  # Artemis 3 款：保留率 58.6%
├── m0/                                # M0 清点数据与工具
│   ├── M0_清点报告_v2a.md
│   ├── m0_census.json                 # 47 款逐游戏测量数据
│   └── m0_census.py                   # 测量脚本（可复跑）
└── tools/                             # Python 原型脚本存档（已被 Go 核心接替，见 tools/README.md）
```

## 快速开始（Go 核心）

```bash
# 构建 CLI
go build -o bin/ucgr.exe ./cmd/ucgr

# S0 体积盘点（按扩展名分类统计）
bin/ucgr.exe scan <解包目录>

# 解析 XP3 索引
bin/ucgr probe <xxx.xp3>

# 提取 .ks 脚本的资源引用并匹配解包池
bin/ucgr refs <解包目录> <脚本|目录...>

# 引用裁剪：保留集复制到目标目录
bin/ucgr prune <解包目录> <脚本|目录...> --out <目标目录>

# 已注册引擎
bin/ucgr engines
```

桌面壳（Wails v3）：

```bash
cd desktop
go mod tidy        # 拉取 github.com/wailsapp/wails/v3（需网络）
wails build
```

## 已验证的实现基线（2026-10-17）

Go 核心 CLI 与 Python 原型在真实数据上逐款对齐：

| 游戏 | Go 保留率 | Python 保留率 | 说明 |
|---|---:|---:|---|
| 9nine春色 | 36.5% | 36.6% | 一致 |
| 枯花 | 16.7% | 16.7% | 一致 |
| 夏空彼方 | 19.5% | 11.4% | Go 更保守（同名多版本全留，多留两个 OP 视频副本 145 MiB） |

XP3 解析与 Python 参考实现逐条目一致（9nine新章 data.xp3：1346 条目）。

## 推荐阅读顺序

| 角色 | 阅读顺序 |
|---|---|
| 第一次接触本项目 | README → docs/01（需求与冻结决策）→ docs/07（里程碑） |
| 实现摄入 / 裁剪 | 01 → 02 → 03 → 04 → 05 |
| 实现客户端 | 01 → 02 → 06 → 07 |
| 实现回放器 | 03 → 04（转场注册表）→ 06 |

## 六条硬约束（摘自上游文档第 14 节「禁忌」，全文见 07 文档）

1. ❌ 禁止擅自删除原资源；一切删除须经**待删区 + 保留期**或明确的 per-game 开关
2. ❌ 禁止在 M0 清点前承诺具体降幅
3. ❌ 禁止过度设计 —— **不做通用引擎模拟器**，只做 IR 回放
4. ❌ 禁止有损转码
5. ❌ 禁止把库锁死在单一网盘
6. ❌ 禁止把「内嵌索引」做成「资源冗余」（重复的只能是 KB~MB 级索引）

## 变更管理

第 3 节决策为**架构冻结**状态。任何对 DR-1 ~ DR-38 的修改必须走**架构变更单**（docs/07 §7），评审通过后同步更新全部受影响文档与 schema，并在 docs/07 留痕。未经变更单，实现方不得自行放宽硬约束。
