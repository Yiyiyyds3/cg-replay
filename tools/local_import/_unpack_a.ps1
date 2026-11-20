# A 线：解包 6 款 Kirikiri 打包游戏并重新导入（产物全在 D:\UCGR_Work，游戏目录只读）
$ErrorActionPreference='Continue'
go build -o bin/spike-a.exe ./cmd/spike-a 2>&1
if ($LASTEXITCODE -ne 0) { Write-Output 'SPIKE-A BUILD FAIL'; exit 1 }
$games = @(
  @{n='共妻之村';   s='E:\Personal_entertainment\game\ero\egame\20260228\ADV\未评级\共妻之村 (牛)'}
  @{n='无法诉说的关系'; s='E:\Personal_entertainment\game\ero\egame\20260612\RPG\中等\无法诉说的关系(中等)'}
  @{n='完堕ちX兄嫁';  s='E:\Personal_entertainment\game\ero\egame\20260730\ADV\未评级\(牛)完堕ちX兄嫁 ～アンタのせいよ！ 私、この浮気チ●ポじゃなきゃダメなのっ！～'}
  @{n='姉妹X催眠';   s='E:\Personal_entertainment\game\ero\egame\20260730\SLG\一般\（一般）姉妹X催眠 ～絶対服従、俺がずっと王様～'}
  @{n='姉妹X催眠2';  s='E:\Personal_entertainment\game\ero\egame\20260730\SLG\一般\（一般）姉妹X催眠2 ～絶対服従、セレブ妻と日焼けJKが堕ちる性感マッサージ～'}
  @{n='魔剣士レーヴェ'; s='E:\Personal_entertainment\game\ero\egame\20260623\RPG\未评级\魔剣士レーヴェ'}
)
New-Item -ItemType Directory -Force -Path 'D:\UCGR_Work\unpack' | Out-Null
New-Item -ItemType Directory -Force -Path 'D:\UCGR_Work\local_import' | Out-Null
foreach ($g in $games) {
  $dst = 'D:\UCGR_Work\unpack\' + $g.n
  Write-Output ('=== UNPACK ' + $g.n + ' ===')
  & 'bin\spike-a.exe' -mode unpack $g.s $dst 2>&1 | Select-Object -Last 3
  Write-Output ('=== EVENTS ' + $g.n + ' ===')
  & 'bin\ucgr.exe' events $dst --out ('D:\UCGR_Work\local_import\' + $g.n + '_unpacked\events.json') 2>&1 | Select-Object -First 6
  Write-Output ('EXIT=' + $LASTEXITCODE)
}
Write-Output 'A-LINE DONE'