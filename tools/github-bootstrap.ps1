# github-bootstrap.ps1 — cg-replay 0.4.0 milestone / labels / issues / 看板 一键引导
# 用法：$env:GH_TOKEN = "<PAT>"; .\tools\github-bootstrap.ps1
# 前置：gh 已安装（winget install GitHub.cli 或解压 zip 后把 gh.exe 目录加入 PATH）
# 幂等：所有资源先查再建，已存在直接跳过。

$ErrorActionPreference = 'Stop'
$Repo = 'Yiyiyyds3/cg-replay'

if (-not $env:GH_TOKEN) {
    Write-Error 'GH_TOKEN 未设置。请先：$env:GH_TOKEN = "<你的PAT>"'
}
$env:GH_TOKEN | gh auth login --with-token
gh auth status

# ---------- 1. Milestone 0.4.0 ----------
$existing = gh api "repos/$Repo/milestones" --jq '.[] | select(.title=="0.4.0") | .number'
if ($existing) {
    Write-Host "milestone 0.4.0 已存在 #$existing"
} else {
    $existing = gh api "repos/$Repo/milestones" -f title='0.4.0' -f state='open' -f description='0.4.0：解包分类/黑帧修复、封面黑屏修复、点击互动与文本/音频、VSCode 风格 UI' --jq '.number'
    Write-Host "milestone 0.4.0 已创建 #$existing"
}
$ms = $existing

# ---------- 2. Labels ----------
foreach ($spec in @(
    @{ name='bug';         color='d73a4a'; desc="Something isn't working" },
    @{ name='enhancement'; color='a2eeef'; desc='New feature or request' },
    @{ name='design';      color='c5def5'; desc='UI / UX design work' }
)) {
    $has = gh label list --repo $Repo --json name --jq ".[] | select(.name==`"$($spec.name)`") | .name"
    if ($has) { Write-Host "label $($spec.name) 已存在" }
    else { gh label create $spec.name --repo $Repo --color $spec.color --description $spec.desc; Write-Host "label $($spec.name) 已创建" }
}

# ---------- 3. Issues ----------
$issues = @(
    @{ title='[Bug] 解包后 CG 分类错位与纯黑界面误判'; labels='bug'; branch='fix/unpack-classify-black'; merge='e5ec406071f7af966b6115ecb01ffa4bdbe00e4a';
       body=@"
## 背景
解包产物分类错位导致 CG/背景/立绘/影片混杂；纯黑帧被当成有效画面，可能触发交互或会话拒绝。

## 验收标准
- [ ] 解包产物按 CG / 背景 / 立绘 / 影片 正确分目录；
- [ ] 纯黑帧不得被当成有效画面，不触发交互、不触发会话拒绝；
- [ ] 分类与黑帧判定补单测。

## 关联
- 分支：``fix/unpack-classify-black``
- Merge commit：e5ec406071f7af966b6115ecb01ffa4bdbe00e4a
- 对比链接：https://github.com/Yiyiyyds3/cg-replay/compare/d24709b...670b870
- 验收报告：docs/12_0.4.0_验收报告.md
"@ },
    @{ title='[Bug] CG 预览图截取入场黑屏'; labels='bug'; branch='fix/cover-frame'; merge='d3637372e41c6b62ca90d50fca2463152f7f8980';
       body=@"
## 背景
封面/预览图取到时间轴开头（入场段），显示为黑屏。

## 验收标准
- [ ] 封面/预览取时间轴中的有效画面（非入场黑屏）；
- [ ] 不再显示入场黑屏；
- [ ] 补单测（含黑帧过滤逻辑）。

## 关联
- 分支：``fix/cover-frame``
- Merge commit：d3637372e41c6b62ca90d50fca2463152f7f8980
- 对比链接：https://github.com/Yiyiyyds3/cg-replay/compare/d24709b...670b870
"@ },
    @{ title='[Feature] 回放点击互动与文本/音频呈现'; labels='enhancement'; branch='feat/click-av'; merge='97269899a03698b4d719ce8b998f56bb8af0343d';
       body=@"
## 背景
当前回放是纯观看，缺少原作点击推进、台词与语音。

## 验收标准
- [ ] 鼠标点击舞台触发一次推进；
- [ ] CG / 事件回放能看到台词并听到语音；
- [ ] 补测试（推进状态机 + 文本/音频链路）。

## 关联
- 分支：``feat/click-av``
- Merge commit：97269899a03698b4d719ce8b998f56bb8af0343d
- 对比链接：https://github.com/Yiyiyyds3/cg-replay/compare/d24709b...670b870
"@ },
    @{ title='[Design] 工作区与导航栏 VSCode 风格重构'; labels='design'; branch='feat/vscode-ui'; merge='96eecdcaf1d3ef057e2233f570d8a1581153deff';
       body=@"
## 背景
现有前端布局与主流编辑器习惯不一致，需要 VSCode 风格。

## 验收标准
- [ ] 活动栏 + 侧栏 + 标签页 + 状态栏 的深色布局；
- [ ] 预览窗与导航布局优化；
- [ ] ``npm run build`` 通过。

## 关联
- 分支：``feat/vscode-ui``
- Merge commit：96eecdcaf1d3ef057e2233f570d8a1581153deff
- 对比链接：https://github.com/Yiyiyyds3/cg-replay/compare/d24709b...670b870
"@ }
)

$issueNums = @()
foreach ($i in $issues) {
    $found = gh issue list --repo $Repo --state all --search "in:title `"$($i.title)`"" --json number,title --jq ".[] | select(.title==`"$($i.title)`") | .number" | Select-Object -First 1
    if ($found) {
        Write-Host "issue 已存在 #$found : $($i.title)"
        $issueNums += $found
    } else {
        $tmp = New-TemporaryFile
        $i.body | Set-Content -Path $tmp -Encoding UTF8
        $found = gh issue create --repo $Repo --title $i.title --body-file $tmp --milestone $ms --label $i.labels --jq '.number'
        Remove-Item $tmp
        Write-Host "issue 已创建 #$found : $($i.title)"
    }
    # 关联 merge commit 评论（幂等：查是否已有该 hash 的评论）
    $shortHash = $i.merge.Substring(0,7)
    $hasComment = gh api "repos/$Repo/issues/$found/comments" --jq ".[] | select(.body | contains(`"$shortHash`")) | .id" | Select-Object -First 1
    if ($hasComment) { Write-Host "issue #$found 已有 merge commit 评论" }
    else {
        $cmt = "Merge commit: https://github.com/Yiyiyyds3/cg-replay/commit/$($i.merge)`n分支 ``$($i.branch)`` 已合并（本任务未走 PR，均为 merge commit 直并入 main）。整体对比：https://github.com/Yiyiyyds3/cg-replay/compare/d24709b...670b870"
        $cmt | gh issue comment $found --repo $Repo --body-file -
        Write-Host "issue #$found 已评论 merge commit"
    }
}

# ---------- 4. 看板（Projects v2） ----------
$projId = gh project list --owner Yiyiyyds3 --format json --jq '.projects[] | select(.title=="cg-replay 0.4.0") | .id' 2>$null | Select-Object -First 1
if ($projId) {
    Write-Host "项目 cg-replay 0.4.0 已存在"
} else {
    $projId = gh project create --owner Yiyiyyds3 --title 'cg-replay 0.4.0' --format json --jq '.id'
    Write-Host "项目 cg-replay 0.4.0 已创建 id=$projId"
}
foreach ($n in ($issueNums | Select-Object -Unique)) {
    $url = "https://github.com/Yiyiyyds3/cg-replay/issues/$n"
    gh project item-add $projId --owner Yiyiyyds3 --url $url 2>$null
    if ($LASTEXITCODE -eq 0) { Write-Host "issue #$n 已加入看板" } else { Write-Host "issue #$n 加入看板失败（可能已在板中）" }
}

Write-Host "`n完成。milestone #$ms；issues: $($issueNums | Select-Object -Unique)"
