# 游戏库验收 · A 档实操 Spike 报告

- 仓库基线：`main @ 3e6d527`
- 日期：2026-11-15
- 目标：对 3 款失败/部分通过的游戏（1 Unity + 1 KiriKiri + 1 rmmz）做实操验证，目标是至少一款通过 `ScanWith` 且 `events > 0`。
- 红线遵守：全程未对任何游戏目录做写操作；所有解包产物仅落在 `D:\UCGR_Work\`。

## 结论速览

| 引擎 | 游戏 | 原判定 | 新判定 | 结论 |
|---|---|---|---|---|
| Unity (IL2CPP) | （一般）1692614 | 通过（部分） | **PASS**：events=60, scenes=60 | 通过 |
| KiriKiri | 姉妹X催眠 | 部分通过 | 部分通过：total=22, scenes=11, cgs=0（图片加密） | 如实记录，未达标 |
| rmmz (MV/MZ) | 呪いの鎧 v3.0.1（夯） | **fail（events=0）** | **PASS：events=222, scenes=222, warn=0** | 通过（本次 spike 修复） |

三款里两款达到「events > 0」目标，其中 rmmz 一款由本次 spike 代码修复直接救回。

## ① Unity（IL2CPP）：《（一般）1692614》

- 实操：`spike-a -mode unity`，走内嵌 assets 模式（该作没有 StreamingAssets/aa，Addressables 数据内嵌在 assets 中）。
- 结果：`{"events":60,"scenes":60,"cgs":0}`，检出 60 个 spine CG 场景。
- 产物：`D:\UCGR_Work\unitycg\1692614\`，60 个 spine cg 目录，约 47.5MB。
- 工具链：UnityPy 1.25.4（Python 3.12.10）。IL2CPP 游戏不依赖 Assembly-CSharp.dll，走 GameAssembly/global-metadata 路径不受影响。

## ② KiriKiri：《姉妹X催眠》

- 实操：`spike-a -mode unpack` 解包 XP3（`ScopeCG`、跳过视频），产物 `D:\UCGR_Work\unpack\`：1669 个文件、301,916,564 字节。
- 结果：`{"total":22,"scenes":11,"cgs":0,"warn":…}`。
- 未达标原因（诚实记录）：CG 图片文件为加密的 `eh_*.png`，解包后仍无法作为普通 PNG 读取，CG 事件无法确认，`cgs=0`。脚本层场景解析正常。
- 结论：保持「部分通过」，不做假通过。

## ③ rmmz（MV/MZ）：《呪いの鎧 v3.0.1（夯）》——本次修复

### 数据层发现

- `www/data` 全部 JSON 中 code 231（显示图片）/232（移动图片）计数为 **0**；CG 演出全部走 code 355/655 脚本指令 `this.cgLoad('picNNN', i)`（命中 CommonEvents.json 与 Map003/006/014/017/024.json 等）。
- 图片素材是**子目录式**：`www/img/pictures/<组名>/<i>_<j>.rpgmvp`（如 `pictures/pic001/1_1.rpgmvp`），`.rpgmvp` 带 16 字节 `RPGMV` 自定义标头。
- 这是 Wolfzq CG 插件（实现在 `www/js/plugins/Z_All.js`）的演出方式：`$wcgs[c][e]` 差分图层表 + `$wcgCounts` 每组张数（pic001–pic047+），另含 pic002↔pic003 等变体替换与 b/v 后缀差分——扫描层只需「组名 → pictures 子目录存在性」。

### 代码修复（`internal/adapter/rmmz/rmmz.go`）

1. 新增 `scriptCGRe` 正则 + `scriptCGName(c Command)`：识别 code 355/655 中 `cgLoad('组名')` 脚本指令。
2. 新增 `locateCGSet(root, imageDirs, name)`：在图片目录下定位 `pictures/<组名>/` 子目录，取第一个文件作为量级判断样本。
3. `Segments` 起段条件扩展：`code 231 || scriptCGName != ""`。
4. `addSegments` 对脚本 CG 组按「组内样本图通过 `pictureLarge` 量级校验 → 计 1 张、组名作预览」接入 pics/texts 统计。

### 验证

- `DecodeImage` 对 16 字节标头 `.rpgmvp` 的 `len>32 + data[16:32]` 解密假设**被实证成立**（scenes=222 意味着真实解码了图片量级），未需修补。
- 修复后：`{"cgs":0,"events":222,"scenes":222,"warn":0}`（原判定 `fail | 0 | 没有识别到图片演出…`）。
- 新增单测：`TestScriptCGName`（355/655 命中、231/401 不误报）、`TestLocateCGSet`（子目录命中/未命中）、`TestSegmentsScriptCG`（cgLoad 开段、picsOut 收段）。

### 备注

- `library.DetectEngine` 现覆盖 kirikiri/artemis/bgi/catsystem2/scn/nscript/unity/rmmz/nexas/nitro；rmmz 适配器未走 `adapter.Register` 竞聘，由 library 内分支调用——现状可用，留作后续统一。
- 备用候选（本次未启用）：`詛咒铠甲2（夯）` warn 1231、`wuxian` warn 13。

## 验证与提交

```
go build ./...   # 通过（无输出）
go vet ./...     # 通过（无输出）
go test ./...    # 30 个包全部 ok，无 FAIL
```

- 提交内容：`internal/adapter/rmmz/rmmz.go`（脚本 CG 修复）、`rmmz_test.go`（单测）、`cmd/spike-a/main.go`（spike 工具，scan 模式加 rmmz 分支）、`reports/游戏库验收_A档spike报告.md`、`reports/audit/` 判定 JSONs。
- 未提交任何 `D:\UCGR_Work\` 产物或游戏目录文件。
