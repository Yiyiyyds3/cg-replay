# 13. GitHub 项目管理（0.4.0）

## 一、当前状态

- GitHub CLI（gh）本机未装成：winget 下载报 `InternetOpenUrl() failed. 0x80072efd`；scoop/choco 不存在；直连 `github.com:443` 下载 zip 超时（本机仅 SSH 22 可达 GitHub）。
- gh 可用凭据：无（`GH_TOKEN` / `GITHUB_TOKEN` / `git config github.token` / cmdkey / `~/.config/gh/hosts.yml` 均无 token；cmdkey 里的 `git:https://github.com` 是 git 推送凭据，不能当 gh token）。
- 因此本轮未在 GitHub 上创建 milestone / issue / 看板，改为落地一键引导脚本，待有 PAT 后一条命令补齐。
- 发布红线：未经用户明确许可，禁止打 tag、禁止 `gh release create`（docs/08）。

## 二、一键引导脚本

- 路径：`tools/github-bootstrap.ps1`（幂等，先查再建，重复运行安全）。
- 运行方法：

```powershell
$env:GH_TOKEN = "<你的 GitHub PAT（需 repo + project 权限）>"
.\tools\github-bootstrap.ps1
```

- 前置：gh 已安装（`winget install --id GitHub.cli -e`，或下载 zip 解压后把 gh.exe 目录加入 PATH）。
- 脚本动作：建 milestone `0.4.0` → 建 labels（bug/enhancement/design）→ 建 4 个 issue（含验收标准、merge hash、compare 链接）→ 每个 issue 评论对应 merge commit → 建 Project `cg-replay 0.4.0` 并加入 4 个 issue。

## 三、四个 issue 草稿

| # | 标题 | 标签 | 分支 | Merge commit |
|---|---|---|---|---|
| 1 | [Bug] 解包后 CG 分类错位与纯黑界面误判 | bug | fix/unpack-classify-black | e5ec406071f7af966b6115ecb01ffa4bdbe00e4a |
| 2 | [Bug] CG 预览图截取入场黑屏 | bug | fix/cover-frame | d3637372e41c6b62ca90d50fca2463152f7f8980 |
| 3 | [Feature] 回放点击互动与文本/音频呈现 | enhancement | feat/click-av | 97269899a03698b4d719ce8b998f56bb8af0343d |
| 4 | [Design] 工作区与导航栏 VSCode 风格重构 | design | feat/vscode-ui | 96eecdcaf1d3ef057e2233f570d8a1581153deff |

验收标准全文见各 issue 正文（与 docs/11 第二章一致）与 docs/12 验收报告。

## 四、对比链接

- 整体（0.4.0 起点 → 当前 main）：https://github.com/Yiyiyyds3/cg-replay/compare/d24709b...670b870
- 各 merge commit：`https://github.com/Yiyiyyds3/cg-replay/commit/<hash>`
- 补 PR 不可行：四个分支均已以 merge commit 并入 main，不硬造 PR；关联关系落在 issue 正文与评论。

## 五、里程碑与看板

- Milestone：`0.4.0`（脚本第一步创建，四个 issue 全部挂载其下）。
- Project（看板 v2）：`cg-replay 0.4.0`，owner `Yiyiyyds3`，四个 issue 加入为 item（列：Todo / In Progress / Done 自行拖动）。
