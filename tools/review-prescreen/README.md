
# review-prescreen · CG 导出目录机器预筛

人工逐张看几千张 CG 不现实。这个工具先把**最容易漏的坏图**用机器挑出来，
人只看可疑处：极小图、尺寸离群、空白帧、重复帧、命名断裂、体积异常。

纯 Go 标准库（`image/png`、`image/jpeg`），无第三方依赖，不联网，不改输入目录。
webp 只读文件头拿尺寸，不解码（标准库没有 webp 解码器），单独计数。

## 跑法

```powershell
# 全量：扫 D:\UCGR_Work\unitycg 下每款游戏
go run ./tools/review-prescreen -root "D:\UCGR_Work\unitycg"

# 只跑指定的几款（逗号分隔，大小写不敏感）
go run ./tools/review-prescreen -root "D:\UCGR_Work\unitycg" -only BloodOfYidhra,EroTavern,GoblinSword

# 阈值/输出可改
go run ./tools/review-prescreen -root "..." -var 30 -min-side 32 -small-kb 8 -max-sample 500 -conc 4
```

默认把结果写到（目录不存在会自动创建）：

- `reports/人工抽检/预筛指标.json` —— 每款游戏一份指标
- `reports/人工抽检/预筛可疑.md` —— 按 severity 降序的表格 + 每款一条结论 + severity=3 的具体文件名

## 源目录模式（rmmz 这类没有导出目录的款）

rmmz 的图不在导出产物里，直接躺在源游戏目录（`www/img`、`img`）。给 `-from-manifest` 就走这个模式：
按清单逐款进 `source_real_path`，递归找 png/jpg/webp，指标口径与导出目录模式完全一致，
另加「图片族分布」「加密图」「扩展名与格式不符」三项。

```powershell
# 扫 pass清单.json 里 engine=rmmz 的 37 款，输出 预筛指标_rmmz.json / 预筛可疑_rmmz.md
go run ./tools/review-prescreen -from-manifest "reports\人工抽检\pass清单.json" -engine rmmz -out "reports\人工抽检"

# 只跑其中一款
go run ./tools/review-prescreen -from-manifest "reports\人工抽检\pass清单.json" -engine rmmz -only "气息（上等）"

# 引擎素材也想算进指标（默认剔除）
go run ./tools/review-prescreen -from-manifest ... -engine rmmz -keep-assets
```

输出文件名按引擎自动取：`-out <目录>` → `预筛指标_<engine>.json` + `预筛可疑_<engine>.md`；
显式给 `-json` / `-md` 则以显式值为准。不给 `-out` 时落在 `reports/人工抽检/`。

### 源目录模式额外做的事

1. **img 子树定位**：递归找名为 `img` 的目录（不分大小写，`冒険者リザシア/www/img` 这种套一层的也能找到），
   父目录命中后不再往下钻；一款有多个 img 根（内嵌 + 外挂、更新档 + 本体）会全扫并在 `scan_roots` 列出。
   找不到 img 就回退扫游戏根目录（`note` 记一笔）。
2. **图片族分布**（`families`）：按相对 img 根的前两段路径分族，如 `pictures`、`pictures/busts`、`pictures/CG`。
   `kind=asset` 的族是引擎素材，默认**不参与打分**（否则 tilesets/characters 的重复与离群会把 CG 信号全冲掉）：
   RPG Maker 标准目录（tilesets / characters / enemies / sv_* / battlebacks* / parallaxes / animations / system / titles* …）
   + 族名里带 ui / icon / hud / menu / logo / font / button / window / cursor / frame 这类词的目录（按 ASCII 词边界匹配，不会误伤 rui 这种人名）。
3. **加密图**（`encrypted`）：`.rpgmvp` / `.png_` 的个数。这些图读得到文件名、读不到像素，
   所以「CG 族里没有明文图只剩加密图」直接给 severity 3 + 建议全量肉眼，并在 Markdown 里单列一节。
4. **扩展名与格式不符**（`misnamed`）：实测有 `.png` 其实是 JPEG（某款 228/1521 张）。图能看，但按扩展名解析的脚本会炸，
   现在按内容取尺寸并单独计数，不再误记成「文件头损坏」。
5. **散图判据只统计不扣分**（`skip_sparse`）：rmmz 的 `pictures/` 常是平铺的人写标题，前缀系列判据不适用；`-sparse-penalty` 可打开扣分。

## 参数

| 参数 | 默认 | 含义 |
| --- | --- | --- |
| `-root` | `D:\UCGR_Work\unitycg` | 导出根目录；其下每个子目录是一款游戏，游戏内优先取 `cg/` 子目录，没有就直接扫游戏目录 |
| `-only` | 空 | 只跑这些游戏（逗号分隔） |
| `-json` / `-md` | `reports/人工抽检/...` | 两个输出的路径 |
| `-max-sample` | 300 | 单目录最多抽样解码多少张 |
| `-max-pixels` | 9000000 | 解码像素上限（≈4096x2200）；超过的只读文件头，不解码 |
| `-conc` | 8 | 并发 goroutine 数，硬上限 8（不把磁盘打满） |
| `-min-side` | 64 | 极小图阈值：任一边小于它 |
| `-var` | 25 | 空白帧阈值：抽样灰度方差小于它 |
| `-small-kb` | 5 | 体积异常：单图小于它（KB） |
| `-outlier-mul` | 1.5 | 尺寸离群：偏离 (w,h) 众数超过它 |
| `-seed` | 20261116 | 抽样随机种子（固定种子 => 可复现） |
| `-quiet` | false | 不打印逐款进度 |
| `-from-manifest` | 空 | 源目录模式：pass清单.json 路径（给出即走源目录模式） |
| `-engine` | 空 | 源目录模式：只跑该引擎的条目（如 `rmmz`） |
| `-out` | 空 | 源目录模式：输出目录，文件名自动取 `预筛指标_<engine>.json` / `预筛可疑_<engine>.md` |
| `-keep-assets` | false | 源目录模式：引擎素材也参与打分 |
| `-sparse-penalty` | false | 源目录模式：散图也扣分 |

## 指标含义（对应 JSON 字段）

| 字段 | 含义 |
| --- | --- |
| `name` / `dir` | 游戏名 / 实际扫描到的图片目录 |
| `images` | 目录内 png/jpg/jpeg/webp 总数（spine 的 .json/.atlas/.skel 不计入，单独记在 `spine_files`） |
| `small` | **极小图**：文件头读出任一边 < 64px 的张数 |
| `outliers` | **尺寸离群**：以 (w,h) 精确众数为基准，宽或高任一方放大/缩小超过 1.5 倍的张数 |
| `blank` | **疑似纯色/空帧**：抽样解码后 32x32 网格灰度方差 < 25 的张数 |
| `dup_groups` | **重复组**：抽样解码张数内，同 (w,h) 且 8x8 灰度 dHash 完全相同的分桶，桶内 ≥2 张记一组 |
| `dup_rate` | **重复冗余率** = 各桶 (n-1) 之和 / 解码张数 |
| `sparse` | **散图**：独占目录且文件名与目录名对不上，或目录内多图但前缀不是该目录多数前缀的张数 |
| `avg_kb` | 目录总字节 / 图片数 / 1024 |
| `severity` | 0-3，下面权重表算出来的分档 |
| `mode_wh` / `mode_count` | 众数尺寸与其张数 |
| `sampled` / `decoded` | 抽样张数 / 实际解码成功张数 |
| `skipped_big` / `skipped_webp` / `decode_err` | 因超像素上限跳过 / webp 跳过 / 解码失败的张数 |
| `broken` | 文件头签名都不对的张数 |
| `exact_dup_groups` / `exact_dup_rate` | 同口径但用文件字节 SHA-1 判等：**真正同一份文件**的重复 |
| `tiny_files` | 单图 < 5KB 的张数 |
| `seq_gaps` / `seq_break_dirs` | 同一目录同前缀的编号序列缺口数 / 出现缺口的目录数 |
| `name_conflicts` | 跨目录重名的张数（同名不同目录） |
| `*_samples` | 每类最多 10 个文件名示例（`outlier_sizes` 是离群尺寸 top5 直方图） |
| `verdict` | `建议全量肉眼` / `抽检 30 张即可` / `无需看` |
| `engine` | 源目录模式才有：清单里的引擎名 |
| `families` | 源目录模式才有：`{name,count,kind}` 图片族分布，`kind=asset` 为引擎素材（默认不参与打分） |
| `scan_roots` / `scanned` / `asset_images` | 实际扫的 img 根 / 扫到的图总数 / 被当素材剔除的张数 |
| `encrypted` / `misnamed` | `.rpgmvp`、`.png_` 加密图个数 / 扩展名与实际格式不符的张数 |
| `manifest_dir` / `manifest_cg` | 清单里的 `source_dir`（游戏库路径）/ `cg_count` |
| `note` | 多 img 根、回退扫根目录这类情况的一句话说明 |

### severity 权重

各判据加权求和，`>=5` 记 3、`3-4` 记 2、`1-2` 记 1、`0` 记 0：

| 判据 | 条件 | 分 |
| --- | --- | --- |
| 文件头损坏 | broken > 0 | +3 |
| 空白帧 | blank/decoded > 20% / > 5% / > 0 | +3 / +2 / +1 |
| 抽样解码失败 | decode_err > 0 | +2 |
| 极小图 | small/images > 30% / > 0 | +2 / +1 |
| 尺寸离群 | 众数占 ≥50% 且离群 > 20% / 众数占 ≥50% 且离群 > 0 / **众数占 <50%**（尺寸本来就离散） | +2 / +1 / +1 |
| 重复冗余率 | > 60% / > 35% / 组数>0 且 >10% | +2 / +1 / +1 |
| 小文件 | <5KB 占比 > 20% / > 2% | +2 / +1 |
| 序列断裂 | 有缺口 / 缺口>10 处 | +1 / +2 |
| 散图 | 散图占比 > 80% | +1 |
| 平均体积 | 均单图 < 30KB | +1 |

## 性能与抽样

- **单目录最多解码 300 张**：按路径排序后取首 5 + 尾 5，其余用固定种子随机补足；目录 ≤300 张就全解。
- **先读文件头再决定解不解**：PNG 取 IHDR、JPEG 逐段走到 SOF、WebP 判 VP8/VP8L/VP8X；像素数超过 `-max-pixels` 的直接跳过（`skipped_big`），仍计入极小/离群/体积这些只看头的指标。
- **限并发**：读头和解码各用一个 `-conc` 大小的 worker 池，硬上限 8。
- 实跑 16 款 / 2756 张图，全量 12.3s（其中 544 张的 SwarmBunker 195ms、524 张的 MuniangTiaojiao 2.7s；最大头是几张 4K 图的解码）。

## 已知局限（照实写，不粉饰）

1. `dup_groups` 是**近似重复**：8x8 dHash 只比低频结构，同一模板换个数字也会撞桶
   （GoblinSword 的 `CGSample01..20` 就是这样：布局相同、只改了角标数字）。
   所以才同时给 `exact_dup_*`（字节级 SHA-1）：近似多、完全少 = 模板复用；两个都高 = 纯冗余。
2. **空白帧按抽样方差判**，32x32 网格会漏掉「大白底 + 一行小字」这类帧：
   MasyouSyoujo 的 `img_011g` 方差 0.0，肉眼是白底一行台词。这种归到「需人工确认」，不是自动判死。
3. **alpha 按白底合成**再取灰度。不这么做的话全透明像素会被当成黑色，spine 图集的透明边会把方差和 dHash 全部拉平。
4. 解码像素超过 `-max-pixels` 的大图不参与空白/重复统计（只参与极小/离群/体积），
   BloodOfYidhra 有 8 张、LoveConfessions 有 54 张因此没解码。
5. webp 不解码（标准库没有），只算尺寸。
6. 300 张抽样意味着 `blank` / `dup_rate` 是**抽样估计**，不是全量真值；`images`、`small`、`outliers`、`tiny_files` 是全量。
7. `sparse` 只表达「命名/目录结构散」，不代表图坏。

## 实跑记录（2026-11-16，D:\UCGR_Work\unitycg）

```
16 款 / 2756 张；severity 分布 0=1 1=8 2=2 3=5；全量 12.3s
```

severity=3 的 5 款：IdealEvesRoom、KanzenChikan、MasyouSyoujo、NTRaholic、SwarmBunker。
明细见 `reports/人工抽检/预筛可疑.md`。

## 实跑记录（2026-11-16，源目录模式）

```
go run ./tools/review-prescreen -from-manifest "reports\人工抽检\pass清单.json" -engine rmmz -out "reports\人工抽检"

37 款（缺目录 0）；扫到 png/jpg/webp 79013 张，参与打分 15151 张，加密图（.rpgmvp/.png_）77551 个
severity 分布 0=0 1=3 2=2 3=32；全量 18.3s（go run，编译缓存已热）
```

- 19 款 CG 族里**一张明文图都没有**（全 `.rpgmvp`），机器读不到像素，只能人工开游戏看 —— 这 19 款占了 severity=3 的大头。
- 其余 18 款有明文图可算；横向可比的排序见 `reports/人工抽检/预筛可疑_rmmz.md` 的「可机检款」表。
- 明细见 `reports/人工抽检/预筛可疑_rmmz.md`。