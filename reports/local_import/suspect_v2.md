# suspect v2 · 加密图解镜像后复检（RPG Maker MV/MZ）

## 一句话结论

- 升级前 pass 19 / 44；升级后 pass **43** / 44（净增 +24）
- 加密图款 24 款，解密镜像 124999 张、26.91 GB（输出全在 D:\UCGR_Work\decrypt，源目录零写入）
- 升级为 pass：24 款；仍 suspect：1 款；解密全失败/未解：0 款；单图失败 1 款

## 判据（与 selfcheck v1 完全一致，判定逻辑未改一行）

1. 导出图数 = 0
2. 空白图占比 > 50%（复用 tools/review-prescreen：32x32 网格灰度方差 < 25）
3. 所有图尺寸完全一致且 ≤ 128×128
4. 总导出体积 < 100KB

- 唯一变量是**输入**：v2 把解密成功的款，manifest 的 source_real_path 换成 D:\UCGR_Work\decrypt\<游戏名> 镜像（保留 www/img 或 img 子树），prescreen 第一次能读到真实像素。

## 前后对比总表（suspect 在前）

| 游戏 | 输入源 | v1 判定 | v2 判定 | 加密图(v1) | v2 图数 | 空白/抽样 | 总KB | 众数尺寸 | v2 命中判据 |
|---|---|---|---|---:|---:|---:|---:|---|---|
| 魔剣士レーヴェ | raw | suspect | **suspect** | 0 | 0 | 0/0 | 0.0 | - | 导出图数=0 |
| (中等)The Forbidden Tomes of Olipos | decrypt | suspect | **pass** | 1957 | 882 | 32/300 | 660794.4 | 1080x720 | - |
| (夯)Stranded with You | raw | pass | **pass** | 0 | 1273 | 103/300 | 153269.2 | 1280x720 | - |
| (牛)RJ01618221 | raw | pass | **pass** | 0 | 706 | 1/297 | 383922.8 | 1280x720 | - |
| (牛)RJ01680223 | decrypt | suspect | **pass** | 2461 | 1840 | 9/299 | 823952.0 | 1020x778 | - |
| 072project_The Gloomy, Busty Half- | decrypt | suspect | **pass** | 1816 | 858 | 5/300 | 103045.8 | 816x624 | - |
| ayuureiCN（夯） | decrypt | suspect | **pass** | 1726 | 1646 | 28/300 | 1514978.4 | 1920x1056 | - |
| Chronicles of Clams Island | raw | pass | **pass** | 0 | 842 | 8/300 | 233234.0 | 816x624 | - |
| Game | raw | pass | **pass** | 0 | 504 | 33/300 | 178567.2 | 816x624 | - |
| Lulumachi | decrypt | suspect | **pass** | 51252 | 50755 | 21/300 | 741023.0 | 960x540 | - |
| MELFIAS －Azure and Crimson Verge－ | decrypt | suspect | **pass** | 8224 | 7023 | 40/300 | 3157540.8 | 1024x640 | - |
| RA26037409( | decrypt | suspect | **pass** | 324 | 189 | 0/189 | 28860.3 | 816x624 | - |
| Restore( | raw | pass | **pass** | 0 | 1072 | 7/300 | 664532.8 | 1280x720 | - |
| Risty and the Village of Bocchino  | raw | pass | **pass** | 0 | 288 | 1/288 | 121104.0 | 868x1228 | - |
| ROOM | raw | pass | **pass** | 0 | 800 | 12/300 | 230400.0 | 1280x720 | - |
| SAO_verβ7 | raw | pass | **pass** | 0 | 1521 | 38/300 | 174002.4 | 1792x1008 | - |
| Scars of Summer After v1.01（牛） | decrypt | suspect | **pass** | 3987 | 3133 | 17/300 | 2016398.8 | 1200x768 | - |
| The Dungeon of Lulu Farea | decrypt | suspect | **pass** | 1070 | 396 | 1/300 | 113493.6 | 896x672 | - |
| The Dungeon of Lulu Farea Rebirth  | decrypt | suspect | **pass** | 1339 | 605 | 1/299 | 162684.5 | 1200x864 | - |
| Unholy_maiden_Ver.1.0.10 | decrypt | suspect | **pass** | 4182 | 2929 | 10/300 | 1507849.2 | 1280x720 | - |
| ボクは種付けアプリを手に入れた！ | decrypt | suspect | **pass** | 236 | 204 | 3/204 | 1897.2 | 300x624 | - |
| 人柱阿拉卡~官方中文版~遊戲本體 | decrypt | suspect | **pass** | 1844 | 987 | 24/300 | 529722.9 | 816x624 | - |
| 内嵌 | raw | pass | **pass** | 0 | 862 | 95/300 | 87493.0 | 1080x720 | - |
| 冒険者リザシア | decrypt | suspect | **pass** | 662 | 366 | 0/300 | 248038.2 | 1280x900 | - |
| 呪いの鎧 v3.0.1（夯） | decrypt | suspect | **pass** | 3385 | 2786 | 63/300 | 271913.6 | 1024x578 | - |
| 女主角（大雷） | raw | pass | **pass** | 0 | 715 | 4/300 | 370441.5 | 1280x720 | - |
| 女大学生 DX(8) | raw | pass | **pass** | 0 | 1404 | 8/300 | 957808.8 | 1280x720 | - |
| 妹神官（牛） | decrypt | suspect | **pass** | 5894 | 2092 | 58/300 | 1996395.6 | 1440x900 | - |
| 对魔战士美月( | raw | pass | **pass** | 1277 | 15 | 0/15 | 28249.5 | 2048x2048 | - |
| 希尔丝大冒险  1.06（夯） | decrypt | suspect | **pass** | 1352 | 296 | 16/293 | 104044.0 | 96x96 | - |
| 忍耐色诱的告白游戏 V2.0（不赖） | decrypt | suspect | **pass** | 1332 | 832 | 27/298 | 181875.2 | 816x624 | - |
| 时勇者 | decrypt | suspect | **pass** | 4530 | 848 | 20/300 | 304177.6 | 816x624 | - |
| 星天之魔女（Arpg） | decrypt | suspect | **pass** | 3271 | 2485 | 55/300 | 1215662.0 | 1200x720 | - |
| 栩的人生已经没救了（一般） | raw | pass | **pass** | 0 | 935 | 18/300 | 180361.5 | 816x624 | - |
| 詛咒铠甲2（夯） | decrypt | suspect | **pass** | 18212 | 17205 | 54/300 | 2424184.5 | 1136x640 | - |
| 魔法 | raw | pass | **pass** | 4535 | 4 | 0/4 | 928.8 | 816x624 | - |
| 魔法学園見聞録 | decrypt | suspect | **pass** | 1475 | 938 | 10/300 | 573587.0 | 1280x720 | - |
| 魔法少女木叶  v2.04（中上） | raw | pass | **pass** | 0 | 1317 | 7/300 | 684049.8 | 1280x960 | - |
| （一般）Shitsuke-ai_dlsite | raw | pass | **pass** | 0 | 1026 | 0/300 | 440872.2 | 1920x1080 | - |
| （一般）命に代えてもお守りします! | raw | pass | **pass** | 0 | 152 | 0/152 | 1102258.4 | 3264x2496 | - |
| （上等）哈尔法斯的恶魔 | decrypt | suspect | **pass** | 2865 | 1365 | 47/300 | 809172.0 | 1280x720 | - |
| （中等）NTR Hunter | raw | pass | **pass** | 0 | 181 | 15/181 | 133505.6 | 1280x420 | - |
| （夯）直到这个魅魔的心灵崩坏 | decrypt | suspect | **pass** | 1580 | 1417 | 9/300 | 1008762.3 | 1920x1080 | - |
| （牛）间谍任务 | raw | pass | **pass** | 0 | 3076 | 41/300 | 611816.4 | 816x624 | - |

## 升级后仍 suspect 的款（逐款原因）

| 游戏 | 引擎 | 图数 | 空白/抽样 | 总KB | 众数尺寸 | 命中判据 | 备注 |
|---|---|---:|---:|---:|---|---|---|
| 魔剣士レーヴェ | kirikiri | 0 | 0/0 | 0.0 | - | 导出图数=0 | 非 MV/MZ 引擎（kirikiri），无 img 子树可读，需先解包资源（如 xp3/unity 包），不在本次解密范围 |

## 像素未参与抽样的款（图过大被 -max-pixels 跳过，判据 2/3 实际未触发）

| 游戏 | 图数 | 抽样 | 已解码 | 跳过(过大) | 众数尺寸 | 说明 |
|---|---:|---:|---:|---:|---|---|
| - | - | - | - | - | - | 无 |

## 解密失败 / 未解密的款（如实记录）

| 游戏 | 状态 | 失败张数 | 原因 |
|---|---|---:|---|
| 魔法学園見聞録 | partial (部分图失败) | 1 | 变体全失败（mvkey:system-hex:sig; mvkey:system-md5:sig; mvkey:default-md5:sig; mvkey:derived:尺寸非法 167772160x268435456；载荷前 4 字节=907f11bc，非 PNG/JPEG） ×1 |

## 说明

- 解密只写 D:\UCGR_Work\decrypt 镜像；游戏源目录零写入、源文件零删除。
- review-prescreen.exe 未改一行（Go 代码零改动），v2 只换了 manifest 的 source_real_path。
- 生成时间：2026-11-21 02:14:49

