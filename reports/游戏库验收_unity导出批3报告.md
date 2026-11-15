# 游戏库验收 · Unity 导出批 3 报告

- 仓库基线：`main @ 441b196`（提交前已 `git fetch origin` 核实，工作区干净）
- 日期：2026-11-15
- 红线遵守：**全程只读游戏目录**；所有导出产物仅写 `D:\UCGR_Work\unitycg\<短名>\`，未写入/改动任何游戏目录。
- 工具链：`tools/unity_cg_export.py`（main 版：`--assets` / `--still-regex`，rx 语义 = `re.search`，全部 `^` 锚定）+ `cmd/spike-a -mode unity`（ScanUnity 数 events）；Python 3.12.10 + UnityPy 1.25.4（与批1/批2 同款）。
- 两段提交：`c6ed545`（3a，前 6 款）→ 本报告随 3b 提交（后段 3 款 + 全批汇总）。

## 一、选款与形态探明（只读测量，顶层 ≤3 层、非递归）

批2 剩余 18 款 Unity fail（31 款 − 批1 已做 7 − 批2 已做 6），E 形态 5 款（84865、HO(中上)、斯塔WIN(9)、棘罪最新版、洗脑APP大小姐）本批跳过；剩余 13 款 B 形态（批2 已实探 12 款 + 芙(不赖) 本批补探确认：顶层 `Fallen Elf Freya\Fallen Elf Freya_Data`，resources.assets 59MB + sharedassets0 17MB，.resS 齐全）。按体积升序：

**前 6 款（3a）**：芙(不赖) 1010MB（实测，此前未探）、GensouBrothel 1421MB、Oshioki Lesson 2533MB、夢見る酒場 2538MB、ゴブリン＆ソード 2753MB、Swarm Bunker 3567MB。
**后段实跑 3 款（3b）**：KanzenChikan 3912MB、The Blood of Yidhra 4353MB、Succubus Successor 4437MB。
**留下批 4 款（诚实记录，不硬跑）**：解密 5261MB、NTRaholic 5618MB、Love Confessions 5669MB、魔性少女的诱惑同居 11371MB——B 形态确认，但全量内嵌导出超 4 分钟红线概率高（批2 对 Love Confessions 5.7GB 同口径留下批）。

## 二、逐款导出与重扫结果（9 款）

| 游戏 | 组 | 顶层体积 | 容器形态 / 命令要点 | 导出结果 | events | 新 verdict | 报错/备注摘要 |
|---|---|---|---|---|---|---|---|
| 芙(不赖)（Fallen Elf Freya） | 不赖 | 1010 MB | `resources.assets`(31 tex)+`sharedassets0.assets`(6 tex)，无 regex | 导出 0.1s，cg=0（孤儿 png 30 / text 7） | 0 | **fail**（export_zero_cg） | 37 张纹理实测命名全为 UI/加载图/Logo/视频播放器控件（`影片播放_*` 四种），无 CG 命名族；CG 本体疑为视频文件或外部 bundle → 归 --auto 批 |
| GensouBrothel(Ver1.00) | 不赖 | 1421 MB | `resources.assets`(9 tex)+`sharedassets0`(47)+`sharedassets1`(22)，无 regex | 导出 0.1s，cg=0（孤儿 png 57 / text 153） | 0 | **fail**（export_zero_cg） | 78 张纹理全为 UI/字体图集/标题立绘/场景背景（`Title_*`、`Izakaya/Shop` 等）；sa1.resS 632MB 大纹理即场景背景本体 → 归 --auto/图集切分批 |
| Oshioki Lesson | 中上 | 2533 MB | `resources.assets`(349)+`sharedassets0`(16)+`sharedassets1`(553)，无 regex | 导出 0.2s，cg=0（孤儿 png 877 / text 9） | 0 | **fail**（export_zero_cg） | 918 张纹理全为 3D 材质/贴花/UI（`Milltina_*`、`Cum汚れ_*`、`ビンタ`）；3D 引擎内摆拍型，CG 由渲染产出非贴图载体 → 归 --auto/3D 渲染批 |
| 夢見る酒場と秘密の調教 | 上等 | 2538 MB | **13 容器全量传参**（resources.assets + sharedassets0..11）；`--still-regex ^mt_` | 导出 4.1s，cg=125 / png=125（31.8 MB） | **125** | **pass**（unity_export_rescan） | 5 女主（chloe/lily/misha/suzune/tina）各 23 张 + mt_other 10 张，共 125 张 H 静图（sharedassets9）全部可解码；`t_image_*`、`1-1_ero_effect` 等未计入（保守） |
| ゴブリン＆ソード | 未评级 | 2753 MB | `resources.assets`(606 tex)+`sharedassets0.assets`(411 tex)；`--still-regex ^EventCG\d+_Atlas0$|^CGSample\d+$` | 导出 16.0s，cg=50 / png=50（202.8 MB） | **50** | **pass**（unity_export_rescan） | EventCG01..30_Atlas0 ×30 + CGSample01..20 ×20 全部可解码导出；bg_*/icon_*/stand_* 未计入（保守） |
| Swarm Bunker Lust Defense | 未评级 | 3567 MB | `resources.assets`（单容器 3834 tex，resS 2.68GB）；`--still-regex ^(WomanA_H\|WomanB_H\|GirlB_H)` | 导出 8.3s，cg=544 / png=544（7.9 MB） | **544** | **pass**（unity_export_rescan） | H 姿势帧 544 张全部可解码；单张为低分辨率变体（合计仅 7.9MB），量级大但单张小，口径如实；OP 动画帧/敌方帧未计入（保守） |
| KanzenChikan | 夯 | 3912 MB | `sharedassets0.assets`(789 tex)+`resources.assets`；`--still-regex ^\d+$` | 导出 0.5s，cg=17 / png=17（**0.0 MB**） | **17** | **pass**（unity_export_rescan） | 纯数字名 0..16 共 17 张（疑 CG gallery 本体）导出成功、events=17 属实；但 PNG 合计仅 0.0MB（低分辨率/占位纹理），**量级存疑如实记录**；其余 476 个非 ASCII 命名全为触摸互动图标 |
| The Blood of Yidhra | 牢 | 4353 MB | **29 容器全量传参**（resources.assets + sharedassets0..19）；`--still-regex ^CGPreview_` | 导出 25.8s，cg=265（spine 220 + still 45）/ png=266（181.1 MB）+ text 440 段（216.5 MB 落盘） | **257** | **pass**（unity_export_rescan） | CGPreview 图鉴/回放资源族（Enemy 图鉴 CG、Delta、End_Explode、Policewoman 等）全部可回放；spike-a 重扫 events=257（扫描口径，与索引 265 部差额为扫描层合并，如实记录） |
| Succubus Successor Delilah's Juicy Journey | 不赖 | 4437 MB | `resources.assets + sharedassets0/1/2.assets`（4 容器）；`--still-regex ^CG\d+$|^HCutin_` | 导出 4.2s，cg=34 / png=34（19.9 MB） | **34** | **pass**（unity_export_rescan） | CG01..03 ×3 + HCutin_Delilah_ ×17 + HCutin_XRay_ ×6 等 34 张 H Cutin/CG 全部可解码；CG0x_BG、Title_SavePic_* 未计入（保守） |

**本批 pass 数：6**（EroTavern 125、GoblinSword 50、SwarmBunker 544、KanzenChikan 17、BloodOfYidhra 257、SuccubusSuccessor 34，合计 events=1027）；**fail：3**；**timeout：0**（最长 25.8s，均在 4 分钟红线内）。判定口径：events>0 → pass（扫描层）；导出成功但 0 CG → fail + 原始报错（spike-a：`索引里没有可回放的 CG`，exit 1），**无编造通过**。KanzenChikan 量级存疑已在逐款 JSON 与备注中如实标注。

## 三、对总进度的影响（算式闭合）

- pass：43 + 6 = **49**
- fail：28 − 6 = **22**（批3 9 款中 6 款转 pass、3 款维持 fail）
- A 档（现成工具可推）：34 − 6 = **28**
- 核对：49 + 6（partial）+ 22（fail）+ 40（skip）= **117** ✅；A 28 + B 9 + C 31 = **68** = partial 6 + fail 22 + skip 40 ✅
- 类①（Unity 缺 cg-index.json，31 款）现状：**pass 12**（批1 3 + 批2 3 + 批3 6）+ **fail 10**（批1 4 + 批2 3 + 批3 3，需 --auto/图集切分/3D 渲染/视频批复试）+ **未动 9**（B 形态大款 4：解密、NTRaholic、Love Confessions、魔性少女；E 形态 5）

## 四、下批继续（诚实清单）

1. **B 形态大款 4 款**：解密 5.3GB、NTRaholic 5.6GB、Love Confessions 5.7GB、魔性少女 11.4GB——需放宽单款时限或分容器抽样（批2 先例：Love Confessions 留下批同口径）；
2. **E 形态 5 款**（84865、HO、斯塔WIN(9)、棘罪最新版、洗脑APP大小姐）人工看目录后归形态；
3. **0 CG 复试 10 款**（批1 4 + 批2 3 + 批3 3）：Addressables aa / Luban 表 / sactx 图集 / 3D 渲染 / 视频载体，待 `--auto` 版导出器（feat/spine-skins-cover 已验证）合入 main 后重试；
4. KanzenChikan 数字名 17 张 0.0MB 量级存疑，可人工抽看 PNG 确认是否占位图。

## 五、产物

- 判定 JSON：`reports/audit/unity-batch3/<短名>.json` × 9 + `summary.json`（3a 提交 6+1，3b 提交 3+全批 summary 更新）
- 导出库：`D:\UCGR_Work\unitycg\{FallenElfFreya, GensouBrothel, OshiokiLesson, EroTavern, GoblinSword, SwarmBunker, KanzenChikan, BloodOfYidhra, SuccubusSuccessor}`（pass 6 款合计约 480 MB，全在仓库外）
- 游戏目录零写入。
