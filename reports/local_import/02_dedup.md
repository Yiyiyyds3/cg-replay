# 02 · 内容哈希去重（2026-11-21 00:52 +08:00，耗时 10.9s）

## 方法

- 真资源包（`.xp3/.arc/.pac/.pck/.noa/.dat`）：对**主资源包**计算 SHA256。文件名相同哈希不同不算重复，文件名不同哈希一致算重复。
- 目录型游戏（rmmz/NW.js，主"资源"实为 natives_blob.bin 等 Chromium 运行时噪声）：改用**目录内容签名**（相对路径+大小排序后 SHA256），排除运行时噪声。
- 与已入库比对：`%LOCALAPPDATA%/cgreplay-ui/games.json` 登记根路径。
- **纠正记录**：初版把 Chromium 运行时 natives_blob.bin/v8_context_snapshot.bin 当主资源包，导致 37 款被误判"内部重复"（实为各 NW.js 游戏共用的 Chromium 二进制）；已按上法修正。

## 三组结果

| 分组 | 数量 |
|---|---|
| 新增（keep，进入试导入） | 57 |
| 内部重复已合并（duplicate_internal） | 0 |
| 与已入库重复已跳过（duplicate_existing） | 1 |

内部重复组数：0。

> 排除 junction 视图（`_游戏库`）与运行时噪声后，真实游戏目录之间**没有内容重复**（0 组），无需合并。

### 与已入库重复（已存在，跳过）

| 游戏名 | 路径 |
|---|---|
| （上等）NTRエロトラップダンジョン | E:\Personal_entertainment\game\ero\egame\20260716\RPG\上等\（上等）NTRエロトラップダンジョン |

### 新增 keep 组（进入试导入，57 款）

| 游戏名 | 哈希方法 | SHA256前16 | 既往判定 |
|---|---|---|---|
| 共妻之村 (牛) | sha256(main_package) | abc146039ef324c5 | partial |
| ayuureiCN（夯） | sha256(dir_signature) | 9647e92d6dc47b8d | pass |
| Risty and the Village of Bocchino v1 | sha256(dir_signature) | fd6cdfe11b5d710c | pass |
| RA26037409( | sha256(dir_signature) | 81aa604a258ec6bb | pass |
| Restore( | sha256(dir_signature) | c53b85c1800733ab | pass |
| 光辉的少女们 | sha256(main_package) | ceb2d4deeebb1dad | skip |
| 女大学生 DX(8) | sha256(dir_signature) | 6c5e280de72b4547 | pass |
| 对魔战士美月( | sha256(dir_signature) | fec75d9508f5fda7 | pass |
| 女主角（大雷） | sha256(dir_signature) | 58430c720096e76a | pass |
| MELFIAS －Azure and Crimson Verge－ | sha256(dir_signature) | 82cf7150618904db | not_in_prior |
| Scars of Summer After v1.01（牛） | sha256(dir_signature) | ee28203589704ce2 | pass |
| 妹神官（牛） | sha256(dir_signature) | ae619b430a460539 | pass |
| wuxian | sha256(dir_signature) | a7cfe23f791e1100 | fail |
| 内嵌 | sha256(dir_signature) | 1b93923afec60b30 | not_in_prior |
| 星天之魔女（Arpg） | sha256(dir_signature) | 4511a263e1584936 | pass |
| SOTDQ_0.3.5.3v_WINDOWS（很牢） | sha256(main_package) | 912e1f9be50e6367 | skip |
| 魔法少女木叶  v2.04（中上） | sha256(dir_signature) | 441a1fc8fea0e8f6 | pass |
| SAO_verβ7 | sha256(dir_signature) | efcf8608567c20de | not_in_prior |
| 无法诉说的关系(中等) | sha256(main_package) | d7f069d3afd0a7af | partial |
| 栩的人生已经没救了（一般） | sha256(dir_signature) | 049558d3bb57bd47 | pass |
| 忍耐色诱的告白游戏 V2.0（不赖） | sha256(dir_signature) | 51ef49bf94031aa3 | pass |
| 魔剣士レーヴェ | sha256(main_package) | 118547451807f601 | partial |
| 希尔丝大冒险  1.06（夯） | sha256(dir_signature) | 666cb48bd086058e | pass |
| 呪いの鎧 v3.0.1（夯） | sha256(dir_signature) | 267f884317a4fc48 | fail |
| 詛咒铠甲2（夯） | sha256(dir_signature) | b865bbad91297298 | fail |
| 魔法学園見聞録 | sha256(dir_signature) | f125da7d18d25de7 | pass |
| Phoenixes-v17.0.1 | sha256(main_package) | a0366cb8e171f7d1 | not_in_prior |
| 冒険者リザシア | sha256(dir_signature) | 72de884bb3d832eb | not_in_prior |
| （一般）Touch Simulation Game | sha256(dir_signature) | d097c03f5dd316f5 | skip |
| （上等）哈尔法斯的恶魔 | sha256(dir_signature) | 3788d979527365a8 | pass |
| （一般）命に代えてもお守りします! | sha256(dir_signature) | 5802696163b87da0 | pass |
| (牛)完堕ちX兄嫁 ～アンタのせいよ！ 私、この浮気チ●ポじゃなきゃダメ | sha256(main_package) | e302eb5345f667f2 | partial |
| （一般）姉妹X催眠 ～絶対服従、俺がずっと王様～ | sha256(main_package) | 7fe5b8d5f5dc5134 | partial |
| （一般）姉妹X催眠2 ～絶対服従、セレブ妻と日焼けJKが堕ちる性感マッサ | sha256(main_package) | 521a4218dddb151c | partial |
| （一般）女王游戏X - 和妖精一起玩卡牌 クイーンズX - 妖精さんとカ | sha256(dir_signature) | 084236b4e9d16de9 | skip |
| （一般）Shitsuke-ai_dlsite | sha256(dir_signature) | 60b19050b986803c | pass |
| （中等）NTR Hunter | sha256(dir_signature) | 65e1ae05e9f2c87d | pass |
| 人柱阿拉卡~官方中文版~遊戲本體 | sha256(dir_signature) | a6253f903d526055 | not_in_prior |
| (中等)The Forbidden Tomes of Olipos | sha256(dir_signature) | 1b06b6192853b94d | pass |
| The Dungeon of Lulu Farea | sha256(dir_signature) | 0732959e6526e6d1 | not_in_prior |
| The Dungeon of Lulu Farea Rebirth v1 | sha256(dir_signature) | 9218a7e8d8facfee | not_in_prior |
| Lulumachi | sha256(dir_signature) | 45e71e2d6f820bee | not_in_prior |
| ボクは種付けアプリを手に入れた！ | sha256(dir_signature) | 7d2a593c2711539d | not_in_prior |
| (牛)RJ01680223 | sha256(dir_signature) | 1820f5b76d300aaf | pass |
| Chronicles of Clams Island | sha256(dir_signature) | 87980e8a45425306 | pass |
| Game | sha256(dir_signature) | c80c0c00368af1e7 | not_in_prior |
| 时勇者 | sha256(dir_signature) | fcb542be59494476 | not_in_prior |
| HOME | sha256(dir_signature) | 8109cb573cc702af | not_in_prior |
| 魔法 | sha256(dir_signature) | 51e2fcecd4d24e57 | not_in_prior |
| （夯）直到这个魅魔的心灵崩坏 | sha256(dir_signature) | 905d4f961d7fb922 | pass |
| (牛)RJ01618221 | sha256(dir_signature) | c4775775ddf99b4e | pass |
| Unholy_maiden_Ver.1.0.10 | sha256(dir_signature) | c35a810510d6ac73 | pass |
| （牛）间谍任务 | sha256(dir_signature) | a041922b4c09511b | pass |
| (夯)Stranded with You | sha256(dir_signature) | 75ec2eb851f9fc73 | not_in_prior |
| ROOM | sha256(dir_signature) | 2c233bb685c1d954 | not_in_prior |
| 072project_The Gloomy, Busty Half-El | sha256(dir_signature) | 8931f9ef9f949f4d | not_in_prior |
| 绝对合格 | sha256(dir_signature) | 1e287098d975cfa7 | not_in_prior |
