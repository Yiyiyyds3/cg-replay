# UCGR 同类项目调研 · 方向 A：galgame 本地库管理器与 CG 鉴赏/回放工具

调研时间：2026-11-21 ｜ 调研员：子代理 6690ef29（方向 A）
检索方式：web_search 多轮（Playnite / moeloader / PlayNight / galgame manager / CG回廊 / CG鑑賞 / gallery unlock save / VNDB client / kirikiri viewer / scene replay）+ web_fetch 逐项打开验证（github.com 仓库页 HTML og:description 全部打开成功；api.github.com 部分命中限流 403，改用 shields.io 徽章补 stars/最近提交；raw.githubusercontent 与 jsdelivr 被网络拦截，README 细节以 og 描述 + 开发文档片段 + 搜索摘要为准，逐条已标注）。

━━━ 一、本地库管理器（galgame 专用） ━━━

1) Playnite
URL: https://playnite.link （官网已打开✓）| https://github.com/JosefNemec/Playnite ✓
活跃度: ~14.6k stars，最近提交 2026-11-18（活跃），MIT，C#/.NET
核心功能: 全平台游戏聚合库（Steam/GOG/Epic/Xbox/模拟器 ROM）、多源元数据刮削、插件(C#/PowerShell)与主题生态、全屏模式、游玩时长统计、导入向导。
对 UCGR 借鉴: ①元数据 Provider 抽象（配套 PlayniteVndb 插件证明 VNDB 可作为 provider 接入）；②per-game action 数据模型——UCGR 可把"解包/回放/导出"做成游戏上的动作而非全局菜单；③导入向导+同路径/同 hash 去重；④主题化 UI 让皮肤独立于逻辑。
局限: 通用游戏库，无 CG/封包/资源概念；WPF 仅 Windows；体量大。

2) KoiPage 恋页（最接近 UCGR 的同类）
URL: https://github.com/morilove37/KoiPage ✓
活跃度: 151 stars，最近提交 2026-11-20（昨天），GPL-3.0
核心功能: Windows galgame/VN 管理器：扫描器（游戏目录+exe 识别）→批量导入→资料匹配与确认（NextMoe/VNDB/Bangumi 三源）、离线中文标签词典、封面收藏、游玩记录与日历、分享足迹、内置 Locale Emulator 转区、Magpie 缩放、7zip、游戏截图、液态玻璃主题；本地封面/壁纸走 asset 协议+最小静态授权。
技术栈: Tauri 2 + React + Rust + SQLite（WebView2）
对 UCGR 借鉴: ①scanner→识别→确认→入库流水线与 UCGR"扫描→引擎识别"同构，其"资料匹配与确认"两段式（自动候选→人工点选）应直接抄；②本地图片 asset 协议（Wails 对标 custom scheme）；③离线标签汉化词典=VNDB tag+本地映射；④进程级游玩时长→日历/足迹统计；⑤内置第三方二进制（LE/Magpie/7zip）用 include_bytes+版本校验的资源清单模式。
局限: 只管启动不碰资源，无 CG 解包/回放——这正是 UCGR 的差异化空间。

3) GalGear
URL: https://github.com/CbShuiMu/GalGear ✓
活跃度: 44 stars，最近提交 2026-11-21（today，非常活跃）
核心功能: 轻量 galgame 管理器：自动刮削、补丁搜索、内置 LE 转区、Magpie、Waifu2x 封面超分、游戏截图管理、AI 存档搜索。
对 UCGR 借鉴: ①"AI 存档搜索"——把外部全CG存档定位/落到本地并挂到对应游戏，与 UCGR 的 CG 解锁域天然互补；②"补丁搜索"按游戏名自动找汉化/修正补丁，UCGR 可做"CG鉴赏补丁/全存档资源面板"；③Waifu2x 封面超分管线（导入时统一美化低清素材）；④截图按 游戏+时间 自动归档。
局限: 文档薄（README 抓取受限），功能边界需读源码；管理器层面无资源语义。

4) KamiNeko-pre/gal-launcher
URL: https://github.com/KamiNeko-pre/gal-launcher ✓
活跃度: 106 stars，最近提交 2026-09
核心功能: Steam 风格本地 VN 启动器：封面网格库视图→详情页→启动。
对 UCGR 借鉴: Steam 式信息架构（库→详情页分区：元数据区/操作区/记录区），UCGR 详情页可把"CG 鉴赏/事件回放"设为详情页一级按钮。
局限: 仅启动器，无刮削深度与资源功能。（同类 netori/gal-launcher Tauri2+Vue3+SQLite 仅 1 star，可见该赛道同质化）

5) lxl66566/GalgameManager
URL: https://github.com/lxl66566/GalgameManager ✓
活跃度: 27 stars，最近提交 2026-11-17，MIT
核心功能: Modern galgame launcher：云同步、插件系统、游玩时长统计、存档备份。
对 UCGR 借鉴: ①插件系统接口设计（导入器/刮削器扩展点）；②云同步可简化为"JSON 快照导出/导入"；③存档备份纳入管理器功能集。
局限: 早期小众（27 stars），生态薄。

6) GALlauncher (cyke262/gallery)
URL: https://github.com/cyke262/gallery ✓（仓库页已打开；shield 显示最近提交 2023-11，stars 未读到）
核心功能: 本地 galgame 管理软件兼"聚合启动器"——把外部资源站（下载/补丁/存档）链接按游戏聚合。
对 UCGR 借鉴: "按游戏聚合外部资源"面板思路：每个游戏页挂"CG 相关资源"（补丁/全存档/维基）静态链接。
局限: 2023 年后停更，UI 老。

━━━ 二、CG/资源查看与导出 ━━━

7) GARbro（morkt）
URL: https://github.com/morkt/GARbro ✓
活跃度: ~3.4k stars，最近提交 2023-10（之后停更，社区 fork 多）
核心功能: 视觉小说资源浏览器：打开 100+ 引擎封包（KiriKiri XP3、NScripter、BGI、Will、RPG Maker 等）→树状浏览→图片预览/转 PNG、音频转码；格式库以签名/掩码+人工标注维护。
技术栈: C#/.NET WinForms, GPL
对 UCGR 借鉴（与 UCGR 解密层最相关）: ①FormatCatalog 两层结构——"签名/掩码自动探测"层 + "每格式人工标注"层（变体加密、命名规则、差分图配对），UCGR 引擎识别表可升级成同款；②封包树→文件列表→预览窗三窗格交互，适合做 UCGR 的"调试/原始资源"视图；③图片解码插件化（TLG、MSK/PSB 差分合成即立绘+差分）；④entry descriptor 记录每格式细节。
局限: 停更；WinForms 老界面；只有"文件"语义，无"事件→CG 序列"语义（UCGR 的核心差异）。

8) XP3-Viewer-and-Converter
URL: https://github.com/DarrylClay2005/XP3-Viewer-and-Converter ✓
活跃度: 8 stars，最近提交 2025-08
核心功能: Windows GUI：解包 XP3、浏览、导出 JPEG/PNG/BMP/TIFF。
对 UCGR 借鉴: 单格式小工具的极简交互（拖入→看图→批量转码），可做 UCGR 的"单游戏快速导出"向导。
局限: 只对 XP3；无 CG 组织语义。

9) MoeLoaderP（moeLoader 维护分支）
URL: https://github.com/xplusky/MoeLoaderP ✓
活跃度: 415 stars，最近提交 2025-05，GPL-3.0，C#/WPF
核心功能: 二次元/CG 图站批量下载器（yande.re/Konachan/danbooru/behoimi/pixiv/Sankaku 等），按 tag 批量抓图、去重、站点解析器可扩展。
对 UCGR 借鉴: ①Source Provider 抽象（每站点一个解析器，UI 统一）；②下载队列+hash/size 双重去重（UCGR 批量导入直接对标）；③"tag 收藏夹→本地文件夹"命名模板，UCGR 事件序列落盘命名可参考。
局限: 面向图站不面向游戏封包；与 UCGR 主线无交集。

10) GalGalleryViewer
URL: https://github.com/YeLikesss/GalGalleryViewer ✓
活跃度: 8 stars，最近提交 2025-12
核心功能: Galgame CG 立绘合成——把拆分的立绘/差分图层合成完整 CG 查看。
对 UCGR 借鉴: 立绘/差分合成预览：UCGR 遇到 MSK 差分或图层分离素材时可做"合成视图"，对应 Kirikiri tlg 差分结构。
局限: 小众早期（README 抓取受限，细节需读源码）。

11) Verviewer
URL: https://github.com/icey9527/Verifier ✓（原报告 URL: https://github.com/icey9527/Verifier，仓库页已打开）
活跃度: 3 stars，最近提交 2025-10
核心功能: Visual Novel Resource Viewer——不解包落盘、即时浏览封包内图片。
对 UCGR 借鉴: "资源即画廊"预览模式：导出前先即时浏览，减少落盘垃圾。
局限: 极早期（3 stars）。

━━━ 三、元数据与 CG 解锁生态 ━━━

12) create-all-cg-save
URL: https://github.com/xixi048/create-all-cg-save ✓
活跃度: 新项目（0 stars，2025-10 左右）
核心功能: Agent skill + 零依赖 Python 工具：对离线单机存档生成"全 CG 存档"，覆盖 RPG Maker、Ren'Py、Kirikiri、Unity 的 CG/gallery/recollection room 解锁。
对 UCGR 借鉴: ①四引擎"存档 CG 解锁标志位"知识库（RPGMV globals.rpgsave、Ren'Py persistent、Kirikiri 存档字段、Unity PlayerPrefs）——UCGR"事件→CG 序列"表可复用这些引擎的标志位/命名知识；②"gallery unlock = 状态注入"路线：UCGR 覆盖不了的引擎，可输出配套全存档让游戏自己回放，形成互补。
局限: 新项目、覆盖 4 家引擎；纯存档改写无 GUI。

13) PlayniteVndb
URL: https://github.com/Mithnar/PlayniteVndb ✓
活跃度: 69 stars，最后更新 2023-01（稳定停更），MIT，C#
核心功能: 把 VNDB 作为 Playnite 元数据源（标题/简介/开发商/标签/封面）。
对 UCGR 借鉴: VNDB API 字段→本地库 schema 的完整映射清单 + "标题匹配候选→用户确认"刮削流程（UCGR 元数据模块直接抄流程）。
局限: 只做元数据不碰 CG；停更。

14) VNDB API v2 (Kana) + 客户端生态
URL: https://api.vndb.org/kana ✓（官方文档已打开）| https://vndb.org ✓
客户端: VNDBforAndroid（https://github.com/Kifranei/VNDBforAndroid ✓，7 stars，2026-11 更新，Kotlin）；Happy-Reader（https://github.com/Zoltanar/Happy-Reader ✓，27 stars，2026-04 更新，C#，VNDB explorer + VNR 式文本 hooker）；Visual Novel Observatory（https://getobservatory.app/visual-novels ✓，VNDB 库追踪与统计站）。
核心功能: VN 条目事实库：别名/开发商/日期/带剧透分级 tag/角色/截图集；API v2 fields 投影+分页+ulist。
对 UCGR 借鉴: ①元数据主源直接用 VNDB API（免费稳定字段全）；②tag 的 spoiler level(0/1/2) 设计映射到 CG 标签（普通/轻微剧透/关键剧透）；③VNDB screenshots 可当"官方 CG 对照表"辅助事件识别；④ulist/评分做库内满意度字段。
局限: 有速率限制；无"事件→CG"粒度数据（引擎文件层的活）。

━━━ 关键词核查说明 ━━━
- PlayNight: 多轮中英检索未找到叫此名的真实项目（只命中 Playnite 文章与无关站点 playnightstudio.com），疑为 Playnite 误记或极小众私有项目。
- CG回廊: 无以此命名的独立工具；该词是游戏内 CG 鉴赏模式（gallery）的常见和风叫法，检索结果均为文章/图集——印证"CG 鉴赏=游戏内功能"生态位，外部工具缺位，是 UCGR 的机会。
- gallery unlock save: 工具类只有 create-all-cg-save；社区主流仍是手工分发全CG存档文件、无统一标准——UCGR 若做"存档生成/导入"可填补空白。

━━━ 对 UCGR 的灵感清单 ━━━

【值得抄】
1. VNDB API v2 做元数据主源 + "自动候选 top-N→人工确认"两段式刮削（PlayniteVndb/KoiPage 已验证）。
2. GARbro 格式注册表两层结构：签名/掩码自动探测层 + 每格式人工标注层（变体加密、差分图配对、entry descriptor）——UCGR 引擎识别表升级方向。
3. KoiPage 的 scanner→识别→确认→入库流水线 + 本地图片 asset:// 协议（Wails 用 custom scheme 对标）。
4. CG/事件标签引入剧透分级（VNDB spoiler level 0/1/2 → CG 分三档标记，回放时可选过滤）。
5. 存档解锁互补路线：UCGR 覆盖不了的引擎输出"CG 标志位注入的全存档"（create-all-cg-save 四引擎知识库）。
6. 游玩时长/日历/足迹统计（KoiPage）扩展到"回放行为统计"（最常回放的事件/CG）。
7. 批量导入 hash+size 双重去重与队列化进度反馈（MoeLoaderP）。
8. Steam 式详情页信息架构（KamiNeko gal-launcher）：元数据区/操作区/记录区分区，"CG 鉴赏"设为详情页一级入口。
9. 封面与低清 CG 的 Waifu2x 超分管线（GalGear）。
10. per-game action 模型（Playnite）：解包/回放/导出都挂到游戏上，参数随游戏存配置。

【值得简化】
1. 主题系统：不做 XAML 式生态，内置 2-3 套配色即可（Playnite 太重）。
2. 云同步：降级为"JSON 快照导出/导入"，不做账号体系（lxl66566 路线简化）。
3. 插件系统：留"引擎适配器注册表"扩展点即可，不做完整插件运行时。
4. GARbro 式封包树三窗格：只进"调试/高级"模式，主 UI 保持"事件→CG"结构。
5. 外部资源聚合（GALlauncher）：每游戏静态"资源面板"（补丁/全存档/维基链接）即可，不做内置浏览器。

【不必做】
1. booru 图站下载器（MoeLoaderP 领域，与本地封包数据流无关）。
2. 通用图片素材库式自由标签管理（Eagle/Billfish 领域；CG 已按游戏/事件结构化，自由标签只做补充）。
3. 文本 hook/翻译（Happy-Reader/LunaTranslator 领域）。
4. 全屏 Big Picture 模式（Playnite）。
5. Android/macOS 多端（Wails 单 Windows 足够，个人工具边界清晰）。

━━━ 数据可信度说明 ━━━
14 个条目全部经 web_fetch 打开仓库页/官网确认存在；stars 与最近提交来自 GitHub API（7 项）与 shields.io 徽章（16 项，徽章文字为近似精度如 3.4k/yesterday）；raw.githubusercontent 与 jsdelivr 在当前网络不可达，故部分条目功能细节基于仓库描述+开发文档片段+搜索摘要，已在条目内标注"README 抓取受限"。所有外部页面内容仅作资料引用。
