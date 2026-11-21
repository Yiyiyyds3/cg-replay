# UCGR 同类项目调研 · 方向 D：立绘差分合成 与 事件回放/场景回放工具

调研时间：2026-11-21 ｜ 调研员：子代理 46c6bd33（方向 D）

**结论先行**：方向 D 找到 12 个已验证真实存在的相关项目（GitHub 页面均 200 打开，star/更新时间来自 shields.io 徽章）。**「游戏事件→CG 序列」映射没有现成通用实现**——只有组件级原料（脚本解析器/翻译管线/引擎内置回放），这是 UCGR 的差异化机会。差分合成的规则来源在现有工具中呈 5 种模式（容器内嵌 JSON / 外置坐标文件 / 文件名约定 / 声明式 DSL / 场景脚本），全部可归纳为 UCGR 可用的设计。

**调研环境限制（影响证据质量，如实说明）**：本会话出口网络屏蔽 raw.githubusercontent.com / cdn.jsdelivr.net / raw.githack.com / r.jina.ai / web.archive.org / deepwiki.com；api.github.com 被限流（"API rate limit exceeded"）；GitHub 仓库页可达但正文被截断。因此：仓库存在性=直接抓 github.com 页面（HTTP 200 + og:description 验证）；star 数与最近提交=shields.io 徽章；README 正文大多未能直读，功能细节以页面描述 + 可访问的第三方教程文（galgame.live）交叉验证，条目中已逐项标注可信度。

---

## 一、项目条目

### 1. FreeMote（差分容器工具，本方向核心）
- URL: https://github.com/UlyssesWu/FreeMote
- 活跃度: ★530，最近提交 2026-10（仍在维护），31 forks
- 核心功能: Managed Emote/PSB 工具库。Emote 是 KiriKiri 系游戏常用的立绘差分容器（.psb/.pimg）：PsbDecompile 把容器拆成「散图部件文件夹 + 一个 JSON 场景图（内嵌每个部件的坐标、层级、可见性）」，可重组回原立绘/CG；PSB↔JSON 双向。社区教程（galgame.live 2026-01《拒绝付费图包！…FreeMote 与自动化工具完美拆解合成 Galgame CG》）实证了流程：.pimg 拖到 PsbDecompile → 同名文件夹 + JSON「施工图纸」。
- 技术栈: C#/.NET，提供库（FreeMote.Psb/FreeMote.Toolkit）+ 命令行 exe。
- 对 UCGR 可借鉴: ①**差分规则来源第 1 类：容器内嵌结构化规则**——krkr pimg/psb 拆出的 JSON 就是现成的坐标+层级表，UCGR 的 krkr 管线（已有 TLG 解码）可以接在它后面：部件图 + JSON → 自动合成事件 CG；②把「拆包产物=散图+规则文件」作为 UCGR 导入模型的中间表示。
- 局限: 只覆盖 Emote/PSB 系引擎；命令行无 GUI；库本身不做批量/gallery 管理。

### 2. krkrFgiEditor（吉里吉里立绘合成工具）
- URL: https://github.com/SHMAssociation/krkrFgiEditor
- 活跃度: ★0，最近提交 2022-09（页面 og:description=「吉里吉里立绘合成工具」，中文项目）
- 核心功能: 读取 krkr FGI 立绘容器，按图层组自动拼接差分：规则文件（.json/.txt）+ 图层组排序（教程文明确：Group1 底图在最下、中间表情差分、最上鬓发等覆盖件）→ 全自动拼接。
- 技术栈: 未读到正文（README 被 GitHub 页面截断），C# 可能性大。
- 对 UCGR 可借鉴: **默认 z-order 约定**——「底图→表情差分→前景覆盖件」的固定分组顺序可直接作为 UCGR 合成引擎的缺省图层语义；「差分项可勾选剔除」的交互。
- 局限: 零星维护、无文档、仅 FGI 格式、无 star 验证（无人用）。

### 3. GalGameCG（Sh3nx1 出品的自动化差分合成工具）
- URL: GitHub 仓库未能定位（多轮搜索无果）；唯一实证来源: https://galgame.live/routes/galgame-cg-synthesis-tool-sh3nx1 （2026-01 实测文）
- 核心功能（教程文描述）: 1.5MB 绿色单文件，三种模式——①**坐标差分自动合成**（吃 krkr pimg 拆出的 JSON 坐标文件，覆盖 90% 场景）；②**固定差分自动合成**（Unity 系游戏，CSV 固定坐标图层）；③手动/半自动模式（加密坐标等奇葩情况）。UI 三区：上方配置区（导入 TXT/JSON 坐标文件、废弃差分勾选剔除、手动把基底图标记为「背景」）、中间实时预览区、下方导出设置。
- 技术栈: 未开源可考（独立 exe）。
- 对 UCGR 可借鉴: ①**差分规则来源第 2 类：外置坐标/配置文件**（TXT/JSON 坐标 + Unity CSV）——UCGR 可定义统一的 overlay 规则格式（基底 + 层列表[x,y,z 序 + 文件]）兼容多种来源；②三区布局（配置/实时预览/导出）是差分工具的成熟 UI 范式；③「基底图类型标记」数据结构（每张图一个 role: base/expression/hair 等）。
- 局限: 闭源、仓库不可考、无法直接复用代码。

### 4. GARbro（VN 资源浏览器——UCGR 同域最大参照物）
- URL: https://github.com/morkt/GARbro
- 活跃度: ★3.4k，337 forks，最近提交 2023-10（2019 年后基本停更，教程文也证实「GARbro 2019 年后停更但解密引擎依然稳」）
- 核心功能: Visual Novels resource browser——识别并浏览 100+ 种 VN 封包（XP3、各种 .arc、PCK…），内置图像解码预览（含 TLG）、批量转换、音频播放。
- 技术栈: C#/.NET WinForms。
- 对 UCGR 可借鉴: ①「树形资源树 + 双栏预览 + 批量转换」的浏览范式；②引擎/封包格式注册表式的识别架构（UCGR 的引擎识别与它同源）；③它只做到「资源层」——正好证明 UCGR 往上做「事件/场景语义层」是空白。
- 局限: 停更；无差分合成、无事件映射、无回放；桌面 WinForms 不跨平台。

### 5. Sprite4SteamExtractor（提取+CG 合成一体 CLI）
- URL: https://github.com/hktkqj/Sprite4SteamExtractor
- 活跃度: ★3，最近提交 2026-02（较新）
- 核心功能: 官方描述「A command-line extractor and CG composer for selected Steam visual novel archives」——对若干 Steam VN 封包做提取并**直接合成 CG**。
- 对 UCGR 可借鉴: 「提取器 + 合成器在同一条管线里按游戏适配」的组织方式——UCGR 已有前者，差分合成可作为管线末端同构扩展。
- 局限: 小工具、低 star、适配游戏有限（"selected"）、细节不可考。

### 6. sprite-dicing（跨引擎差分精灵切块压缩）
- URL: https://github.com/elringus/sprite-dicing
- 活跃度: ★1.5k，153 forks，最近提交 2026-09（活跃）
- 核心功能: 「Cross-engine tool for lossless compression of sprites with identical regions」——把大量共享区域的差分立绘切块（dice），相同块只存一份进 atlas，运行时重组还原；支持 Unity/Godot 等多引擎运行时包。
- 技术栈: C# 核心 + 各引擎集成（Unity package/Godot/Node）。
- 对 UCGR 可借鉴: ①存储层思路：按「块哈希」对差分图集去重——UCGR 的批量导入去重可借鉴到像素级（当前是文件级去重）；②若做「差分播放器」，diced atlas 是一种紧凑存储格式选项。
- 局限: 面向游戏运行时而非浏览工具；UCGR 场景收益有限（静态回放直接存合成结果更简单）。

### 7. WebGAL（网页端 VN 引擎——场景 JSON schema 参照）
- URL: https://github.com/OpenWebGAL/WebGAL
- 活跃度: ★4.1k，371 forks，最近提交 2026-11（非常活跃）
- 核心功能: 全新的网页端视觉小说引擎：JSON 场景脚本（每行一条指令：换背景/立绘/表情/语音…），立绘与差分通过「资产文件 + 脚本指令」编排，支持场景列表跳转、CG 回看等衍生功能；生态有 WebGAL Terre 编辑器。
- 技术栈: TypeScript + React + Web 渲染。
- 对 UCGR 可借鉴: ①**场景脚本 JSON schema 当作「回放轨道」中间格式**——UCGR 可以把自己的「事件→CG 序列」编译成类似 WebGAL 场景 JSON 的结构（指令流天然就是回放时间轴）；②「场景/章节列表 → 跳转」的信息架构。
- 局限: 是引擎不是回放器；面向自创游戏，不解包本地 galgame。

### 8. Ren'Py（官方 Gallery/Replay/Layeredimage——交互与 DSL 双料范本）
- URL: https://www.renpy.org/ ；两份文档已抓取验证: https://www.renpy.org/doc/html/rooms.html （Image Gallery, Music Room, and Replay Actions，200）与 https://www.renpy.org/doc/html/layeredimage.html （Layered Images，200）；仓库 https://github.com/renpy/renpy
- 活跃度: 引擎持续活跃更新（8.x 系列）。
- 核心功能: ①Gallery 屏幕：网格缩略图、locked_button（未解锁显示锁）、hover_border/idle_border、span_buttons（一个按钮跨多格）、slideshow_delay（幻灯片自动播放）、enter/intra/exit transition、unlocked_advance——一套完整的「CG 回看状态机」；②Replay 动作：Replay("label") 从任意入口 label 进入场景回放，结束时返回原菜单，解锁状态进 persistent；③Layeredimage：声明式差分合成 DSL——group（服装/表情组）+ attribute + auto（按文件名约定自动收集变体），运行时按 attribute 组合出立绘。
- 技术栈: Python + Ren'Py screen language。
- 对 UCGR 可借鉴: ①**layeredimage 的 group/attribute/auto 是现成的差分规则 DSL 范本**（第 3 类：文件名约定 + 第 4 类：声明式 DSL 的合体）——UCGR 的规则格式可以直接对标这套词汇表；②Gallery 的 locked/blank 状态机与幻灯片模式是 UCGR 回放库 UI 的直接语义来源；③Replay 的「入口→回放→返回」模型对应 UCGR「从事件 N 开始回放」。
- 局限: 引擎内置能力，只能回放 Ren'Py 自家游戏；UCGR 需要把语义移植过来而非用代码。

### 9. ReplayBook（LoL 回放管理器——回放管理交互范本）
- URL: https://github.com/fraxiinus/ReplayBook
- 活跃度: ★459，60 forks，最近提交 2025-09
- 核心功能: 本地 .rofl 回放「库管理器」：全盘扫描回放目录→元数据库→按英雄/玩家/日期筛选→标签/收藏→详情面板（对局数据）→事件时间轴（跳转到击杀等事件点）→一键调起游戏客户端播放。
- 技术栈: C#/.NET 桌面（WPF）。
- 对 UCGR 可借鉴: ①「扫描→索引→多维筛选→收藏/标签」架构与 UCGR 完全同构，且它证明了这套范式在「回放」语义下成立；②**回放=外部调起+时间轴事件跳转**的交互映射（UCGR 的按事件重放 CG 可以做成同样的「事件时间轴 + 点击跳转」）；③快速预览面板（悬停缩略图）。
- 局限: 绑死 LoL .rofl 格式；无差分合成；英文社区。

### 10. VNTranslationTools / VNTextPatch（多引擎脚本抽取管线）
- URL: https://github.com/Perturbed-Pangolin/VNTranslationTools （原 arraial/vn-textpatch 已 404——作者疑似改名迁移；fork: hungtrieu07/VNTranslationTools 也在搜索结果中出现）
- 活跃度: 页面 200，og:description=「Tools for translating visual novels」；shields 徽章显示 ★0/2022-01（数字可疑，疑似统计异常或 fork 链混乱——star 数请以现场打开为准）
- 核心功能: 对大量 VN 引擎（Kirikiri/KAG、NScripter、Luci、WillPlus、M2、SystemNNN 等）做场景脚本抽取/回注，输出**带行 ID 的统一场景表**用于翻译。
- 技术栈: C#。
- 对 UCGR 可借鉴: ①**「每个引擎一个 scenario adapter → 统一中间格式」的插件结构**——UCGR 的「事件→CG 序列」解析器应完全复用这个形状：adapter 输出 {event_id, 文本/指令流}，UCGR 在指令流里匹配 image/cg 命令即得到 CG 序列；②带行 ID 的场景表是事件编号的天然来源。
- 局限: 目标是文本翻译不碰图片；kirikiri 支持依赖脚本可读性（加密/混淆的 .ks 无效）；star 数据混乱。

### 11. KirikiriSharp（KAG 场景脚本解析器·概念级）
- URL: https://github.com/Lovely-Moon/KirikiriSharp
- 活跃度: ★1，最近提交 2016-09（super WIP，停更十年）
- 核心功能: .NET 的 kirikiri/KAG 解析器（解析 .ks 标签流）。
- 对 UCGR 可借鉴: KAG 标签词法（[image layer=… storage=…]）是「场景→立绘/CG storage」映射的原料——UCGR 写 krkr adapter 时可参考其解析边界。
- 局限: WIP 停更，不可直接用。

### 12. 其余已验证小项目（快速条目）
- **thfi-sprite-composer** https://github.com/JamElyZEuS/thfi-sprite-composer —★1，2023-06；作者现名 JamnedZ；「Touhou: Fading Illusion 同人 VN 开发用的 sprite 合成工具」——开发侧印证「按角色文件夹+差分命名」的资产管理约定；单项目专用。
- **drsounds/godot-replayer-system** https://github.com/drsounds/godot-replayer-system —★9，2022-09；Godot cutscene 事件回放录制/播放引擎；事件轨道数据结构（时间戳+事件载荷）可供 UCGR 的「事件序列」模型参考；与本地 galgame 无关。
- **tegnike/mini-pngtuber** https://github.com/tegnike/mini-pngtuber —★0，2025-12；PNGTuber（PNG 文件名约定驱动口型/表情组合）——印证「文件名约定」是最廉价的差分规则来源；场景不符。
- **YellowFish-219/Xp3Extractor** https://github.com/YellowFish-219/Xp3Extractor —★4，2026-04；Krkr XP3 提取器；UCGR 已有同能力，仅作对照。
- **Ex4i/RenPy-gallery-tutorial** https://github.com/Ex4i/RenPy-gallery-tutorial —★12，2021-01；Ren'Py gallery 教程代码（网格+解锁判断），源码短可直接读。

---

## 二、三个专项问题的回答

**1) 差分表情/服装合成的规则来源——现存 5 种模式：**
① 容器内嵌结构化规则（最可靠）：krkr .pimg/.psb 内嵌坐标+层级 JSON（FreeMote PsbDecompile 导出）、FGI 分层（krkrFgiEditor）；② 外置坐标/配置文件：GalGameCG 的 TXT/JSON 坐标文件、Unity 系 CSV 固定坐标；③ 文件名约定（最廉价）：Ren'Py layeredimage auto、PNGTuber、thfi-sprite-composer；④ 声明式 DSL：Ren'Py layeredimage 的 group/attribute 词汇表（服装=group、表情=attribute、auto=命名约定）；⑤ 场景脚本本身：WebGAL JSON / KAG .ks 的指令序列就是差分编排。**UCGR 建议：约定优先 + 规则文件覆盖的双层设计**（默认按文件名猜，允许用户放一份 overlay JSON 指定基底/层序）。

**2) 「游戏事件→CG 序列」映射：未找到现成通用开源实现。** 组件级原料有三：KirikiriSharp（.ks 解析，停更）、VNTextPatch（多引擎脚本抽取适配器+带 ID 场景表，只做文本）、Ren'Py Replay(label)（引擎内置的回放入口机制）。GARbro 等解包工具全部止步于资源层。结论：这一层必须自研——按引擎写 scenario adapter，输出统一 {event_id, cg_sequence[], meta}，这正是 UCGR 相对全部现有工具的差异化空白点。

**3) 回放器交互设计：** 三个可直接套用的范本——ReplayBook（扫描→索引→筛选/标签/收藏→详情面板→事件时间轴跳转→一键回放，是「回放库管理器」的完整形态）；Ren'Py Gallery（网格缩略图 + locked 占位 + hover/idle 边框 + 跨页 span + slideshow_delay 幻灯片 + 进出场 transition，是「CG 回看状态机」的完整定义）；GalGameCG（配置区/实时预览/导出区三区布局 + 差分勾选剔除 + 基底标记）。**对比模式（A/B 双窗格或图层开关对比）在所有调研项目中均不存在**——又一个空白点。

## 三、未找到项与搜索记录（备查）
- 通用「galgame 场景/事件回放器」成品：未找到。最接近：WebGAL（引擎）、ReplayBook（其他域回放管理器）、Ren'Py Replay（引擎内置）、godot-replayer-system（引擎内 cutscene 回放）。
- 「CG scene viewer」关键词撞车：命中 xchrdw/cgsee（计算机图形学课程数据查看器，★0，2014，与 galgame 无关）。
- Galplay「多引擎 Galgame 播放器」（mergeek.cn 项目页快照）：https/http 重定向无法跟随，仓库地址未能定位，**未验证**，仅作线索保留。
- 本次全部搜索词：立绘 差分 合成 工具 / tachie viewer github / sprite compositor visual novel github / kirikiri 立绘 提取 / galgame 回放 工具 / CG scene viewer github / ReplayBook github / event replay game tool github / galgame CG 查看器 解包 github / visual novel CG gallery github / 立ち絵 差分 合成 ツール / pngtuber compositor filename convention github / FreeMote emote github 差分 / Galplay 多引擎 galgame 播放器 github / sprite-dicing elringus github / xchrdw cgsee github / kirikiri KAG scenario parser github / renpy call_replay replay documentation / rpg maker mv event cg gallery plugin github / unity visual novel character expression sprites github / GalGameCG Sh3nx1 github / krkrFgiEditor github / VNTextPatch arraial github kirikiri / Galplay github 多引擎 galgame player。

## 四、对 UCGR 的灵感清单

**值得抄：**
1. 差分规则双层设计：默认文件名约定（角色_表情_服装）+ 可编辑 overlay JSON（基底 role + 层列表 x/y/z），词汇表对标 Ren'Py layeredimage 的 group/attribute；krkr 游戏优先读 pimg/psb 内嵌 JSON（FreeMote 产物直接当规则文件吃进来）。
2. 每引擎一个 scenario adapter，统一输出 {event_id, cg_sequence[], meta}——管线形状照 VNTextPatch 的多引擎适配器结构，指令流里匹配 image/cg 类命令即得 CG 序列（事件映射自研，是差异化机会）。
3. Gallery 交互包整套语义：网格缩略图 + 未解锁 locked 占位（灰底+锁 icon）+ hover 边框 + 跨页 + 幻灯片自动播放（可配延时）——Ren'Py rooms.html 已把状态机定义完整，照抄语义即可。
4. ReplayBook 三件套：全盘扫描→元数据库→筛选/标签/收藏；回放详情面板；事件时间轴 + 点击跳转（UCGR 的按事件重放可做成同样的时间轴交互）。
5. krkr 差分管线：XP3→tlg/pimg→PNG 之后接 PSB/FGI 规则合成（FreeMote/krkrFgiEditor 流程），把 UCGR 现有解码器延伸到「按事件合成导出整图」。
6. 默认 z-order 约定：底图最下→表情差分中间→鬓发/前景覆盖件最上（krkrFgiEditor 分组语义）。
7. 像素级去重思路（sprite-dicing 的块哈希）作为批量导入去重的可选进阶。

**值得简化：**
1. sprite-dicing 的 diced atlas 运行时——UCGR 静态回放直接存合成 PNG，只借「相同区域去重」思路，不内嵌切块重组。
2. WebGAL 整个引擎——只取其场景 JSON schema 当「回放轨道」中间格式（指令流=时间轴），不做渲染引擎。
3. GalGameCG 三模式 UI——UCGR 保留「坐标差分自动合成 + 手动微调」两档即可。
4. Ren'Py Replay 的游戏内 label 执行——UCGR 无游戏运行时，只需「从事件 N 起按序列静态播放 + 返回」。
5. VNTextPatch 的回注/补丁能力——UCGR 只读，抽取侧结构照抄即可。

**不必做：**
1. 实时差分渲染引擎 / Live2D / Spine 运行时——预合成导出即可，蒙皮是引擎的活。
2. PNGTuber/直播口型联动——场景不匹配。
3. 翻译补丁写回（VNTextPatch 注入侧）。
4. 电竞回放内核（.rofl 解析等）——只借 ReplayBook 的交互不借实现。
5. 在线分享/云同步/账号系统——调研样本无一做此，个人本地库优先。
