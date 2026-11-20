# failed · 试导入失败/超时清单（2026-11-21 00:58:37）

- keep 组：57；import_ok：44；import_fail：13；import_timeout：0

判据：exit!=0 或 events.json 为空 = import_fail；超过 300s = import_timeout。stderr 末 20 行见下。

## import_fail

| 游戏 | 耗时 | 事件 | stderr 末 20 行（折叠为末 3 行摘要） |
|---|---:|---:|---|
| 共妻之村 (牛) | 0.0s | 0 | 错误: 这层只有打包文件（data.xp3，属 kirikiri 引擎），还没解包：先解包成可读剧本再扫（解包产物在别的目录时，直接对那个目录扫） |
| 光辉的少女们 | 5.5s | 0 | 错误: 没有找到可回放的剧本（.ks/.scn/.ast/.bs/.cst/nscript.dat/Script.pac/nss.npa），也没认出打包签名：可能是其它引擎，或还没解包 |
| wuxian | 0.1s | 0 | 错误: 图片由插件/脚本驱动：Show Picture 命令仅 2 条、检测到脚本函数 $utils.shakeScreen, $utils.setupTime, $battleManager.initConfig, $utils.playHint, $gameSelfSwitches.setValue；静态事件命令提取不到 CG，需插件级解析器（暂不纳入回放库） |
| SOTDQ_0.3.5.3v_WINDOWS（很牢） | 0.7s | 0 | 错误: 没有找到可回放的剧本（.ks/.scn/.ast/.bs/.cst/nscript.dat/Script.pac/nss.npa），也没认出打包签名：可能是其它引擎，或还没解包 |
| 无法诉说的关系(中等) | 0.0s | 0 | 错误: 这层只有打包文件（data.xp3，属 kirikiri 引擎），还没解包：先解包成可读剧本再扫（解包产物在别的目录时，直接对那个目录扫） |
| Phoenixes-v17.0.1 | 6.5s | 0 | 错误: 没有找到可回放的剧本（.ks/.scn/.ast/.bs/.cst/nscript.dat/Script.pac/nss.npa），也没认出打包签名：可能是其它引擎，或还没解包 |
| （一般）Touch Simulation Game | 1.7s | 0 | 错误: 没有找到可回放的剧本（.ks/.scn/.ast/.bs/.cst/nscript.dat/Script.pac/nss.npa），也没认出打包签名：可能是其它引擎，或还没解包 |
| (牛)完堕ちX兄嫁 ～アンタのせいよ！ 私、この浮気チ●ポじゃな | 0.0s | 0 | 错误: 这层只有打包文件（append.xp3，属 kirikiri 引擎），还没解包：先解包成可读剧本再扫（解包产物在别的目录时，直接对那个目录扫） |
| （一般）姉妹X催眠 ～絶対服従、俺がずっと王様～ | 0.0s | 0 | 错误: 这层只有打包文件（append.xp3，属 kirikiri 引擎），还没解包：先解包成可读剧本再扫（解包产物在别的目录时，直接对那个目录扫） |
| （一般）姉妹X催眠2 ～絶対服従、セレブ妻と日焼けJKが堕ちる性 | 0.0s | 0 | 错误: 这层只有打包文件（append.xp3，属 kirikiri 引擎），还没解包：先解包成可读剧本再扫（解包产物在别的目录时，直接对那个目录扫） |
| （一般）女王游戏X - 和妖精一起玩卡牌 クイーンズX - 妖精 | 3.0s | 0 | 错误: 没有找到可回放的剧本（.ks/.scn/.ast/.bs/.cst/nscript.dat/Script.pac/nss.npa），也没认出打包签名：可能是其它引擎，或还没解包 |
| HOME | 13.7s | 0 | 错误: 没有找到可回放的剧本（.ks/.scn/.ast/.bs/.cst/nscript.dat/Script.pac/nss.npa），也没认出打包签名：可能是其它引擎，或还没解包 |
| 绝对合格 | 0.1s | 0 | 错误: 图片由插件/脚本驱动：Show Picture 命令仅 4 条、检测到脚本函数 $gameScreen.showPicture, $gameScreen.erasePicture, $gameScreen.movePicture, $gameVariables.setValue, $gameMap.autoplay；静态事件命令提取不到 CG，需插件级解析器（暂不纳入回放库） |

## import_timeout

（无）
