$ErrorActionPreference='Continue'
$repo = (Get-Location).Path
$snapDir = Join-Path $repo 'reports\local_import\snapshot'
New-Item -ItemType Directory -Force -Path $snapDir | Out-Null
$stamp = Get-Date -Format 'yyyy-MM-dd HH:mm:ss zzz'
$lines = @()
$lines += "SNAPSHOT_TIME=$stamp"
$lines += "REPO=$repo"
$lines += "--- GIT ---"
$lines += "BRANCH=" + (git branch --show-current)
$lines += "HEAD=" + (git rev-parse HEAD)
$lines += "STATUS_BEGIN"
$lines += (git status --short)
$lines += "STATUS_END"
$lines += "--- LIB DATA (LOCALAPPDATA) ---"
$lib='C:\Users\Administrator\AppData\Local\cgreplay-ui'
$lines += "LIB_DIR=$lib exists=" + (Test-Path -LiteralPath $lib)
Get-ChildItem -LiteralPath $lib -File -Force -ErrorAction SilentlyContinue | Sort-Object Name | ForEach-Object {
  $h = (Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash
  $lines += ("FILE|{0}|{1}|{2}" -f $_.Name, $_.Length, $h)
}
$lines += "--- GAMES.JSON ---"
$lines += (Get-Content -LiteralPath (Join-Path $lib 'games.json') -Raw -ErrorAction SilentlyContinue)
$lines += "--- EXPORTED LIBS (D:\UCGR_Work\unitycg) ---"
Get-ChildItem -LiteralPath 'D:\UCGR_Work\unitycg' -Directory -Force -ErrorAction SilentlyContinue | Sort-Object Name | ForEach-Object {
  $idx = Join-Path $_.FullName 'cg-index.json'
  $lines += ("LIB|{0}|cg-index={1}" -f $_.Name, (Test-Path -LiteralPath $idx))
}
$out = Join-Path $snapDir 'state.txt'
$lines | Set-Content -LiteralPath $out -Encoding UTF8
Write-Output ("SNAPSHOT_WRITTEN=" + $out)
Copy-Item -LiteralPath (Join-Path $lib 'catalog.db') -Destination (Join-Path $snapDir 'catalog.db.bak') -Force -ErrorAction SilentlyContinue
Copy-Item -LiteralPath (Join-Path $lib 'games.json') -Destination (Join-Path $snapDir 'games.json.bak') -Force -ErrorAction SilentlyContinue
Copy-Item -LiteralPath (Join-Path $lib 'cgreplay.conf') -Destination (Join-Path $snapDir 'cgreplay.conf.bak') -Force -ErrorAction SilentlyContinue
Get-ChildItem -LiteralPath $snapDir -Force | Select-Object Name,Length | Format-Table -AutoSize | Out-String
Write-Output ("CATALOG_BAK=" + (Test-Path -LiteralPath (Join-Path $snapDir 'catalog.db.bak')))
