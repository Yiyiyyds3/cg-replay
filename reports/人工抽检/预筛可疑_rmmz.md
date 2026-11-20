# 机器预筛 · rmmz 源目录可疑清单

由 tools/review-prescreen -from-manifest 生成，所有数字来自实跑。

- 来源清单：reports\人工抽检\pass清单.json（engine=rmmz，命中 37 款）
- 生成时间：2026-11-16T10:44:09+08:00
- 缺目录：0 款
- 图：扫到 png/jpg/webp 79013 张，参与打分 15151 张，加密图（.rpgmvp/.png_）77551 个

- 抽样：每款最多 300 张（首 5 + 尾 5 + 随机补足），解码像素上限 9000000（超过只读头），并发 8
- 阈值：极小图 任一边<64px；离群 偏离众数>1.5倍；空白 抽样灰度方差<25；重复 同尺寸+8x8 dHash；体积 <5KB
- 散图判据：源目录模式只统计不扣分（-sparse-penalty 可打开）——rmmz 的 pictures/ 常是平铺的人写标题，前缀系列判据不适用
- 族过滤：tilesets/characters/enemies/sv_*/battlebacks/parallaxes/animations/system/titles* 等引擎素材默认剔除，不参与打分（族分布里仍列全）

severity 分布：0=0，1=3，2=2，3=32

## 机检覆盖

- 有明文图、机器能算：**18 款**（参与打分 15151 张）
- CG 族全加密、机器读不到像素：**19 款**（加密图 77551 个）
- sev 3 有两类原因：真的查出问题（空白/重复/离群/命名断裂…），或「图加密了机器查不了」。后者只能人工开游戏看，已单列在下文「加密款」。

### 可机检款（按 severity 降序，这才是能横向比的）

| sev | 游戏 | 图片 | 极小 | 离群 | 空白 | 近似重复组 | 冗余率 | 均KB | 结论 |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 3 | (牛)RJ01618221 | 706 | 0 | 55 | 1 | 52 | 31.3% | 543.8 | 建议全量肉眼 |
| 3 | Chronicles of Clams Island | 842 | 26 | 106 | 8 | 52 | 34.0% | 277.0 | 建议全量肉眼 |
| 3 | Restore( | 1072 | 23 | 274 | 7 | 40 | 37.0% | 619.9 | 建议全量肉眼 |
| 3 | Risty and the Village of Bocchino v1.0.0 | 288 | 0 | 131 | 1 | 30 | 75.3% | 420.5 | 建议全量肉眼 |
| 3 | 女主角（大雷） | 715 | 13 | 274 | 4 | 52 | 42.3% | 518.1 | 建议全量肉眼 |
| 3 | 栩的人生已经没救了（一般） | 935 | 3 | 397 | 18 | 20 | 45.0% | 192.9 | 建议全量肉眼 |
| 3 | 气息（上等） | 1521 | 3 | 127 | 38 | 13 | 70.3% | 114.4 | 建议全量肉眼 |
| 3 | 裏垢JKは露出でバズりたい（一般） | 1724 | 8 | 876 | 96 | 29 | 67.7% | 101.5 | 建议全量肉眼 |
| 3 | 魔法少女木叶  v2.04（中上） | 1317 | 151 | 475 | 7 | 28 | 13.0% | 519.4 | 建议全量肉眼 |
| 3 | （一般）Shitsuke-ai_dlsite | 1026 | 0 | 146 | 0 | 50 | 47.3% | 429.7 | 建议全量肉眼 |
| 3 | （上等）NTRエロトラップダンジョン | 172 | 1 | 63 | 0 | 21 | 20.4% | 548.9 | 建议全量肉眼 |
| 3 | （中等）NTR Hunter | 181 | 6 | 156 | 15 | 9 | 14.4% | 737.6 | 建议全量肉眼 |
| 3 | （牛）间谍任务 | 3076 | 0 | 2 | 41 | 50 | 67.7% | 198.9 | 建议全量肉眼 |
| 2 | 女大学生 DX(8) | 1404 | 0 | 146 | 8 | 54 | 43.0% | 682.2 | 抽检 30 张即可 |
| 2 | （一般）命に代えてもお守りします! | 152 | 0 | 31 | 0 | 20 | 32.2% | 7251.7 | 抽检 30 张即可 |
| 1 | (中等)魔法是无所不能 | 4 | 0 | 0 | 0 | 0 | 0.0% | 232.2 | 抽检 30 张即可 |
| 1 | 对魔战士美月( | 15 | 0 | 0 | 0 | 0 | 0.0% | 1883.3 | 抽检 30 张即可 |
| 1 | 星天之魔女（Arpg） | 1 | 0 | 0 | 0 | 0 | 0.0% | 42.2 | 抽检 30 张即可 |

## 总览（全部 37 款，按 severity 降序）

| sev | 游戏 | 图片 | 族数 | 极小 | 离群 | 空白 | 近似重复组 | 冗余率 | 均KB | 加密图 | 结论 |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 3 | (中等)The Forbidden Tomes of Olipos | 0 | 1 | 0 | 0 | 0 | 0 | 0.0% | 0.0 | 1957 | 建议全量肉眼 |
| 3 | (夯)魔塔 | 0 | 2 | 0 | 0 | 0 | 0 | 0.0% | 0.0 | 2409 | 建议全量肉眼 |
| 3 | (牛)RJ01618221 | 706 | 57 | 0 | 55 | 1 | 52 | 31.3% | 543.8 | 0 | 建议全量肉眼 |
| 3 | (牛)RJ01680223 | 0 | 1 | 0 | 0 | 0 | 0 | 0.0% | 0.0 | 2461 | 建议全量肉眼 |
| 3 | Chronicles of Clams Island | 842 | 15 | 26 | 106 | 8 | 52 | 34.0% | 277.0 | 0 | 建议全量肉眼 |
| 3 | RA26037409( | 0 | 1 | 0 | 0 | 0 | 0 | 0.0% | 0.0 | 324 | 建议全量肉眼 |
| 3 | Restore( | 1072 | 16 | 23 | 274 | 7 | 40 | 37.0% | 619.9 | 0 | 建议全量肉眼 |
| 3 | Risty and the Village of Bocchino v1.0.0 | 288 | 7 | 0 | 131 | 1 | 30 | 75.3% | 420.5 | 0 | 建议全量肉眼 |
| 3 | Scars of Summer After v1.01（牛） | 0 | 1 | 0 | 0 | 0 | 0 | 0.0% | 0.0 | 3987 | 建议全量肉眼 |
| 3 | Unholy_maiden_Ver.1.0.10 | 0 | 0 | 0 | 0 | 0 | 0 | 0.0% | 0.0 | 4182 | 建议全量肉眼 |
| 3 | ayuureiCN（夯） | 0 | 1 | 0 | 0 | 0 | 0 | 0.0% | 0.0 | 1726 | 建议全量肉眼 |
| 3 | 人柱阿拉卡 v230216(创意) | 0 | 3 | 0 | 0 | 0 | 0 | 0.0% | 0.0 | 1911 | 建议全量肉眼 |
| 3 | 呪いの鎧 v3.0.1（夯） | 0 | 3 | 0 | 0 | 0 | 0 | 0.0% | 0.0 | 3385 | 建议全量肉眼 |
| 3 | 女主角（大雷） | 715 | 7 | 13 | 274 | 4 | 52 | 42.3% | 518.1 | 0 | 建议全量肉眼 |
| 3 | 妹神官（牛） | 0 | 1 | 0 | 0 | 0 | 0 | 0.0% | 0.0 | 5894 | 建议全量肉眼 |
| 3 | 希尔丝大冒险  1.06（夯） | 0 | 1 | 0 | 0 | 0 | 0 | 0.0% | 0.0 | 1352 | 建议全量肉眼 |
| 3 | 忍耐色诱的告白游戏 V2.0（不赖） | 0 | 1 | 0 | 0 | 0 | 0 | 0.0% | 0.0 | 1332 | 建议全量肉眼 |
| 3 | 时间勇者（中等） | 0 | 1 | 0 | 0 | 0 | 0 | 0.0% | 0.0 | 4530 | 建议全量肉眼 |
| 3 | 栩的人生已经没救了（一般） | 935 | 14 | 3 | 397 | 18 | 20 | 45.0% | 192.9 | 0 | 建议全量肉眼 |
| 3 | 气息（上等） | 1521 | 38 | 3 | 127 | 38 | 13 | 70.3% | 114.4 | 0 | 建议全量肉眼 |
| 3 | 苍红界域（夯） | 0 | 1 | 0 | 0 | 0 | 0 | 0.0% | 0.0 | 8224 | 建议全量肉眼 |
| 3 | 裏垢JKは露出でバズりたい（一般） | 1724 | 46 | 8 | 876 | 96 | 29 | 67.7% | 101.5 | 0 | 建议全量肉眼 |
| 3 | 詛咒铠甲2（夯） | 0 | 3 | 0 | 0 | 0 | 0 | 0.0% | 0.0 | 18212 | 建议全量肉眼 |
| 3 | 魔法学園見聞録 | 0 | 2 | 0 | 0 | 0 | 0 | 0.0% | 0.0 | 1475 | 建议全量肉眼 |
| 3 | 魔法少女木叶  v2.04（中上） | 1317 | 89 | 151 | 475 | 7 | 28 | 13.0% | 519.4 | 0 | 建议全量肉眼 |
| 3 | （一般） 电脑 冒险者莉扎西亚 冒険者リザシア -葛藤と屈辱の日間- | 0 | 1 | 0 | 0 | 0 | 0 | 0.0% | 0.0 | 662 | 建议全量肉眼 |
| 3 | （一般）Shitsuke-ai_dlsite | 1026 | 30 | 0 | 146 | 0 | 50 | 47.3% | 429.7 | 0 | 建议全量肉眼 |
| 3 | （上等）NTRエロトラップダンジョン | 172 | 21 | 1 | 63 | 0 | 21 | 20.4% | 548.9 | 0 | 建议全量肉眼 |
| 3 | （上等）哈尔法斯的恶魔 | 0 | 1 | 0 | 0 | 0 | 0 | 0.0% | 0.0 | 2865 | 建议全量肉眼 |
| 3 | （中等）NTR Hunter | 181 | 34 | 6 | 156 | 15 | 9 | 14.4% | 737.6 | 0 | 建议全量肉眼 |
| 3 | （夯）直到这个魅魔的心灵崩坏 | 0 | 0 | 0 | 0 | 0 | 0 | 0.0% | 0.0 | 1580 | 建议全量肉眼 |
| 3 | （牛）间谍任务 | 3076 | 14 | 0 | 2 | 41 | 50 | 67.7% | 198.9 | 0 | 建议全量肉眼 |
| 2 | 女大学生 DX(8) | 1404 | 13 | 0 | 146 | 8 | 54 | 43.0% | 682.2 | 0 | 抽检 30 张即可 |
| 2 | （一般）命に代えてもお守りします! | 152 | 20 | 0 | 31 | 0 | 20 | 32.2% | 7251.7 | 0 | 抽检 30 张即可 |
| 1 | (中等)魔法是无所不能 | 4 | 2 | 0 | 0 | 0 | 0 | 0.0% | 232.2 | 4535 | 抽检 30 张即可 |
| 1 | 对魔战士美月( | 15 | 17 | 0 | 0 | 0 | 0 | 0.0% | 1883.3 | 1277 | 抽检 30 张即可 |
| 1 | 星天之魔女（Arpg） | 1 | 2 | 0 | 0 | 0 | 0 | 0.0% | 42.2 | 3271 | 抽检 30 张即可 |

## 每款一句话结论
- **(中等)The Forbidden Tomes of Olipos** — sev 3 — 建议全量肉眼（CG 族里没有明文图，只剩加密图 1957 个（.rpgmvp/.png_；另有引擎素材 2 张已剔除），机器读不到像素 (+5)）
- **(夯)魔塔** — sev 3 — 建议全量肉眼（CG 族里没有明文图，只剩加密图 2409 个（.rpgmvp/.png_；另有引擎素材 4 张已剔除），机器读不到像素 (+5)）
- **(牛)RJ01618221** — sev 3 — 建议全量肉眼（空白帧 1/297 (0.3%) (+1)；尺寸离群 55/706 (7.8%) (+1)；重复帧 52 组，冗余率 31.3% (+1)；序列断裂 92 处 / 14 个目录 (+2)；另有引擎素材 232 张（tilesets/characters 等，已剔除，见 families））
- **(牛)RJ01680223** — sev 3 — 建议全量肉眼（CG 族里没有明文图，只剩加密图 2461 个（.rpgmvp/.png_；另有引擎素材 2 张已剔除），机器读不到像素 (+5)）
- **Chronicles of Clams Island** — sev 3 — 建议全量肉眼（空白帧 8/300 (2.7%) (+1)；极小图 26/842 (3.1%) (+1)；尺寸离群 106/842 (12.6%) (+1)；重复帧 52 组，冗余率 34.0% (+1)；<5KB 小文件 32/842 (3.8%) (+1)；序列断裂 529 处 / 25 个目录 (+2)；另有引擎素材 1151…）
- **RA26037409(** — sev 3 — 建议全量肉眼（CG 族里没有明文图，只剩加密图 324 个（.rpgmvp/.png_；另有引擎素材 2 张已剔除），机器读不到像素 (+5)）
- **Restore(** — sev 3 — 建议全量肉眼（空白帧 7/300 (2.3%) (+1)；极小图 23/1072 (2.1%) (+1)；尺寸离群 274/1072 (26%) (+2)；重复冗余率 37.0%（近似重复 40 组） (+1)；<5KB 小文件 32/1072 (3.0%) (+1)；序列断裂 3 处 / 3 个目录 (+1)；另有引擎素材 98…）
- **Risty and the Village of Bocchino v1.0.0** — sev 3 — 建议全量肉眼（空白帧 1/288 (0.3%) (+1)；尺寸离群 131/288 (45%) (+2)；重复冗余率 75.3%（近似重复 30 组） (+2)；另有引擎素材 806 张（tilesets/characters 等，已剔除，见 families））
- **Scars of Summer After v1.01（牛）** — sev 3 — 建议全量肉眼（CG 族里没有明文图，只剩加密图 3987 个（.rpgmvp/.png_；另有引擎素材 2 张已剔除），机器读不到像素 (+5)）
- **Unholy_maiden_Ver.1.0.10** — sev 3 — 建议全量肉眼（CG 族里没有明文图，只剩加密图 4182 个（.rpgmvp/.png_；另有引擎素材 0 张已剔除），机器读不到像素 (+5)）
- **ayuureiCN（夯）** — sev 3 — 建议全量肉眼（CG 族里没有明文图，只剩加密图 1726 个（.rpgmvp/.png_；另有引擎素材 2 张已剔除），机器读不到像素 (+5)）
- **人柱阿拉卡 v230216(创意)** — sev 3 — 建议全量肉眼（CG 族里没有明文图，只剩加密图 1911 个（.rpgmvp/.png_；另有引擎素材 8 张已剔除），机器读不到像素 (+5)）
- **呪いの鎧 v3.0.1（夯）** — sev 3 — 建议全量肉眼（CG 族里没有明文图，只剩加密图 3385 个（.rpgmvp/.png_；另有引擎素材 54 张已剔除），机器读不到像素 (+5)）
- **女主角（大雷）** — sev 3 — 建议全量肉眼（空白帧 4/300 (1.3%) (+1)；极小图 13/715 (1.8%) (+1)；尺寸离散（众数 1280x720 仅占 38%，离群 274/715） (+1)；重复冗余率 42.3%（近似重复 52 组） (+1)；<5KB 小文件 30/715 (4.2%) (+1)；序列断裂 2 处 / 1 个目录 …）
- **妹神官（牛）** — sev 3 — 建议全量肉眼（CG 族里没有明文图，只剩加密图 5894 个（.rpgmvp/.png_；另有引擎素材 2 张已剔除），机器读不到像素 (+5)）
- **希尔丝大冒险  1.06（夯）** — sev 3 — 建议全量肉眼（CG 族里没有明文图，只剩加密图 1352 个（.rpgmvp/.png_；另有引擎素材 2 张已剔除），机器读不到像素 (+5)）
- **忍耐色诱的告白游戏 V2.0（不赖）** — sev 3 — 建议全量肉眼（CG 族里没有明文图，只剩加密图 1332 个（.rpgmvp/.png_；另有引擎素材 2 张已剔除），机器读不到像素 (+5)）
- **时间勇者（中等）** — sev 3 — 建议全量肉眼（CG 族里没有明文图，只剩加密图 4530 个（.rpgmvp/.png_；另有引擎素材 2 张已剔除），机器读不到像素 (+5)）
- **栩的人生已经没救了（一般）** — sev 3 — 建议全量肉眼（空白帧 18/300 (6%) (+2)；极小图 3/935 (0.3%) (+1)；尺寸离群 397/935 (42%) (+2)；重复冗余率 45.0%（近似重复 20 组） (+1)；<5KB 小文件 73/935 (7.8%) (+1)；序列断裂 81 处 / 4 个目录 (+2)；另有引擎素材 1070 张…）
- **气息（上等）** — sev 3 — 建议全量肉眼（文件头损坏 1 张 (+3)；扩展名与实际格式不符 228/1521 张 (+1)；空白帧 38/300 (13%) (+2)；极小图 3/1521 (0.2%) (+1)；尺寸离群 127/1521 (8.3%) (+1)；重复冗余率 70.3%（近似重复 13 组） (+2)；序列断裂 1 处 / 1 个目录 (…）
- **苍红界域（夯）** — sev 3 — 建议全量肉眼（CG 族里没有明文图，只剩加密图 8224 个（.rpgmvp/.png_；另有引擎素材 2 张已剔除），机器读不到像素 (+5)）
- **裏垢JKは露出でバズりたい（一般）** — sev 3 — 建议全量肉眼（空白帧 96/300 (32%) (+3)；极小图 8/1724 (0.5%) (+1)；尺寸离散（众数 1080x720 仅占 48%，离群 876/1724） (+1)；重复冗余率 67.7%（近似重复 29 组） (+2)；<5KB 小文件 38/1724 (2.2%) (+1)；另有引擎素材 5276 张（t…）
- **詛咒铠甲2（夯）** — sev 3 — 建议全量肉眼（CG 族里没有明文图，只剩加密图 18212 个（.rpgmvp/.png_；另有引擎素材 53 张已剔除），机器读不到像素 (+5)）
- **魔法学園見聞録** — sev 3 — 建议全量肉眼（CG 族里没有明文图，只剩加密图 1475 个（.rpgmvp/.png_；另有引擎素材 3 张已剔除），机器读不到像素 (+5)）
- **魔法少女木叶  v2.04（中上）** — sev 3 — 建议全量肉眼（空白帧 7/300 (2.3%) (+1)；极小图 151/1317 (11.5%) (+1)；尺寸离群 475/1317 (36%) (+2)；重复帧 28 组，冗余率 13.0% (+1)；<5KB 小文件 27/1317 (2.1%) (+1)；序列断裂 21 处 / 3 个目录 (+2)；另有引擎素材 260…）
- **（一般） 电脑 冒险者莉扎西亚 冒険者リザシア -葛藤と屈辱の日間-** — sev 3 — 建议全量肉眼（CG 族里没有明文图，只剩加密图 662 个（.rpgmvp/.png_；另有引擎素材 2 张已剔除），机器读不到像素 (+5)）
- **（一般）Shitsuke-ai_dlsite** — sev 3 — 建议全量肉眼（扩展名与实际格式不符 759/1026 张 (+1)；尺寸离群 146/1026 (14.2%) (+1)；重复冗余率 47.3%（近似重复 50 组） (+1)；序列断裂 533 处 / 2 个目录 (+2)；另有引擎素材 850 张（tilesets/characters 等，已剔除，见 families））
- **（上等）NTRエロトラップダンジョン** — sev 3 — 建议全量肉眼（极小图 1/172 (0.6%) (+1)；尺寸离群 63/172 (37%) (+2)；重复帧 21 组，冗余率 20.4% (+1)；序列断裂 5 处 / 3 个目录 (+1)；另有引擎素材 297 张（tilesets/characters 等，已剔除，见 families））
- **（上等）哈尔法斯的恶魔** — sev 3 — 建议全量肉眼（CG 族里没有明文图，只剩加密图 2865 个（.rpgmvp/.png_；另有引擎素材 2 张已剔除），机器读不到像素 (+5)）
- **（中等）NTR Hunter** — sev 3 — 建议全量肉眼（空白帧 15/181 (8%) (+2)；极小图 6/181 (3.3%) (+1)；尺寸离散（众数 1280x420 仅占 11%，离群 156/181） (+1)；重复帧 9 组，冗余率 14.4% (+1)；<5KB 小文件 6/181 (3.3%) (+1)；序列断裂 424 处 / 10 个目录 (+2)；…）
- **（夯）直到这个魅魔的心灵崩坏** — sev 3 — 建议全量肉眼（CG 族里没有明文图，只剩加密图 1580 个（.rpgmvp/.png_；另有引擎素材 0 张已剔除），机器读不到像素 (+5)）
- **（牛）间谍任务** — sev 3 — 建议全量肉眼（空白帧 41/300 (14%) (+2)；尺寸离群 2/3076 (0.1%) (+1)；重复冗余率 67.7%（近似重复 50 组） (+2)；<5KB 小文件 72/3076 (2.3%) (+1)；序列断裂 24 处 / 3 个目录 (+2)；另有引擎素材 845 张（tilesets/characters …）
- **女大学生 DX(8)** — sev 2 — 抽检 30 张即可（空白帧 8/300 (2.7%) (+1)；尺寸离群 146/1404 (10.4%) (+1)；重复冗余率 43.0%（近似重复 54 组） (+1)；序列断裂 10 处 / 1 个目录 (+1)；另有引擎素材 374 张（tilesets/characters 等，已剔除，见 families））
- **（一般）命に代えてもお守りします!** — sev 2 — 抽检 30 张即可（尺寸离群 31/152 (20%) (+2)；重复帧 20 组，冗余率 32.2% (+1)；另有引擎素材 513 张（tilesets/characters 等，已剔除，见 families））
- **(中等)魔法是无所不能** — sev 1 — 抽检 30 张即可（无明显异常；加密图 4535 个未计入（明文只 4 张） (+2)；另有引擎素材 2 张（tilesets/characters 等，已剔除，见 families））
- **对魔战士美月(** — sev 1 — 抽检 30 张即可（无明显异常；加密图 1277 个未计入（明文只 15 张） (+2)；另有引擎素材 5 张（tilesets/characters 等，已剔除，见 families））
- **星天之魔女（Arpg）** — sev 1 — 抽检 30 张即可（无明显异常；加密图 3271 个未计入（明文只 1 张） (+2)；另有引擎素材 2 张（tilesets/characters 等，已剔除，见 families））

## 图片族分布（每款前 6 族）
- **(中等)The Forbidden Tomes of Olipos**：system 2(素材)
- **(夯)魔塔**：The Dungeon of Lulu Farea Rebirth v1.01/www/img/system 2(素材)、The Dungeon of Lulu Farea/www/img/system 2(素材)
- **(牛)RJ01618221**：characters 86(素材)、pictures/02 オナニーと手マン 83、pictures/立ち絵 82、tilesets 59(素材)、pictures/12 酒場1 50、pictures/22 酒場VIP4P 34、…等 57 族
- **(牛)RJ01680223**：system 2(素材)
- **Chronicles of Clams Island**：pictures 829、characters 366(素材)、tilesets 268(素材)、animations 117(素材)、enemies 101(素材)、battlebacks1 79(素材)、…等 15 族
- **RA26037409(**：system 2(素材)
- **Restore(**：pictures 1020、tilesets 265(素材)、characters 251(素材)、enemies 133(素材)、sv_enemies 105(素材)、battlebacks1 64(素材)、…等 16 族
- **Risty and the Village of Bocchino v1.0.0**：characters 469(素材)、pictures 288、tilesets 182(素材)、animations 117(素材)、system 22(素材)、parallaxes 15(素材)、…等 7 族
- **Scars of Summer After v1.01（牛）**：system 2(素材)
- **Unholy_maiden_Ver.1.0.10**：(无)
- **ayuureiCN（夯）**：system 2(素材)
- **人柱阿拉卡 v230216(创意)**：人柱阿拉卡~官方中文版~遊戲本體/www/img/characters 5(素材)、人柱阿拉卡~官方中文版~遊戲本體/www/img/system 2(素材)、人柱阿拉卡~官方中文版~遊戲本體/www/img/tilesets 1(素材)
- **呪いの鎧 v3.0.1（夯）**：battlebacks1 50(素材)、system 2(素材)、titles1 2(素材)
- **女主角（大雷）**：pictures 715、animations 117(素材)、tilesets 31(素材)、system 18(素材)、titles2 5(素材)、titles1 4(素材)、…等 7 族
- **妹神官（牛）**：system 2(素材)
- **希尔丝大冒险  1.06（夯）**：system 2(素材)
- **忍耐色诱的告白游戏 V2.0（不赖）**：system 2(素材)
- **时间勇者（中等）**：system 2(素材)
- **栩的人生已经没救了（一般）**：pictures 581、pictures/busts 354、characters/nona 238(素材)、tilesets 172(素材)、animations 171(素材)、characters 140(素材)、…等 14 族
- **气息（上等）**：Special__actionSeq 44817(素材)、pictures 1521、Menu__layer_gif 901(素材)、characters 550(素材)、layers 409(素材)、tilesets 374(素材)、…等 38 族
- **苍红界域（夯）**：system 2(素材)
- **裏垢JKは露出でバズりたい（一般）**：内嵌/img/pictures/ホーム画面UI達 1743(素材)、外挂/Game/img/pictures/ホーム画面UI達 1743(素材)、内嵌/img/tilesets 317(素材)、外挂/Game/img/tilesets 317(素材)、内嵌/img/pictures/立ち絵 296、外挂/Game/img/pictures/立ち絵 296、…等 46 族
- **詛咒铠甲2（夯）**：battlebacks1 50(素材)、system 2(素材)、titles1 1(素材)
- **魔法学園見聞録**：system 2(素材)、enemies 1(素材)
- **魔法少女木叶  v2.04（中上）**：animations 627(素材)、characters 404(素材)、sv_enemies 342(素材)、spines/Battle 320(素材)、pictures 276、parallaxes 197(素材)、…等 89 族
- **（一般） 电脑 冒险者莉扎西亚 冒険者リザシア -葛藤と屈辱の日間-**：system 2(素材)
- **（一般）Shitsuke-ai_dlsite**：pictures/CG 781、enemies 187(素材)、pictures 127、system/Novel_UI 118(素材)、sv_enemies 105(素材)、characters 73(素材)、…等 30 族
- **（上等）NTRエロトラップダンジョン**：pictures 160、characters 110(素材)、tilesets 62(素材)、pictures/UI sozai 31(素材)、pictures/massage-window 28(素材)、parallaxes 22(素材)、…等 21 族
- **（上等）哈尔法斯的恶魔**：system 2(素材)
- **（中等）NTR Hunter**：tilesets 92(素材)、pictures 91、characters 79(素材)、sv_actors 41(素材)、parallaxes 21(素材)、faces 15(素材)、…等 34 族
- **（夯）直到这个魅魔的心灵崩坏**：(无)
- **（牛）间谍任务**：pictures 3076、tilesets 215(素材)、characters 188(素材)、animations 118(素材)、enemies 72(素材)、sv_enemies 70(素材)、…等 14 族
- **女大学生 DX(8)**：pictures/event scene 693、pictures/recollection_mode 604、tilesets 316(素材)、pictures/ero_status 45、pictures/thumbnail 40、pictures/BackView 21、…等 13 族
- **（一般）命に代えてもお守りします!**：pictures 143、animations 117(素材)、enemies 78(素材)、sv_enemies 70(素材)、battlebacks1 50(素材)、battlebacks2 50(素材)、…等 20 族
- **(中等)魔法是无所不能**：special 4、system 2(素材)
- **对魔战士美月(**：characters 3(素材)、system 2(素材)、live2d/00_akari_opening 1、live2d/22_mizuki 1、live2d/22_seciria 1、live2d/34 1、…等 17 族
- **星天之魔女（Arpg）**：system 2(素材)、pictures 1

## 加密款（机器读不到像素）

| 游戏 | 加密图 | 明文图 | 扫描根 |
| --- | ---: | ---: | --- |
| (中等)The Forbidden Tomes of Olipos | 1957 | 0 | www/img |
| (夯)魔塔 | 2409 | 0 | The Dungeon of Lulu Farea/www/img、The Dungeon of Lulu Farea Rebirth v1.01/www/img |
| (牛)RJ01680223 | 2461 | 0 | www/img |
| RA26037409( | 324 | 0 | www/img |
| Scars of Summer After v1.01（牛） | 3987 | 0 | www/img |
| Unholy_maiden_Ver.1.0.10 | 4182 | 0 | img |
| ayuureiCN（夯） | 1726 | 0 | www/img |
| 人柱阿拉卡 v230216(创意) | 1911 | 0 | 人柱阿拉卡~官方中文版~遊戲本體/www/img、人柱阿拉卡~简体中文更新档/www/img |
| 呪いの鎧 v3.0.1（夯） | 3385 | 0 | www/img |
| 妹神官（牛） | 5894 | 0 | www/img |
| 希尔丝大冒险  1.06（夯） | 1352 | 0 | www/img |
| 忍耐色诱的告白游戏 V2.0（不赖） | 1332 | 0 | www/img |
| 时间勇者（中等） | 4530 | 0 | 时勇者/www/img |
| 苍红界域（夯） | 8224 | 0 | MELFIAS －Azure and Crimson Verge－/www/img |
| 詛咒铠甲2（夯） | 18212 | 0 | www/img |
| 魔法学園見聞録 | 1475 | 0 | www/img |
| （一般） 电脑 冒险者莉扎西亚 冒険者リザシア -葛藤と屈辱の日間- | 662 | 0 | 冒険者リザシア/www/img |
| （上等）哈尔法斯的恶魔 | 2865 | 0 | www/img |
| （夯）直到这个魅魔的心灵崩坏 | 1580 | 0 | img |
| (中等)魔法是无所不能 | 4535 | 4 | 魔法/www/img |
| 对魔战士美月( | 1277 | 15 | www/img |
| 星天之魔女（Arpg） | 3271 | 1 | www/img |

## severity=3 具体文件

### (中等)The Forbidden Tomes of Olipos（图 0 张，扫到 2 张，加密 1957 个）
- 目录：E:\Personal_entertainment\game\ero\egame\20260818\RPG\中等\(中等)The Forbidden Tomes of Olipos

### (夯)魔塔（图 0 张，扫到 4 张，加密 2409 个）
- 目录：E:\Personal_entertainment\game\ero\egame\20260818\RPG\夯\(夯)魔塔
- 备注：扫到 2 个 img 根，两套目录各存一份图，重复率会被抬高

### (牛)RJ01618221（图 706 张，扫到 938 张，加密 0 个）
- 目录：E:\Personal_entertainment\game\ero\egame\20260930\RPG\未评级\(牛)RJ01618221
- 空白/解码异常（共 1 张，列举前 1）：
  - img/pictures/背景/黒背景.png (var=0.0)
- 近似重复组（共 52 组，列举前 8 组）：
  - `img/pictures/12 酒場1/背景無し/酒場 (20).png` = `img/pictures/12 酒場1/背景無し/酒場 (22).png` = `img/pictures/12 酒場1/背景無し/酒場 (26).png` = `img/pictures/12 酒場1/背景無し/酒場 (31).png` = `img/pictures/12 酒場1/背景無し/酒場 (7).png` = `img/pictures/12 酒場1/背景無し/酒場 (8).png`
  - `img/pictures/立ち絵/立ち絵_バニー/arina_バニー_困り_照れ.png` = `img/pictures/立ち絵/立ち絵_バニー/arina_バニー_困り笑顔照れ.png` = `img/pictures/立ち絵/立ち絵_バニー/arina_バニー_怒り.png` = `img/pictures/立ち絵/立ち絵_バニー/arina_バニー_笑顔.png` = `img/pictures/立ち絵/立ち絵_バニー/arina_バニー_通常.png` = `img/pictures/立ち絵/立ち絵_服/arina_服_笑顔_妊婦.png`
  - `img/pictures/01 正常位/1 アリナ 正常位１－1_0010.png` = `img/pictures/01 正常位/1 アリナ 正常位１－1_0011.png` = `img/pictures/01 正常位/1 アリナ 正常位１－1_0012.png` = `img/pictures/01 正常位/1 アリナ 正常位１－1_0013.png` = `img/pictures/01 正常位/1 アリナ 正常位１－1_007.png`
  - `img/pictures/02 オナニーと手マン/アリナオナニー１ー4_020.png` = `img/pictures/02 オナニーと手マン/アリナオナニー１ー4_028.png` = `img/pictures/02 オナニーと手マン/アリナオナニー１ー4_031.png` = `img/pictures/02 オナニーと手マン/アリナオナニー１ー4_032.png` = `img/pictures/02 オナニーと手マン/アリナオナニー１ー4_043.png`
  - `img/pictures/12 酒場1/背景無し/酒場 (1).png` = `img/pictures/12 酒場1/背景無し/酒場 (15).png` = `img/pictures/12 酒場1/背景無し/酒場 (17).png` = `img/pictures/12 酒場1/背景無し/酒場 (19).png` = `img/pictures/12 酒場1/背景無し/酒場 (2).png`
  - `img/pictures/43 娼婦ダンス改/アリナ＿ダンス改1-3.png` = `img/pictures/43 娼婦ダンス改/アリナ＿ダンス改1-4.png` = `img/pictures/43 娼婦ダンス改/アリナ＿ダンス改3.png` = `img/pictures/43 娼婦ダンス改/アリナ＿ダンス改7-3.png` = `img/pictures/43 娼婦ダンス改/アリナ＿ダンス改7.png`
  - `img/pictures/立ち絵/立ち絵_服/arina_服_泣き笑い.png` = `img/pictures/立ち絵/立ち絵_服/arina_服_笑顔.png` = `img/pictures/立ち絵/立ち絵_服/arina_服_通常.png` = `img/pictures/立ち絵/立ち絵_服/arina_服_通常_照れ.png` = `img/pictures/立ち絵/立ち絵_服/arina_服_驚き_照れ.png`
  - `img/pictures/隠れNTRエンド/隠れNTRエンド2_001.png` = `img/pictures/隠れNTRエンド/隠れNTRエンド2_003.png` = `img/pictures/隠れNTRエンド/隠れNTRエンド2_003_催眠.png` = `img/pictures/隠れNTRエンド/隠れNTRエンド2_004.png` = `img/pictures/隠れNTRエンド/隠れNTRエンド2_004_催眠.png`
- 尺寸离群（共 55 张），尺寸分布：374x468 x12、467x112 x9、419x468 x7、632x468 x6、504x700 x5
  - img/pictures/02 オナニーと手マン/服あり/_一覧.png (1500x1184)
  - img/pictures/37 追加_戦闘手つかみ/アリナ_手つかみ_仮.png (4359x3864)
  - img/pictures/38 断面図/断面図_01_未挿入.png (456x305)
  - img/pictures/38 断面図/断面図_02_未挿入_濡れ.png (456x305)
  - img/pictures/38 断面図/断面図_03_手マン.png (456x305)
  - img/pictures/38 断面図/断面図_04_ちんぽ挿入.png (456x305)
  - img/pictures/39 後ろ蹲踞/アリナ_後ろ蹲踞_02_手マン.png (640x468)
  - img/pictures/39 後ろ蹲踞/アリナ_後ろ蹲踞_05_手マン_汁_動き.png (640x468)
  - img/pictures/39 後ろ蹲踞/アリナ_後ろ蹲踞_06_絶頂_潮吹き.png (640x468)
  - img/pictures/40 手マンクンニ/アリナ_手マンクンニ1.png (419x468)
- 体积异常（共 1 张，列举前 1）：
  - img/pictures/メニュー画面_素材/whiteButton.png (0.8KB)

### (牛)RJ01680223（图 0 张，扫到 2 张，加密 2461 个）
- 目录：E:\Personal_entertainment\game\ero\egame\20260820\RPG\未评级\(牛)RJ01680223

### Chronicles of Clams Island（图 842 张，扫到 1993 张，加密 0 个）
- 目录：E:\Personal_entertainment\game\ero\egame\20260820\RPG\未评级\Chronicles of Clams Island
- 空白/解码异常（共 8 张，列举前 8）：
  - www/img/pictures/BDCG2-レディスライムZ.png (var=15.8)
  - www/img/pictures/BHCG-1BハウンドZ.png (var=18.7)
  - www/img/pictures/Hステータス立ち絵ザーメン0.png (var=0.0)
  - www/img/pictures/amyF17.png (var=5.9)
  - www/img/pictures/galleryチェック1.png (var=1.7)
  - www/img/pictures/menu冒険者ランク0.png (var=0.0)
  - www/img/pictures/oliviaF10.png (var=10.4)
  - www/img/pictures/oliviaF5.png (var=1.5)
- 近似重复组（共 52 组，列举前 8 组）：
  - `www/img/pictures/Hステータス立ち絵ザーメン0.png` = `www/img/pictures/PSCおまんこ敏感lv1.png` = `www/img/pictures/PSCアナル敏感lv2.png` = `www/img/pictures/PSFゴブリンの玩具.png` = `www/img/pictures/PSFレイプ願望.png` = `www/img/pictures/PS心理1.png` = `…等 18 张`
  - `www/img/pictures/amyF11.png` = `www/img/pictures/amyF17.png` = `www/img/pictures/amyF18.png` = `www/img/pictures/amyF21.png` = `www/img/pictures/amyF6.png` = `www/img/pictures/amyF7.png`
  - `www/img/pictures/questorder_paper13.png` = `www/img/pictures/questorder_paper22.png` = `www/img/pictures/questorder_paper26.png` = `www/img/pictures/questorder_paper41.png` = `www/img/pictures/questorder_paper43.png` = `www/img/pictures/questorder_paper46.png`
  - `www/img/pictures/BHCGｰ4B取り憑かれた冒険者2.png` = `www/img/pictures/BHCGｰ4B取り憑かれた冒険者3.png` = `www/img/pictures/BHCGｰ4B取り憑かれた冒険者3ex.png` = `www/img/pictures/BHCGｰ4B取り憑かれた冒険者6ex.png` = `www/img/pictures/BHCGｰ4B取り憑かれた冒険者7ex.png`
  - `www/img/pictures/BPmadman10.png` = `www/img/pictures/BPmadman30.png` = `www/img/pictures/BPmadman3Z.png` = `www/img/pictures/BPmadmanB10.png` = `www/img/pictures/BPmadmanB1Z.png`
  - `www/img/pictures/oliviaF10.png` = `www/img/pictures/oliviaF4.png` = `www/img/pictures/oliviaF5.png` = `www/img/pictures/oliviaF6.png` = `www/img/pictures/oliviaF7.png`
  - `www/img/pictures/BHCG-2スライム3B.png` = `www/img/pictures/BHCG-2スライム4Bex.png` = `www/img/pictures/BHCG-2スライム5Bex.png` = `www/img/pictures/BHCG-2スライム6Bex.png`
  - `www/img/pictures/BPorc1Z.png` = `www/img/pictures/BPorc20.png` = `www/img/pictures/BPorcB10.png` = `www/img/pictures/BPorcB1Z.png`
- 尺寸离群（共 106 张），尺寸分布：389x624 x27、195x33 x20、287x624 x15、391x624 x15、86x84 x9
  - www/img/bosshp/Boss_HP_A.png (482x29)
  - www/img/bosshp/Boss_HP_B.png (464x22)
  - www/img/bosshp/Boss_HP_C.png (200x28)
  - www/img/bosshp/Boss_HP_D.png (29x26)
  - www/img/bosshp/Face_70.png (86x84)
  - www/img/bosshp/Face_71.png (86x84)
  - www/img/bosshp/Face_72.png (86x84)
  - www/img/bosshp/Face_73.png (86x84)
  - www/img/bosshp/Face_74.png (86x84)
  - www/img/bosshp/Face_78.png (86x84)
- 极小图（共 26 张，列举前 10）：
  - www/img/bosshp/Boss_HP_A.png
  - www/img/bosshp/Boss_HP_B.png
  - www/img/bosshp/Boss_HP_C.png
  - www/img/bosshp/Boss_HP_D.png
  - www/img/pictures/GaugeFrame.png
  - www/img/pictures/GoldWindowPictue.png
  - www/img/pictures/questchoice_11.png
  - www/img/pictures/questchoice_12.png
  - www/img/pictures/questchoice_13.png
  - www/img/pictures/questchoice_14.png
- 体积异常（共 32 张，列举前 10）：
  - www/img/bosshp/Boss_HP_A.png (4.2KB)
  - www/img/bosshp/Boss_HP_B.png (4.2KB)
  - www/img/pictures/Black2.png (2.4KB)
  - www/img/pictures/FaceLayer1.png (3.1KB)
  - www/img/pictures/GaugeFrame.png (1.9KB)
  - www/img/pictures/Window_Original.png (2.4KB)
  - www/img/pictures/amyF12.png (3.8KB)
  - www/img/pictures/amyF17.png (5.0KB)
  - www/img/pictures/amyZ1.png (2.7KB)
  - www/img/pictures/menu冒険者ランク0.png (3.8KB)

### RA26037409(（图 0 张，扫到 2 张，加密 324 个）
- 目录：E:\Personal_entertainment\game\ero\egame\20260420\RPG\未评级\RA26037409(

### Restore(（图 1072 张，扫到 2061 张，加密 0 个）
- 目录：E:\Personal_entertainment\game\ero\egame\20260420\RPG\未评级\Restore(
- 空白/解码异常（共 7 张，列举前 7）：
  - img/actorhud/Face_1.png (var=0.0)
  - img/actorhud/Face_2.png (var=0.0)
  - img/actorhud/Face_3.png (var=0.0)
  - img/actorhud/Face_4.png (var=0.0)
  - img/actorhud/HP_Meter.png (var=3.3)
  - img/battlehud/ATB_Meter.png (var=0.0)
  - img/pictures/fe-do.png (var=0.0)
- 近似重复组（共 40 组，列举前 8 组）：
  - `img/pictures/tk emiri 16.png` = `img/pictures/tk emiri 19.png` = `img/pictures/tk emiri 20.png` = `img/pictures/tk emiri　1.png` = `img/pictures/tk emiri　3.png` = `img/pictures/tk emiri　4.png` = `…等 25 张`
  - `img/pictures/item HPring.png` = `img/pictures/item ba-sa-ka-ring.png` = `img/pictures/item bannouyaku.png` = `img/pictures/item binbin.png` = `img/pictures/item buri-fu.png` = `img/pictures/item doroppu.png` = `…等 17 张`
  - `img/pictures/fe-do.png` = `img/pictures/h op 2.png` = `img/pictures/tk maou 4.png` = `img/pictures/tk mika　2.png` = `img/pictures/tk mika　6.png` = `img/pictures/tk mika　8.png` = `…等 11 张`
  - `img/pictures/t emiri 1280 10.png` = `img/pictures/t emiri 1280 15.png` = `img/pictures/t emiri 1280 20.png` = `img/pictures/t emiri 1280 3.png` = `img/pictures/t emiri 1280 4.png` = `img/pictures/t emiri 1280 5.png` = `…等 9 张`
  - `img/actorhud/Face_1.png` = `img/actorhud/Face_2.png` = `img/actorhud/Face_3.png` = `img/actorhud/Face_4.png`
  - `img/pictures/h scene azuha tyoukyou 57.png` = `img/pictures/h scene azuha tyoukyou 58.png` = `img/pictures/h scene azuha tyoukyou 59.png` = `img/pictures/h scene azuha tyoukyou 61.png`
  - `img/pictures/t karudlie 1280 4.png` = `img/pictures/t karudlie 1280 5.png` = `img/pictures/t karudlie 1280 6.png` = `img/pictures/t karudlie 1280 9.png`
  - `img/pictures/tk a syouiti 4.png` = `img/pictures/tk a syouiti 6.png` = `img/pictures/tk a syouiti 7.png` = `img/pictures/tk a syouiti n.png`
- 尺寸离群（共 274 张），尺寸分布：640x720 x201、190x158 x14、160x80 x12、500x250 x7、169x19 x6
  - img/actorhud/EXP_Meter.png (73x6)
  - img/actorhud/Face_1.png (190x158)
  - img/actorhud/Face_2.png (190x158)
  - img/actorhud/Face_3.png (190x158)
  - img/actorhud/Face_4.png (190x158)
  - img/actorhud/Face_5.png (190x158)
  - img/actorhud/Face_7.png (150x165)
  - img/actorhud/Face_8.png (190x158)
  - img/actorhud/HP_Meter.png (164x36)
  - img/actorhud/HP_Number.png (169x19)
- 极小图（共 23 张，列举前 10）：
  - img/actorhud/EXP_Meter.png
  - img/actorhud/HP_Meter.png
  - img/actorhud/HP_Number.png
  - img/actorhud/HP_Number2.png
  - img/actorhud/LV_Number.png
  - img/actorhud/MP_Meter.png
  - img/actorhud/MP_Number.png
  - img/actorhud/MP_Number2.png
  - img/actorhud/TP_Meter.png
  - img/actorhud/TP_Number.png
- 体积异常（共 32 张，列举前 10）：
  - img/actorhud/EXP_Meter.png (0.2KB)
  - img/actorhud/Face_1.png (0.2KB)
  - img/actorhud/Face_2.png (0.2KB)
  - img/actorhud/Face_3.png (0.2KB)
  - img/actorhud/Face_4.png (0.2KB)
  - img/actorhud/Face_5.png (0.2KB)
  - img/actorhud/Face_8.png (0.2KB)
  - img/actorhud/HP_Meter.png (0.4KB)
  - img/actorhud/MP_Meter.png (0.3KB)
  - img/actorhud/TP_Meter.png (0.3KB)

### Risty and the Village of Bocchino v1.0.0（图 288 张，扫到 1094 张，加密 0 个）
- 目录：E:\Personal_entertainment\game\ero\egame\20260303\RPG\未评级\Risty and the Village of Bocchino v1.0.0
- 空白/解码异常（共 1 张，列举前 1）：
  - www/img/pictures/blackBG.png (var=0.0)
- 近似重复组（共 30 组，列举前 8 组）：
  - `www/img/pictures/r0_0.png` = `www/img/pictures/r0_1.png` = `www/img/pictures/r0_10.png` = `www/img/pictures/r0_11.png` = `www/img/pictures/r0_12.png` = `www/img/pictures/r0_13.png` = `…等 100 张`
  - `www/img/pictures/r1_0.png` = `www/img/pictures/r1_1.png` = `www/img/pictures/r1_10.png` = `www/img/pictures/r1_11.png` = `www/img/pictures/r1_12.png` = `www/img/pictures/r1_13.png` = `…等 51 张`
  - `www/img/pictures/o2-06.png` = `www/img/pictures/o2-07.png` = `www/img/pictures/o2-08.png` = `www/img/pictures/o2-09.png` = `www/img/pictures/o2-10.png` = `www/img/pictures/o2-11.png` = `…等 9 张`
  - `www/img/pictures/munemomi1_01.png` = `www/img/pictures/munemomi1_02.png` = `www/img/pictures/munemomi1_03.png` = `www/img/pictures/munemomi1_04.png` = `www/img/pictures/munemomi2_01.png` = `www/img/pictures/munemomi2_02.png` = `…等 8 张`
  - `www/img/pictures/sl1_01.png` = `www/img/pictures/sl1_02.png` = `www/img/pictures/sl1_03.png` = `www/img/pictures/sl1_04.png` = `www/img/pictures/sl1_05.png` = `www/img/pictures/sl1_06.png` = `…等 7 张`
  - `www/img/pictures/s1_01.png` = `www/img/pictures/s1_02.png` = `www/img/pictures/s1_03.png` = `www/img/pictures/s1_04.png` = `www/img/pictures/s1_05.png` = `www/img/pictures/s1_06.png`
  - `www/img/pictures/kabe2-04.png` = `www/img/pictures/kabe2-05.png` = `www/img/pictures/kabe2-06.png` = `www/img/pictures/kabe2-07.png` = `www/img/pictures/kabe2-08.png`
  - `www/img/pictures/o2-01.png` = `www/img/pictures/o2-02.png` = `www/img/pictures/o2-03.png` = `www/img/pictures/o2-04.png` = `www/img/pictures/o2-05.png`
- 尺寸离群（共 131 张），尺寸分布：1280x720 x125、120x120 x4、1282x714 x1、1282x719 x1
  - www/img/pictures/bad-01.png (1280x720)
  - www/img/pictures/bad-02.png (1280x720)
  - www/img/pictures/bad-03.png (1280x720)
  - www/img/pictures/bad-04.png (1280x720)
  - www/img/pictures/bad-05.png (1280x720)
  - www/img/pictures/bad-06.png (1280x720)
  - www/img/pictures/bad-07.png (1280x720)
  - www/img/pictures/bad-08.png (1280x720)
  - www/img/pictures/bad-09.png (1280x720)
  - www/img/pictures/bad-10.png (1280x720)

### Scars of Summer After v1.01（牛）（图 0 张，扫到 2 张，加密 3987 个）
- 目录：E:\Personal_entertainment\game\ero\egame\20260425\RPG\未评级\Scars of Summer After v1.01（牛）

### Unholy_maiden_Ver.1.0.10（图 0 张，扫到 0 张，加密 4182 个）
- 目录：E:\Personal_entertainment\game\ero\egame\20260930\RPG\未评级\Unholy_maiden_Ver.1.0.10

### ayuureiCN（夯）（图 0 张，扫到 2 张，加密 1726 个）
- 目录：E:\Personal_entertainment\game\ero\egame\20260303\RPG\夯\ayuureiCN（夯）

### 人柱阿拉卡 v230216(创意)（图 0 张，扫到 8 张，加密 1911 个）
- 目录：E:\Personal_entertainment\game\ero\egame\20260811\其他\创意\人柱阿拉卡 v230216(创意)
- 备注：扫到 2 个 img 根，两套目录各存一份图，重复率会被抬高

### 呪いの鎧 v3.0.1（夯）（图 0 张，扫到 54 张，加密 3385 个）
- 目录：E:\Personal_entertainment\game\ero\egame\20260629\RPG\夯\呪いの鎧 v3.0.1（夯）

### 女主角（大雷）（图 715 张，扫到 893 张，加密 0 个）
- 目录：E:\Personal_entertainment\game\ero\egame\20260424\RPG\大雷\女主角（大雷）
- 空白/解码异常（共 4 张，列举前 4）：
  - www/img/pictures/BCE73_26.png (var=15.5)
  - www/img/pictures/メイン3part4.png (var=5.6)
  - www/img/pictures/メイン6part2.png (var=0.0)
  - www/img/pictures/背景_黒幕.png (var=0.0)
- 近似重复组（共 52 组，列举前 8 组）：
  - `www/img/pictures/heroine_angry.png` = `www/img/pictures/heroine_normalE.png` = `www/img/pictures/heroine_normalF.png` = `www/img/pictures/heroine_smile.png` = `www/img/pictures/heroine_talk1.png` = `www/img/pictures/heroine_talk1C.png` = `…等 10 张`
  - `www/img/pictures/NCE81_12.png` = `www/img/pictures/NCE81_2.png` = `www/img/pictures/NCE81_3.png` = `www/img/pictures/NCE81_4.png` = `www/img/pictures/NCE81_5.png` = `www/img/pictures/NCE81_6.png` = `…等 8 张`
  - `www/img/pictures/メイン3part3.png` = `www/img/pictures/メイン4part1.png` = `www/img/pictures/メイン5part3.png` = `www/img/pictures/メイン5part4.png` = `www/img/pictures/メイン6part2.png` = `www/img/pictures/メイン6part3.png` = `…等 8 张`
  - `www/img/pictures/BCE73_14.png` = `www/img/pictures/BCE73_16.png` = `www/img/pictures/BCE73_20.png` = `www/img/pictures/BCE73_21.png` = `www/img/pictures/BCE73_22.png` = `www/img/pictures/BCE73_23.png` = `…等 7 张`
  - `www/img/pictures/heroine_clothes.png` = `www/img/pictures/heroine_clothesA.png` = `www/img/pictures/heroine_clothesC.png` = `www/img/pictures/heroine_clothesD.png` = `www/img/pictures/heroine_clothesE.png` = `www/img/pictures/heroine_clothesF.png` = `…等 7 张`
  - `www/img/pictures/BCE73_1.png` = `www/img/pictures/BCE73_10.png` = `www/img/pictures/BCE73_11.png` = `www/img/pictures/BCE73_12.png` = `www/img/pictures/BCE73_13.png` = `www/img/pictures/BCE73_3.png`
  - `www/img/pictures/SCE83_2.png` = `www/img/pictures/SCE83_3.png` = `www/img/pictures/SCE83_5.png` = `www/img/pictures/SCE93_2.png` = `www/img/pictures/SCE93_5.png` = `www/img/pictures/SCE93_6.png`
  - `www/img/pictures/heroine_sex.png` = `www/img/pictures/heroine_sex1.png` = `www/img/pictures/heroine_sex3.png` = `www/img/pictures/heroine_sex4.png` = `www/img/pictures/heroine_sex5.png` = `www/img/pictures/heroine_sex6.png`
- 尺寸离群（共 274 张），尺寸分布：716x1200 x125、210x70 x78、895x1500 x43、1193x2000 x8、101x30 x6
  - www/img/pictures/UI_AUTO.png (101x30)
  - www/img/pictures/UI_EXIT.png (101x30)
  - www/img/pictures/UI_EXIT2.png (150x45)
  - www/img/pictures/UI_LOAD.png (101x30)
  - www/img/pictures/UI_LOAD2.png (150x45)
  - www/img/pictures/UI_SAVE.png (101x30)
  - www/img/pictures/UI_SAVE2.png (150x45)
  - www/img/pictures/UI_SKIP.png (101x30)
  - www/img/pictures/UI_SYSTEM.png (101x30)
  - www/img/pictures/UI_SYSTEM2.png (150x45)
- 极小图（共 13 张，列举前 10）：
  - www/img/pictures/UI_AUTO.png
  - www/img/pictures/UI_EXIT.png
  - www/img/pictures/UI_EXIT2.png
  - www/img/pictures/UI_LOAD.png
  - www/img/pictures/UI_LOAD2.png
  - www/img/pictures/UI_SAVE.png
  - www/img/pictures/UI_SAVE2.png
  - www/img/pictures/UI_SKIP.png
  - www/img/pictures/UI_SYSTEM.png
  - www/img/pictures/UI_SYSTEM2.png
- 体积异常（共 30 张，列举前 10）：
  - www/img/pictures/UI_AUTO.png (3.6KB)
  - www/img/pictures/UI_EXIT.png (3.6KB)
  - www/img/pictures/UI_LOAD.png (3.6KB)
  - www/img/pictures/UI_SAVE.png (3.8KB)
  - www/img/pictures/UI_SKIP.png (3.6KB)
  - www/img/pictures/UI_SYSTEM.png (4.0KB)
  - www/img/pictures/シナリオ0.png (2.0KB)
  - www/img/pictures/シナリオ00.png (1.9KB)
  - www/img/pictures/シナリオ012.png (5.0KB)
  - www/img/pictures/シナリオA.png (2.2KB)

### 妹神官（牛）（图 0 张，扫到 2 张，加密 5894 个）
- 目录：E:\Personal_entertainment\game\ero\egame\20260425\RPG\未评级\妹神官（牛）

### 希尔丝大冒险  1.06（夯）（图 0 张，扫到 2 张，加密 1352 个）
- 目录：E:\Personal_entertainment\game\ero\egame\20260625\RPG\夯\希尔丝大冒险  1.06（夯）

### 忍耐色诱的告白游戏 V2.0（不赖）（图 0 张，扫到 2 张，加密 1332 个）
- 目录：E:\Personal_entertainment\game\ero\egame\20260613\RPG\不赖\忍耐色诱的告白游戏 V2.0（不赖）

### 时间勇者（中等）（图 0 张，扫到 2 张，加密 4530 个）
- 目录：E:\Personal_entertainment\game\ero\egame\20260904\RPG\中等\时间勇者（中等）

### 栩的人生已经没救了（一般）（图 935 张，扫到 2005 张，加密 0 个）
- 目录：E:\Personal_entertainment\game\ero\egame\20260613\RPG\一般\栩的人生已经没救了（一般）
- 空白/解码异常（共 18 张，列举前 10）：
  - www/img/pictures/!makkuro.png (var=0.0)
  - www/img/pictures/!yoru.png (var=0.0)
  - www/img/pictures/Mgaku2.png (var=6.4)
  - www/img/pictures/_LV1.png (var=0.0)
  - www/img/pictures/_LV8.png (var=0.0)
  - www/img/pictures/_bunbo35.png (var=0.0)
  - www/img/pictures/_bunbo55.png (var=0.0)
  - www/img/pictures/_bunsi00.png (var=0.0)
  - www/img/pictures/_bunsi03.png (var=0.0)
  - www/img/pictures/_bunsi08.png (var=0.0)
- 近似重复组（共 20 组，列举前 8 组）：
  - `www/img/pictures/busts/kunu/bat_aiso.png` = `www/img/pictures/busts/kunu/bat_default.png` = `www/img/pictures/busts/kunu/bat_fu-n.png` = `www/img/pictures/busts/kunu/bat_guru.png` = `www/img/pictures/busts/kunu/bat_hazu.png` = `www/img/pictures/busts/kunu/bat_komari.png` = `…等 91 张`
  - `www/img/pictures/!makkuro.png` = `www/img/pictures/!yoru.png` = `www/img/pictures/A.png` = `www/img/pictures/Dname_ekimae.png` = `www/img/pictures/Dname_kouen.png` = `www/img/pictures/Mgaku2.png` = `…等 20 张`
  - `www/img/pictures/busts/m05/normal_default.png` = `www/img/pictures/busts/m08/normal_default.png` = `www/img/pictures/busts/m09/normal_default.png` = `www/img/pictures/busts/mama/normal_default.png`
  - `www/img/pictures/busts/nona予備/normal_doka.png` = `www/img/pictures/busts/nona予備/normal_miku.png` = `www/img/pictures/busts/nona予備/normal_niko.png` = `www/img/pictures/busts/nona予備/normal_sin.png`
  - `www/img/pictures/sede1_02.png` = `www/img/pictures/sede1_04.png` = `www/img/pictures/sede1_05.png` = `www/img/pictures/sede1_08.png`
  - `www/img/pictures/aura.png` = `www/img/pictures/aura_r.png` = `www/img/pictures/busts/aura/normal.png`
  - `www/img/pictures/haga_5.png` = `www/img/pictures/haga_6.png` = `www/img/pictures/haga_8.png`
  - `www/img/pictures/B.png` = `www/img/pictures/C.png`
- 尺寸离群（共 397 张），尺寸分布：422x624 x293、422x732 x44、352x674 x16、116x116 x11、192x192 x7
  - www/img/pictures/BoxA.png (96x96)
  - www/img/pictures/BoxA_waifu2x_art_noise3_scale.png (192x192)
  - www/img/pictures/BoxB.png (96x96)
  - www/img/pictures/BoxB_waifu2x_art_noise3_scale.png (192x192)
  - www/img/pictures/BoxC.png (96x96)
  - www/img/pictures/BoxC_waifu2x_art_noise3_scale.png (192x192)
  - www/img/pictures/_HPgauge2.png (141x13)
  - www/img/pictures/_HPgaugeRED.png (141x13)
  - www/img/pictures/_HPgaugeTP.png (141x13)
  - www/img/pictures/aura.png (422x624)
- 极小图（共 3 张，列举前 3）：
  - www/img/pictures/_HPgauge2.png
  - www/img/pictures/_HPgaugeRED.png
  - www/img/pictures/_HPgaugeTP.png
- 体积异常（共 73 张，列举前 10）：
  - www/img/pictures/!makkuro.png (2.9KB)
  - www/img/pictures/!yoru.png (2.9KB)
  - www/img/pictures/_HPgauge.png (3.2KB)
  - www/img/pictures/_HPgauge2.png (0.3KB)
  - www/img/pictures/_HPgaugeRED.png (0.3KB)
  - www/img/pictures/_HPgaugeTP.png (0.3KB)
  - www/img/pictures/_HPgaugeWAKU.png (3.7KB)
  - www/img/pictures/_LV0.png (3.2KB)
  - www/img/pictures/_LV1.png (3.0KB)
  - www/img/pictures/_LV2.png (3.2KB)

### 气息（上等）（图 1521 张，扫到 49775 张，加密 0 个）
- 目录：E:\Personal_entertainment\game\ero\egame\20260612\RPG\上等\气息（上等）
- 空白/解码异常（共 38 张，列举前 10）：
  - SAO_verβ7/www/img/pictures/384白边框 - 副本.png (var=0.0)
  - SAO_verβ7/www/img/pictures/FOG爱丽丝催眠发情.png (var=18.6)
  - SAO_verβ7/www/img/pictures/FOG爱丽丝催眠回应.png (var=21.0)
  - SAO_verβ7/www/img/pictures/FOG爱丽丝催眠得意.png (var=18.4)
  - SAO_verβ7/www/img/pictures/FOG爱丽丝催眠愉悦发情.png (var=19.2)
  - SAO_verβ7/www/img/pictures/FOG爱丽丝催眠服从.png (var=18.9)
  - SAO_verβ7/www/img/pictures/FOG爱丽丝催眠温柔脸红.png (var=18.4)
  - SAO_verβ7/www/img/pictures/FOG爱丽丝催眠脸红.png (var=19.0)
  - SAO_verβ7/www/img/pictures/FOG爱丽丝斥责脸红.png (var=22.1)
  - SAO_verβ7/www/img/pictures/FOG爱丽丝鄙夷脸红.png (var=22.1)
- 近似重复组（共 13 组，列举前 8 组）：
  - `SAO_verβ7/www/img/pictures/FOG明日奈严肃脸红.png` = `SAO_verβ7/www/img/pictures/FOG明日奈失望.png` = `SAO_verβ7/www/img/pictures/FOG明日奈接吻.png` = `SAO_verβ7/www/img/pictures/FOG明日奈无奈.png` = `SAO_verβ7/www/img/pictures/FOG明日奈激动.png` = `SAO_verβ7/www/img/pictures/FOG明日奈理解.png` = `…等 164 张`
  - `SAO_verβ7/www/img/pictures/FOG莉兹凛然.png` = `SAO_verβ7/www/img/pictures/FOG莉兹凛然脸红.png` = `SAO_verβ7/www/img/pictures/FOG莉兹嘲弄.png` = `SAO_verβ7/www/img/pictures/FOG莉兹微笑.png` = `SAO_verβ7/www/img/pictures/FOG莉兹批评.png` = `SAO_verβ7/www/img/pictures/FOG莉兹温柔.png` = `…等 21 张`
  - `SAO_verβ7/www/img/pictures/FOG莉兹.png` = `SAO_verβ7/www/img/pictures/FOG莉兹对魔忍.png` = `SAO_verβ7/www/img/pictures/fog亚丝娜裸.png` = `SAO_verβ7/www/img/pictures/fog莉兹内衣.png` = `SAO_verβ7/www/img/pictures/立绘-妓女亚2-为难.png` = `SAO_verβ7/www/img/pictures/立绘-妓女亚2-微笑.png` = `…等 11 张`
  - `SAO_verβ7/www/img/pictures/FOG诗乃立绘.png` = `SAO_verβ7/www/img/pictures/明日奈双马尾体操服立绘.png` = `SAO_verβ7/www/img/pictures/明日奈居家服立绘.png` = `SAO_verβ7/www/img/pictures/明日奈浴巾.png` = `SAO_verβ7/www/img/pictures/诗乃伪装服.png`
  - `SAO_verβ7/www/img/pictures/0717-契约反转-吃惊.png` = `SAO_verβ7/www/img/pictures/0717-契约反转-思考.png` = `SAO_verβ7/www/img/pictures/0717-契约反转-愤怒.png` = `SAO_verβ7/www/img/pictures/0717-契约反转-齁哦.png`
  - `SAO_verβ7/www/img/pictures/SYNCHRONIZED.png` = `SAO_verβ7/www/img/pictures/教程1.png` = `SAO_verβ7/www/img/pictures/猪田手机-日记.png` = `SAO_verβ7/www/img/pictures/透视镜-渐变异形椭圆形1440x1008.png`
  - `SAO_verβ7/www/img/pictures/京子cos.png` = `SAO_verβ7/www/img/pictures/莉兹泳装.png` = `SAO_verβ7/www/img/pictures/莉兹私服-黑渐变.png`
  - `SAO_verβ7/www/img/pictures/716-浴室-傲娇.png` = `SAO_verβ7/www/img/pictures/716-浴室-叹气.png`
- 尺寸离群（共 127 张），尺寸分布：336x240 x25、800x1008 x18、1024x768 x15、138x120 x11、433x817 x7
  - SAO_verβ7/www/img/pictures/384白边框 - 副本.png (384x384)
  - SAO_verβ7/www/img/pictures/384白边框.png (384x384)
  - SAO_verβ7/www/img/pictures/716-浴室-猪田炫耀自拍.png (433x775)
  - SAO_verβ7/www/img/pictures/AVntrs.png (1024x768)
  - SAO_verβ7/www/img/pictures/AV亚丝娜.png (1024x768)
  - SAO_verβ7/www/img/pictures/AV精灵母女.png (1024x768)
  - SAO_verβ7/www/img/pictures/AV莉法.png (1024x768)
  - SAO_verβ7/www/img/pictures/AV蛇岛.png (1024x768)
  - SAO_verβ7/www/img/pictures/AV迷奸.png (1024x768)
  - SAO_verβ7/www/img/pictures/AV逆寝取.png (1024x768)
- 极小图（共 3 张，列举前 3）：
  - SAO_verβ7/www/img/pictures/text_win.png
  - SAO_verβ7/www/img/pictures/txt_back.png
  - SAO_verβ7/www/img/pictures/退出键.png
- 体积异常（共 23 张，列举前 10）：
  - SAO_verβ7/www/img/pictures/384白边框 - 副本.png (2.7KB)
  - SAO_verβ7/www/img/pictures/384白边框.png (1.4KB)
  - SAO_verβ7/www/img/pictures/text_win.png (0.2KB)
  - SAO_verβ7/www/img/pictures/txt_back.png (2.8KB)
  - SAO_verβ7/www/img/pictures/心跳3 - 副本.png (4.1KB)
  - SAO_verβ7/www/img/pictures/心跳3.png (4.1KB)
  - SAO_verβ7/www/img/pictures/心跳4.png (2.1KB)
  - SAO_verβ7/www/img/pictures/心跳5.png (2.1KB)
  - SAO_verβ7/www/img/pictures/心跳6.png (4.6KB)
  - SAO_verβ7/www/img/pictures/白色线条864-576x1008.png (4.4KB)

### 苍红界域（夯）（图 0 张，扫到 2 张，加密 8224 个）
- 目录：E:\Personal_entertainment\game\ero\egame\20260424\RPG\夯\苍红界域（夯）

### 裏垢JKは露出でバズりたい（一般）（图 1724 张，扫到 7000 张，加密 0 个）
- 目录：E:\Personal_entertainment\game\ero\egame\20260501\RPG\一般\裏垢JKは露出でバズりたい（一般）
- 备注：扫到 2 个 img 根，两套目录各存一份图，重复率会被抬高
- 空白/解码异常（共 96 张，列举前 10）：
  - 内嵌/img/pictures/CGイベント/CG_02/56_放尿.png (var=8.6)
  - 内嵌/img/pictures/CGイベント/CG_02/57_愛液_腿.png (var=0.3)
  - 内嵌/img/pictures/CGイベント/CG_02/61_股間いじり用_正、淫紋.png (var=19.0)
  - 内嵌/img/pictures/CGイベント/CG_07/53_苦悶.png (var=0.0)
  - 内嵌/img/pictures/CGイベント/CG_07/60_愛液大.png (var=0.5)
  - 内嵌/img/pictures/CGイベント/CG_07/61_射精.png (var=0.2)
  - 内嵌/img/pictures/CGイベント/CG_07/64_吐息.png (var=0.0)
  - 内嵌/img/pictures/CGイベント/CG_フェラ/52_汗.png (var=0.0)
  - 内嵌/img/pictures/CGイベント/CG_フェラ/54_熱気.png (var=0.0)
  - 内嵌/img/pictures/CGイベント/CG_フェラ/56_通常ピアス.png (var=0.0)
- 近似重复组（共 29 组，列举前 8 组）：
  - `内嵌/img/pictures/CGイベント/01_Sex_eve_popup.png` = `内嵌/img/pictures/CGイベント/01_mustor_eve_popup.png` = `内嵌/img/pictures/CGイベント/02_Sex_eve_popup.png` = `内嵌/img/pictures/CGイベント/02_mustor_eve_popup.png` = `内嵌/img/pictures/CGイベント/03_Sex_eve_popup.png` = `内嵌/img/pictures/CGイベント/03_mustor_eve_popup.png` = `…等 68 张`
  - `内嵌/img/pictures/立ち絵/57_首輪03_金玉ベル.png` = `内嵌/img/pictures/立ち絵/58_乳首ピアス02_鎖_変形.png` = `内嵌/img/pictures/立ち絵/58_乳首ピアス02_鎖_通常.png` = `内嵌/img/pictures/立ち絵/59_愛液大_コート通常時.png` = `内嵌/img/pictures/立ち絵/59_愛液小_私服通常時.png` = `内嵌/img/pictures/立ち絵/60_垂れてる.png` = `…等 68 张`
  - `内嵌/img/pictures/立ち絵/63_頬両方.png` = `内嵌/img/pictures/立ち絵/表情差分/04_驚顔.png` = `内嵌/img/pictures/立ち絵/表情差分/08_絶望顔.png` = `内嵌/img/pictures/立ち絵/表情差分/10_警戒.png` = `内嵌/img/pictures/立ち絵/表情差分/猫耳専用/14_恥ずかしそう_01.png` = `外挂/Game/img/pictures/立ち絵/63_頬両方.png` = `…等 10 张`
  - `内嵌/img/pictures/Result素材/写真部分影.png` = `内嵌/img/pictures/Result素材/背景/06_遊園地_昼_01.png` = `外挂/Game/img/pictures/CGイベント/CG_05/51_絶頂顔.png` = `外挂/Game/img/pictures/CGイベント/CG_05/51_驚き_ハイライト.png` = `外挂/Game/img/pictures/Result素材/背景/02_公園_夜_02.png` = `外挂/Game/img/pictures/Result素材/背景/05_カフェ_夜_01.png` = `…等 8 张`
  - `内嵌/img/pictures/自撮りスキル/CG_02/乳首ピアス_通常.png` = `内嵌/img/pictures/自撮りスキル/CG_02/吐息.png` = `内嵌/img/pictures/自撮りスキル/CG_05/吐息.png` = `内嵌/img/pictures/自撮りスキル/CG_05/正文字.png` = `内嵌/img/pictures/自撮りスキル/CG_07/吐息.png` = `外挂/Game/img/pictures/自撮りスキル/CG_03/乳首ピアス_鈴.png` = `…等 8 张`
  - `内嵌/img/pictures/立ち絵/ガーターベルト赤/コート全出し.png` = `外挂/Game/img/pictures/立ち絵/ガーターベルト白/コート全出し.png` = `外挂/Game/img/pictures/立ち絵/濡れ透け/胸上げたくし上げ/赤_ガーターベルト.png` = `外挂/Game/img/pictures/立ち絵/濡れ透け/通常、たくし上げ/赤_ガーターベルト.png` = `外挂/Game/img/pictures/立ち絵/裸/白_ガーターベルト.png` = `外挂/Game/img/pictures/立ち絵/裸/赤_ガーターベルト.png`
  - `内嵌/img/pictures/立ち絵/60_垂れてる_私服通常時.png` = `内嵌/img/pictures/立ち絵/60_潮吹き、放尿.png` = `内嵌/img/pictures/立ち絵/黒手/股間_手.png` = `外挂/Game/img/pictures/立ち絵/60_お漏らし.png` = `外挂/Game/img/pictures/立ち絵/60_垂れてる_私服通常時.png`
  - `内嵌/img/pictures/立ち絵/ガーターベルト白/制服_胸出し、変形.png` = `内嵌/img/pictures/立ち絵/ガーターベルト黒/制服_たくし上げ、両方.png` = `内嵌/img/pictures/立ち絵/濡れ透け/通常、通常/黒_ガーターベルト.png` = `外挂/Game/img/pictures/立ち絵/ガーターベルト白/コート_たくし上げ.png` = `外挂/Game/img/pictures/立ち絵/ガーターベルト白/私服_通常、胸出し.png`
- 尺寸离群（共 876 张），尺寸分布：739x1116 x598、580x387 x172、580x435 x64、384x580 x6、102x37 x4
  - 内嵌/img/pictures/OP_CG/OP用_TL.png (384x580)
  - 内嵌/img/pictures/OP_CG/OP用_私服.png (384x580)
  - 内嵌/img/pictures/OP_CG/チュートリアル/01_操作説明ロゴ.png (1073x100)
  - 内嵌/img/pictures/OP_CG/チュートリアル/02_WASD対応.png (687x114)
  - 内嵌/img/pictures/OP_CG/チュートリアル/03_おっぱい見せる.png (607x440)
  - 内嵌/img/pictures/OP_CG/チュートリアル/tutorial_上半身ボタン当たり判定.png (104x104)
  - 内嵌/img/pictures/OP_CG/チュートリアル/tutorial_胸見せボタン当たり判定_01.png (102x37)
  - 内嵌/img/pictures/OP_CG/チュートリアル/tutorial_自撮りボタン当たり判定.png (102x102)
  - 内嵌/img/pictures/OP_CG/チュートリアル/tutorial_通常自撮りボタン当たり判定.png (102x37)
  - 内嵌/img/pictures/OP_CG/チュートリアル/上半身枠.png (109x109)
- 极小图（共 8 张，列举前 8）：
  - 内嵌/img/pictures/OP_CG/チュートリアル/tutorial_胸見せボタン当たり判定_01.png
  - 内嵌/img/pictures/OP_CG/チュートリアル/tutorial_通常自撮りボタン当たり判定.png
  - 内嵌/img/pictures/OP_CG/チュートリアル/胸見せ枠.png
  - 内嵌/img/pictures/OP_CG/チュートリアル/黄色_通常枠.png
  - 外挂/Game/img/pictures/OP_CG/チュートリアル/tutorial_胸見せボタン当たり判定_01.png
  - 外挂/Game/img/pictures/OP_CG/チュートリアル/tutorial_通常自撮りボタン当たり判定.png
  - 外挂/Game/img/pictures/OP_CG/チュートリアル/胸見せ枠.png
  - 外挂/Game/img/pictures/OP_CG/チュートリアル/黄色_通常枠.png
- 体积异常（共 38 张，列举前 10）：
  - 内嵌/img/pictures/OP_CG/チュートリアル/tutorial_上半身ボタン当たり判定.png (2.8KB)
  - 内嵌/img/pictures/OP_CG/チュートリアル/tutorial_胸見せボタン当たり判定_01.png (1.8KB)
  - 内嵌/img/pictures/OP_CG/チュートリアル/tutorial_自撮りボタン当たり判定.png (2.1KB)
  - 内嵌/img/pictures/OP_CG/チュートリアル/tutorial_通常自撮りボタン当たり判定.png (1.8KB)
  - 内嵌/img/pictures/OP_CG/チュートリアル/上半身枠.png (2.5KB)
  - 内嵌/img/pictures/OP_CG/チュートリアル/胸見せ枠.png (1.3KB)
  - 内嵌/img/pictures/OP_CG/チュートリアル/黄色_自撮り枠.png (2.6KB)
  - 内嵌/img/pictures/OP_CG/チュートリアル/黄色_通常枠.png (1.3KB)
  - 内嵌/img/pictures/背景/全てを覆い隠す真っ黒な背景.png (4.7KB)
  - 内嵌/img/pictures/自撮りスキル/CG_02/乳首ピアス_通常.png (4.5KB)

### 詛咒铠甲2（夯）（图 0 张，扫到 53 张，加密 18212 个）
- 目录：E:\Personal_entertainment\game\ero\egame\20260629\RPG\夯\詛咒铠甲2（夯）

### 魔法学園見聞録（图 0 张，扫到 3 张，加密 1475 个）
- 目录：E:\Personal_entertainment\game\ero\egame\20260709\RPG\未评级\魔法学園見聞録

### 魔法少女木叶  v2.04（中上）（图 1317 张，扫到 3920 张，加密 0 个）
- 目录：E:\Personal_entertainment\game\ero\egame\20260519\RPG\中上\魔法少女木叶  v2.04（中上）
- 空白/解码异常（共 7 张，列举前 7）：
  - www/img/menus/file/Layout.png (var=0.0)
  - www/img/pictures/Ma_fa_bare_04.png (var=17.7)
  - www/img/pictures/Pi_heapin.png (var=0.0)
  - www/img/pictures/Scene_EnemyBook_back.png (var=5.3)
  - www/img/pictures/Scene_Quest_back.png (var=5.3)
  - www/img/weather/Cloud_01C.png (var=0.0)
  - www/img/weather/Wind_01.png (var=0.0)
- 近似重复组（共 28 组，列举前 8 组）：
  - `www/img/pictures/H_022/sekkanA_06.png` = `www/img/pictures/H_022/sekkanA_08.png` = `www/img/pictures/H_022/sekkanAb_07.png` = `www/img/pictures/H_022/sekkanB_06.png` = `www/img/pictures/H_022/sekkanB_08.png` = `www/img/pictures/H_022/sekkanBb_07.png`
  - `www/img/pictures/H_012/eido_01.png` = `www/img/pictures/H_012/eido_02.png` = `www/img/pictures/H_012/eido_06.png` = `www/img/pictures/H_012/eido_07.png`
  - `www/img/pictures/H_026/bai1_i_A_03.png` = `www/img/pictures/H_026/bai1_i_A_04.png` = `www/img/pictures/H_026/bai1_i_B_03.png`
  - `www/img/pictures/H_035/H_sakaba_b02.png` = `www/img/pictures/H_sakaba/H_sakaba_02.png` = `www/img/pictures/H_sakaba/H_sakaba_b02.png`
  - `www/img/pictures/H_035/H_sakaba_c01.png` = `www/img/pictures/H_035/H_sakaba_c02.png` = `www/img/pictures/H_035/H_sakaba_c03.png`
  - `www/img/pictures/H_044/D_hanyo1B_02.png` = `www/img/pictures/H_044/D_hanyo1B_i_02.png` = `www/img/pictures/H_044/D_hanyo1B_i_03.png`
  - `www/img/weather/Fog_01a.png` = `www/img/weather/Fog_01b.png` = `www/img/weather/Fog_01c.png`
  - `www/img/BJ/h06.png` = `www/img/BJ/h08.png`
- 尺寸离群（共 475 张），尺寸分布：140x210 x53、144x48 x44、334x439 x33、192x192 x32、640x360 x28
  - www/img/BJ/BJ_JUDGE.png (885x139)
  - www/img/BJ/BJ_VICTORY.png (1097x166)
  - www/img/BJ/BJ_announce01.png (630x128)
  - www/img/BJ/BJ_announce02.png (630x128)
  - www/img/BJ/BJ_d_cole.png (182x52)
  - www/img/BJ/BJ_d_drop.png (182x52)
  - www/img/BJ/BJ_d_reise.png (182x52)
  - www/img/BJ/BJ_d_stay.png (182x52)
  - www/img/BJ/BJ_dealerturn.png (1162x141)
  - www/img/BJ/BJ_dealerwin.png (1095x141)
- 极小图（共 151 张，列举前 10）：
  - www/img/BJ/BJ_d_cole.png
  - www/img/BJ/BJ_d_drop.png
  - www/img/BJ/BJ_d_reise.png
  - www/img/BJ/BJ_d_stay.png
  - www/img/BJ/BJ_mark.png
  - www/img/BJ/BJ_p_cole_off.png
  - www/img/BJ/BJ_p_cole_on.png
  - www/img/BJ/BJ_p_drop_off.png
  - www/img/BJ/BJ_p_drop_on.png
  - www/img/BJ/BJ_p_reise_off.png
- 体积异常（共 27 张，列举前 10）：
  - www/img/BJ/BJ_mark.png (1.8KB)
  - www/img/BJ/c01.png (4.7KB)
  - www/img/BJ/d01.png (3.7KB)
  - www/img/BJ/d02.png (4.0KB)
  - www/img/BJ/d03.png (4.7KB)
  - www/img/BJ/h01.png (3.6KB)
  - www/img/BJ/h02.png (4.2KB)
  - www/img/BJ/h03.png (4.9KB)
  - www/img/BJ/s02.png (5.0KB)
  - www/img/menus/main/FaceArrow.png (2.9KB)

### （一般） 电脑 冒险者莉扎西亚 冒険者リザシア -葛藤と屈辱の日間-（图 0 张，扫到 2 张，加密 662 个）
- 目录：E:\Personal_entertainment\game\ero\egame\20260716\RPG\一般\（一般） 电脑 冒险者莉扎西亚 冒険者リザシア -葛藤と屈辱の日間-

### （一般）Shitsuke-ai_dlsite（图 1026 张，扫到 1876 张，加密 0 个）
- 目录：E:\Personal_entertainment\game\ero\egame\20260808\RPG\一般\（一般）Shitsuke-ai_dlsite
- 近似重复组（共 50 组，列举前 8 组）：
  - `img/pictures/Lunaal_Stand/Lunaal_Anger.png` = `img/pictures/Lunaal_Stand/Lunaal_Despair.png` = `img/pictures/Lunaal_Stand/Lunaal_Lose.png` = `img/pictures/Lunaal_Stand/Lunaal_Mad.png` = `img/pictures/Lunaal_Stand/Lunaal_Normal.png` = `img/pictures/Lunaal_Stand/Lunaal_Realize.png` = `…等 10 张`
  - `img/pictures/CG/SariaH3/SariaH3_03JP.png` = `img/pictures/CG/SariaH3/SariaH3_04JP.png` = `img/pictures/CG/SariaH3/SariaH3_07JP.png` = `img/pictures/CG/SariaH3/SariaH3_18JP.png` = `img/pictures/CG/SariaH3/SariaH3_20JP.png` = `img/pictures/CG/SariaH6/SariaH6_01JP.png` = `…等 9 张`
  - `img/pictures/CG/ChelseaH8/ChelseaH8_06.png` = `img/pictures/CG/SariaH1/SariaH1_09.png` = `img/pictures/CG/SariaH1/SariaH1_10.png` = `img/pictures/CG/SariaH1/SariaH1_11.png` = `img/pictures/CG/SariaH4/SariaH4_08.png` = `img/pictures/CG/SariaH4/SariaH4_12.png` = `…等 8 张`
  - `img/pictures/CG/LilisH7/LilisH7_015JP.png` = `img/pictures/CG/LilisH7/LilisH7_021JP.png` = `img/pictures/CG/LilisH7/LilisH7_023JP.png` = `img/pictures/CG/LilisH7/LilisH7_033JP.png` = `img/pictures/CG/LilisH7/LilisH7_034JP.png` = `img/pictures/CG/LilisH7/LilisH7_035JP.png` = `…等 7 张`
  - `img/pictures/CG/LunaalH3/LunaalH3_01JP.png` = `img/pictures/CG/LunaalH3/LunaalH3_02JP.png` = `img/pictures/CG/LunaalH3/LunaalH3_12JP.png` = `img/pictures/CG/LunaalH3/LunaalH3_14JP.png` = `img/pictures/CG/LunaalH6/LunaalH6_05JP.png` = `img/pictures/CG/LunaalH6/LunaalH6_12JP.png` = `…等 7 张`
  - `img/pictures/CG/LunaalH7/LunaalH7_011JP.png` = `img/pictures/CG/LunaalH7/LunaalH7_012JP.png` = `img/pictures/CG/LunaalH7/LunaalH7_013JP.png` = `img/pictures/CG/LunaalH7/LunaalH7_01JP.png` = `img/pictures/CG/LunaalH7/LunaalH7_021JP.png` = `img/pictures/CG/LunaalH7/LunaalH7_025JP.png` = `…等 7 张`
  - `img/pictures/Player_Stand/Player_Anger.png` = `img/pictures/Player_Stand/Player_Desperate.png` = `img/pictures/Player_Stand/Player_ForSystem1.png` = `img/pictures/Player_Stand/Player_Glare.png` = `img/pictures/Player_Stand/Player_Sad.png` = `img/pictures/Player_Stand/Player_Sermon.png` = `…等 7 张`
  - `img/pictures/Saria_Stand/Saria_Abuse.png` = `img/pictures/Saria_Stand/Saria_Anxiety.png` = `img/pictures/Saria_Stand/Saria_Glad.png` = `img/pictures/Saria_Stand/Saria_Normal.png` = `img/pictures/Saria_Stand/Saria_Sad.png` = `img/pictures/Saria_Stand/Saria_Smile1.png` = `…等 7 张`
- 尺寸离群（共 146 张），尺寸分布：330x350 x120、1920x200 x11、695x1080 x8、144x144 x2、1064x1080 x1
  - img/pictures/Actor1_1.png (330x350)
  - img/pictures/Actor1_2.png (330x350)
  - img/pictures/Actor1_3.png (330x350)
  - img/pictures/Actor1_4.png (330x350)
  - img/pictures/Actor1_5.png (330x350)
  - img/pictures/Actor1_6.png (330x350)
  - img/pictures/Actor1_7.png (330x350)
  - img/pictures/Actor1_8.png (330x350)
  - img/pictures/Actor2_1.png (330x350)
  - img/pictures/Actor2_2.png (330x350)
- 体积异常（共 1 张，列举前 1）：
  - img/pictures/BackgroundCG/BlackFade.png (0.4KB)

### （上等）NTRエロトラップダンジョン（图 172 张，扫到 469 张，加密 0 个）
- 目录：E:\Personal_entertainment\game\ero\egame\20260716\RPG\上等\（上等）NTRエロトラップダンジョン
- 近似重复组（共 21 组，列举前 8 组）：
  - `img/pictures/I-barance-0.png` = `img/pictures/I-barance-1-0.png` = `img/pictures/I-barance-1-1.png` = `img/pictures/I-barance-1-2.png` = `img/pictures/I-barance-1-3.png` = `img/pictures/I-barance-1-4.png` = `…等 8 张`
  - `img/pictures/ED-gokkun-0.png` = `img/pictures/ED-gokkun-1.png` = `img/pictures/ED-gokkun-hart-0.png` = `img/pictures/ED-gokkun-hart-1.png`
  - `img/pictures/munemomiv2-0-0.png` = `img/pictures/munemomiv2-1-0.png` = `img/pictures/munemomiv2-2-0.png` = `img/pictures/munemomiv2-3-0.png`
  - `img/pictures/paizuri-1-1.png` = `img/pictures/paizuri-1.png` = `img/pictures/paizuri-2-1.png` = `img/pictures/paizuri-2.png`
  - `img/pictures/ED-fella-0.png` = `img/pictures/ED-fella-1.png` = `img/pictures/ED-fella-2.png`
  - `img/pictures/ED-fella-cutin-0.png` = `img/pictures/ED-fella-cutin-1.png`
  - `img/pictures/ED-miseyari-0-0.png` = `img/pictures/ED-miseyari-0-1.png`
  - `img/pictures/ED-miseyari-1-0.png` = `img/pictures/ED-miseyari-1-1.png`
- 尺寸离群（共 63 张），尺寸分布：2560x1440 x13、288x162 x12、320x180 x10、288x163 x4、4096x4096 x3
  - img/live2d/eriza/eriza.2048/texture_00.png (2048x2048)
  - img/live2d/eriza/eriza.2048/texture_01.png (2048x2048)
  - img/live2d/h_dakituki/dakituki_sex.4096/texture_00.png (4096x2048)
  - img/live2d/h_fella/eriza_fella.4096/texture_00.png (4096x4096)
  - img/live2d/h_i_balance/eriza_i.4096/texture_00.png (4096x2048)
  - img/live2d/h_ne_back/ne_back.8192/texture_00.png (8192x4096)
  - img/live2d/pisuton_mission/pisuton_mission.4096/texture_00.png (4096x4096)
  - img/live2d/saimin_kiss/saimin_kiss.4096/texture_00.png (4096x4096)
  - img/live2d/sirimomi/sirimomi.8192/texture_00.png (8192x4096)
  - img/menus/Cursor_A.png (32x32)
- 极小图（共 1 张，列举前 1）：
  - img/menus/Cursor_A.png
- 体积异常（共 2 张，列举前 2）：
  - img/menus/Cursor_A.png (4.2KB)
  - img/pictures/1701.png (1.3KB)

### （上等）哈尔法斯的恶魔（图 0 张，扫到 2 张，加密 2865 个）
- 目录：E:\Personal_entertainment\game\ero\egame\20260722\RPG\上等\（上等）哈尔法斯的恶魔

### （中等）NTR Hunter（图 181 张，扫到 450 张，加密 0 个）
- 目录：E:\Personal_entertainment\game\ero\egame\20260808\RPG\中等\（中等）NTR Hunter
- 空白/解码异常（共 15 张，列举前 10）：
  - img/pSpineAssets/EV_new/ev_new8.png (var=0.0)
  - img/pSpineAssets/Heading/Heading2.png (var=0.0)
  - img/pSpineAssets/animate_01_Lick_pussy_hotel/animate_01_Lick_pussy_hotel3.png (var=0.0)
  - img/pSpineAssets/animate_05_sex_glass_hotel/animate_05_sex_glass_hotel3.png (var=0.0)
  - img/pSpineAssets/animate_06_pickpocketing/animate_06_pickpocketing3.png (var=0.0)
  - img/pSpineAssets/animate_07_Lick_pussy_hotel CT6/animate_07_Lick_pussy_hotel CT63.png (var=0.0)
  - img/pSpineAssets/animate_08_Lick_pussy_hotel CT7/animate_08_Lick_pussy_hotel CT73.png (var=0.0)
  - img/pSpineAssets/camera_scene/camera_scene3.png (var=0.0)
  - img/pSpineAssets/ex011/ex0113.png (var=0.0)
  - img/pSpineAssets/ex011_2/ex011_23.png (var=0.0)
- 近似重复组（共 9 组，列举前 8 组）：
  - `img/pSpineAssets/Heading/Heading2.png` = `img/pSpineAssets/animate_01_Lick_pussy_hotel/animate_01_Lick_pussy_hotel3.png` = `img/pSpineAssets/animate_05_sex_glass_hotel/animate_05_sex_glass_hotel3.png` = `img/pSpineAssets/animate_06_pickpocketing/animate_06_pickpocketing3.png` = `img/pSpineAssets/animate_07_Lick_pussy_hotel CT6/animate_07_Lick_pussy_hotel CT63.png` = `img/pSpineAssets/animate_08_Lick_pussy_hotel CT7/animate_08_Lick_pussy_hotel CT73.png` = `…等 13 张`
  - `img/pictures/Wife_angry.png` = `img/pictures/Wife_embarrassing_lying.png` = `img/pictures/Wife_fall.png` = `img/pictures/Wife_nomal.png`
  - `img/pictures/Wife_ass_01.png` = `img/pictures/Wife_ass_02.png` = `img/pictures/Wife_ass_dirt.png`
  - `img/pictures/Wife_ass_03.png` = `img/pictures/Wife_ass_10.png` = `img/pictures/Wife_ass_11.png`
  - `img/pictures/Wife_ass_09.png` = `img/pictures/Wife_ass_semen.png` = `img/pictures/Wife_ass_semen_02.png`
  - `img/pictures/status_cn.png` = `img/pictures/status_en.png` = `img/pictures/status_jp.png`
  - `img/pSpineAssets/animate_04_sex_reveseOnTop_hotel/animate_04_sex_reveseOnTop_hotel3.png` = `img/pSpineAssets/animate_04_sex_reveseOnTop_hotel/animate_04_sex_reveseOnTop_hotel4.png`
  - `img/pictures/Wife_ass_06.png` = `img/pictures/Wife_ass_wet.png`
- 尺寸离群（共 156 张），尺寸分布：394x698 x18、1924x1084 x13、1984x1084 x9、540x1080 x9、1280x720 x6
  - img/pSpineAssets/EV_new/ev_new.png (2007x2008)
  - img/pSpineAssets/EV_new/ev_new2.png (2025x1923)
  - img/pSpineAssets/EV_new/ev_new3.png (1562x1757)
  - img/pSpineAssets/EV_new/ev_new4.png (1964x1778)
  - img/pSpineAssets/EV_new/ev_new5.png (1751x2045)
  - img/pSpineAssets/EV_new/ev_new6.png (2010x1916)
  - img/pSpineAssets/EV_new/ev_new7.png (1619x1816)
  - img/pSpineAssets/EV_new/ev_new8.png (1234x1084)
  - img/pSpineAssets/EV_new/ev_new9.png (1174x1086)
  - img/pSpineAssets/Heading/Heading.png (1980x1785)
- 极小图（共 6 张，列举前 6）：
  - img/pictures/1-star.png
  - img/pictures/2-star.png
  - img/pictures/3-star.png
  - img/pictures/4-star.png
  - img/pictures/5-star.png
  - img/pictures/Pink_arrow.png
- 体积异常（共 6 张，列举前 6）：
  - img/pictures/1-star.png (4.0KB)
  - img/pictures/2-star.png (4.4KB)
  - img/pictures/3-star.png (4.0KB)
  - img/pictures/4-star.png (4.0KB)
  - img/pictures/5-star.png (4.1KB)
  - img/pictures/Pink_arrow.png (1.6KB)

### （夯）直到这个魅魔的心灵崩坏（图 0 张，扫到 0 张，加密 1580 个）
- 目录：E:\Personal_entertainment\game\ero\egame\20260930\RPG\夯\（夯）直到这个魅魔的心灵崩坏

### （牛）间谍任务（图 3076 张，扫到 3921 张，加密 0 个）
- 目录：E:\Personal_entertainment\game\ero\egame\20260930\RPG\未评级\（牛）间谍任务
- 空白/解码异常（共 41 张，列举前 10）：
  - www/img/pictures/Blakout.png (var=0.0)
  - www/img/pictures/Grayout.png (var=0.0)
  - www/img/pictures/PC(ヒント)_4_？-4_選択済.png (var=21.3)
  - www/img/pictures/パソコン_更新_赤3.png (var=24.4)
  - www/img/pictures/パソコン_選択3.png (var=7.4)
  - www/img/pictures/パソコン_選択4.png (var=7.4)
  - www/img/pictures/メニュー(本編)_5_セーブ_未選択.png (var=22.5)
  - www/img/pictures/モブ質問_ソラ.png (var=0.0)
  - www/img/pictures/ラブレター_ソラ.png (var=12.9)
  - www/img/pictures/個別イベタイトル_メイド長1.png (var=1.5)
- 近似重复组（共 50 组，列举前 8 组）：
  - `www/img/pictures/Grayout.png` = `www/img/pictures/PC(説明)_4_選択済.png` = `www/img/pictures/エロステコメ_あなる_6.png` = `www/img/pictures/エロステコメ_おくち_3.png` = `www/img/pictures/エロステコメ_おまんこ_0(経験済み).png` = `www/img/pictures/パソコン_更新_赤3.png` = `…等 90 张`
  - `www/img/pictures/H5_お掃除なめ_1.png` = `www/img/pictures/H5_お掃除なめ_2.png` = `www/img/pictures/H5_お掃除なめ_4.png` = `www/img/pictures/H5_クンニ_12.png` = `www/img/pictures/H5_クンニ_2.png` = `www/img/pictures/H5_クンニ_3.png` = `…等 9 张`
  - `www/img/pictures/屋敷図_水・夕.png` = `www/img/pictures/屋敷図_水・夜_アザミ・私室.png` = `www/img/pictures/屋敷図_水・夜_アロエ・第二寝室.png` = `www/img/pictures/屋敷図_水・夜_ソラ・浴室.png` = `www/img/pictures/屋敷図_水・夜_マロン・第二寝室.png` = `www/img/pictures/屋敷図_水・夜_メイド長・浴室.png` = `…等 8 张`
  - `www/img/pictures/H7_射精_16.png` = `www/img/pictures/H7_射精_22.png` = `www/img/pictures/H7_射精_4.png` = `www/img/pictures/H7_挿入_4.png` = `www/img/pictures/H7_男影_1.png` = `www/img/pictures/H7_男影_14.png` = `…等 7 张`
  - `www/img/pictures/H1_お掃除フェラ_1.png` = `www/img/pictures/H1_お掃除フェラ_2.png` = `www/img/pictures/H1_挿入_3.png` = `www/img/pictures/H1_顔射_1.png` = `www/img/pictures/H1_顔射_2.png` = `www/img/pictures/H1_顔射_なみだ2.png`
  - `www/img/pictures/Hモモ_初回(挿入)_10.png` = `www/img/pictures/Hモモ_初回(挿入)_15.png` = `www/img/pictures/Hモモ_初回(挿入)_3.png` = `www/img/pictures/Hモモ_初回(挿入)_4.png` = `www/img/pictures/Hモモ_初回(挿入)_5.png` = `www/img/pictures/Hモモ_初回(未挿入)_3.png`
  - `www/img/pictures/立ち絵(カリン)_忍装束_まったり3.png` = `www/img/pictures/立ち絵(カリン)_忍装束_泣き2.png` = `www/img/pictures/立ち絵(カリン)_忍装束_照れ4.png` = `www/img/pictures/立ち絵(カリン)_忍装束_通常3.png` = `www/img/pictures/立ち絵(カリン)_忍装束_通常4.png` = `www/img/pictures/立ち絵(カリン)_忍装束_驚き2.png`
  - `www/img/pictures/H7_未挿入_10.png` = `www/img/pictures/H7_未挿入_19.png` = `www/img/pictures/H7_未挿入_4.png` = `www/img/pictures/H7_未挿入_7.png` = `www/img/pictures/H7_未挿入_9.png`
- 尺寸离群（共 2 张），尺寸分布：1700x624 x1、384x120 x1
  - www/img/pictures/ItemSlot.png (384x120)
  - www/img/pictures/身体検査.png (1700x624)
- 体积异常（共 72 张，列举前 10）：
  - www/img/pictures/ItemSlot.png (2.3KB)
  - www/img/pictures/PC(ヒント)_1_？-1_選択済.png (4.8KB)
  - www/img/pictures/PC(ヒント)_2_？-2_選択済.png (4.8KB)
  - www/img/pictures/PC(ヒント)_3_？-3_選択済.png (4.8KB)
  - www/img/pictures/PC(ヒント)_4_？-4_選択済.png (4.8KB)
  - www/img/pictures/PC(ヒント)_5_？-5_選択済.png (4.8KB)
  - www/img/pictures/PC(最初の画面)_2_選択_未選択.png (4.5KB)
  - www/img/pictures/PC(最初の画面)_3_回想_未選択.png (4.4KB)
  - www/img/pictures/PC(最初の画面)_3_回想_選択済.png (4.8KB)
  - www/img/pictures/PC(説明)_0.png (4.8KB)

## 指标口径
- images：参与打分的 png/jpg/webp 张数（引擎素材族默认剔除；加密图 .rpgmvp/.png_ 不算图，单独计 encrypted）。
- families：相对 img 根的前两段路径（如 pictures、pictures/busts、pictures/CG）。kind=asset 表示判成引擎素材：RPG Maker 标准目录（tilesets/characters/enemies/sv_*/battlebacks/parallaxes/animations/system/titles*…）+ 族名里带 ui/icon/menu/hud/logo/font/button/window/cursor/frame 这类词的目录。
- small / outliers / blank / dup_groups / dup_rate / exact_dup_* / sparse / avg_kb：与导出目录模式完全同一套口径。
- encrypted：源目录里的 .rpgmvp/.png_ 数量；这些图读不到像素，指标只能覆盖明文部分。
- misnamed：扩展名与实际格式不符（如 .png 实为 JPEG）；图能正常看，但按扩展名解析的脚本会失败。
- severity：通用判据 + 源目录补充判据（全加密 +3、多数加密 +2、部分加密 +1、明文图远少于清单 cg_count +2）加权后分档。
- verdict：3=建议全量肉眼 / 1-2=抽检 30 张即可 / 0=无需看。
