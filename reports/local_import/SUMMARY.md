# 本地游戏批量导入 · 总报告（2026-11-21 00:58:37）

## 流水线结果

| 阶段 | 数量 | 产物 |
|---|---:|---|
| 候选游戏盘点 | 58 | 01_inventory.md |
| 去重后 keep | 57 | 02_dedup.md |
| 内容重复 internal | 0 | 02_dedup.md（排除运行时噪声与 junction 后 0 组） |
| 已入库重复跳过 | 1 | 02_dedup.md |
| 试导入 import_ok | 44 | import_results.json |
| 试导入 import_fail | 13 | failed.md |
| 试导入 import_timeout | 0 | failed.md |
| 有效性自检 suspect | 25 | suspect.md（不进 pass） |
| 有效性自检 pass | 19 | suspect.md |

## 红线遵守

- 未执行 `archify deliver` / `archify finalize`
- 全程只读用户原始游戏目录，未删改任何游戏文件
- 回滚快照：`snapshot/state.txt` + catalog.db/games.json/cgreplay.conf 备份
- 有效性自检复用 `tools/review-prescreen`，未重写空白判定逻辑

## 说明

- 导入入口：`bin/ucgr.exe events <目录> --out <events.json>`（项目主入口）；包装脚本 `tools/local_import/import_batch.py`。
- 去重口径：真资源包用主包 SHA256；rmmz/NW.js 目录型用目录内容签名（相对路径+大小排序 SHA256）。
- suspect 判据任一命中即降级：0 图 / 空白>50% / 尺寸全一致且<=128x128 / 总<100KB。

## 收尾分类表（任务要求）

| 类别 | 数量 | 去向 |
|---|---:|---|
| 新导入且自检通过 | 19 | 已 commit 到分支 dsh-0f13c1bf-6649（events.json 在 D:\UCGR_Work\local_import\，报告入库） |
| 疑似无效（suspect） | 25 | suspect.md，待人工翻图，不进 pass |
| 导入失败（import_fail） | 13 | failed.md，记 stderr 末 20 行 |
| 超时（import_timeout） | 0 | 无；可重跑 |
| 与已入库重复 | 1 | 跳过，未动 |
| 候选内部重复 | 0 | 无（排除 junction 与运行时噪声后 0 组），无需合并 |
