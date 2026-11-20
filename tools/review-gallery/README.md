# review-gallery · 人工抽检画廊生成器

把「游戏库验收 pass 清单」渲染成一份**单文件、自包含**的 HTML 画廊，用来肉眼验收 pass 款游戏的 CG 导出效果。

- 不生成缩略图、不拷贝原图：页面里的 `<img>` 直接指原始绝对路径（`file://`），像素尺寸从文件头解析；
- 不依赖 CDN / 外网：CSS、JS 全部内联，`rg 'https?://'` 对产物零命中；
- 判定结果存浏览器 localStorage，可一键导出 CSV。
- **v1.1 起是「能带着线索快速过图」的画布**：顶部目录导航（53 款锚点，贴顶可折叠）、每款来源徽章（导出产物 / 源目录预览）、每款预筛线索条（severity / verdict / small / outliers / blank / dup_rate / avg_kb / tiny_files）、顶部纯前端筛选按钮（全部 / 导出产物 / rmmz 源目录 / 大款 / 预筛⚠）。

---

## 1. 一条命令重新生成

在仓库根目录（`E:\Software_tools\DevSpace\Projects\Agent\Unified_CG_Replay_Library`）执行：

```powershell
# 编译（产物落在 tools/review-gallery/ 内，不污染仓库根目录）
go build -o tools/review-gallery/review-gallery.exe ./tools/review-gallery

# 生成 → out/review/index.html
go run ./tools/review-gallery
```

`go run ./tools/review-gallery` 不带参数即可：优先读 `reports\人工抽检\pass清单.json`（本库现有 53 款，验收侧产出的裸数组），清单缺失时才从 `reports/audit/**` 自动发现。等价于：

```powershell
go run ./tools/review-gallery -list "reports\人工抽检\pass清单.json" -out "out\review\index.html" -reports "reports" -unitycg "D:\UCGR_Work\unitycg" -auto-list "tools\review-gallery\.cache\pass清单.auto.json"
```

运行完 stdout 两行，第一行统计、第二行产物绝对路径：

```
[prescreen] 加载 预筛指标.json + 预筛指标_rmmz.json · 覆盖 53/53 款 · 跳过 （无）
games=53 images_shown=5600 images_found=18689 bytes_found=8699008812 bytes_shown=4622423965 truncated_games=13 encrypted_games=21 missing_games=0 html_bytes=3045056 elapsed=4.327s
html=E:\Software_tools\DevSpace\Projects\Agent\Unified_CG_Replay_Library\out\review\index.html
```

### 参数

| 参数 | 默认 | 说明 |
|---|---|---|
| `-list` | `reports\人工抽检\pass清单.json` | pass 清单 JSON；**不存在**则自动发现 |
| `-out` | `out\review\index.html` | 输出 HTML（目录自动创建） |
| `-reports` | `reports` | 自动发现的数据源目录（读其 `audit\` 子目录） |
| `-unitycg` | `D:\UCGR_Work\unitycg` | Unity 导出 cg-index 根目录 |
| `-auto-list` | `tools\review-gallery\.cache\pass清单.auto.json` | 自动发现出来的临时清单落盘位置（可当模板改） |
| `-max-per-game` | `0` | 覆盖单款渲染上限；0 表示按清单里的 `max_images` |
| `-title` | `UCGR 人工抽检画廊` | 页面标题 |
| `-prescreen` | `reports\人工抽检\预筛指标.json` | 预筛指标（unity 侧）；缺失则跳过，页面显示「未收录」 |
| `-prescreen-rmmz` | `reports\人工抽检\预筛指标_rmmz.json` | 预筛指标（rmmz 侧，可选）；存在时与上面**按 name 合并** |
| `-quiet` | `false` | 只输出最终统计行 |

### 自动发现逻辑（清单缺失时）

1. **rmmz**：读 `reports/audit/batch-1.json … batch-7.json` 与 `batch-rmmz-recheck.json`，取 `verdict == "pass"` 的款；同名款按批次顺序**后者覆盖前者**（复扫口径）。目录取记录里的 `real_path`（回退 `path`），图片根取该款 `<img>/pictures`（限深 5 层，兼容「外层多套一层游戏目录」的款）。
2. **unity**：读 `reports/audit/unity-batch1..4/summary.json`，取 `verdict == "pass"` 的款，按 `short` 名解析 `D:\UCGR_Work\unitycg\<dir>`；`short` 与目录名不一致的（`OneSixNine` → `1692614`）回退到「目录名出现在游戏名里」的最长匹配。
3. 结果写进 `-auto-list`，同时作为本次渲染输入。本库当前应得 **53 款（rmmz 37 + unity 16）**。

---

## 2. 清单 JSON 字段格式

工具接受**两种**顶层形态：

1. 对象式（本工具 `-auto-list` 产出的形态）：

```json
{
  "schema": 1,
  "generated_at": "2026-11-16T10:22:41+08:00",
  "source": "auto-discover: reports/audit/batch-1..7.json + batch-rmmz-recheck.json + unity-batch1..4/summary.json",
  "games": [ { "name": "…", "cg_dir": "…" } ]
}
```

2. 裸数组式（仓库现有 `reports/人工抽检/pass清单.json` 的形态，由验收侧产出）：

```json
[
  { "id": 1, "name": "女主角（大雷）", "engine": "rmmz", "batch": "batch1", "status": "pass",
    "cg_index_dir": null, "source_dir": "E:\\…\\女主角（大雷）",
    "source_real_path": "E:\\…\\20260424\\RPG\\大雷\\女主角（大雷）", "events": 44 }
]
```

单款字段按**别名**取值，两套写法都能吃：

`cg_dir` ← `cg_dir` / `cg_index_dir` / `source_real_path` / `source_dir`（按顺序取第一个非空）；
`note` ← `note` / `cg_index_note`；`id` 可以是字符串或数字（数字会转成 `g001` 形态）；其余字段同名直取。

`games[]` 每项：

| 字段 | 必填 | 类型 | 说明 |
|---|---|---|---|
| `name` | 必填 | string | 游戏名，直接作为 section 标题 |
| `cg_dir` | 必填 | string | cg-index 目录的**绝对路径**（unity 为导出目录，rmmz 为游戏根目录）。缺失/不可读时本款标 `missing_dir` |
| `id` | 可选 | string | 稳定 id（localStorage 的键）；缺省按序号生成 `g001`、`g002`… |
| `group` | 可选 | string | 评价组（大雷/很牢/牢/拉/一般/未评级/中等/中上/不赖/夯/创意/上等） |
| `batch` | 可选 | string | 批次标签，如 `batch-6`、`unity-batch4` |
| `engine` | 可选 | string | `rmmz` / `unity`；影响默认扫描根 |
| `image_root` | 可选 | string | **直接指定扫描根**（绝对路径），优先级最高。rmmz 自动发现即用它指向 `…\img\pictures` |
| `image_subdirs` | 可选 | string[] | 相对 `cg_dir` 的子目录列表（`/` 或 `\` 分隔均可），多目录合并 |
| `max_images` | 可选 | int | 单款渲染上限（>0 生效）。超出按**等步长抽样**，不是截断尾部 |
| `note` | 可选 | string | 清单侧备注，显示在标题条下方（与页面里人工填的备注不是同一份） |

不给 `image_root` / `image_subdirs` 时的默认探测：

- `engine == "unity"` → `<cg_dir>\cg`，不存在则用 `<cg_dir>`；
- 其它 → 依次试 `img\pictures`、`www\img\pictures`、`img\cg`、`img`、`www\img`，取第一个存在的。

最小清单示例：

```json
{
  "schema": 1,
  "games": [
    { "name": "EroTavern", "group": "上等", "batch": "unity-batch3", "engine": "unity",
      "cg_dir": "D:\\UCGR_Work\\unitycg\\EroTavern" },
    { "name": "气息（上等）", "group": "上等", "batch": "batch-7", "engine": "rmmz",
      "cg_dir": "E:\\Personal_entertainment\\game\\ero\\egame\\20260612\\RPG\\上等\\气息（上等）",
      "max_images": 120 }
  ]
}
```

> 想固化清单：把 `tools/review-gallery/.cache/pass清单.auto.json` 复制成 `reports/人工抽检/pass清单.json` 再改，之后 `go run ./tools/review-gallery` 会直接用它。

---

## 3. 页面怎么用

- **顶部汇总条**：游戏数 / 总图数（括号里是目录内实际发现数）/ 总体积 / 已判定 `n/53` / 当前游戏序号，下面是进度条与快捷键提示；
- **筛选条 + 目录导航**：见 3.1（纯前端筛选、贴顶可折叠的 53 款目录）；
- **每款 section**：标题条给出 游戏名 · **来源徽章** · **预筛 severity 警示** · 评价组 · 批次 · 引擎 · `CG 数 · 体积` · 状态，下面紧跟**预筛线索条**；下面是 **cg-index 绝对路径**（`user-select:all`，可整段选中）与「复制路径」按钮；
- **缩略图网格**：`<img loading="lazy">` + `object-fit: contain`，卡片下方是**文件名**（basename，完整相对路径挂在卡片 `title` 上）与 `宽×高 · 体积`；
- **判定**：每款一个「结论」下拉（未判定/好/小问题/严重/待查）+ 一个备注输入框，改动即写 localStorage；
- **导出 CSV**：把**全部 53 行**（含未判定的空结论）一次性下载成 `review-verdicts-YYYYMMDD-HHmm.csv`（UTF-8 BOM，Excel 打开不乱码）。列：游戏/组/批次/引擎/图片数/体积字节/cg目录/结论/备注/更新时间ISO。

快捷键（焦点不在输入框/下拉时生效）：

| 键 | 行为 |
|---|---|
| `←` / `→` | 上一个 / 下一个游戏（滚动定位并高亮） |
| `J` / `K` | 向下 / 向上滚一屏（85% 视口高） |
| `1` `2` `3` `4` | 给**当前游戏**标记 好 / 小问题 / 严重 / 待查 |

localStorage 键：`ucgrReviewGallery.v1`，结构 `{v:1, items:{g001:{v:"good", note:"…", ts:1699999999999}}}`。
若浏览器在 `file://` 下禁用 localStorage（少数策略场景），页面顶部会出现黄色提示条，此时判定只存在内存里 —— 用「导出 CSV」保存。

---

### 3.1 目录导航 / 来源徽章 / 预筛线索 / 筛选（v1.1）

> 这一节是 v1.1 加的四件套；规则写死在这里，页面表现与之一一对应。

#### 目录导航（贴顶、可折叠）

页面最上方「目录导航 · 53 款」：每条是「编号 + 游戏名 + CG数 + 预筛 severity」，点一下跳到对应 section 并自动收起面板；页面下滑超过 180px 也会自动收起（标题条常驻顶部）。被当前筛选隐藏的款在目录里变暗 —— 仍然可点，点了会自动切回「全部」，不会出现"点了没反应"。

#### 来源徽章（判定证据等级）

每款 section 标题条挂一枚徽章，机械判定，不看备注：

| 徽章 | 条件 | 含义 |
|---|---|---|
| 绿「导出产物（一级证据）」 | engine 是 unity 或 spike **且** cg_index_dir 非空 | 看的是导出链路产物，canonical CG 证据 |
| 黄「rmmz 源目录预览（无导出物）」 | engine = rmmz，**或** cg_index_dir 为空 | 没有独立 CG 索引库，页面直接读游戏源目录（source_real_path → source_dir），看到的是原始素材而非导出结果 |

判定用的是清单里 **cg_index_dir 原文**，不是回退后的 cg_dir：rmmz 款的 cg_dir 会回退到 source_dir，只有单独记住"有没有真实索引目录"才分得清两者。徽章的 tooltip 带 engine 与具体路径，可核对。本库：绿 16 款 / 黄 37 款。

#### 预筛线索条

每款 section 内嵌一条线索条，数据源：

- reports/人工抽检/预筛指标.json（unity 侧，本库 16 款）；
- reports/人工抽检/预筛指标_rmmz.json（rmmz 侧，**可选**；本库已存在，37 款；缺了就跳过并记进页脚）。

两份**按 name 合并**：同名款按字段覆盖（后加载者赢），所以把 rmmz 指标补进仓库后，同一款会同时显示两边字段。展示字段：

severity · verdict · small · outliers · blank · dup_rate · avg_kb · tiny_files，右下角标出该条来自哪个文件（如「源 预筛指标.json」）。

容错口径（JSON 字段可能缺、类型可能变）：

- 数字与数字字符串都吃；dup_rate 按比例算（0.0588 → 5.9%，大于 1 视为已是百分数）；
- 任何字段缺失都渲染成灰虚线「xxx n/a」，**不猜值、不隐藏**；
- 整款没记录 → 灰条「预筛指标未收录本款」；
- 文件缺失 / JSON 坏掉 → 只打 [prescreen] ... 跳过 日志，生成不中断。

severity >= 2 的款在标题条额外挂「⚠ 预筛 severity N」（tooltip 是 severity_reasons 原文；本库 41 款 = unity 7 + rmmz 34）。匹配顺序：同名 → 忽略大小写同名 → 目录名（预筛 dir 的 .../<Dir>/cg → <Dir>，对清单 cg_index_dir 的 basename）→ 最长子串。本库 16 款 unity 按目录名全部命中、37 款 rmmz 按同名直连（OneSixNine 在预筛里叫 1692614，靠目录名对上）。

#### 筛选按钮（纯前端，不刷新）

| 按钮 | 判定 | 本库计数 |
|---|---|---|
| 全部 | 不筛选 | 53 |
| 只看导出产物 | 与绿徽章同规则（data-src=export） | 16 |
| 只看 rmmz 源目录 | 与黄徽章同规则（data-src=source） | 37 |
| 只看大款 cg_count>200 | 清单 cg_count > 200（缺失回退扫描发现数） | 20 |
| 只看预筛⚠ severity≥2 | severity >= 2 | 41 |

切换只改 section.game 的 hidden 类：不重载页面、不写 localStorage，右侧常驻「当前可见 n/53 款」。**判定（1–4 键 /「只看未判定」/ CSV 导出）逻辑与时序都不变**：已判定计数与 CSV 永远按全部 53 款出，筛选只是换视角。

---

## 4. 已知限制

### 4.1 图片缺失 / 不可渲染的目录如何标记

每款 section 都带 `data-status` 与标题条状态 chip，取值四态：

| status | 触发条件 | 页面表现 |
|---|---|---|
| `ok` | 扫到 ≥1 张 png/jpg/webp | 正常网格 |
| `missing_dir` | `cg_dir` 不存在或不可读（目录被移走、盘符没挂） | 红色 chip「目录缺失」+ 琥珀色占位块，写明 `status=missing_dir` 与路径 |
| `encrypted` | 目录里只有 RPG Maker 加密图（`.png_` / `.jpg_` / `.rpgmvp` 等），明文图为 0 | 红色 chip「加密图不可渲染」+ 占位块，写明加密图**个数** |
| `no_images` | 目录存在但没有任何 png/jpg/webp | 黄色 chip「无明文图片」+ 占位块 |

**四种状态都照样计入清单条数、照样可以填结论/备注、照样进 CSV** ——「图片缺失」本身就是要人工记录的验收结论。

本库实测（2026-11-16）：53 款里 `missing_dir` 0 款、`encrypted` 21 款（全是 rmmz 加密图款，如 `呪いの鎧 v3.0.1`、`詛咒铠甲2`、`苍红界域（夯）`）。这 21 款要肉眼验收，需先用项目内的解密/导出链路产出明文 png，再把该款 `image_root` 指到明文目录。

### 4.2 单款抽样上限

rmmz 的 `img/pictures` 单款可达 3000+ 张（本库 16 款明文合计 15933 张），全量内联会让 HTML 到 10MB 量级。自动发现因此给 rmmz 款 `max_images = 200`，按**等步长**抽样（覆盖整个目录，不是只取前 200 张）；unity 导出款不设上限（单款最多 544 张）。

所以「总图数」有两个数：页面渲染张数（`images_shown`）与目录内发现张数（`images_found`），标题条会显示「显示 200/715」这类 chip。要全量看某款，把清单里的 `max_images` 删掉、并用 `-max-per-game 0` 跑一次即可。

### 4.3 其他

- **`file://` 图像加载**：用 Chrome / Edge 直接双击打开页面即可；个别浏览器或加了 `--disable-local-file-access` 类策略会拦本地子资源，表现为缩略图全空 —— 换 Chrome/Edge 打开即可，路径本身是好的（可用标题条里的路径手动核对）。
- **尺寸未知**：极少数文件是「内容全 0 的占位 png」或非 PNG/JPEG/WebP 的真实格式（本库 1 张：`气息（上等）…\712测试用图.png`，前 32 字节全 0），此时卡片显示「尺寸未知」而不是猜一个数。
- **不拷贝、不转码**：页面只引用原图，浏览器按需缩放；原图被移动/删除后页面立即失效（这也是「验收时看到的就是原始导出物」的保证）。
- **只读**：生成器不写 `reports/`、不改仓库其它文件；唯一产物是 `out/review/index.html`（外加 `.cache/` 下的临时清单）。

---

## 5. 目录结构

```
tools/review-gallery/
├── main.go               # 生成器（Go 单文件，仅标准库：清单解析 / 自动发现 / 扫描 / 取尺寸 / 渲染）
├── assets/gallery.css    # 内联进 HTML 的样式（//go:embed）
├── assets/gallery.js     # 内联进 HTML 的脚本（快捷键 / localStorage / CSV）
├── README.md             # 本文件
└── .cache/               # 自动发现临时清单 + chrome 预览截图（可删）
```

产物：`out/review/index.html`（本库实测 3,045,056 字节，53 款 / 5,600 张 <img> / 53 条目录锚点 / 5 个筛选按钮；v1.1 之前是 2,949,600 字节）。
