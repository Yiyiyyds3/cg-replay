# 13. GitHub 项目管理（0.4.0）

## 一、当前状态（2026-11-13 已落库）

- GitHub CLI（gh）本机仍未装成：winget 下载报 `InternetOpenUrl() failed. 0x80072efd`；scoop/choco 不存在；直连 `github.com:443` 下载 zip 超时（本机仅 SSH 22 可达 GitHub）。gh 可用凭据：无（`GH_TOKEN` / `GITHUB_TOKEN` / `git config github.token` / cmdkey / `~/.config/gh/hosts.yml` 均无 token）。
- 改走 GitHub Actions 内置 `GITHUB_TOKEN`：workflow `.github/workflows/bootstrap-0.4.0.yml`（push 本文件到 main 或手动 `workflow_dispatch` 触发，权限只申请 `contents: read` + `issues: write`）。
- 结果：milestone **`0.4.0` = #1**（state open，open 4 / closed 0）已建；4 个 issue **#9 / #10 / #11 / #12** 全部落库并挂载 #1。
- 发布红线：未经用户明确许可，禁止打 tag、禁止 `gh release create`（docs/08）。

### Actions run 记录

| run | commit | 结论 | 说明 |
|---|---|---|---|
| [43450435061](https://github.com/Yiyiyyds3/cg-replay/actions/runs/43450435061) | `d0a94c8` | failure | `gh issue create --milestone` 传了 milestone 编号 `1`，issue 全部未建（milestone #1 已建成功） |
| [43452287099](https://github.com/Yiyiyyds3/cg-replay/actions/runs/43452287099) | `ae466fc` | success | milestone 改用名称 `0.4.0` + issue 去重改全量标题精确匹配（`grep -Fxq`），4 个 issue 一次建成 |

- 失败修复提交：`ae466fc27f44a94d0018bf5eaeaba95957822e4f`（`ci: 修复 issue 引导的 milestone 名称与去重判断`）。

## 二、落库方式

- 主路径（已跑通）：`.github/workflows/bootstrap-0.4.0.yml`，push 本文件到 main 即触发；无需本机 gh / PAT。
- 幂等：milestone 先查再建（按标题 `0.4.0` 匹配）；label 用 `gh label list` 判重；issue 用全量标题 `--state all --limit 200` + `grep -Fxq` 精确匹配去重。
- 关键坑：`gh issue create --milestone` 只认 **milestone 标题**，传编号（`1`）会整条失败且 milestone 本身已建成——首次 run 43450435061 即栽在这里。
- 备用路径：`tools/github-bootstrap.ps1`（幂等，先查再建），需本机 gh + PAT（repo + project 权限）：

```powershell
$env:GH_TOKEN = "<你的 GitHub PAT（需 repo + project 权限）>"
.\tools\github-bootstrap.ps1
```

## 三、四个 issue（已建）

| # | 标题 | 标签 | URL | 分支 | Merge commit |
|---|---|---|---|---|---|
| 9 | [Bug] 解包后 CG 分类错位与纯黑界面误判 | bug | https://github.com/Yiyiyyds3/cg-replay/issues/9 | fix/unpack-classify-black | e5ec406071f7af966b6115ecb01ffa4bdbe00e4a |
| 10 | [Bug] CG 预览图截取显示入场黑屏 | bug | https://github.com/Yiyiyyds3/cg-replay/issues/10 | fix/cover-frame | d3637372e41c6b62ca90d50fca2463152f7f8980 |
| 11 | [Feature] 回放点击互动与文本/音频 | enhancement | https://github.com/Yiyiyyds3/cg-replay/issues/11 | feat/click-av | 97269899a03698b4d719ce8b998f56bb8af0343d |
| 12 | [Design] 工作区与导航栏 VSCode 风格重构 | design | https://github.com/Yiyiyyds3/cg-replay/issues/12 | feat/vscode-ui | 96eecdcaf1d3ef057e2233f570d8a1581153deff |

四个 issue 均 state=open，milestone 均为 `0.4.0`（#1）。验收标准全文见各 issue 正文（与 docs/11 第二章一致）与 docs/12 验收报告。

## 四、对比链接

- 整体（0.4.0 起点 → 当前 main）：https://github.com/Yiyiyyds3/cg-replay/compare/d24709b...670b870
- 各 merge commit：`https://github.com/Yiyiyyds3/cg-replay/commit/<hash>`
- 补 PR 不可行：四个分支均已以 merge commit 并入 main，不硬造 PR；关联关系落在 issue 正文与评论。

## 五、里程碑与看板

- Milestone：`0.4.0` = **#1**（https://github.com/Yiyiyyds3/cg-replay/milestone/1），state=open，4 open / 0 closed，四个 issue 已全部挂载。
- 教训：`--milestone` 参数必须传标题 `0.4.0`；传编号 `1` 会报错并中断脚本（`set -euo pipefail`），导致 4 个 issue 全部未建。
- Project（看板 v2）：`cg-replay 0.4.0` 仍未建——`GITHUB_TOKEN` 无权创建 Project v2，需 PAT 后补；补建后把 #9~#12 加入为 item（列：Todo / In Progress / Done 自行拖动）。
