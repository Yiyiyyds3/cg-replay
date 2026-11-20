# 10 款 fail 游戏 · `--auto` 导出器复跑报告

- 日期：2026-11-21
- 分支：`agent/p3-auto-rerun-fail`
- 任务：Agent-B / P3 —— 用已合入 main 的 `--auto` 导出器复试历史扫描 fail 的 10 款游戏
- 工具链：Python 3.12.10 + UnityPy 1.25.4（`C:\Users\Administrator\AppData\Local\Programs\Python\Python312\python.exe`）；`tools/unity_cg_export.py`（`--auto` 定义于 L688）；`go run ./cmd/spike-a -mode unity`（数可回放 events）
- 游戏库根：`E:\Personal_entertainment\game\ero\egame\_游戏库\按评价`（按评价分组，历史 117 款扫描同源）
- 导出产物：`D:\UCGR_Work\unitycg_auto\<短名>`（全部在仓库外，游戏目录零写入）

## 一、命令格式（从 README / tools/README.md 确认）

`--auto` **不是** `archify` 命令，而是本仓库 `tools/unity_cg_export.py` 的参数。正确用法：

```powershell
python.exe tools/unity_cg_export.py --game "<含 *_Data 的游戏根目录>" --out "D:\UCGR_Work\unitycg_auto\<短名>" --auto --force
go run ./cmd/spike-a -mode unity "D:\UCGR_Work\unitycg_auto\<短名>"
```

- `--auto`：自动发现游戏目录里的全部容器（.bundle/.assets/.unity3d），覆盖 catalog.json / catalog.bin / 散装 ab / 资源内嵌四种布局；**必须配 `--game`**。
- `--out` 默认拒绝写进 git 仓库（输出在 D: 盘，未加 `--allow-repo-output`）。
- 判定口径：`spike-a` 输出 `{"cgs":X,"events":Y,"scenes":Z}` 且 **Y（events）> 0 → pass**；`FAIL: 索引里没有可回放的 CG` 或 events=0 → fail。
- 注意：导出器在有 `problems`（如二进制骨架自检失败）时 **exit code = 1，但索引照常生成**，故最终状态以 events 为准，不单看退出码。

## 二、逐款结果（任务清单顺序）

| # | 游戏（任务名） | 实际目录 / 短名 | 状态 | events | 关键输出 |
|---|---|---|---|---|---|
| 1 | MonsterBlackMarket | 夯\モンスターブラックマーケット（夯） | **pass** | 2 | 32 容器/245MB，候选 CG 2 部，落盘 0.6MB；导出器 EXIT=1（382 个二进制骨架头部自检失败，疑似自研编码）；spike `events=2` |
| 2 | 克蕾娅 | 夯\(夯)克蕾娅 / Kureya | fail | 0 | 70 容器/388MB，候选 CG 0 部（孤儿 png 588/text 178）；spike `FAIL: 索引里没有可回放的 CG`；CG 本体为 HIDC 分块 TextAsset |
| 3 | Tavern | 中等\Tavern, Inn, and the Halberd（中等） | fail | 0 | 1 容器（data.unity3d）/208MB，候选 CG 0；1 个二进制骨架自检失败（样例 `01`）；spike 无 CG |
| 4 | Idol_Forging | 未评级\Idol_Forging_V1.0.1.23 | fail | 0 | 2797 容器/220MB，候选 CG 0（孤儿 png 114/text 3）；spike 无 CG；`--auto` 解决了历史命令行超长，但无配对骨架/图集仍归 0 |
| 5 | ToxicMarriagePunisher | 一搬\(一搬)ToxicMarriagePunisher | fail | 0 | 109 容器/612MB，候选 CG 0（孤儿 png 721/text 4）；spike 无 CG；CG 索引疑在 Luban 二进制表，`--auto` 无 still-regex 不识别 |
| 6 | ElectronicStore | 中等\ElectronicStore（中等） | **pass** | 31 | 186 容器/1505MB，候选 CG 32 部（spine 32），落盘 91.8MB；导出器 EXIT=1（16 个二进制骨架自检失败，样例 A01/A03/B01）；spike `events=31` |
| 7 | fallen | 未评级\fallen_1.0.5b | fail | 0 | 50 容器/384MB，候选 CG 0（孤儿 png 963/text 15）；spike 无 CG；纹理在 sactx SpriteAtlas 内，无独立 CG 命名 |
| 8 | Fen | 不赖\芙(不赖)\Fallen Elf Freya（FallenElfFreya） | fail | 0 | 2 容器/76MB，候选 CG 0（孤儿 png 30/text 7）；spike 无 CG；无 CG 命名族 |
| 9 | GensouBrothel | 不赖\GensouBrothel(Ver1.00)（不赖） | fail | 0 | 4 容器/130MB，候选 CG 0（孤儿 png 108/text 153）；spike 无 CG；纹理全为 UI/图集/背景 |
| 10 | Oshioki | 中上\Oshioki Lesson(中上) | fail | 0 | 11 容器/346MB，候选 CG 0（孤儿 png 877/text 9）；spike 无 CG；3D 摆拍型，CG 由渲染产出非贴图载体 |

> 名称映射说明：任务第 8 项的 "Fen" 在历史 fail 清单中记为「不赖/芙（Fallen Elf Freya）」，本机目录为 `不赖\芙(不赖)\Fallen Elf Freya`，即本报告第 8 行；其余 9 款与总报告「fail 10 款」逐款一致。

## 三、汇总

- **pass 2 / fail 8 / 环境限制跳过 0**（10 款游戏目录均存在且导出器全部正常执行完毕，无 E: 盘类环境阻断）。
- 救回 2 款：
  - **MonsterBlackMarket**：events 0 → 2（`--auto` 从 32 个 bundle 里识别出 2 部 binary-skel CG，量级极小但可回放）。
  - **ElectronicStore**：events 0 → 31（`--auto` 扫到 catalog 里 32 部 spine CG，是本次最大收益；此前 main 版导出器不解析 Addressables catalog.bin）。
- 仍 fail 8 款，归三类原因（与历史 spike 结论一致）：
  1. **自研/加密容器解析缺失**：克蕾娅（HIDC 分块）、Idol_Forging（哈希 bundle 清单）、Tavern（自研二进制骨架）、ToxicMarriagePunisher（Luban 二进制 CG 表）；
  2. **无 CG 命名族 / 图集未切分**：fallen（sactx SpriteAtlas）、Fen/芙、GensouBrothel（UI/背景纹理）、Oshioki（3D 材质贴花）；
  3. 共同表现：`--auto` 能发现容器、能列候选对象，但「骨架 JSON + atlas 配对」或「still-regex 命名族」缺失 → 候选 CG=0 → 无可回放事件。

## 四、复现与产物

- 单款原始日志：`D:\UCGR_Work\unitycg_auto\_logs\<短名>.log`（含完整 `--auto` 输出）；批处理汇总：`_logs\batch_stdout.log`。
- 批量脚本：`D:\UCGR_Work\unitycg_auto\run_batch.ps1`（仓库外，未入库）。
- 导出索引与素材：`D:\UCGR_Work\unitycg_auto\<短名>\cg-index.json` + `cg/`，全部在仓库外。
- 全程未执行 `archify deliver/finalize`，未做人工看图/UI 交互。
