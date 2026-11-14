# 15 游戏库验收 · GitHub 管理流

本文记录「游戏库全量验收」在 GitHub 侧的管理结构、自动化 workflow、实际运行结果与验证方式。

## 1. 管理结构（Milestone + 4 个状态标签）

GITHUB_TOKEN 没有 Project v2 权限（`gh project` 相关 API 需要 PAT 的 `project` scope），
因此看板**暂以 Milestone + 状态标签代替 Project v2 看板**：
每条游戏 Issue 用 `status/*` 标签表示验收阶段，用 `gh issue list --label` 就能拉出「待处理 / 进行中 / 已验收 / 卡点跳过」四列视图。

| 项目 | 值 |
| --- | --- |
| Milestone | `游戏库全量验收`（编号 #2，state=open，description=`按评价低→高全量自测验收`） |
| 状态标签 | `status/待处理`（ededed）、`status/进行中`（1d76db）、`status/已验收`（0e8a16）、`status/卡点跳过`（b60205） |
| Issue 标题格式 | `[游戏] {分组}/{游戏名}` |
| Issue 数量 | 117（与 `reports/游戏库验收_任务清单.tsv` 行数一一对应） |
| Issue 编号区间 | #13 ~ #129（仓库共 129 条 issue，其中 12 条为 0.4.0 版本 issue 等历史条目） |

状态流转：`待处理` → `进行中` →（7 条清单全过）`已验收`；遇卡点 15 分钟未解 → `卡点跳过`。

## 2. 自动化 workflow

路径：`.github/workflows/bootstrap-game-library.yml`

- 触发：`workflow_dispatch`，或 `push` 到 main 且改动命中
  `.github/workflows/bootstrap-game-library.yml`、`reports/游戏库验收_任务清单.tsv`、`reports/audit/**`（其它 push 不触发）。
- 权限：`contents: read` + `issues: write`，用 runner 自带 `gh` 与 `secrets.GITHUB_TOKEN`（`env: GH_TOKEN`）。
- 幂等：全部「先查再建」，可反复运行：
  1. `Checkout` —— `actions/checkout@v4`。
  2. `Ensure milestone and status labels` —— 先 `gh api "repos/$REPO/milestones?state=all&per_page=100"` 查 milestone，没有再建；4 个 `status/*` 标签先 `gh label list` 查再建。
  3. `Ensure per-game issues` —— `while IFS=$'\t' read -r group game rel engine difficulty` 逐行读 TSV，
     标题严格为 `[游戏] ${group}/${game}`；去重靠一次性拉全量标题
     （`gh issue list --state all --limit 1000 --json title`）后 `grep -Fxq -- "$title"` 精确匹配；
     body 用 heredoc 写临时文件再 `--body-file`，含库内相对路径 / 引擎推断 / 难度预估 / 7 条验收清单 / 卡点处理说明。
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
| commit | `fa8db7b`（`ci: 游戏库全量验收 Issue 批量引导与状态同步`，parent `28b5e15`） |
| workflow run | `43617084266`，event=push，head_sha=`fa8db7b` |
| 结论 | `completed / success` |
| 步骤 | Checkout ✅ / Ensure milestone and status labels ✅ / Ensure per-game issues ✅ / Sync audit verdicts (optional) ✅ |
| 创建结果 | TSV 行数 117，本次创建 117，已存在跳过 0 |

本地预演（stub `gh`，不碰真实仓库）验证过：首跑建 117 条、预置 2 条已存在时建 115 跳过 2、无 `reports/audit` 时同步步骤直接跳过且 exit 0。

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
- 4 个 `status/*` 标签全部存在，颜色与描述与设计一致。

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
`pass/ok/已验收` → `status/已验收`，`fail/blocked/skip/卡点跳过` → `status/卡点跳过`，其余 → `status/进行中`。

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
