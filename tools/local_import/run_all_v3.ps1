# run_all_v3.ps1 —— V3 全库批跑编排：每款游戏 events(--validate) + textdump，失败不中断。
#
# 用法：
#   pwsh -NoProfile -File tools\local_import\run_all_v3.ps1                       # 全量（清单缺省 import_results_v2.json）
#   pwsh -NoProfile -File tools\local_import\run_all_v3.ps1 -Limit 3              # 只跑前 3 款（调试）
#   pwsh -NoProfile -File tools\local_import\run_all_v3.ps1 -Only '游戏A,游戏B'   # 只跑指定几款（逗号分隔）
#   pwsh -NoProfile -File tools\local_import\run_all_v3.ps1 -DryRun               # 只打印将要执行的命令
#
# 产物（只写 D:\UCGR_Work，不碰仓库与 E: 盘）：
#   D:\UCGR_Work\local_import_v3\<游戏>\events.json(+.validate.json)
#   D:\UCGR_Work\text\<游戏>\_textdump.json + txt
#   D:\UCGR_Work\local_import_v3\_runlog.txt     全程一行一游戏
#
# 解包目录定位（与 reconcile_v3.py 同一套规则）：
#   unpack -> decrypt -> unitycg -> unitycg_auto -> godot -> tyrano
#   名字按「原样 / 去后缀(_unpacked _cglib _all _godot) / 加后缀」三档试；junction 由 Test-Path 自然跟随。
#   选中的目录必须「真的有可回放的剧本」（探测 *.ks / data\*.json / cg-index.json+cg\），
#   否则退到清单里的原始游戏目录（只读）。rmmz 的 decrypt 镜像只有 www\img，不会被误选。

[CmdletBinding()]
param(
    [string]$GamesFile = "",
    [int]$Limit = 0,
    [string]$Only = "",      # 逗号分隔的游戏名（-File 调用时数组参数不好传，用字符串更省事）
    [string]$Ucgr = "",
    [string]$OutRoot = "D:\UCGR_Work\local_import_v3",
    [string]$TextRoot = "D:\UCGR_Work\text",
    [int]$TimeoutSec = 900,
    [string]$AssetRoot = "",
    [switch]$SkipEvents,
    [switch]$SkipText,
    [switch]$NoValidate,
    [switch]$DryRun
)

$ErrorActionPreference = 'Continue'
try { [Console]::OutputEncoding = [System.Text.Encoding]::UTF8 } catch { }
$NL = [Environment]::NewLine

# ---------- 基础工具 ----------
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$RepoRoot = (Resolve-Path (Join-Path $ScriptDir '..\..')).Path

function Get-SafeName {
    param([string]$Name)
    $s = $Name
    foreach ($ch in [System.IO.Path]::GetInvalidFileNameChars()) { $s = $s.Replace([string]$ch, '_') }
    $s = $s.Trim()
    $s = $s.TrimEnd('.', ' ')
    if ([string]::IsNullOrWhiteSpace($s)) { $s = 'unnamed' }
    if ($s.Length -gt 120) { $s = $s.Substring(0, 120) }
    return $s
}

function Write-Utf8 {
    param([string]$Path, [string]$Text)
    $enc = New-Object System.Text.UTF8Encoding($false)
    [System.IO.File]::AppendAllText($Path, $Text + $NL, $enc)
}

# ---------- 解包目录定位 ----------
$UnpackRoots = @(
    @{ Root = 'D:\UCGR_Work\unpack';        Kind = 'unpack' },
    @{ Root = 'D:\UCGR_Work\decrypt';       Kind = 'decrypt' },
    @{ Root = 'D:\UCGR_Work\unitycg';       Kind = 'unitycg' },
    @{ Root = 'D:\UCGR_Work\unitycg_auto';  Kind = 'unitycg_auto' },
    @{ Root = 'D:\UCGR_Work\godot';         Kind = 'godot' },
    @{ Root = 'D:\UCGR_Work\tyrano';        Kind = 'tyrano' }
)
$MarkerFiles = @('ucgr-convention.json', 'cg-index.json', '.ucgr-source.json', '_carve_manifest.json')
$MarkerDirs = @('raw', 'www', 'data', 'cg')
$NameSuffixes = @('_unpacked', '_cglib', '_all', '_godot', '_base')

# 名字归一（去掉空格/括号/标点，只留字母数字与汉字）：用来兜住
# 「共妻之村 (牛)」这种清单名 vs「共妻之村」这种解包目录名的差异。
function Get-NameKey {
    param([string]$Name)
    $sb = New-Object System.Text.StringBuilder
    foreach ($ch in $Name.ToLower().ToCharArray()) {
        if ([char]::IsLetterOrDigit($ch)) { [void]$sb.Append($ch) }
    }
    return $sb.ToString()
}

function Resolve-FuzzyDir {
    param([string]$Root, [string]$Name)
    $hits = @()
    if (-not (Test-Path -LiteralPath $Root -PathType Container)) { return $hits }
    $key = Get-NameKey -Name $Name
    if ($key.Length -lt 3) { return $hits }
    foreach ($k in @(Get-ChildItem -LiteralPath $Root -Directory -ErrorAction SilentlyContinue)) {
        $dk = Get-NameKey -Name $k.Name
        if ($dk.Length -lt 3) { continue }
        if ($dk -eq $key) {
            $hits += [pscustomobject]@{ Path = $k.FullName; Score = 1000 + $dk.Length }
        } elseif ($dk.StartsWith($key) -or $key.StartsWith($dk)) {
            $hits += [pscustomobject]@{ Path = $k.FullName; Score = [math]::Min($dk.Length, $key.Length) }
        }
    }
    if ($hits.Count -eq 0) { return $hits }
    return @($hits | Sort-Object -Property Score -Descending | ForEach-Object { $_.Path })
}

function Test-UnpackDir {
    param([string]$Path)
    if (-not (Test-Path -LiteralPath $Path -PathType Container)) { return $false }
    $kids = @(Get-ChildItem -LiteralPath $Path -Force -ErrorAction SilentlyContinue)
    $names = @($kids | ForEach-Object { $_.Name.ToLower() })
    foreach ($m in $MarkerFiles) { if ($names -contains $m.ToLower()) { return $true } }
    foreach ($m in $MarkerDirs) { if ($names -contains $m.ToLower()) { return $true } }
    return $false
}

function Get-CandidateNames {
    param([string]$Name)
    $cands = New-Object System.Collections.Generic.List[string]
    $cands.Add($Name)
    foreach ($suf in $NameSuffixes) {
        if ($Name.ToLower().EndsWith($suf)) { $cands.Add($Name.Substring(0, $Name.Length - $suf.Length)) }
    }
    foreach ($suf in $NameSuffixes) {
        if (-not $Name.ToLower().EndsWith($suf)) { $cands.Add($Name + $suf) }
    }
    return $cands
}

# 「这个目录里有没有可回放的剧本」——只看目录存在会把 decrypt 镜像（只有图、没有脚本）
# 当成运行目录，ucgr 会直接报「没找到可回放的剧本」。这里按结构探一下：
#   *.ks/*.scn/*.ast/*.bs/*.cst/*.wsc/*.tjs/*.rpy  /  data 下的 *.json  /  cg-index.json + 非空 cg\
function Test-ScriptSignal {
    param([string]$Path)
    if (-not (Test-Path -LiteralPath $Path -PathType Container)) { return $false }
    $exts = @('.ks', '.scn', '.ast', '.bs', '.cst', '.wsc', '.tjs', '.rpy')
    $probe = @(
        $Path,
        (Join-Path $Path 'raw'),
        (Join-Path $Path 'raw\data'),
        (Join-Path $Path 'raw\data\scenario'),
        (Join-Path $Path 'data'),
        (Join-Path $Path 'scenario'),
        (Join-Path $Path 'www'),
        (Join-Path $Path 'www\data')
    )
    foreach ($d in $probe) {
        if (-not (Test-Path -LiteralPath $d -PathType Container)) { continue }
        $files = @(Get-ChildItem -LiteralPath $d -File -ErrorAction SilentlyContinue | Select-Object -First 400)
        foreach ($fl in $files) {
            $ext = $fl.Extension.ToLower()
            if ($exts -contains $ext) { return $true }
            if ($ext -eq '.json' -and $d -like '*data*') { return $true }
        }
    }
    $cgIdx = Join-Path $Path 'cg-index.json'
    $cgDir = Join-Path $Path 'cg'
    if ((Test-Path -LiteralPath $cgIdx) -and (Test-Path -LiteralPath $cgDir -PathType Container)) {
        # unity 的 cg\ 下面常常是一格一目录（子目录里放帧序列），所以不能只看文件
        if (@(Get-ChildItem -LiteralPath $cgDir -ErrorAction SilentlyContinue | Select-Object -First 1).Count -gt 0) { return $true }
    }
    return $false
}

function Resolve-UnpackDir {
    param([string]$Name, [string]$GamePath = '')
    $cands = Get-CandidateNames -Name $Name
    $dirs = @()
    foreach ($r in $UnpackRoots) {
        if (-not (Test-Path -LiteralPath $r.Root -PathType Container)) { continue }
        foreach ($c in $cands) {
            $p = Join-Path $r.Root $c
            if (Test-UnpackDir -Path $p) { $dirs += [pscustomobject]@{ Path = $p; Kind = $r.Kind } }
        }
    }
    # 模糊兜底：清单名带「(牛)」这类评级后缀、解包目录不带，反过来的情况也不少
    foreach ($r in $UnpackRoots) {
        foreach ($fp in (Resolve-FuzzyDir -Root $r.Root -Name $Name)) {
            if (Test-UnpackDir -Path $fp) { $dirs += [pscustomobject]@{ Path = $fp; Kind = $r.Kind } }
        }
    }
    if (-not [string]::IsNullOrWhiteSpace($GamePath) -and (Test-Path -LiteralPath $GamePath -PathType Container)) {
        $dirs += [pscustomobject]@{ Path = $GamePath; Kind = 'orig' }
    }
    # 去重保序
    $seenP = @{}
    $uniq = @()
    foreach ($d in $dirs) {
        $k = $d.Path.ToLower()
        if ($seenP.ContainsKey($k)) { continue }
        $seenP[$k] = $true
        $uniq += $d
    }
    $dirs = $uniq
    # 第一遍：要「真的有剧本」（rmmz 的 decrypt 镜像只有图，跳过）
    foreach ($d in $dirs) {
        if (Test-ScriptSignal -Path $d.Path) {
            return [pscustomobject]@{ Path = (Resolve-Path -LiteralPath $d.Path).Path; Kind = $d.Kind }
        }
    }
    # 第二遍：退回第一个存在的（会让 ucgr 自己报错，日志里留真原因）
    if ($dirs.Count -gt 0) {
        return [pscustomobject]@{ Path = (Resolve-Path -LiteralPath $dirs[0].Path).Path; Kind = $dirs[0].Kind }
    }
    return $null
}

# ---------- 游戏清单 ----------
function Get-GameList {
    param([string]$Path)
    if (-not (Test-Path -LiteralPath $Path)) { throw "游戏清单不存在：$Path" }
    $rawText = Get-Content -LiteralPath $Path -Raw -Encoding utf8
    $doc = $rawText | ConvertFrom-Json
    $rows = @()
    if ($doc -is [System.Array]) {
        $rows = @($doc)
    } else {
        foreach ($k in @('results', 'games', 'items', 'verdicts')) {
            $prop = $doc.PSObject.Properties[$k]
            if ($prop -and $null -ne $prop.Value -and @($prop.Value).Count -gt 0) { $rows = @($prop.Value); break }
        }
    }
    $out = @()
    $seen = @{}
    foreach ($r in $rows) {
        if ($null -eq $r) { continue }
        $name = $r.name
        if (-not $name) { $name = $r.game }
        if (-not $name) { continue }
        $name = [string]$name
        if ($seen.ContainsKey($name)) { continue }
        $seen[$name] = $true
        $out += [pscustomobject]@{
            Name   = $name
            Path   = [string]$r.path
            Engine = [string]$r.engine
            Status = [string]$r.status
        }
    }
    return , $out
}

# ---------- 外部命令捕获（进程式带超时；ConstrainedLanguage 下回退调用运算符）----------
function Invoke-Capture {
    param([string]$Exe, [string[]]$Argv, [int]$TimeoutSec = 900)
    $res = [ordered]@{ code = -1; stdout = ''; stderr = ''; timed_out = $false; mode = 'process' }
    try {
        $psi = New-Object System.Diagnostics.ProcessStartInfo
        $psi.FileName = $Exe
        foreach ($a in $Argv) { $psi.ArgumentList.Add([string]$a) }
        $psi.UseShellExecute = $false
        $psi.CreateNoWindow = $true
        $psi.RedirectStandardOutput = $true
        $psi.RedirectStandardError = $true
        $psi.StandardOutputEncoding = [System.Text.Encoding]::UTF8
        $psi.StandardErrorEncoding = [System.Text.Encoding]::UTF8
        $p = [System.Diagnostics.Process]::Start($psi)
        $tOut = $p.StandardOutput.ReadToEndAsync()
        $tErr = $p.StandardError.ReadToEndAsync()
        $finished = $p.WaitForExit($TimeoutSec * 1000)
        if (-not $finished) {
            $res.timed_out = $true
            try { $p.Kill($true) } catch { }
        }
        $res.stdout = $tOut.GetAwaiter().GetResult()
        $res.stderr = $tErr.GetAwaiter().GetResult()
        if ($res.timed_out) { $res.code = -999 } else { $res.code = $p.ExitCode }
    } catch {
        $res.mode = 'calloperator'
        $rawLines = & $Exe @Argv 2>&1
        $code = $LASTEXITCODE
        if ($null -eq $code) { $code = 0 }
        $res.code = $code
        $res.stdout = (@($rawLines | ForEach-Object { [string]$_ }) -join "`n")
        $res.stderr = ''
        $res.timed_out = $false
    }
    return [pscustomobject]$res
}

function Get-JsonField {
    param([string]$Path, [string]$Field)
    try {
        $d = Get-Content -LiteralPath $Path -Raw -Encoding utf8 | ConvertFrom-Json
        $p = $d.PSObject.Properties[$Field]
        if ($p) { return $p.Value }
    } catch { }
    return $null
}

# ---------- 参数落地 ----------
if ([string]::IsNullOrWhiteSpace($GamesFile)) {
    $GamesFile = Join-Path $RepoRoot 'reports\local_import\data\import_results_v2.json'
}
if ([string]::IsNullOrWhiteSpace($Ucgr)) {
    foreach ($cand in @('D:\UCGR_Work\bin\ucgr.exe', (Join-Path $RepoRoot 'bin\ucgr.exe'))) {
        if (Test-Path -LiteralPath $cand) { $Ucgr = $cand; break }
    }
}
New-Item -ItemType Directory -Force -Path $OutRoot | Out-Null
New-Item -ItemType Directory -Force -Path $TextRoot | Out-Null
$RunLog = Join-Path $OutRoot '_runlog.txt'

$out = @()
$out += '=== run_all_v3 @ ' + (Get-Date).ToString('yyyy-MM-dd HH:mm:ss') + ' ==='
$out += ('games_file = ' + $GamesFile)
$out += ('ucgr       = ' + $(if ($Ucgr) { $Ucgr } else { '(未找到，需先 go build)' }))
$out += ('out        = ' + $OutRoot + '   text = ' + $TextRoot)
$out += ('limit=' + $Limit + ' only=' + $Only + ' dryrun=' + $DryRun + ' novalidate=' + $NoValidate)

if (-not $Ucgr -or -not (Test-Path -LiteralPath $Ucgr)) {
    $msg = 'FATAL: 找不到 ucgr.exe。先在仓库根构建：go build -o D:\UCGR_Work\bin\ucgr.exe .\cmd\ucgr'
    $out += $msg
    Write-Utf8 -Path $RunLog -Text $msg
    $out -join "`n"
    exit 2
}

$games = Get-GameList -Path $GamesFile
$onlyList = @()
if (-not [string]::IsNullOrWhiteSpace($Only)) {
    $onlyList = @($Only -split ',' | ForEach-Object { $_.Trim() } | Where-Object { $_ })
}
if ($onlyList.Count -gt 0) { $games = @($games | Where-Object { $onlyList -contains $_.Name }) }
if ($Limit -gt 0 -and $games.Count -gt $Limit) { $games = @($games[0..($Limit - 1)]) }
$out += ('清单 ' + $games.Count + ' 款')

Write-Utf8 -Path $RunLog -Text ''
Write-Utf8 -Path $RunLog -Text ('--- run_all_v3 start ' + (Get-Date).ToString('yyyy-MM-dd HH:mm:ss') + ' games=' + $games.Count + ' ucgr=' + $Ucgr + ' ---')

$stat = [ordered]@{ ok = 0; ev_only = 0; tx_only = 0; fail = 0; skip = 0; timeout = 0 }
$total = $games.Count
$idx = 0
$encOut = New-Object System.Text.UTF8Encoding($false)

foreach ($g in $games) {
    $idx++
    $t0 = Get-Date
    $safe = Get-SafeName -Name $g.Name
    $up = Resolve-UnpackDir -Name $g.Name -GamePath $g.Path
    if ($null -eq $up) {
        $secs0 = [math]::Round(((Get-Date) - $t0).TotalSeconds, 1)
        $line = '[' + $idx + '/' + $total + '] ' + $g.Name + '  SKIP 未找到解包目录（' + $secs0 + 's）'
        $out += $line
        Write-Utf8 -Path $RunLog -Text $line
        $stat.skip++
        continue
    }

    $evDir = Join-Path $OutRoot $safe
    $txDir = Join-Path $TextRoot $safe
    New-Item -ItemType Directory -Force -Path $evDir | Out-Null
    if (-not $SkipText) { New-Item -ItemType Directory -Force -Path $txDir | Out-Null }
    $evPath = Join-Path $evDir 'events.json'
    $vaPath = $evPath + '.validate.json'

    $evArgv = @('events', $up.Path, '--out', $evPath)
    if (-not $NoValidate) { $evArgv += '--validate' }
    if (-not [string]::IsNullOrWhiteSpace($AssetRoot)) { $evArgv += @('--assets', $AssetRoot) }
    $txArgv = @('textdump', $up.Path, '--out', $txDir)

    if ($DryRun) {
        $line = '[' + $idx + '/' + $total + '] ' + $g.Name + '  DRYRUN kind=' + $up.Kind + '  ' + $up.Path
        $out += $line
        $out += ('       ' + $Ucgr + ' ' + ($evArgv -join ' '))
        $out += ('       ' + $Ucgr + ' ' + ($txArgv -join ' '))
        Write-Utf8 -Path $RunLog -Text $line
        continue
    }

    $okEv = $false
    $okTx = $false
    $reason = ''
    $secEv = 0.0
    $secTx = 0.0

    if (-not $SkipEvents) {
        $te = Get-Date
        $r1 = Invoke-Capture -Exe $Ucgr -Argv $evArgv -TimeoutSec $TimeoutSec
        $secEv = [math]::Round(((Get-Date) - $te).TotalSeconds, 1)
        $okEv = ($r1.code -eq 0) -and (Test-Path -LiteralPath $evPath) -and ((Get-Item -LiteralPath $evPath).Length -gt 0)
        if (-not $okEv) {
            $reason = 'events rc=' + $r1.code
            if ($r1.timed_out) { $reason = 'events TIMEOUT'; $stat.timeout++ }
            $errFile = Join-Path $evDir '_events.err.txt'
            $dump = 'argv: ' + ($evArgv -join ' ') + $NL + 'rc=' + $r1.code + $NL + 'STDERR:' + $NL + $r1.stderr + $NL + 'STDOUT:' + $NL + $r1.stdout
            [System.IO.File]::WriteAllText($errFile, $dump, $encOut)
        }
    } else { $okEv = $true }

    if (-not $SkipText) {
        $tt = Get-Date
        $r2 = Invoke-Capture -Exe $Ucgr -Argv $txArgv -TimeoutSec $TimeoutSec
        $secTx = [math]::Round(((Get-Date) - $tt).TotalSeconds, 1)
        $tdJson = Join-Path $txDir '_textdump.json'
        $okTx = ($r2.code -eq 0) -and (Test-Path -LiteralPath $tdJson)
        if (-not $okTx) {
            if ($reason) { $reason += '; ' }
            $reason += 'textdump rc=' + $r2.code
            if ($r2.timed_out) { $reason += '(TIMEOUT)'; $stat.timeout++ }
            $errFile2 = Join-Path $txDir '_textdump.err.txt'
            $dump2 = 'argv: ' + ($txArgv -join ' ') + $NL + 'rc=' + $r2.code + $NL + 'STDERR:' + $NL + $r2.stderr + $NL + 'STDOUT:' + $NL + $r2.stdout
            [System.IO.File]::WriteAllText($errFile2, $dump2, $encOut)
        }
    } else { $okTx = $true }

    # 对账关键数字（读不到显示 -）
    $cg = '-'
    $cgDrop = '-'
    $sc = '-'
    $scDrop = '-'
    $au = '-'
    $tlines = '-'
    if (Test-Path -LiteralPath $vaPath) {
        $cg = Get-JsonField -Path $vaPath -Field 'cg_kept'
        $cgDrop = Get-JsonField -Path $vaPath -Field 'cg_dropped_empty'
        $sc = Get-JsonField -Path $vaPath -Field 'scene_kept'
        $scDrop = Get-JsonField -Path $vaPath -Field 'scene_dropped'
        $au = Get-JsonField -Path $vaPath -Field 'audio_count'
    }
    $tdPath = Join-Path $txDir '_textdump.json'
    if (Test-Path -LiteralPath $tdPath) { $tlines = Get-JsonField -Path $tdPath -Field 'total_lines' }

    $secs = [math]::Round(((Get-Date) - $t0).TotalSeconds, 1)
    if ($okEv -and $okTx) { $state = 'OK'; $stat.ok++ }
    elseif ($okEv) { $state = 'EV_ONLY'; $stat.ev_only++ }
    elseif ($okTx) { $state = 'TEXT_ONLY'; $stat.tx_only++ }
    else { $state = 'FAIL'; $stat.fail++ }

    $line = '[' + $idx + '/' + $total + '] ' + $g.Name + '  ' + $state + '  ' + $secs + 's (ev ' + $secEv + 's / tx ' + $secTx + 's) kind=' + $up.Kind + ' cg=' + $cg + '/' + $cgDrop + ' sc=' + $sc + '/' + $scDrop + ' au=' + $au + ' txt=' + $tlines
    if ($reason) { $line += '  <- ' + $reason }
    $out += $line
    Write-Utf8 -Path $RunLog -Text $line
}

$summary = '--- DONE ok={0} ev_only={1} text_only={2} fail={3} skip={4} timeout={5} ---' -f $stat.ok, $stat.ev_only, $stat.tx_only, $stat.fail, $stat.skip, $stat.timeout
$out += $summary
Write-Utf8 -Path $RunLog -Text $summary
Write-Utf8 -Path $RunLog -Text ('--- run_all_v3 end ' + (Get-Date).ToString('yyyy-MM-dd HH:mm:ss') + ' ---')
$out += ('runlog: ' + $RunLog)

$out -join "`n"
