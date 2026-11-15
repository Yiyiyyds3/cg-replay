# 游戏库验收 · Unity 导出批 2 报告

- 仓库基线：`main @ a4410c4`（提交前已 `git fetch origin` 核实，工作区干净）
- 日期：2026-11-15
- 红线遵守：**全程只读游戏目录**；所有导出产物仅写 `D:\UCGR_Work\unitycg\<短名>\`，未写入/改动任何游戏目录。
- 工具链：`tools/unity_cg_export.py`（main 版：`--assets` / `--still-regex`，rx 语义 = `re.search`，本次全部加 `^` 锚定）+ `cmd/spike-a -mode unity`（ScanUnity 数 events）；Python 3.12.10 + UnityPy 1.25.4。

## 一、选款与形态探明（只读测量，顶层 ≤3 层、非递归）

批1 剩余 24 款 Unity fail（31 款含 cg-index 错误 − 批1 已做 7 款），剔除主人点名排除的 芙(不赖) 后实探 23 款顶层容器：

| 形态 | 款数 | 结论 |
|---|---|---|
| **B 形态**（`*_Data` 内嵌 `resources.assets` + `sharedassetsN.assets`，且 `.resS` 像素流齐全） | **18** | 候选（比批1 LastTown 缺 .resS 的残缺形态更好） |
| E 形态（顶层无 `*_Data`/`resources.assets`，结构非标准） | 5（84865、HO(中上)、斯塔WIN(9)、棘罪最新版、洗脑APP大小姐） | 暂缓，待人工看目录 |
| 芙(不赖) | 1 | 按点名排除，未探 |

**取前 6 款候选**：主人点名 4 款（ElectronicStore、fallen、My Lovable Demon、Flee My Elven Ninja）+ 其余 B 形态中体积最小 2 款（ToxicMarriagePunisher 382MB、ideal-eves-room 670MB）。**Love Confessions on the Adventure（5.7GB）虽被点名，但全量内嵌导出超 4 分钟红线概率高，留下批**（诚实记录，不硬跑）。

18 款 B 形态全名单（按顶层体积升序）：ElectronicStore 203MB、ToxicMarriagePunisher 382MB、ideal-eves-room 670MB、fallen 765MB、Flee My Elven Ninja 793MB、My Lovable Demon 1092MB、GensouBrothel 1421MB、Oshioki Lesson 2533MB、夢見る酒場 2538MB、ゴブリン＆ソード 2753MB、Swarm Bunker Lust Defense 3567MB、KanzenChikan 3912MB、The Blood of Yidhra 4353MB、Succubus Successor 4437MB、解密 5261MB、NTRaholic 5618MB、Love Confessions 5669MB、魔性少女的诱惑同居 11371MB。

## 二、逐款导出与重扫结果

| 游戏 | 组 | 顶层体积 | 容器形态 / 命令要点 | 导出结果 | events | 新 verdict | 报错/备注摘要 |
|---|---|---|---|---|---|---|---|
| ElectronicStore | 中等 | 203 MB | `resources.assets`(62 tex)+`sharedassets0.assets`(6 tex)，无 regex | 导出 0.1s，cg=0（孤儿 png 49 / text 11） | 0 | **fail**（export_zero_cg） | 内嵌纹理全为 UI/字体/Logo（实测命名），CG 本体在 `StreamingAssets/aa` Addressables bundle（catalog.bin），main 版导出器不解析 → 归 --auto 批 |
| ToxicMarriagePunisher | 一搬 | 382 MB | `resources.assets`(42 tex)+`sharedassets0.assets`(0 tex)+`ab/DefaultPackage`(0 tex)，无 regex | 导出 0.2s，cg=0（孤儿 png 18 / text 4） | 0 | **fail**（export_zero_cg） | CG 索引疑在 `StreamingAssets/Bin/lubandata_tbcg.bytes`（Luban 二进制表），main 版导出器不认该格式 |
| （不赖）ideal-eves-room-_1.0.0 | 不赖 | 670 MB | `resources.assets`(254 tex)+`sharedassets0..5`；`--still-regex ^（H行为命名）\|vt\d` 锚定 | 导出 0.8s，cg=17 / png=17（0.1 MB） | **17** | **pass**（unity_export_rescan） | resources.assets 中按 H 行为命名（footjob/anal/eye_socket_sex/pet_play/saliva_exchange/pose_* 等）保守锚定 17 张；量级小（孤儿 png 493 张多为 UI/表情/状态图标，未计入） |
| fallen_1.0.5b | 未评级 | 765 MB | `resources.assets`(171 tex)+`sharedassets0.assets`(214 tex 全 sactx-* 图集页)，无 regex | 导出 0.2s，cg=0（孤儿 png 365 / text 15） | 0 | **fail**（export_zero_cg） | 无独立 CG 命名，CG 疑在 aa/Addressables 或 SpriteAtlas 内部 → 归 --auto/图集切分批 |
| Flee My Elven Ninja | 中等 | 793 MB | `resources.assets`(329 tex)+`sharedassets0.assets`；`--still-regex ^CG\d+\|^Recap_` | 导出 13.6s，cg=117 / png=117（83.7 MB） | **117** | **pass**（unity_export_rescan） | CG02a..CG23a 系列 + Recap_* 事件回顾图全部可解码导出；SKILL_/IN_/CB_/ET_ 等技能/图标未计入（保守） |
| My Lovable Demon(一般) | 一般 | 1092 MB | **96 容器全量传参**（resources.assets + sharedassets0..94）；`--still-regex ^（50 个场景名前缀族）` | 导出 40.1s，cg=131 / png=135（244.7 MB）+ text 134 段 + spine 67 | **131** | **pass**（unity_export_rescan） | doggy/missionary/Jess Fallen/Delph Gang BJ/knee spank 等场景名锚定匹配；部分场景为纯数字命名（1/2/3/5）无法锚定未计入（孤儿 png 469 张，保守口径） |

**本轮 pass 数：3**（IdealEvesRoom、ElvenNinja、MyLovableDemon）；**fail：3**；**timeout：0**（最长 40.1s，均在 4 分钟红线内；6 款导出+重扫并行执行）。判定口径：events>0 → pass（扫描层）；导出成功但 0 CG → fail + 原始报错（spike-a：`索引里没有可回放的 CG`，exit 1），**无编造通过**。

## 三、对总进度的影响（算式闭合）

- pass：39 + 3 = **42**
- fail：32 − 3 = **29**
- A 档（现成工具可推）：38 − 3 = **35**
- 核对：42 + 6（partial）+ 29（fail）+ 40（skip）= **117** ✅；A 35 + B 9 + C 31 = **75** = partial 6 + fail 29 + skip 40 ✅

## 四、下批继续（诚实清单）

类① 31 款现状：**pass 5**（批1 LastTownFelixia 1 + MUNIANGTIAOJIAO 524；批2 IdealEvesRoom 17 + ElvenNinja 117 + MyLovableDemon 131）+ spike 已验证 pass 1（1692614，判定 JSON 待补）+ **fail 7**（批1 4 款 + 批2 3 款，需 `--auto` 版导出器/Luban 表解析/图集切分复试）+ **未动 18 款**（12 款 B 形态中体积 + 5 款 E 形态 + 芙(不赖)）。下一批建议：

1. **同形态大款**：Love Confessions 5.7GB、魔性少女 11.4GB、NTRaholic 5.6GB、解密 5.3GB 等——B 形态确认，但需放宽单款时限或分容器抽样；
2. **中体积补扫**：GensouBrothel 1.4GB、Oshioki Lesson 2.5GB、夢見る酒場 2.5GB、ゴブリン＆ソード 2.8GB、Swarm Bunker 3.6GB、KanzenChikan 3.9GB、Blood of Yidhra 4.4GB、Succubus Successor 4.4GB（先探纹理命名再定 regex）；
3. **E 形态 5 款**（84865、HO、斯塔WIN(9)、棘罪最新版、洗脑APP大小姐）人工看目录后归形态；
4. `--auto`（feat/spine-skins-cover 分支已验证）合入 main 后重试 7 款 fail 与 Addressables/Luban/图集型。

## 五、产物

- 判定 JSON：`reports/audit/unity-batch2/<短名>.json` × 6 + `summary.json`
- 导出库：`D:\UCGR_Work\unitycg\{ElectronicStore, ToxicMarriagePunisher, IdealEvesRoom, Fallen, ElvenNinja, MyLovableDemon}`（合计约 245 MB，全在仓库外）
- 游戏目录零写入。
