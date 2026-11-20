# 本地游戏批量导入 v2 · 总报告（2026-11-21 02:48:05）

## 主台账（keep 57 款）

| 类别 | v1 | v2 |
|---|---:|---:|
| import_ok | 44 | **54** |
| import_fail | 13 | **3** |
| import_timeout | 0 | 0 |

## 已解决明细（fail → ok，7 款）

| 游戏 | events | 方式 |
|---|---:|---|
| 共妻之村 (牛) | 38 | spike-a 解包 data.xp3 → kirikiri 重扫 |
| 无法诉说的关系(中等) | 7 | 解包重扫 |
| (牛)完堕ちX兄嫁 | 70 | 解包 scenario+append.xp3，兼容台本生成 |
| （一般）姉妹X催眠 | 11 | 解包重扫 |
| （一般）姉妹X催眠2 | 14 | 解包重扫 |
| SOTDQ（很牢） | 461 | **Godot 4.3 pck 导出器**（新增 godot_cg_export.py，274 张压缩纹理未切如实记） |
| Phoenixes-v17.0.1（浴火重火） | 2596 | **Godot 3.6 pck 导出器**，1700.pck 全量 413MB |

## 剩余 6 款 fail 的定性

| 游戏 | 引擎定性 | 卡点 |
|---|---|---|
| 光辉的少女们 | TyranoBuilder（SRTS/SDTS 私有容器） | 容器格式未公开，需逆向 |
| wuxian / 绝对合格 | RPG Maker 插件驱动 | ScanRmmz 已加 Script 图片提取（本轮 D 线），以实测为准 |
| TouchSim / 女王X / HOME | Electron portable（asar 内嵌 exe） | 无 7z；TEMP 解压试验无产物；需 exe 级提取 |

## suspect 重自检；自检 v2：import_ok 44 中 pass 43 / suspect 1

- B 线：rpg_decrypt.py 解密 .rpgmvp/.png_ 到 D:\UCGR_Work\decrypt\，selfcheck_v2 用解密镜像重跑 prescreen（结果见 suspect.md v2 段）

## 红线遵守

- 全程只读游戏目录；解包/导出/解密产物全部在 D:\UCGR_Work\；未执行 archify deliver/finalize
- 回滚快照不变：reports/local_import/snapshot/

