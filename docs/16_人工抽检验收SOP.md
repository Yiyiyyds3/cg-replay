# 16 pass 游戏人工抽检 SOP（53 款）

> 配套打分表：`reports/人工抽检/打分表.csv`（53 行，name/cg_count/batch 已按 `reports/audit/**/*.json` 的 pass 判定预填）。
> 问题款回流：`reports/人工抽检/问题款清单.md`（不存在则新建，格式见 §7）。
> 上游口径：docs/15 `status/已验收` 推算 53 款，与本表 53 行一一对应。
> 机器预筛：`reports/人工抽检/预筛可疑.md`（Unity 16 款已生成；rmmz 版 `预筛可疑_rmmz.md` 按 §3 命令补跑，见 §3）。

## 0. pass 是什么口径（为什么必须人工看）

工具层的 pass = 「管线跑通并产出可见图」：扫描/解包成功、`cg-index.json` 或事件索引生成、导出了能显示的 PNG/帧。
它**不等于**画质与完整性达标——工具只验证「管子通」，验证不了「图对」：裁切错位、半张图、纯黑纯白空帧、重复帧、透明通道全黑、整体偏色，这些全是只有人眼翻图才能发现的问题。所以 53 款 pass 全部要人工过一遍再下最终判定。

53 款里两类 pass 的**证据等级不同**，看图姿势也不同，先读 §2 再开工。

## 1. 快速入口（PowerShell，直接复制）

抽检画布 = 画廊生成器产出的自包含页面 `out/review/index.html`（53 款、判定写 localStorage、可导出 CSV）。生成命令与页面说明见 `tools/review-gallery/README.md`。

```powershell
$repo = "E:\Software_tools\DevSpace\Projects\Agent\Unified_CG_Replay_Library"

# 1) 打开抽检画廊（判定画布，判定写进 localStorage）
start "" "$repo\out\review\index.html"

# 2) 重建画廊（清单或导出物更新过之后跑；默认读 reports\人工抽检\pass清单.json）
go build -o tools/review-gallery/review-gallery.exe ./tools/review-gallery
go run ./tools/review-gallery

# 3) 打开机器预筛报告（§3 的排序依据）
start "" "$repo\reports\人工抽检\预筛可疑.md"

# 4) 打开打分表
start "" "$repo\reports\人工抽检\打分表.csv"

# 5) 打开某款 Unity 导出目录示例（cg\ + cg-index.json；1692614 是 spike 款）
start "" "D:\UCGR_Work\unitycg\1692614"
```

## 2. 两类 pass 怎么分别看（Unity/spike 有导出产物，rmmz 没有）

53 款 = **Unity 导出 15 + spike 1 + rmmz 37**。两类 pass 的含义不同，判定动作必须分开：

### 2.1 Unity 导出 + spike（16 款）——有一级证据，看导出图

- pass 的含义：`tools/unity_cg_export.py` 已把 CG 导出成 `D:\UCGR_Work\unitycg\<游戏名>\cg\` 下的明文 PNG，并生成 `cg-index.json`。
- gallery 里每款标题条挂的 **cg-index 绝对路径就是导出目录**，缩略图直接来自导出图——看到的即导出效果，判定以它为准。
- spike 款（1692614 events=60、Last Town events=1）同属此类：有真实导出产物，只是覆盖率低，按全量看、判定从严。
- 辅助核对：先开 `cg-index.json` 对 `counts.cg`；spine 款核对 `.json + .atlas + .png` 三件套数量一致。

### 2.2 rmmz（37 款）——无导出产物，pass = 扫描 + 回放读源目录

- pass 的含义：扫描器读通 `www/data` 事件 JSON、事件索引生成、回放链路能按事件解析出素材引用；**没有导出 PNG**，清单里 `cg_index_dir` 为空。
- gallery 里这类款标「**rmmz 源目录预览**」：缩略图来自 `source_dir` 下的 `www/img` 或 `img`（扫描根按 `img\pictures`、`www\img\pictures`、`img\cg`、`img`、`www\img` 顺序探测）。
- **源目录图只能验证可读性/路径正确，不等于导出效果**——游戏实际运行时的合成结果还要靠回放抽验兜底。
- rmmz 抽检盯四件事：
  1. **差分是否错位**：rmmz 画面是立绘/差分**叠加合成帧**，重点看叠加后相对位置、遮挡关系是否合理；
  2. **透明通道**：半透明边缘是否平滑，有没有全黑背景/alpha 丢失；
  3. **命名族是否缺族**：缩略图命中的是 `img/pictures` 命名族，按文件名前缀+序号查连续性（§5 的跳号命令把路径换成 `img/pictures` 即可）；
  4. **事件引用的图是否都存在**：抽几款用回放链路对事件→图映射跑一遍，引用路径解析不到图的记问题款。
- 回放抽验命令（素材根指到游戏 `www` 目录，`<ir.json>` 为该款事件索引产物）：

```powershell
go run ./cmd/ucgr replay "<该款 ir.json 路径>" --assets "<游戏根>\www" --render "$repo\out\replay\<游戏名>"
```

- 另外 21 款 rmmz 是加密图款（gallery 里标「加密图不可渲染」），要肉眼必须先走项目内解密/导出链路产出明文 png，再把清单里该款 `image_root` 指到明文目录后重建画廊（见 `tools/review-gallery/README.md` §4.1）。

## 3. 预筛优先（先用机器预筛排序，再排肉眼量）

逐款随机翻图不看重点，命中率低。先跑/先读机器预筛报告，把肉眼火力对准可疑款：

1. **Unity 16 款已预筛完**：直接读 `reports/人工抽检/预筛可疑.md`（生成器 `tools/review-prescreen`，指标口径见其 README；原始指标在 `reports/人工抽检/预筛指标.json`）。当前 severity 分布 0=1、1=8、2=2、3=5，severity=3 的 5 款：IdealEvesRoom、KanzenChikan、MasyouSyoujo、NTRaholic、SwarmBunker。
2. **rmmz 版用同一工具补跑**（rmmz 源目录分散在多个盘，按分组根分批跑；输出写独立文件，别覆盖 Unity 结果）：

```powershell
go run ./tools/review-prescreen -root "<rmmz 分组根目录>" `
  -md  "$repo\reports\人工抽检\预筛可疑_rmmz.md" `
  -json "$repo\reports\人工抽检\预筛指标_rmmz.json"
```

3. **肉眼量按 severity 分配**（Unity 与 rmmz 版同规则）：

| severity | 肉眼动作 |
| --- | --- |
| 3（建议全量肉眼） | **全量看**，可疑文件名已在报告 severity=3 段列出，直接定位 |
| 2（抽检 30 张） | **全量看**（预筛只给概率不给结论，2 分档说明有实锤信号，如离群/空白占比高） |
| 1（抽检 30 张） | 抽检 30 张：报告列出的可疑样本 + 随机补足 |
| 0（无需看） | 抽检 10 张确认无系统性问题即可 |

4. 报告里 severity=3 的「可疑文件名样本」是硬线索：肉眼时**先看这些文件**，再看周边同族图，不要从头随机翻。

## 4. 抽检策略（分层）

| 层 | 条件 | 动作 |
| --- | --- | --- |
| 大款层 | 体积 > 1GB 或 cg_count > 200 | 全量看，或至少看满 100 张 |
| 常规层 | 其余款 | 随机 30 张 + 序列首 5 张 + 尾 5 张（共 40 张/款）；已进 §3 预筛清单的按 severity 档执行 |
| spike 层 | 只跑通少量事件的验证款（1692614 events=60、Last Town events=1） | 全量看，判定从严 |

本批 cg_count > 200 的大款（打分表已预填 cg_count，直接筛）：Scars of Summer After 1350、(牛)RJ01680223 1025、Unholy_maiden 843、（牛）间谍任务 716、MUNIANGTIAOJIAO 524、魔法学園見聞録 523、Swarm Bunker 544、忍耐色诱 478、魔性少女 425、气息 348、Restore( 338、Forbidden Tomes 336、詛咒铠甲2 267、The Blood of Yidhra 257、星天之魔女 238、（中等）NTR Hunter 216、妹神官 211、苍红界域 209、(牛)RJ01618221 203、RA26037409( 198、呪いの鎧 222。

### 引擎分类注意点

- **rmmz（RPG Maker MZ）**：事件来自 `www/data` JSON 解析，画面是立绘/差分**叠加合成帧**——重点查透明通道与差分错位；缩略图命中的是 `img/pictures` 命名族，查编号连续性以图片文件名为准；cg_count 实为事件数，含对话事件，不用按纯 CG 数要求它。判定动作详见 §2.2。
- **Unity**：图来自 `tools/unity_cg_export.py` 导出的 `<out>/cg/<CG名>/` + `cg-index.json`，先开 cg-index.json 对 `counts.cg`；spine 款核对 `.json + .atlas + .png` 三件套数量一致；已知存疑款重点盯：KanzenChikan（17 部 PNG 体积 0MB 量级）、NTRaholic（PNG 合计 1MB 量级，Live2D 主体另批）、「解密 → 内层 Lifeguard Holic」（确认导出的是内层真实 CG 而非解密中间产物）。
- **spike**：只证明管线通、覆盖率低（1~60 个事件），按全量看，任何一项不合格都降级。

## 5. 单款判定清单（逐项勾，全过 = PASS）

1. [ ] 分辨率一致且非极低（同款主图分辨率一致；无 <256px 的糊图充当 CG）
2. [ ] 无裁切错位/半张图
3. [ ] 无纯黑纯白空帧
4. [ ] 无重复帧（重复率 < 5%）
5. [ ] 编号连续无跳号（按文件名序号抽查首/中/尾三段）
6. [ ] 文件名可追溯到原资源（能对应回游戏目录内的原始文件名）
7. [ ] spine 配对件与 png 数量匹配（.json / .atlas / .png 三件一致；无 spine 的款直接勾过）
8. [ ] 透明通道正常（半透明边缘平滑，不是全黑背景）
9. [ ] 色彩未偏移（无整体偏色/通道错乱/过饱和）

rmmz 款的 2/5/8 三条按 §2.2 的四件事执行（差分错位、透明通道、命名族缺族、事件引用图存在性）；spine 条无 spine 直接勾过。

辅助排查命令（大款必跑，常规款抽查）：

```powershell
# 重复帧：按 MD5 分组，Count>1 即重复（可先看比例再人工确认是不是剧情复用图）
Get-ChildItem "D:\UCGR_Work\unitycg\<游戏名>\cg" -Recurse -Filter *.png |
  Get-FileHash -Algorithm MD5 |
  Group-Object Hash |
  Where-Object Count -gt 1 |
  Sort-Object Count -Descending |
  Select-Object Count,Name

# 疑似空帧/占位图（<2KB 的 PNG）
Get-ChildItem "D:\UCGR_Work\unitycg\<游戏名>\cg" -Recurse -Filter *.png |
  Where-Object Length -lt 2KB |
  Select-Object Name,Length

# 编号跳号：列出文件名里的数字序列（rmmz 同理，把路径换成 img/pictures）
Get-ChildItem "D:\UCGR_Work\unitycg\<游戏名>\cg" -Recurse -Filter *.png |
  ForEach-Object { if ($_.BaseName -match '(\d+)') { [int]$Matches[1] } } |
  Sort-Object |
  ForEach-Object { $prev = -1 } { if ($_ -ne $prev + 1) { Write-Output "跳号: $prev -> $_" }; $prev = $_ }
```

## 6. 降级规则

| 情形 | 判定 | issue_type |
| --- | --- | --- |
| ≥ 1 张裁切错位/半张图 | **PARTIAL** | `crop_misalign` |
| 重复帧 ≥ 5% | **PARTIAL** | `duplicate_frames` |
| 整目录空 / 打不开 / 全是占位图 | **FAIL** | `empty_or_broken` |
| 透明通道全黑 / 整体偏色 | **PARTIAL** | `alpha` / `color` |
| 只有 note 类小瑕疵（个别糊图、命名可读性差） | PASS | `none`，note 记一句 |
| rmmz 命名族缺族 / 事件引用图缺失 | **PARTIAL** | `number_gap` / `trace` |

判 FAIL 时必须在 note 写清**复现命令**（一行可复制），例如：
`go run ./cmd/ucgr rescan "D:\UCGR_Work\unitycg\<游戏名>"` 或 `start "" "D:\UCGR_Work\unitycg\<游戏名>"` 后目录为空。

## 7. 记录与回流

1. 在画廊画布上逐款点判定（好/小问题/严重/待查，快捷键 1~4）+ 备注 → 画布把判定写进浏览器 **localStorage**，页面刷新不丢，可以分多次看完再统一导出。
2. 全部看完后点画布的 **「导出 CSV」** → 下载到与打分表同列结构的 CSV。
3. 用导出的 CSV **整体覆盖** `reports/人工抽检/打分表.csv`（id/name/batch/cg_count 四列预填值会被一起带出，不用手抄；覆盖前旧版无版本管理，想留底就先改名 `打分表_YYYYMMDD.csv` 放同目录）。
4. 判定 ≠ PASS 的款，另开/追加 `reports/人工抽检/问题款清单.md`，每款一小节：

```markdown
## <游戏名>（打分表 id=N，判定 PARTIAL/FAIL）
- 问题：<类型，一句话>
- 证据：reports/人工抽检/证据/<游戏名>/xxx.png（把 2~3 张问题图拷进该目录）
- 复现：<一行命令>
```

5. 回流后按 docs/15 的流程给对应 GitHub issue 换标签：PASS 维持 `status/已验收`；PARTIAL/FAIL 换 `status/待修` 并在 issue 里贴问题款清单小节。

## 8. 节奏建议（与 §3 预筛优先对齐）

- **先清预筛**：severity=3 的款全量肉眼，是本批最容易翻车的地方，排在最前面整块时间做；severity≤1 的款按抽检档过，单款耗时显著更短。
- 预筛后总量收窄：Unity 侧 severity≥2 只有 7 款要全量，其余 9 款抽检；rmmz 版跑完后按同一规则切。
- 单人 **约 8 款/小时**（按常规款 40 张/款估算，抽检档可到 12~15 款/小时）；大款层单款独占一个时间块，别和常规款混着排。
- **每 45 分钟休息 5 分钟**——眼睛会钝，重复帧最容易被看漏，宁可歇也不要硬撑。
- 单次 **2 小时封顶**；一天最多两轮。原「53 款 3~4 个工作时段」的估算按预筛口径重排：全量档（severity≥2 + 大款层）约 2 个时段，抽检档（severity≤1 常规款 + rmmz 源目录款）约 1~2 个时段。

## 9. 打分表字段说明（reports/人工抽检/打分表.csv）

| 列 | 含义 |
| --- | --- |
| id | 1~53 固定序号，回流时别改 |
| name | 游戏名（已按审计 JSON 预填；与画廊显示名不一致时以画廊为准并顺手改） |
| batch | 审计批次：`rmmz-<分组>`（rmmz 分组批）/ `rmmz-批6` / `unity-批N` / `unity-批1(spike)` |
| cg_count | 预填 = 审计 JSON 的 events 数；rmmz 款是事件数非纯 CG 数，以画廊实际图数为准 |
| sampled | 实际看了多少张（大款 ≥100、常规 40、spike 全量；预筛 severity≥2 款全量） |
| verdict | PASS / PARTIAL / FAIL（大写） |
| issue_type | crop_misalign / duplicate_frames / blank_frame / number_gap / trace / spine_mismatch / alpha / color / empty_or_broken / none |
| severity | high / medium / low |
| note | 一句话问题或备注；FAIL 必须含复现命令 |
| reviewer | 谁看的（名字/代号） |
| date | YYYY-MM-DD |
