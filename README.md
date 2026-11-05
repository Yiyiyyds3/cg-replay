# 统一 CG 演出回放库（Unified CG Replay Library）

> 实现仓库 · 架构冻结版 v1.1 · 2026-10-17
> **技术栈：Wails v2（Go 后端 + 系统 WebView）+ Go 核心 CLI**（2026-10-17 由 Tauri+Rust 切换，AC-2026-1017-01）

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
├── desktop/                           # Wails v2 桌面壳（独立子模块）
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

桌面壳（Wails v2）：

```bash
cd desktop
go mod tidy        # 拉取 github.com/wailsapp/wails/v2（需网络）
wails build
```

> 下载源码后的本机路径怎么填，见 [docs/10_本地环境配置.md](docs/10_本地环境配置.md)：仓库不含任何本机路径，脚本与测试需要的目录通过环境变量或 `<...>` 占位符指定。

## 9nine 系列（PSB 剧本）适配

9nine 五部（九次九日 / 天色 / 新章 / 春色 / 雪色）没有可读的 KAG 脚本：剧情编译在 PSB 容器（`.scn`）里，CG 鉴赏表另外放在 `main/cglist.csv`。适配口径：

- 事件单位对齐原作自己：**场景事件** = 一个 `.scn` 里有台词的 scene，**CG 事件** = `cglist.csv` 的一行
- 回放台本从 PSB 值树还原（图层显示/切换、转场、台词、BGM/SE/语音），舞台 1280×720
- 三种解包布局（`raw/data/...`、`原始归档解包/...`、`_分类/...`）都能直接扫，不必手写 `ucgr-convention.json`

```bash
# 列出原作自己的事件表（CG + 场景）；--out 落盘给桌面壳用
bin/ucgr.exe events <9nine解包目录> [--out lib.json]

# 直接按某个 .scn 回放（--event/--size 等参数同 ir.json 回放）
bin/ucgr.exe play --script <xxx.txt.scn> --assets <9nine解包目录>
```

已知边界：

- **E-mote 立绘不复刻**（`.psb`/`.psz`/`.lpb`，硬约束 FR-19），只在事件里标记，画面用背景 + EV 图 + 对话框
- 部分解包产物本身损坏：0x20 混淆变体（整份文件异或 0x20，头写作 `psb `）里有一批 `.scn` 的名字表/字符串表对不齐。解析器会**跳过单个坏节点**（同层子节点各带独立偏移，坏一个不连累兄弟），整份无法解析的剧本则跳文件；两种情况都写进事件库 `warnings`，不会静默少事件
- `cglist.csv` 在春色/雪色/新章被单字节异或（键 0x13/0x45/0x10），读取时穷举 1..255 并用「像不像 CG 表」自校验

## 多引擎批量适配（CatSystem2 / Artemis / BGI / NScripter / NeXAS / Nitro+ / AliceSoft）

在 9nine（PSB/scn）之后，同一套「约定 + 事件库 + IR 回放」管线又接入了七个明文/半明文/字节码剧本引擎，注册表现共 9 个：`kirikiri / scn / artemis / bgi / catsystem2 / nscript / nexas / nitro / alice`。

- **事件粒度对齐原作**：CatSystem2 一个 `.cst` 一个场景事件（标题取首句对白）；Artemis 一份 `.ast` 一个事件；BGI 一份 `.bs` 按「换背景/换事件」切多个事件；scn 仍按 scene / cglist 行
- **剧本目录自动探测**：约定里 `scenario_dir` 留空时按扩展名递归收集（`.ast`/`.bs`/`.cst`），KiriKiri 按「scenario/scn/script 目录名优先、`.ks` 密度兜底」选层；scn 追加「解包整理/data_脚本与数据/scn」等整理布局
- **舞台尺寸自动推断**：取背景图（`bg*` 文件名或 `/bg/` 目录）尺寸众数；壁纸/特典/OST 目录不参与，推断不出就如实报「舞台未推断」
- **回放台本**：三引擎都补齐了「一句一屏」（`text_clear + text + text_wait`）与底部对话框（`ir.MessageBoxOps`），素材加载带整树文件名索引兜底
- **没解包的目录不再误报**：目录里没有可回放剧本但有原作打包签名（`.xp3`/`.pfs`/`.int`/`BGI.hvl`…）时，直接报「这层只有打包文件（…，属 X 引擎），还没解包」，而不是含糊的「可能不是 KiriKiri 引擎」

```bash
# 任一引擎的解包目录：扫出原作自己的事件表
bin/ucgr.exe events <解包目录> [--out lib.json]

# 直接按剧本回放（按扩展名/文件名分派 .scn/.ast/.cst/.bs/.ks/nscript.dat/Script.pac/nss.npa/.ain）
bin/ucgr.exe play --script <xxx.cst> --assets <解包目录> [--shot 目录 --at ms]
```

实测抽样（本机解包目录）：

| 游戏 | 引擎 | 舞台 | 事件 |
|---|---|---|---|
| 樱之刻 | artemis | 1920×1080 | 535 |
| 甜蜜女友3 | artemis | 1920×1080 | 1300 |
| 果实（灰色的果实） | catsystem2 | 1024×576 | 476 |
| 甜蜜女友2 | catsystem2 | 1280×720 | 7860 |
| 樱之诗 | bgi | 1280×751 | 5042 |
| 秽翼的尤斯蒂娅 | bgi | 1280×720 | 401 |
| 灰色的乐园FHD | catsystem2 | 未推断（素材未解出） | 228 |
| 寒蝉鸣泣之时·解题篇 | nscript | 640×480 | 5636 |
| 海猫鸣泣之时 出题篇 | nscript | 640×480 | 8270 |
| 海猫鸣泣之时 散 | nscript | 640×480 | 10326 |
| Baldr Sky（Steam 中文版） | nexas | 1024×768 | 203 |
| BALDRSKY DIVEX | nexas | 1024×768 | 141 |
| 鬼哭街重制版 | nitro | 1024×768 | 61 |
| 兰斯10 | alice | 1920×1080 | 7256 |

**覆盖进度（2026-11-05）**：9 个引擎适配器覆盖 33 款 / 258.72 GB（82.7%）。
NScripter 适配器要点：`nscript.dat` = XOR 0x84 的 CP932 明文（密文/明文关键字
嗅探，寒蝉解的剧本解出来以空行开头）；NSA 归档大端索引，压缩 0=无 / 1=SPB /
2=LZSS / 3=NBZ(bzip2)，SPB 解出 24bpp BMP（蛇形行序）；`bg sora` 这类裸名是
不带 `$` 的 stralias（07th 惯例）；图片全在归档里的游戏（海猫出题）由
GameLoader 的 NSA 兜底池取图。
NeXAS 适配器要点：PAC 索引在尾部、逐字节取反后 NeXAS-Huffman 解码，数据区
zlib/LZSS/直存按打包版本区分（Baldr Sky Steam 打包器尾部不补位，残缺条目
zlib 自描述兜底）；.bin 脚本 Steam/DIVEX 两种头变体；文本逐段按平假名率
判 GBK/CP932，条目名纯 Go CP932 合法性投票；图片全在 Bmp/Stand/System.pac
（PNG/BMP 直读），由 GameLoader 的 PAC 兜底池取图。
Nitro+ 适配器要点：NPA 归档（名字/内容双解密）+ NSS 明文脚本；名字解密的
index 是含目录条目的循环序号；内容 key 按明文名派生
`((nameKey - Σ字节) * len + arcKey) * unpacked & 0xff`（Garbro 的 RawName
公式只覆盖部分条目，明文名优先 + 魔数校验兜底，全 4 包 3354 条目 100%）；
汉化覆盖包 nss-1.npa（GBK）同格兼容、按平假名率自动分流；图片由
GameLoader 的 NPA 兜底池取图。
AliceSoft（System 4）适配器要点：`Rance10.ain` = AI2/zlib 包裹的区段流脚本
（v12：FUNC 37698 函数 / MSG1 269676 条加密消息，`b[i]-=i+0x60`），函数名含
CJK 的就是原作场景函数（「萨娜琪亚／参战」），反汇编 S_PUSH+MSG 切事件；
图片全在 `Rance10CG*.afa`（AJP=JPEG+XOR 掩码、QNT=双布局+2×2 重排+减法预测、
DCF=16×16 差分块合成，底图链式递归、悬空降级直解），由 GameLoader 的 AFA
兜底池取图。未适配 14 款 / 54.29 GB：Unity 3、FAVORITE 2、其余单件 9。

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
