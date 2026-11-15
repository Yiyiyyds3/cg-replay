# 游戏库验收 · Unity 导出批 1 报告

- 仓库基线：`main @ 6438f48`（提交前已 `git fetch origin` 核实，工作区干净）
- 日期：2026-11-15
- 红线遵守：**全程只读游戏目录**；所有导出产物仅写 `D:\UCGR_Work\unitycg\<短名>\`，未写入/改动任何游戏目录。
- 工具链：`tools/unity_cg_export.py`（main 版：`--assets` / `--still-regex`）+ `cmd/spike-a -mode unity`（ScanUnity 数 events）；Python 3.12.10 + UnityPy 1.25.4。

## 一、选款（只读测量）

类①「Unity 原始安装目录缺 cg-index.json」31 款（总报告 §四），剔除 spike 已做的（一般）1692614 后剩 30 款，按 `Get-ChildItem -Recurse` 只读体积升序取**最小 6 款**：

| # | 游戏 | 组 | 体积 |
|---|---|---|---|
| 1 | モンスターブラックマーケット（夯） | 夯 | 429,341,720 B（409 MB） |
| 2 | Tavern, Inn, and the Halberd（中等） | 中等 | 597,402,175 B（570 MB） |
| 3 | Idol_Forging_V1.0.1.23 | 未评级 | 668,295,113 B（637 MB） |
| 4 | (夯)克蕾娅 | 夯 | 805,480,618 B（768 MB） |
| 5 | （中等）`[ACT`]ラストタウン…Last Town - Fall of Felixia【密码是四夜】 | 中等 | 899,550,630 B（858 MB） |
| 6 | MUNIANGTIAOJIAO（中等） | 中等 | 902,856,999 B（861 MB） |

（第 7 小的 (一搬)ToxicMarriagePunisher 939 MB 未入选。）

## 二、逐款导出与重扫结果

| 游戏 | 体积 | 容器形态 / 命令要点 | 导出结果 | events | 新 verdict | 报错/备注摘要 |
|---|---|---|---|---|---|---|
| モンスターブラックマーケット | 409 MB | 31 个 `<角色>_assets_all.bundle` + `data.unity3d`，`--assets`×32 | 导出成功 4.3s，cg=0（孤儿 png 1485 / text 822） | 0 | **fail**（export_zero_cg） | 骨架为二进制 `.skel`（自研编码），main 版导出器只认 JSON 骨架（spike 报告 §6 结论复现） |
| Tavern, Inn, and the Halberd | 570 MB | `Mira_Data\data.unity3d`（218 MB），`--assets`×1 | 导出成功 2.5s，cg=0（孤儿 png 153 / text 27） | 0 | **fail**（export_zero_cg） | 无 JSON 骨架/.atlas 配对，无 CG 表 CSV |
| Idol_Forging_V1.0.1.23 | 637 MB | 2791 个哈希 bundle；全量传参 180 个即超命令行上限，改抽最大 60 个 | 抽样导出成功 1.2s，cg=0 | 0 | **fail**（export_zero_cg） | `python.exe 启动失败：文件名或扩展名太长`（CreateProcess 32KB 上限）；全量导出需 `--auto`/清单解析版导出器 |
| (夯)克蕾娅 | 768 MB | `data.unity3d`（224 MB）+ 69 bundle，`--assets`×70 | 导出成功 6.9s，cg=0（孤儿 png 588 / text 178） | 0 | **fail**（export_zero_cg） | CG 本体是自定义 HIDC 分块 TextAsset（spike §1），导出器无法解析 |
| Last Town - Fall of Felixia | 858 MB | 内嵌 `resources.assets + sharedassets0/1.assets` + `--still-regex`（H 场景纹理名） | 导出 2.9s，cg=1 / png=1（332 KB） | **1** | **pass**（unity_export_rescan） | 29 张 `Overlay_Sex_*` 纹理因**缺 `resources.assets.resS`** 解码失败（重打包缺外部像素流），仅 1 张可解码；events=1 属实但量级极小 |
| MUNIANGTIAOJIAO | 861 MB | `Mother's Conditioning_Data\StreamingAssets\windows.ab`（756 MB）+ `--still-regex a[1-4]-\d+$` | 导出 89.3s，cg=524 / png=524（736 MB），索引自检通过 | **524** | **pass**（unity_export_rescan） | 524 部静图变体 CG 全部可播（still 类型） |

**本轮 pass 数：2**（LastTownFelixia、MuniangTiaojiao）；**fail：4**；**timeout：0**（最长 89.3s，均在 4 分钟红线内）。判定口径：events>0 → pass（扫描层）；导出成功但 0 CG / 无法传参 → fail + 原始报错，**无编造通过**。

## 三、对总进度的影响（算式闭合）

- pass：37 + 2 = **39**
- fail：34 − 2 = **32**
- A 档（现成工具可推）：40 − 2 = **38**
- 核对：39 + 6（partial）+ 32（fail）+ 40（skip）= **117** ✅；A 38 + B 9 + C 31 = **78** = partial 6 + fail 32 + skip 40 ✅

## 四、下批继续（诚实清单）

类① 31 款现状：**pass 2**（本批）+ spike 已验证 pass 1（1692614，判定 JSON 待补）+ **fail 4**（本批，需 `--auto` 版导出器 / 专用解析器复试：MonsterBlackMarket 二进制骨架、克蕾娅 HIDC、IdolForging 哈希 bundle 清单解析、Tavern 无配对纹理）+ **未动 24 款**。下一批建议：

1. 优先跑与 MUNIANG 同形态的「windows.ab / 内嵌 assets + `--still-regex`」款（中等/ElectronicStore、未评级/fallen、一般/My Lovable Demon 等先探容器）；
2. `--auto`（feat/spine-skins-cover 分支已验证）合入 main 后重试 4 款 fail 与哈希 bundle 型；
3. LastTown 类「.resS 缺流」款如实标注量级，不硬凑 pass。

## 五、产物

- 判定 JSON：`reports/audit/unity-batch1/<短名>.json` × 6 + `summary.json`
- 导出库：`D:\UCGR_Work\unitycg\{MonsterBlackMarket, TavernInnHalberd, IdolForging, Kureya, LastTownFelixia, MuniangTiaojiao}`（合计约 736 MB，全在仓库外）
- 游戏目录零写入。
