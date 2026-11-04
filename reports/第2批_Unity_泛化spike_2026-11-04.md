# 第 2 批 Unity 泛化 spike（2026-11-04）

> 对应 `reports/适配排期_按评价_2026-11-04.md` 第 2 批。
> 目标：把 `tools/unity_cg_export.py` 从「写死 bundle 名 / 手工传 `--assets`」泛化成自动发现容器，并摸清这批 Unity 游戏的真实形态差异。
> 对游戏目录只读；导出产物写在仓库外临时目录。

## 1. 布局普查（5 款实探 + 3 款深探）

| 游戏（评价） | 容器形态 | 骨架格式 | 可播性判定 |
|---|---|---|---|
| モンスターブラックマーケット（夯） | catalog.json + 31 个命名 bundle（`<角色>_assets_all.bundle`）+ `data.unity3d` | **二进制 `.skel`** | ❌ 渲染器只有 `SkeletonJson` |
| ElectronicStore（中等） | catalog.bin + 184 个哈希命名 bundle + 内嵌 `resources.assets` | 二进制（且部分纹理 m_Name 为空） | ❌ |
| 克蕾娅（夯） | catalog.bin + 69 个 bundle（名含 `himage`/`scg00` 线索）+ `data.unity3d`（578 纹理） | CG 本体是自定义 `HIDC` 分块 TextAsset | ❌ 需专用解析器 |
| ToxicMarriagePunisher（一般） | `StreamingAssets/ab/DefaultPackage/*.bundle`（107 个） | UnityPy 读出 0 对象（加密/包装容器） | ❌ |
| Tavern, Inn, and the Halberd（中等） | 仅 `data.unity3d` + `sharedassetsN.resource` | 未探 | 待查 |

**关键结论：这批 Unity 游戏与此前 3 款「一般」实测款（JSON 骨架/散装 bundle + GUID 头）完全不同。**
- Addressables 清单路径不再是 `Art/CG/<CG名>/`，而是 `Assets/50. Spine/<组>/<骨架>/`——`read_catalog` 的归属正则落空；
- 骨架普遍是二进制 `.skel`（Spine 4.x binary），JSON 骨架反而少见；
- 纹理跨 bundle 分离（骨架/atlas 在角色 bundle，图集页在 `image_assets_all.bundle`）——**按骨架基名跨容器归组**是唯一稳健策略。

## 2. 本次改动

### `tools/unity_cg_export.py`

- 新增 `--auto`：`find_containers()` 自动遍历游戏根（深度 ≤5，跳过 `Managed/`、`Plugins/`、`globalgamemanagers`/`levelN` 引擎场景容器），收集 `.bundle/.assets/.unity3d`，按 CG 命名线索 + 体积排序。
- 二进制骨架识别：`.skel`/`.skel.bytes`（首字节非 `{`）单独收集，导出 `<base>.skel + .atlas + 同名贴图`，索引 `kind="binary-skel"`——素材齐全但如实标记「webview 渲染器只有 SkeletonJson，暂不可播」。不再把二进制骨架静默丢进孤儿。
- 自检规则不受影响（binary-skel 不按 spine/frames 校验）。

### `internal/library/unity.go`

- `ScanUnity` 全部不可播时的报错改为**如实给出类型分布**（如 `binary-skel=1`）并指向原因（渲染器缺 SkeletonBinary / 导出日志），替代笼统的「kind=unknown」。新增 `TestScanUnityBinarySkel`。

## 3. spike 实测（モンスターブラックマーケット）

```
--auto --dry-run
  32 个容器 / 245 MB（31 bundle + data.unity3d，resource 与引擎容器已正确跳过）
  384 部 CG · PNG 1274 张 · 文本 768 个 · 3.4s
  类型：spine 0 · frames 0 · still 0（全部 binary-skel）

--auto --only bed_anna_1（真实导出）
  cg/bed_anna_1/: bed_anna_1.skel + .atlas + 3 张差分贴图（.png/_f/_f_o）
  自检通过：索引与磁盘一致；problems=0
  ucgr events → 「这 1 部 CG 都不可播（类型分布: binary-skel=1）」——门禁如实拦截
```

自动发现、跨容器按基名归组、诚实分类、索引自检、消费端门禁——**链路全部按设计工作**。384 部 CG 数量与 catalog 的 459 个 Spine id 量级吻合（差额 = Base/Dlc 等目录的骨架没有 atlas 配对，归入孤儿，符合「宁可留空也不错配」）。

## 4. 验证

```
py_compile                        → 通过
gofmt / vet / build               → 干净
go test ./...                     → 21 包全绿（含新增 TestScanUnityBinarySkel）
--auto dry-run / --only 实导       → 见上
```

本次改动文件：`tools/unity_cg_export.py`、`internal/library/unity.go`、`internal/library/unity_test.go`（均在 `feat/spine-skins-cover` 工作树）。

## 5. 结论与待办

- **`--auto` 已可用**：对这批 Unity 游戏能一键出索引 + 诚实分类，不再逐 bundle 手工传参。
- ~~**这批游戏的主缺口是二进制 Spine**~~ → 已深探，结论见 §6：MonsterBlackMarket 的骨架是**游戏自研编码**，标准 runtime 全系读不了；`binary-skel` 播放通道已打通并保留，但对这批游戏暂无用武之地。
- ~~**三个自研/加密变体暂缓**~~ → 更新：ToxicMarriage 的 ab 并非加密，只是 UnityFS 魔数藏在偏移 9923（见 §7），已全链路打通；剩余真变体是克蕾娅 `HIDC` 分块、ElectronicStore 空名纹理。
- 下一步（按收益排序）：① 桌面端 GUI 画廊选片的端到端（frames 通路 CLI 已闭环，见 §7）；② GensouBrothel 等立绘/背景型游戏的 `--still-prefix` 样例；③ 自研编码逆向只在用户明确要某款游戏时再投入。

## 6. SkeletonBinary 通道攻坚记录（2026-11-04 补）

目标：让 binary-skel 可播。通道本身已打通：

- `desktop/spine.go`：`SpineDTO` 加 `binary` 字段，`.skel` 二进制走 base64 下发；
- `desktop/frontend`：`SpineStage.tsx` 加 `SkeletonBinary` 加载分支（spine-webgl 4.2 自带），`api.ts` 同步类型；
- `unitycg.Playable` 接受 `binary-skel`（有文本+贴图即列事件），`ScanUnity` 报错如实给类型分布。

但**实测被数据打脸**：用 spine-core 4.2 解析 MonsterBlackMarket 的 `bed_anna_1.skel` 报 `Bone name must not be null`；查头部发现版本串 `4.0.61` 在字节 6（标准布局应在字节 9+），逐段对齐失败；GitHub 对比 4.0.31→4.0.64 标签的 `SkeletonBinary.ts` 仅 2 行无关差异——即**文件不是标准 Spine 二进制，是游戏自研运行时的改写格式**（明文只剩版本串、贴图路径、韩文路径等零散片段；用 Python 按 4.0 语义暴力扫描 0~8 字节哈希长度 × 24 偏移，无任何自洽组合）。

收口处理（都保持诚实）：

- 导出器加 `_skel_header_ok` 头部自检（版本串/画布 float 合理性）：**只有标准骨架才标 `binary-skel`**；自研编码归 `unknown` 并聚合一条可行动诊断（`382 个二进制骨架头部自检失败…`）。合成标准头单测通过。
- `ScanUnity` 门禁保持拦截，报错含类型分布。
- 通道对**真正的标准 4.1/4.2 二进制骨架有效**（未遇实机样本，标记为「通道就绪、未实测」）。

涉及文件：`desktop/spine.go`、`desktop/spine_test.go`（`TestLoadSpineAssetsBinarySkel`）、`desktop/replay.go`、`desktop/frontend/src/components/SpineStage.tsx`、`desktop/frontend/src/lib/api.ts`、`internal/adapter/unitycg/unitycg.go`（`IsSpine`）、`internal/library/unity.go`、`tools/unity_cg_export.py`（头部自检）。

## 7. frames 通路验收（2026-11-04 补）：ToxicMarriagePunisher 全链路打通

普查静图款时发现 ToxicMarriagePunisher 的 ab bundle 并非加密——**UnityFS 魔数在偏移 0x2683（9923 字节）**，此前 4KB 的 auto 搜索窗口漏判。修两处后全链路通过：

- `load_container` 的 auto 搜索窗口 4KB → 64KB；
- `export_embedded` 新增 `--frame-regex`（第 1 组 = CG 名，把同名前缀贴图归组成帧序列 CG）。

实测（`--auto --frame-regex "^(cg_animation_\d+)_\d+"`）：

```
109 个容器自动发现（107 ab + 内嵌 assets + 引擎容器排除）
14 部 frames CG（cg_animation_01~14，123 张 PNG / 134 MB），索引自检通过
ucgr events → 14 个场景事件全部可播
unitycg.BuildShow（cg_animation_01，13 帧 @12fps = 1079ms）→ IR JSON
ucgr play --assets <导出目录> --shot --at 0,500,1000
  → 帧序正确推进（_01 → _05 → _10），离线渲染 3 帧全部成功，人工检视画面完整清晰
```

**验收达成**：第 2 批「导出 → 添加 → 事件库 → 播放」的 CLI 全链路在 frames 通路闭环，且全程零新依赖（纯 Go 渲染）。余下待办：桌面端 GUI 画廊选片的端到端（复用既有会话框架，无新逻辑）；GensouBrothel 等立绘/背景型游戏的 `--still-prefix` 样例。
