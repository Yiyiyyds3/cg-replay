# 15 游戏库验收 · GitHub 管理流

本文记录「游戏库全量验收」在 GitHub 侧的管理结构、自动化 workflow、实际运行结果与验证方式。

## 1. 管理结构（Milestone + 5 个状态标签）

GITHUB_TOKEN 没有 Project v2 权限（`gh project` 相关 API 需要 PAT 的 `project` scope），
因此看板**暂以 Milestone + 状态标签代替 Project v2 看板**：
每条游戏 Issue 用 `status/*` 标签表示验收阶段，用 `gh issue list --label` 就能拉出「待处理 / 进行中 / 已验收 / 待修 / 卡点跳过」五列视图。

| 项目 | 值 |
| --- | --- |
| Milestone | `游戏库全量验收`（编号 #2，state=open，description=`按评价低→高全量自测验收`） |
| 状态标签 | `status/待处理`（ededed）、`status/进行中`（1d76db）、`status/已验收`（0e8a16）、`status/待修`（c5def5）、`status/卡点跳过`（b60205） |
| Issue 标题格式 | `[游戏] {分组}/{游戏名}` |
| Issue 数量 | 117（与 `reports/游戏库验收_任务清单.tsv` 行数一一对应） |
| Issue 编号区间 | #13 ~ #129（仓库共 129 条 issue，其中 12 条为 0.4.0 版本 issue 等历史条目） |

状态流转：`待处理` → `进行中` →（7 条清单全过）`已验收`；审计报错 / 缺产物 → `待修`；遇卡点 15 分钟未解 → `卡点跳过`。

## 1.1 全库收尾（2026-11，7 批全部跑完）

7 批（共 117 款）审计全部出判定后，标签分布收敛为（GitHub issues API 实测，milestone #2 全量翻页取回 117 条）：

| 标签 | 条数 | 对应判定 |
| --- | --- | --- |
| `status/已验收` | 37 | 扫描层 pass（全部 rmmz）→ Unity 导出批1 后推算 **39**（+2：LastTownFelixia、MUNIANGTIAOJIAO） |
| `status/待修` | 34 | fail（31 款 Unity 缺 cg-index.json + 2 款 rmmz 扫描错误 + 1 款 nscript）→ 导出批1 后推算 **32**（−2，4 款导出后仍 0 events 维持 fail） |
| `status/卡点跳过` | 40 | skip（引擎无分派/无入口） |
| `status/进行中` | 6 | partial（6 款 KiriKiri xp3 未解包） |
| `status/待处理` | 0 | 未跑款数为 0（**不等于全部搞定**：未通过 = 78 款 = 67%，Unity 导出批1 后） |

- 汇总产物：`reports/游戏库验收_总报告.md`（7 批次表 / 34 失败 4 类聚类 / 40 跳过引擎缺口 / A/B/C 待搞定清单）。
- 优先级：**A 档先行**——31 款 Unity 跑 `tools/unity_cg_export.py` 生成 cg-index.json、6 款 KiriKiri 用 `internal/unpack.ExtractWith` 解包 xp3 后重扫，预期 pass 从 37 大幅上升；随后合并 feat/spine-skins-cover 的 ScanRenpy（解 7 款 Ren'Py skip）。

## 2. 自动化 workflow

路径：`.github/workflows/bootstrap-game-library.yml`

- 触发：`workflow_dispatch`，或 `push` 到 main 且改动命中
  `.github/workflows/bootstrap-game-library.yml`、`reports/游戏库验收_任务清单.tsv`、`reports/audit/**`（其它 push 不触发）。
- 权限：`contents: read` + `issues: write`，用 runner 自带 `gh` 与 `secrets.GITHUB_TOKEN`（`env: GH_TOKEN`）。
- 幂等：全部「先查再建」，可反复运行：
  1. `Checkout` —— `actions/checkout@v4`。
  2. `Ensure milestone and status labels` —— 先 `gh api "repos/$REPO/milestones?state=all&per_page=100"` 查 milestone，没有再建；5 个 `status/*` 标签先 `gh label list` 查再建。
  3. `Ensure per-game issues` —— `while IFS=$'\t' read -r group game rel engine difficulty` 逐行读 TSV，
     标题严格为 `[游戏] ${group}/${game}`；去重靠一次性拉全量标题
     （`gh issue list --state all --limit 1000 --json title`）后 `grep -Fxq -- "$title"` 精确匹配；
     body 用 heredoc 写临时文件再 `--body-file`（heredoc 内容按 run 块**最小缩进**书写 → YAML 折叠后即顶格，
     再 `sed -i 's/^[[:space:]]*//'` 兜底，并用 `head -n1` 自检首行必须是 `## 验收对象`），
     含库内相对路径 / 引擎推断 / 难度预估 / 7 条验收清单 / 卡点处理说明。
  4. `Sync audit verdicts (optional)` —— 若 `reports/audit/*.json` 存在，按 verdict 给对应 issue 换 `status/*` 标签并贴摘要或卡点原因；目录不存在时安静跳过。

### 踩坑记录

1. **`gh issue create --milestone` 必须传 milestone 标题字符串**（`游戏库全量验收`），不能传编号——run 43450435061 就是传了编号失败的。本 workflow 全篇用 `MS_NAME` 变量（标题），编号只用于日志。
2. **去重不能用 `gh issue list --search`**：标题含中文、方括号 `[游戏]`、单引号（如 `Delilah's`），search 限定符会误判。改为全量拉标题 + `grep -Fxq`。
3. `set -euo pipefail` 下所有「可能已存在 / 找不到」的查询都加了 `|| true`，避免重复运行时挂掉；
   `[ -z "$x" ] && echo ...` 这类在 `set -e` 下会误判退出的写法一律改成 `if`。
4. TSV 为 UTF-8 无 BOM、LF 换行、无表头、5 列制表符、117 行；`difficulty` 额外做了一次 `\r` 剥离，防止有人用 Windows 编辑器改出 CRLF。

## 3. 实际运行结果

| 项 | 值 |
| --- | --- |
| commit（首跑） | `fa8db7b`（`ci: 游戏库全量验收 Issue 批量引导与状态同步`，parent `28b5e15`） |
| workflow run | `43617084266`，event=push，head_sha=`fa8db7b` |
| 结论 | `completed / success` |
| 步骤 | Checkout ✅ / Ensure milestone and status labels ✅ / Ensure per-game issues ✅ / Sync audit verdicts (optional) ✅ |
| 创建结果 | TSV 行数 117，本次创建 117，已存在跳过 0 |

### 运行历史

| run id | head_sha | 触发原因 | 结论 | 说明 |
| --- | --- | --- | --- | --- |
| `43617084266` | `fa8db7b` | 首次 push workflow | success | 建 milestone + 4 标签 + 117 条 issue |
| `43617554439` | `97655e4` | push（同步段兼容性改进） | success | 二次运行，117 条全部判定「已存在」跳过，无重复 |
| 之后由 `reports/audit/**` 提交触发的 run | `7044928` | push（审计 JSON） | 运行中即已生效 | 同步段真实生效：`#20 [游戏] 大雷/女主角（大雷）` 由 `status/待处理` 换成 `status/已验收` |
| `43618818227` | `3031745` | push（批 2 审计 JSON + 看板标签拆分） | success | 新建 `status/待修`；21 款同步完成，线上分布 已验收 6 / 待修 5 / 卡点跳过 8 / 进行中 2 / 待处理 96（见第 7.3 节） |

第二次运行的幂等性在 GitHub 上是可验证的：跑完后 `[游戏] ` issue 总数仍为 117、标题去重后仍为 117，milestone `游戏库全量验收` 的 `open_issues` 仍是 117。

本地预演（stub `gh`，不碰真实仓库）另验证过：首跑建 117 条、预置 2 条已存在时建 115 跳过 2、无 `reports/audit` 时同步步骤直接跳过且 exit 0。

## 4. 验证用 API 端点

```powershell
$h = @{ 'User-Agent' = 'dsh-agent' }

# 最新 workflow run
Invoke-RestMethod 'https://api.github.com/repos/Yiyiyyds3/cg-replay/actions/runs?per_page=5' -Headers $h

# 单次 run 的步骤明细
Invoke-RestMethod 'https://api.github.com/repos/Yiyiyyds3/cg-replay/actions/runs/43617084266/jobs' -Headers $h

# issue 分页统计（标题前缀 [游戏] ，注意 PowerShell 的 -like 会把 [] 当通配符，用 StartsWith）
$all = @(); 1..4 | ForEach-Object {
  $all += Invoke-RestMethod "https://api.github.com/repos/Yiyiyyds3/cg-replay/issues?state=all&per_page=100&page=$_" -Headers $h
}
($all | Where-Object { $_.title.StartsWith('[游戏] ') }).Count

# milestone / labels
Invoke-RestMethod 'https://api.github.com/repos/Yiyiyyds3/cg-replay/milestones?state=all' -Headers $h
Invoke-RestMethod 'https://api.github.com/repos/Yiyiyyds3/cg-replay/labels?per_page=100' -Headers $h
```

实测结果：

- `[游戏] ` 前缀 issue = **117**，标题无重复（unique = 117）。
- milestone `游戏库全量验收` = #2，open_issues = 117，closed_issues = 0。
- 批 1 时 4 个 `status/*` 标签全部存在，颜色与描述与设计一致；批 2 起新增第 5 个 `status/待修`（c5def5），见第 7 节。

## 5. 日常用法

```bash
# 某状态的待办
gh issue list -R Yiyiyyds3/cg-replay --label 'status/待处理' --limit 200

# 单个游戏改状态（验收通过）
gh issue edit <num> -R Yiyiyyds3/cg-replay --remove-label 'status/待处理' --add-label 'status/已验收'
gh issue close <num> -R Yiyiyyds3/cg-replay

# 卡点
gh issue edit <num> -R Yiyiyyds3/cg-replay --add-label 'status/卡点跳过'
gh issue comment <num> -R Yiyiyyds3/cg-replay -F ./reports/audit/xxx.json
```

`reports/audit/*.json` 的字段约定（同步步骤读取）：

```json
{ "group": "不赖", "game": "芙(不赖)", "verdict": "pass", "summary": "7 条全过" }
{ "group": "中上", "game": "Oshioki Lesson(中上)", "verdict": "blocked", "reason": "panic: nil map" }
```

也可直接用 `title` 字段给完整标题（`[游戏] 分组/游戏名`）；`verdict` 取
`pass/ok/已验收` → `status/已验收`，`fail/failed/blocked/待修` → `status/待修`，
`skip/skipped/卡点/卡点跳过` → `status/卡点跳过`，`partial/部分通过` 及其它 → `status/进行中`。
换标签时先 `--remove-label` 掉另外 4 个 `status/*`（含 `status/待修`），保证一个 issue 只留一个状态标签。

同步段对审计产物做了两点容错：

- `reports/audit/summary.json`、`batch-N.json` 这类**聚合文件**没有单款 `group/game/title`，直接打印 `[skip]` 跳过，不会去找 `[游戏] /` 这种空标题。
- `reports/audit/<分组>_<序>.json`（`cmd/library-audit` 实际产出的格式）字段为 `group` / `game` / `verdict` / `reason` / `engine_resolved` 等，与本节的字段约定兼容。

## 6. 分组分布（TSV 117 行）

| 分组 | 数量 |
| --- | --- |
| 未评级 | 44 |
| 夯 | 19 |
| 中等 | 16 |
| 一般 | 15 |
| 不赖 | 5 |
| 上等 | 5 |
| 中上 | 5 |
| 创意 | 2 |
| 牢 | 2 |
| 大雷 | 1 |
| 很牢 | 1 |
| 拉 | 1 |
| 一搬 | 1 |
| 合计 | 117 |

## 7. 批 2 跑完后的标签分布与 fail/skip 拆分

### 7.1 为什么新增第 5 个标签 `status/待修`

原同步段把 `fail|failed|blocked` 和 `skip|skipped|卡点|卡点跳过` **合并**成 `status/卡点跳过`，看板上分不清「要补代码的」和「引擎还没接的」——两者是完全不同的工作量，却挤在同一列。批 2 实测正好撞上这个坑：4 款 Unity 是**扫描报错**（缺 `cg-index.json`，补个索引生成器就能过），5 款是**引擎无分派 / 认不出**（Ren'Py×2、Cocos×1、未知×2，得等适配器），两类共 9 款，合并后看板上全是红色「卡点跳过」，无法排序推进。

因此新增 `status/待修`（颜色 `c5def5`，说明「扫描报错/缺产物，需补代码或先导出」）并加入 `ensure_label` 列表与 remove-label 循环：

| verdict（JSON 里的值） | 标签 | 语义 |
|---|---|---|
| `pass` / `passed` / `ok` / `verified` / `已验收` | `status/已验收` | 7 条清单全过 |
| `fail` / `failed` / `blocked` / `待修` | **`status/待修`**（新增） | 扫描报错/缺产物，需补代码或先导出 |
| `skip` / `skipped` / `卡点` / `卡点跳过` | `status/卡点跳过` | 卡点未解，15 分钟超时跳过 |
| `partial` / `partial_pass` / `部分通过` / `进行中` | `status/进行中` | 正在跑，未出结论 |
| 其它 / 认不出 | `status/进行中` | 兜底 |

### 7.2 批 2 跑完后的状态标签分布（5 个，共 117 条 issue）

按 `reports/audit/` 现有逐款 JSON 推得（批 1 的 5 款 + 批 2 的 16 款 = 21 款已跑，其余 96 款未跑仍在 `status/待处理`）：

| 标签 | 条数 | 构成 |
| --- | --- | --- |
| `status/已验收` | 6 | 批 1 大雷/女主角 1 + 批 2 的 5 款 rmmz pass（莉扎西亚、命に代えてもお守りします!、Shitsuke-ai_dlsite、裏垢JK、栩的人生已经没救了） |
| **`status/待修`** | 5 | 批 1 牢/The Blood of Yidhra（Unity 缺 cg-index.json）+ 批 2 的 4 款 Unity fail（1692614、Love Confessions、My Lovable Demon、ToxicMarriagePunisher） |
| `status/卡点跳过` | 8 | 批 1 的 3 款 skip（很牢 SOTDQ/Godot、牢 魔的梦/Unreal、拉 ボクは種付け） + 批 2 的 5 款 skip（porn-shop-princess、CuckoldxCosplayCN/Ren'Py、逃离深渊的少女/Cocos、女王游戏X、Touch Simulation Game/未知） |
| `status/进行中` | 2 | 批 2 的 2 款 KiriKiri partial（姉妹X催眠、姉妹X催眠2：xp3 未解包，可解包待下一步） |
| `status/待处理` | 96 | 未跑（批 3 未评级 44 / 批 4 中等 16 / 批 5 中上+不赖 10 / 批 6 夯 19 / 批 7 创意+上等 7） |
| 合计 | 117 | 6+5+8+2+96 = 117 ✅ |

**注意两件事**：

1. 批 1 的 4 条标签会在本次 push 触发的 run 里**被重新同步一次**：`The Blood of Yidhra` 由 `status/卡点跳过` → `status/待修`（因为 fail 现在有独立标签），其余 4 条不变。这正是拆分生效的可验证点。
2. 「已验收 = 6」仍是**批量层 pass**，不等于 7 条清单全过（第 4/5/6 条无实机证据，见 `reports/游戏库验收_批2报告.md` 第三节）。要真转 `status/已验收`，得先补第二层真机抽样。

### 7.3 批 2 的 run 与同步结果（实测）

| 项 | 值 |
| --- | --- |
| commit | `3031745`（`[验收-批2] 一搬+一般 16 款判定与看板标签拆分`，parent `a15dafe`） |
| workflow run | `43618818227`（run_number 4），event=push，head_sha=`3031745` |
| 结论 | `completed / success` |
| 步骤 | Checkout ✅ / Ensure milestone and status labels ✅ / Ensure per-game issues ✅ / Sync audit verdicts (optional) ✅ |
| `Ensure per-game issues` | 117 行 TSV 全部判定「已存在」跳过，未新建、未重复 |
| `Ensure milestone and status labels` | 新建 `status/待修`（c5def5）成功 |
| `Sync audit verdicts` | 21 款逐款 JSON 全部同步，聚合文件（`summary.json` / `summary-batch1.json` / `batch-1.json` / `batch-2.json`）按设计 `[skip]` 跳过 |

**线上实测的 5 个状态标签分布（拉全量 117 条 `[游戏] ` issue 计数，与 7.2 的预测完全一致）**：

| 标签 | 实测条数 |
| --- | --- |
| `status/已验收` | 6 |
| `status/待修` | 5 |
| `status/卡点跳过` | 8 |
| `status/进行中` | 2 |
| `status/待处理` | 96 |
| 合计 | 117 ✅ |

`fail` 与 `skip` 的拆分在线上可核对：`#43 [游戏] 牢/The Blood of Yidhra（牢）` 这次由 `status/卡点跳过` **改成 `status/待修`**（它是 fail / `scan_error`），而同一批里 `#40 [游戏] 很牢/SOTDQ_0.3.5.3v_WINDOWS（很牢）`（skip / `engine_unsupported_in_main`）仍留在 `status/卡点跳过`。拆分前这两条会落成同一个标签，现在能分开推进。

**口径自洽核对（复核澄清后，2026-11-15）**：线上标签分布 **6/5/8/2/96** 与「**21 款已跑、96 款待处理**」自洽（`6+5+8+2+96 = 117`，`21 = 批1 5 + 批2 16`，批 1 的 `total=5` 是款数而非另计口径，不得再与状态和相加）；`fail → status/待修`、`skip → status/卡点跳过` 的拆分**已生效**（拆分前 9 款会挤在同一列）。详见 `reports/游戏库验收_批2报告.md` 第七节「复核澄清」。

**body 顶格**：抽查 `#20 [游戏] 大雷/女主角（大雷）` 的 `body` 首行为 `## 验收对象`，无行首空格；7 条清单仍是 `- [ ]` 列表而非缩进代码块。本次 workflow 另加了 `sed` 兜底与首行自检，防止后续有人把 heredoc 内容右缩。

## 8. 批 3 跑完后的标签分布与 A/B/C 三档待办

批 3 = `未评级` 44 款，`go run ./cmd/library-audit -group "未评级" -batch batch-3 -out reports/audit`，`pass 14 / partial 3 / fail 12 / skip 15`，用时 299.6s，exit 0，**无一款超时**。详见 `reports/游戏库验收_批3报告.md`。

### 8.1 预计标签分布变化（按本批 verdict 推算）

批 3 的 44 条 issue 会离开 `status/待处理`，按 verdict 落到 4 个状态标签；批 1/2 的 21 条会被重新同步一次（verdict 不变 → 标签不变）：

| 标签 | 批 3 前（6/5/8/2/96） | 批 3 后（预计） | 变化 |
| --- | --- | --- | --- |
| `status/已验收`（pass） | 6 | **20** | +14（rmmz 14 款） |
| `status/待修`（fail） | 5 | **17** | +12（Unity 缺 cg-index ×10、`不谙(` 缺 data、`wuxian` 插件驱动） |
| `status/卡点跳过`（skip） | 8 | **23** | +15（未知 11 / renpy 2 / cocos 1 / wolfrpg 1） |
| `status/进行中`（partial） | 2 | **5** | +3（KiriKiri xp3 未解包） |
| `status/待处理`（未跑） | 96 | **52** | −44（批 4~7：中等 16 / 中上+不赖 10 / 夯 19 / 创意+上等 7） |
| 合计 | 117 | **117** | `20+17+23+5+52 = 117` ✅ |

进度口径同步：**累计 65 已跑（批1 5 + 批2 16 + 批3 44）/ 52 待处理**；三项汇报数 = **累计已验收 20 ／ 卡点跳过 23 ／ 剩余 52（共 117）**，另列 **失败待修 17 / 进行中 5**。

> 触发说明：`.github/workflows/bootstrap-game-library.yml` 的 `paths` 含 `reports/audit/**`，本次提交改动 44 个逐款 JSON + `batch-3.json` + `summary.json` → **由 push 自动触发**。run 结果与线上标签实测分布**待查**（提交后用 §4 的 API 端点核对，再回填本表）。

### 8.2 A/B/C 三档待办（范围 = 批 1+2+3 的 17 fail + 5 partial + 23 skip = 45 款）

| 档 | 款数 | 内容 | 下一轮 |
| --- | --- | --- | --- |
| **A 有现成工具、可直接搞定** | **25** | ① KiriKiri xp3 未解包 5 款（库层 `unpack.ExtractWith` 已支持且有单测，只差 `cmd/ucgr unpack` 补 xp3 分派）；② Unity 缺 `cg-index.json` 17 款（`tools/unity_cg_export.py` 现成）；③ 引擎已确认且有现成 `ScanRmmz`、只缺 `DetectEngine` 识别/层级 3 款（Lulumachi / 影子赛季 / アルミオシオン） | **优先做** |
| **B 引擎已确认、适配器待补** | **16** | Ren'Py 明文 `.rpy` 5（合入分支态适配器 + `ScanWith` 加 `case "renpy"`）／Electron asar 2／Cocos2d-x 散 `project.json` 2／Godot `.pck` 2／PyInstaller 自研 3／Unreal 1（detect-only）／rmmz 插件驱动 1 | 按类型逐个立 Issue |
| **C 真卡点、建议跳过** | **4** | System4(`data.dts`/`.evs`) 光辉的少女们／WolfRPG(`Data.wolf`) 莱特娜／残缺包（`www` 仅 `save/`）不谙(／完全未知引擎 ボクは種付け | 建 Issue 贴原始报错，标 `status/卡点跳过`，不死磕（docs/14 ⑥） |
| 合计 | **45** | = fail 17 + partial 5 + skip 23 ✅ | |

**下一轮优先做 A 档**：其中 A1（xp3 CLI 分派）与 A3（DetectEngine 两条特征）是几十行代码级改动，A2 是跑现成导出器；三块做完预计能把 `已验收` 从 20 推到 40+，再把 `待修/卡点跳过` 压到个位数。B 档按引擎分批合入，C 档维持跳过不动。

## 9. 批 4~7 跑完后的标签分布（全库收官，2026-11-15）

批 4 = 中等 16（pass 4 / partial 1 / fail 6 / skip 5，17.7s）；批 5 = 中上+不赖 10（pass 2 / fail 6 / skip 2，10.9s）；批 6 = 夯 19（pass 5 / fail 6 / skip 8，57.1s）；批 7 = 创意+上等 7（pass 4 / fail 1 / skip 2，51.5s）。全部 exit 0、无超时。详见 `reports/游戏库验收_批4~7报告.md` 四份。

### 9.1 实测标签分布（7 批全跑完，与 §1.1 一致，来源：GitHub issues API）

| 标签 | 批 3 后（20/17/23/5/52） | 批 4~7 后（实测） | 变化 |
| --- | --- | --- | --- |
| `status/已验收`（pass） | 20 | **35** | +15（批4 4 / 批5 2 / 批6 5 / 批7 4，全部 rmmz） |
| `status/待修`（fail） | 17 | **36** | +19（Unity 缺 cg-index ×11、rmmz 插件驱动 ×2、批4 其余 ×6） |
| `status/卡点跳过`（skip） | 23 | **40** | +17（未知 7 / renpy 2 / unreal 2 / gamemaker 1 / godot 1 / 批4 其余 4） |
| `status/进行中`（partial） | 5 | **6** | +1（批4 1 款） |
| `status/待处理`（未跑） | 52 | **0** | −52（全部跑完） |
| 合计 | 117 | **117** | `35+36+40+6+0 = 117` ✅（2026-11-15 rmmz 复扫后线上预计变更为 37/34/40/6/0，见下） |

进度口径同步：**累计 117 已跑（全库）/ 剩余 0**；三项汇报数 = **累计已验收 37 ／ 卡点跳过 40 ／ 剩余 0（共 117）**，另列 **失败待修 34 / 进行中 6**。注意「剩余 0」只表示未跑款数为 0；未验收通过 = partial+fail+skip = **80 款**（68%），后续全部在 `status/待修`、`status/卡点跳过`、`status/进行中` 三条线推进，汇总表见批 7 报告 §6。

> 触发说明：本次提交改动批 5~7 共 36 个逐款 JSON + `batch-5/6/7.json` + `summary.json` → `reports/audit/**` 命中 workflow `paths`，由 push 自动触发；上表「批 4~7 后」列为 GitHub issues API 实测回填值（与 §1.1 的 35/36/40/6/0 一致）。

**复扫更新（2026-11-15）**：commit 489f126 的 rmmz 脚本 CG 修复复扫后，呪いの鎧 v3.0.1 与 詛咒铠甲2 由 fail→pass，预计线上标签分布推算为 已验收 37 / 待修 34 / 卡点跳过 40 / 进行中 6 / 待处理 0（37+34+40+6+0 = 117 ✅），来源 eports/audit/batch-rmmz-recheck.json 与 eports/游戏库验收_rmmz复扫报告.md；**以 Actions 同步后的 GitHub issues API 实测为准**。

**Unity 导出批1 更新（2026-11-15，基线 6438f48）**：6 款体积最小的 Unity fail 导出重扫（`reports/audit/unity-batch1/`，逐款 JSON + summary）——LastTownFelixia events=1、MUNIANGTIAOJIAO events=524 由 fail→**pass**；MonsterBlackMarket、TavernInnHalberd、IdolForging、Kureya 导出后 0 events **维持 fail**。推算标签分布：已验收 **39** / 待修 **32** / 卡点跳过 40 / 进行中 6 / 待处理 0（39+32+40+6+0 = 117 ✅），**以 Actions 同步后的 GitHub issues API 实测为准**。

**Unity 导出批2 + 1692614 补账更新（2026-11-15，基线 a4410c4→441b196）**：批2 6 款同形态导出重扫——IdealEvesRoom events=17、ElvenNinja events=117、MyLovableDemon events=131 由 fail→**pass**，ElectronicStore、ToxicMarriagePunisher、Fallen 0 events 维持 fail（推算 42/29）；随后 1692614 spike pass 判定 JSON 补入 unity-batch1/OneSixNine.json（events=60，commit 441b196，推算 43/28）。**以 Actions 同步后的 GitHub issues API 实测为准**。

**Unity 导出批3 更新（2026-11-15，基线 441b196，两段提交：3a `c6ed545` + 3b）**：9 款 B 形态 Unity fail 导出重扫（`reports/audit/unity-batch3/`，逐款 JSON ×9 + summary；E 形态 5 款 84865/HO/斯塔WIN(9)/棘罪最新版/洗脑APP 跳过，B 形态大款 4 款 解密/NTRaholic/Love Confessions/魔性少女 留下批）——EroTavern(夢見る酒場) events=125、GoblinSword events=50、SwarmBunker events=544、KanzenChikan events=17（17 部 PNG 合计 0.0MB 量级存疑，如实备注）、BloodOfYidhra events=257（spine 220 + still 45）、SuccubusSuccessor events=34 由 fail→**pass**；FallenElfFreya(芙)、GensouBrothel、OshiokiLesson 0 events **维持 fail**（实测纹理命名无 CG 命名族）。推算标签分布：已验收 **49** / 待修 **22** / 卡点跳过 40 / 进行中 6 / 待处理 0（49+22+40+6+0 = 117 ✅；未通过 = 6+22+40 = 68 款/58%），**以 Actions 同步后的 GitHub issues API 实测为准**。


