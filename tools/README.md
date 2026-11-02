# tools/ — Python 原型脚本（历史存档）

> ⚠️ **已被 Go 核心接替**。这些脚本是 2026-10-15/16 的测量原型，
> 用于在写正式实现前验证「脚本解析 → 引用提取 → 匹配 → 保守保留」全链路的假设。
> 其结论（保留率、误裁规则、编码探测）已固化进仓库根的 Go 代码
> （`internal/adapter/kirikiri` + `internal/prune`）。
>
> **新开发请用 Go**：`ucgr scan / probe / refs / prune / engines`。
> 保留此处仅供：回溯判定规则的来龙去脉、处理 Go 版尚未覆盖的引擎。

| 文件 | 作用 | 状态 |
|---|---|---|
| `verify_engine_dist.py` | 47 款引擎分布复核（删除旧目录后重测） | 已完成使命 |
| `m0_census.py` | M0 四类体积测量（本体/归档/解包/引用） | 结论见 ../m0/ |
| `ks_refs.py` | KiriKiri 引用提取（编码探测/噪声过滤/保守保留/3-gram 索引） | ✅ 已移植到 Go |
| `ks_fix9.py` | 9nine CxEncryption 脚本补解（VM/结构分/首字节启发式） | **暂存**：依赖未入库的 peel.py，当前无法运行；结论已固化，Go 版暂不含解密 |
| `ks_reextract.py` | G線「假加密」重提取（zlib 段原样取出） | ✅ 已移植到 Go（probe/parse） |
| `cst_refs.py` | CatSystem2 .cst 引用提取（组匹配/take 后缀/E-mote 剥出） | 结论已固化；Go 版适配器待写 |
| `ast_refs.py` | Artemis .ast 引用提取（明文 Lua 表） | 结论已固化；Go 版适配器待写 |
| `probe_xp3.py` | XP3 索引解析（Python 参考实现） | ✅ 已移植到 Go |
| `extract_cst_text4.py` | .cst 解压/编码判别/条目解析（前人工具） | 结论已固化 |
| `unity_cg_export.py` | Unity/Addressables 里的 Spine CG 导出 + 索引 | ✅ **在用**（不是历史原型，见下） |

**移植对照（Go 侧）**：

| Python | Go |
|---|---|
| `ks_refs.decode_ks` | `internal/adapter/kirikiri.DecodeScript` |
| `ks_refs.parse_refs` | `internal/adapter/kirikiri.(*Ast).ExtractRefs` |
| `ks_refs.match_ref` | `internal/prune.(*Pool).Match` |
| `ks_refs.dynamic_candidates` + `build_substr_index` | `internal/prune.(*Pool).DynamicCandidates` |
| `ks_refs.weak_ok` | `internal/adapter/kirikiri.weakAccept` |
| `probe_xp3.parse` | `internal/adapter/kirikiri.ParseArchive` |

## 本地生成的数据文件（不入库）

下列结果文件由脚本在本机生成，已在仓库根 `.gitignore` 中忽略，需要时在本地重新生成。`m0/engine_dist_verify.json` 已确认含本机解包根路径，因此不入库。

| 文件 | 生成命令 |
|---|---|
| `tools/ks_refs_out.json` | `python tools/ks_refs.py --all --json tools/ks_refs_out.json` |
| `tools/ast_refs_out.json` | `python tools/ast_refs.py --all --json tools/ast_refs_out.json` |
| `tools/cst_refs_out.json` | `python tools/cst_refs.py --all --json tools/cst_refs_out.json` |
| `m0/engine_dist_verify.json` | `python tools/verify_engine_dist.py --root <解包根目录> --json m0/engine_dist_verify.json` |

`peel.py` 与 `mod_all.json` 未入库，因此 `ks_fix9.py` 标为暂存。

环境变量与占位符（运行上述脚本前需要设置）见 [docs/10_本地环境配置.md](../docs/10_本地环境配置.md)。


## unity_cg_export.py（现在仍在用，不属于上面的历史原型）

把 Unity 游戏里的 CG 从 AssetBundle 转成播放器能读的普通文件，并生成 `cg-index.json`。
它在这条链路里的位置与 VN 的解包器完全对应：

    VN:   .xp3/.pfs/.arc/.kif  → 解包器    → raw/data/image/*.png + *.ks
    ACT:  cg_assets_all.bundle → 本脚本    → cg/<CG名>/*.png + *.json + *.atlas

**为什么不能省这一层**：播放器跑在 webview，只认 PNG + Spine 的 `.atlas`/`.json`；
bundle 是 UnityFS（LZ4 + 序列化对象 + 未压缩像素流），webview 没有 Unity 运行时。

用法（**必须用装了 UnityPy 的 Python**，仓库自带的 `python3` 没有）：

```bash
"C:/Users/<你>/AppData/Local/Programs/Python/Python312/python.exe" tools/unity_cg_export.py \
  --game "E:/…/v0.7.5 步兵班" --out "D:/cgreplay/cg/thornsin"
# 只看索引不落盘：--dry-run    只导一部：--only HCG02    全量重写：--force
```

`--only` 会读旧 `cg-index.json` 合并写回（不会把整库索引删成只剩一部）。
输出默认必须落在 git 仓库外；确实要写进仓库时加 `--allow-repo-output`，
否则 100+ MB 的 PNG 很容易被 `git add -A` 顺手带上。

产出：

```
<out>/cg/<CG名>/  X.png · X.atlas · X.json …
<out>/cg-index.json
```

索引里每部 CG 记录：`kind`（spine / frames / still）、`category`（战斗H·哥布林 / 商店 / 过场…）、
`event_group`（匹配到的 DialogueData 组，**高置信才填，宁可留空也不错配**）、
`characters`（从贴图/皮肤名抽到的角色词）、`spine`（骨骼/插槽/皮肤/每段动画时长）。

实测（棘罪 v0.7.5）：bundle 686 MB → 落盘 **111.4 MB**，22 部 CG（spine 16 / 逐帧 4 / 单图 2），
8.8 万像素级贴图 68 张，自检通过。

**它不做的**：不渲染、不播放、不改游戏、不碰存档。渲染是 spine runtime 的事。
