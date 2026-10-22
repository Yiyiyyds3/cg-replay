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
