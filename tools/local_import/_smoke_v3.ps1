# _smoke_v3.ps1 —— V3 链路冒烟：1 kirikiri + 1 rmmz + 1 unity，各跑一遍 events(--validate) + textdump，
# 逐项校验目录结构与 JSON 字段，输出 PASS/FAIL 清单（任一项 FAIL 则退出码 1）。
#
# 用法：
#   pwsh -NoProfile -File tools\local_import\_smoke_v3.ps1
#   pwsh -NoProfile -File tools\local_import\_smoke_v3.ps1 -Games '魔剣士レーヴェ_all,妹神官（牛）,ElectronicStore'
#   pwsh -NoProfile -File tools\local_import\_smoke_v3.ps1 -Ucgr 'D:\UCGR_Work\bin\ucgr.exe'
#
# 产物（不污染正式目录：写在 _smoke 子目录里）：
#   D:\UCGR_Work\local_import_v3\_smoke\<engine>_<游戏>\events.json(+.validate.json)
#   D:\UCGR_Work\text\_smoke\<engine>_<游戏>\_textdump.json
#
# 目录定位规则与 run_all_v3.ps1 / reconcile_v3.py 完全一致：
#   unpack -> decrypt -> unitycg -> unitycg_auto -> godot -> tyrano
#   kirikiri 在 unpack\，rmmz 加密镜像在 decrypt\，unity 在 unitycg(_auto)\；名字按「原样/去后缀/加后缀」三档试。

[CmdletBinding()]
param(
    [string]$Ucgr = "",
    [string]$Games = "",
    [string]$GamesFile = "",
    [string]$WorkRoot = "D:\UCGR_Work\local_import_v3\_smoke",
    [string]$TextRoot = "D:\UCGR_Work\text\_smoke",
    [int]$TimeoutSec = 600
)

$ErrorActionPreference = 'Continue'
try { [Console]::OutputEncoding = [System.Text.Encoding]::UTF8 } catch { }
$NL = [Environment]::NewLine
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$RepoRoot = (Resolve-Path (Join-Path $ScriptDir '..\..')).Path


# 每类引擎的候选样本（按顺序取第一个「解包目录真的存在」的）
$Candidates = [ordered]@{
    kirikiri = @('魔剣士レーヴェ_all', '姉妹X催眠', '共妻之村', '完堕ちX兄嫁', '无法诉说的关系')
    rmmz     = @('妹神官（牛）', 'ayuureiCN（夯）', '魔法学園見聞録', '冒険者リザシア', '妹神官')
    unity    = @('ElectronicStore', 'EroTavern', 'GoblinSword', 'Fallen', 'BloodOfYidhra')
}

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

function Get-SafeName {
    param([string]$Name)
    $s = $Name
    foreach ($ch in [System.IO.Path]::GetInvalidFileNameChars()) { $s = $s.Replace([string]$ch, '_') }
    $s = $s.Trim().TrimEnd('.', ' ')
    if ([string]::IsNullOrWhiteSpace($s)) { $s = 'unnamed' }
    if ($s.Length -gt 120) { $s = $s.Substring(0, 120) }
    return $s
}

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

# 「里面有可回放的剧本吗」——只看目录存在会把 decrypt 镜像（只有 www\img，没有脚本）
# 当成运行目录，ucgr 直接报「没找到可回放的剧本」。Unity 侧同理：cg-index.json 有但 cg\ 是空的
# （ElectronicStore）也算没货，要跳到下一个候选。
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
    $cands = New-Object System.Collections.Generic.List[string]
    $cands.Add($Name)
    foreach ($suf in $NameSuffixes) {
        if ($Name.ToLower().EndsWith($suf)) { $cands.Add($Name.Substring(0, $Name.Length - $suf.Length)) }
    }
    foreach ($suf in $NameSuffixes) {
        if (-not $Name.ToLower().EndsWith($suf)) { $cands.Add($Name + $suf) }
    }
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
    foreach ($d in $dirs) {
        if (Test-ScriptSignal -Path $d.Path) {
            return [pscustomobject]@{ Path = (Resolve-Path -LiteralPath $d.Path).Path; Kind = $d.Kind }
        }
    }
    if ($dirs.Count -gt 0) {
        return [pscustomobject]@{ Path = (Resolve-Path -LiteralPath $dirs[0].Path).Path; Kind = $dirs[0].Kind }
    }
    return $null
}

# 清单里的原始游戏目录（rmmz 的剧本站在这儿；解包/镜像里只有图）
function Get-GamePathMap {
    param([string]$Path)
    $map = @{}
    if (-not (Test-Path -LiteralPath $Path)) { return $map }
    try {
        $doc = Get-Content -LiteralPath $Path -Raw -Encoding utf8 | ConvertFrom-Json
        $rows = @()
        if ($doc -is [System.Array]) {
            $rows = @($doc)
        } else {
            foreach ($k in @('results', 'games', 'items', 'verdicts')) {
                $p = $doc.PSObject.Properties[$k]
                if ($p -and $null -ne $p.Value -and @($p.Value).Count -gt 0) { $rows = @($p.Value); break }
            }
        }
        foreach ($r in $rows) {
            if ($null -eq $r) { continue }
            $n = $r.name
            if (-not $n) { $n = $r.game }
            if ($n -and $r.path) { $map[[string]$n] = [string]$r.path }
        }
    } catch { }
    return $map
}

if ([string]::IsNullOrWhiteSpace($GamesFile)) {
    $GamesFile = Join-Path $RepoRoot 'reports\local_import\data\import_results_v2.json'
}
$GamePaths = Get-GamePathMap -Path $GamesFile

function Invoke-Capture {
    param([string]$Exe, [string[]]$Argv, [int]$TimeoutSec = 600)
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
    }
    return [pscustomobject]$res
}

function Get-Json {
    param([string]$Path)
    try { return (Get-Content -LiteralPath $Path -Raw -Encoding utf8 | ConvertFrom-Json) } catch { return $null }
}

function Has-Field {
    param($Obj, [string]$Field)
    if ($null -eq $Obj) { return $false }
    return ($null -ne $Obj.PSObject.Properties[$Field])
}

function Get-FirstLine {
    param([string]$Text)
    if ([string]::IsNullOrWhiteSpace($Text)) { return '' }
    foreach ($ln in ($Text -split '[\r\n]+')) {
        if (-not [string]::IsNullOrWhiteSpace($ln)) { return $ln.Trim() }
    }
    return ''
}

# ---------- 执行 ----------
$out = @()
$checks = @()

function Add-Check {
    param([string]$State, [string]$Engine, [string]$Game, [string]$Item, [string]$Detail)
    $script:checks += [pscustomobject]@{ State = $State; Engine = $Engine; Game = $Game; Item = $Item; Detail = $Detail }
}

if ([string]::IsNullOrWhiteSpace($Ucgr)) {
    foreach ($cand in @('D:\UCGR_Work\bin\ucgr.exe', (Join-Path $RepoRoot 'bin\ucgr.exe'))) {
        if (Test-Path -LiteralPath $cand) { $Ucgr = $cand; break }
    }
}
$out += '=== _smoke_v3 @ ' + (Get-Date).ToString('yyyy-MM-dd HH:mm:ss') + ' ==='
$out += ('ucgr     = ' + $(if ($Ucgr) { $Ucgr } else { '(未找到)' }))
$out += ('workRoot = ' + $WorkRoot)
$out += ('textRoot = ' + $TextRoot)
$out += ''

$fatal = $null
if (-not $Ucgr -or -not (Test-Path -LiteralPath $Ucgr)) {
    $fatal = '找不到 ucgr.exe：先在仓库根 go build -o D:\UCGR_Work\bin\ucgr.exe .\cmd\ucgr'
} else {
    New-Item -ItemType Directory -Force -Path $WorkRoot | Out-Null
    New-Item -ItemType Directory -Force -Path $TextRoot | Out-Null
}

$override = @()
if (-not [string]::IsNullOrWhiteSpace($Games)) {
    $override = @($Games -split ',' | ForEach-Object { $_.Trim() } | Where-Object { $_ })
}

$picks = @()
if ($fatal) {
    $out += 'FATAL: ' + $fatal
} else {
    $pickIdx = 0
    foreach ($engine in $Candidates.Keys) {
        $chosen = $null
        if ($override.Count -gt $pickIdx) { $tryList = @($override[$pickIdx]) } else { $tryList = $Candidates[$engine] }
        $pickIdx++
        # 先把这批候选都定位出来，再优先挑「真的有剧本」的那个：
        # ElectronicStore 有 cg-index.json 但 cg\ 是空的，只看目录存在会选中它然后必挂。
        $resolved = @()
        foreach ($nm in $tryList) {
            $gp = ''
            if ($GamePaths -and $GamePaths.ContainsKey($nm)) { $gp = $GamePaths[$nm] }
            $up = Resolve-UnpackDir -Name $nm -GamePath $gp
            if ($null -ne $up) { $resolved += [pscustomobject]@{ Name = $nm; Up = $up } }
        }
        foreach ($r in $resolved) {
            if (Test-ScriptSignal -Path $r.Up.Path) {
                $chosen = [pscustomobject]@{ Engine = $engine; Name = $r.Name; Up = $r.Up }
                break
            }
        }
        if ($null -eq $chosen -and $resolved.Count -gt 0) {
            $chosen = [pscustomobject]@{ Engine = $engine; Name = $resolved[0].Name; Up = $resolved[0].Up }
        }
        if ($null -eq $chosen) {
            Add-Check -State 'FAIL' -Engine $engine -Game ($tryList -join '/') -Item '目录定位' -Detail '候选样本都没有解包目录'
        } else {
            $picks += $chosen
        }
    }

    foreach ($pk in $picks) {
        $engine = $pk.Engine
        $game = $pk.Name
        $up = $pk.Up
        $safe = Get-SafeName -Name $game
        $tag = $engine + '_' + $safe
        $evDir = Join-Path $WorkRoot $tag
        $txDir = Join-Path $TextRoot $tag
        New-Item -ItemType Directory -Force -Path $evDir | Out-Null
        New-Item -ItemType Directory -Force -Path $txDir | Out-Null
        $evPath = Join-Path $evDir 'events.json'
        $vaPath = $evPath + '.validate.json'

        # 1) 目录定位
        Add-Check -State 'PASS' -Engine $engine -Game $game -Item '目录定位' -Detail ($up.Path + ' (' + $up.Kind + ')')

        # 2) events
        $evArgv = @('events', $up.Path, '--out', $evPath, '--validate')
        $te = Get-Date
        $r1 = Invoke-Capture -Exe $Ucgr -Argv $evArgv -TimeoutSec $TimeoutSec
        $secEv = [math]::Round(((Get-Date) - $te).TotalSeconds, 1)
        $evOk = ($r1.code -eq 0) -and (Test-Path -LiteralPath $evPath) -and ((Get-Item -LiteralPath $evPath).Length -gt 0)
        if ($evOk) {
            Add-Check -State 'PASS' -Engine $engine -Game $game -Item 'events 生成' -Detail ('rc=0 ' + $secEv + 's ' + (Get-Item -LiteralPath $evPath).Length + ' bytes')
        } else {
            Add-Check -State 'FAIL' -Engine $engine -Game $game -Item 'events 生成' -Detail ('rc=' + $r1.code + ' ' + (Get-FirstLine ($r1.stderr + $NL + $r1.stdout)))
            continue
        }

        # 3) events.json 字段
        $evj = Get-Json -Path $evPath
        $needEv = @('schema', 'root', 'engine_id', 'game', 'cgs', 'scenes')
        $missEv = @($needEv | Where-Object { -not (Has-Field -Obj $evj -Field $_) })
        $ncg = 0
        $nsc = 0
        if ($missEv.Count -eq 0) {
            $ncg = @($evj.cgs).Count
            $nsc = @($evj.scenes).Count
            Add-Check -State 'PASS' -Engine $engine -Game $game -Item 'events.json 字段' -Detail ('schema=' + $evj.schema + ' engine_id=' + $evj.engine_id + ' cgs=' + $ncg + ' scenes=' + $nsc)
        } else {
            Add-Check -State 'FAIL' -Engine $engine -Game $game -Item 'events.json 字段' -Detail ('缺 ' + ($missEv -join ','))
        }

        # 4) validate 报告
        if (Test-Path -LiteralPath $vaPath) {
            $vaj = Get-Json -Path $vaPath
            $needVa = @('cg_kept', 'cg_dropped_empty', 'scene_kept', 'scene_dropped', 'audio_count')
            $missVa = @($needVa | Where-Object { -not (Has-Field -Obj $vaj -Field $_) })
            if ($missVa.Count -eq 0) {
                Add-Check -State 'PASS' -Engine $engine -Game $game -Item 'validate 报告' -Detail ('cg_kept=' + $vaj.cg_kept + ' cg_dropped_empty=' + $vaj.cg_dropped_empty + ' scene=' + $vaj.scene_kept + '/' + $vaj.scene_dropped + ' audio=' + $vaj.audio_count)
                # 对账口径：报告里的 kept 是「过滤后留下的」，dropped 是「过滤前被删的」，
                # 落盘 events.json 只有 kept 那部分 —— 所以只能拿 kept 比落盘条数。
                # （scene_kept+dropped = 过滤前场景总数，落盘文件里看不到，不能拿来比。）
                if ($null -ne $evj) {
                    $kv = [int]$vaj.cg_kept
                    $sv = [int]$vaj.scene_kept
                    $dv = [int]$vaj.scene_dropped
                    if ($kv -eq $ncg -and $sv -eq $nsc) {
                        Add-Check -State 'PASS' -Engine $engine -Game $game -Item '口径一致' -Detail ('cg_kept=' + $kv + '=落盘cgs(' + $ncg + ') scene_kept=' + $sv + '=落盘scenes(' + $nsc + ') 过滤前场景=' + ($sv + $dv))
                    } else {
                        Add-Check -State 'FAIL' -Engine $engine -Game $game -Item '口径一致' -Detail ('cg_kept=' + $kv + ' vs 落盘cgs=' + $ncg + ' scene_kept=' + $sv + ' vs 落盘scenes=' + $nsc)
                    }
                }
            } else {
                Add-Check -State 'FAIL' -Engine $engine -Game $game -Item 'validate 报告' -Detail ('缺字段 ' + ($missVa -join ','))
            }
        } else {
            Add-Check -State 'FAIL' -Engine $engine -Game $game -Item 'validate 报告' -Detail ('未生成 ' + $vaPath + ' rc=' + $r1.code + ' ' + (Get-FirstLine ($r1.stderr + $NL + $r1.stdout)))
        }

        # 5) textdump
        $txArgv = @('textdump', $up.Path, '--out', $txDir)
        $tt = Get-Date
        $r2 = Invoke-Capture -Exe $Ucgr -Argv $txArgv -TimeoutSec $TimeoutSec
        $secTx = [math]::Round(((Get-Date) - $tt).TotalSeconds, 1)
        $tdPath = Join-Path $txDir '_textdump.json'
        if (($r2.code -eq 0) -and (Test-Path -LiteralPath $tdPath)) {
            $tdj = Get-Json -Path $tdPath
            $needTd = @('total_scripts', 'total_lines', 'total_chars')
            $missTd = @($needTd | Where-Object { -not (Has-Field -Obj $tdj -Field $_) })
            if ($missTd.Count -eq 0) {
                Add-Check -State 'PASS' -Engine $engine -Game $game -Item 'textdump' -Detail ('scripts=' + $tdj.total_scripts + ' lines=' + $tdj.total_lines + ' chars=' + $tdj.total_chars + ' ' + $secTx + 's')
            } else {
                Add-Check -State 'FAIL' -Engine $engine -Game $game -Item 'textdump' -Detail ('_textdump.json 缺字段 ' + ($missTd -join ','))
            }
        } else {
            Add-Check -State 'FAIL' -Engine $engine -Game $game -Item 'textdump' -Detail ('rc=' + $r2.code + ' ' + (Get-FirstLine ($r2.stderr + $NL + $r2.stdout)))
        }

        # 6) 产物落点符合批跑约定
        $expectEv = Join-Path $WorkRoot $tag
        if ((Split-Path -Parent $evPath) -eq $expectEv) {
            Add-Check -State 'PASS' -Engine $engine -Game $game -Item '产物落点' -Detail $evPath
        } else {
            Add-Check -State 'FAIL' -Engine $engine -Game $game -Item '产物落点' -Detail $evPath
        }
    }
}

# ---------- 输出 PASS/FAIL 清单 ----------
foreach ($c in $checks) {
    $out += '[' + $c.State + '] ' + $c.Engine.PadRight(9) + ' ' + $c.Game.PadRight(28) + ' ' + $c.Item.PadRight(14) + ' ' + $c.Detail
}

$pass = @($checks | Where-Object { $_.State -eq 'PASS' }).Count
$fail = @($checks | Where-Object { $_.State -eq 'FAIL' }).Count
$verdict = 'FAIL'
if ($fail -eq 0 -and -not $fatal) { $verdict = 'PASS' }
$out += ''
$out += ('=== SMOKE ' + $verdict + '  pass=' + $pass + ' fail=' + $fail + ' ===')
if ($fail -gt 0) {
    $out += '失败项：'
    foreach ($c in ($checks | Where-Object { $_.State -eq 'FAIL' })) {
        $out += ('  - ' + $c.Engine + '/' + $c.Game + ' :: ' + $c.Item + ' :: ' + $c.Detail)
    }
}

$out -join "`n"
if ($fatal -or $fail -gt 0) { exit 1 } else { exit 0 }
