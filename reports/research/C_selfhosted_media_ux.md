# UCGR 同类项目调研 · 方向 C：自托管媒体库/相册应用的 UX 与架构借鉴

调研时间：2026-11-21 ｜ 调研员：子代理 e15a3412（方向 C）

**一句话结论**：这一类项目已经把「本地大图库 + 标签 + 去重」的完整工程答案跑过一遍——去重的共识是「字节哈希 → 感知哈希 → 嵌入 → 人工复核」四层漏斗、没人敢全自动删；Hydrus 与 UCGR 同构且官方文档里**正好有 game CG 章节承认自己的成对去重模型在 CG 场景会爆炸**（UCGR 应直接用「事件=组」绕开这个坑）。可抄的东西按价值排序：Hydrus 的去重关系模型与复核 UI、Immich 的自动预选+元数据合并、Stash 的多指纹+筛选体系、TagStudio 的别名/shorthand 标签模型、LibrePhotos 的变体合并。

**方法与验证**：web_search 定向检索 + web_fetch 逐页打开核实。star/最近推送经 GitHub API（代理通道）于 2026-11-21 取样；引用的文档页（Immich duplicates-utility/searching/system-settings、Hydrus duplicates.html 与 auto-resolution 页、Stash deduplication/browsing、LibrePhotos search/duplicate-detection/stacks、TagStudio tags/library-changes、PhotoStructure 去重 FAQ、Photoview README）全部 HTTP 200 打开过。个别 CDN（raw.githubusercontent/jsdelivr）直连失败，改走代理完成，不影响事实。PhotoStructure 的 GitHub 主仓库未能确认公开（API 404），以官网为准。

---

## 1. 总览表（9+1 个项目，均为真实存在并已验证 URL）

| 项目 | URL | 活跃度（2026-11-21 取样） | 技术栈 | 一句话定位 |
|---|---|---|---|---|
| Immich | https://github.com/immich-app/immich | 119,771★，pushed 11-21 | TS(NestJS)+SvelteKit+Flutter+Python ML；PostgreSQL+VectorChord | 自托管 Google Photos，CLIP 语义搜索+嵌入去重 |
| PhotoPrism | https://github.com/photoprism/photoprism | 40,468★，pushed 11-20 | Go+TensorFlow+Vue；SQLite/MariaDB | 单二进制 AI 相册，索引管道成熟 |
| Stash | https://github.com/stashapp/stash | 13,450★，pushed 11-15 | Go+React+SQLite+ffmpeg | 场景/图集库，phash 去重+最强筛选器 |
| LibrePhotos | https://github.com/LibrePhotos/librephotos | 8,181★，pushed 11-21 | Django+React+celery | MD5+pHash 双层去重+variants 合并 |
| TagStudio | https://github.com/TagStudioDev/TagStudio | 7,309★，pushed 11-14 | Python+PySide6 桌面；JSON→SQLite(v9.5) | tag-first 文件组织器，别名/消歧设计最好 |
| Photoview | https://github.com/photoview/photoview | 6,577★，pushed 11-20 | Go+GraphQL+React | 目录即相册，极简低资源 |
| Hydrus Network | https://github.com/hydrusnetwork/hydrus | 3,277★，pushed 11-18（近乎每周发版） | Python+Qt 桌面；多 SQLite 分库 | 本地文件库+标签+去重的天花板，与 UCGR 同构 |
| PiGallery2 | https://github.com/bpatrik/pigallery2 | 2,330★，pushed 11-21 | Node+Angular+SQLite | 目录优先、极低内存相册 |
| Damselfly | https://github.com/Webreaper/Damselfly | 1,799★，pushed 09-29 | C#/.NET+Blazor | 服务端 phash 相似搜索+AI 标签 |
| PhotoStructure | https://photostructure.com | 官网活跃（FAQ 页 2026-10 更新）；GitHub 主仓库未能确认公开（API 404，官网可开） | Node/TS，source-available | asset=primary+variants 去重组模型 |

---

## 2. Hydrus Network（与 UCGR 最接近，单独深入）

**URL**：https://github.com/hydrusnetwork/hydrus ；文档 https://hydrusnetwork.github.io/hydrus/
**活跃度**：3,277★，最近推送 2026-11-18，发版极勤。Python + Qt（PyQt6）桌面单机，无服务端。
**核心功能**：本地大容量文件库（图/视频/音频），三件套 = 指纹去重系统 + 命名空间标签体系 + 键盘流浏览/下载。与 UCGR 的「本地库+标签+去重」完全同构，差别只在它面向全网采集、UCGR 面向本地 galgame 导出。

### 去重系统全链路（duplicates.html 与 auto-resolution 页逐段核实）
1. **指纹层**：每文件 SHA256 为主哈希，另有 MD5/SHA1/SHA512/像素哈希查找表；相似检测用 **64-bit pHash + VP-tree** 按汉明距离检索（原文：*uses a 64-bit phash to represent the image shape and a VPTree to search different files' phashes' relative hamming distance*）。
2. **距离语义**（官方给的经验值）：0 = 同图缩放/低质重压；4 = 发型/服装级别的变化；>8 基本无用。官方建议从 0 开始清队列再逐步拉大——往队列里灌假阳性不会让活变轻松。
3. **发现**：后台常驻任务在库内做潜在重复对（potential duplicate pairs）发现；发现队列可与任意文件搜索组合（system:everything / creator:xxx / system:archived），还有 pair 级谓词「至少一个文件匹配 / 两个都匹配」。
4. **复核 UI**：duplicate filter（类似 archive/delete filter 的队列流），双栏并排、缩放同步、键盘判定，处理几千对也不累。
5. **关系模型**（这是全篇最值钱的设计）：决策 = **duplicates**（组内选 king「最佳文件」，其余可删）/ **alternates**（同源但不是同一张：换装、WIP）/ **not duplicates**。决策入库形成逻辑一致的组，并做**传递推理**：A=B 且 B=C 则 A=C 自动成立；已判定的 pair 不再回到潜在队列。
6. **元数据合并**：判定重复时把差文件的 tags/notes 并入 king。
7. **自动化长尾**：auto-resolution 规则引擎——「搜索条件 → 比较方式 → 动作」三段式规则，带预览，可半自动/全自动；内置场景分类：像素级完全相同对、视觉相似对、jpeg/png 近完美对。放在人工复核之后吃掉机械判定。
8. **game CG 专门警告**（文档原文，对 UCGR 是金子）：官方文档明说 game CG 动辄几十上百个同源变体，alternates 成对存储模型会扩展失败（选 500 个相似文件设为 alternates 会触发上百万关系警告），正在重构为组/簇存储。
9. **better/worse 启发式**：original 特征清单（更老、分辨率更高、截图类用 PNG、无水印、裁剪更完整、被更多人打过 tag=流行度代理）；警告坏超分会假高分辨率；另有「省 10KB 不如保元数据」的论证——倾向保留元数据更全的文件。
10. **教训**：作者自评「没有漂亮的关系审查 UI、没有逐决策审计日志」是短板。

### 标签体系
namespace（character:xxx / creator:xxx）+ **siblings**（同义写法合并到规范名，搜索自动展开）+ **parents**（层级展开）+ **多 tag service**（本地、下载源、公共仓库 PTR 各自独立计数/撤销）+ system: 谓词族（尺寸/大小/时长/有无音频…）+ ratings/favourites；搜索全键盘驱动。

### 存储布局
原始文件**内容寻址**：按哈希前缀两级目录存（同一文件全库只存一份），元数据是一套**多 SQLite 分库**（mappings/tags/files/hashes/thumbnails/caches 等），缩略图独立分库，支持多磁盘存储位置迁移。

### UCGR 能抄什么
- 「去重 = 关系标注」数据结构：potential_pair → {duplicate|alternate|none} + king 选择，落库可查、可反悔；UCGR 现在的批量导入去重可升级成这个。
- pHash（64-bit DCT）+ VP 树 + 汉明距离滑条，导入时顺手算。
- 复核队列 UI（键盘判定键）+ pair 可搜索谓词 + 传递推理省一半人工。
- king 概念 → UCGR 事件内「代表 CG」。
- tag 别名/父子/namespace；「引擎自动 tag」与「用户 tag」分两个字段集存储（多 service 的简化版）。
- 内容寻址存储选项：UCGR 导出缓存库按内容哈希存，原游戏目录不动。
- **反面教材同样值钱**：变体不要做全库两两关系——UCGR 直接用「事件/场景 = 组」的一级结构，变体挂组内，从数据模型上绕开 Hydrus 自己承认的百万级关系爆炸。

**局限**：桌面 Qt 单机无 Web UI；CG 规模的关系模型官方自认失败正在重构；学习曲线陡；Python 性能一般（靠 VP 树+后台任务补）。

---

## 3. Immich

**URL**：https://github.com/immich-app/immich ；https://immich.app
**活跃度**：119,771★，pushed 2026-11-21，当前 v2.x，社区最活跃。
**核心功能**：自托管 Google Photos：时间线、人脸、语义搜索、去重复核、共享、移动端备份。
**技术栈**：NestJS(TS) 服务端 + SvelteKit Web + Flutter 移动端 + **独立 Python ML 服务**（ONNX）；PostgreSQL，语义搜索由 **VectorChord** 扩展承载。
**去重**（duplicates-utility 页核实）：用 **CLIP 图像嵌入**找疑似重复；**最大检测距离 0.001–0.1 可调**（系统设置），调大误报增多。"Review duplicates" 页：勾选保留/丢弃，保留多个时可自动组成 **stack**；**自动预选**规则——字节更大者优先（通常质量更高）、EXIF 字段更多者优先；**恰好保留一个时**把被丢弃者的 Name/Description/Album/Favorite/Rating **同步合并进保留者**（保留多个则不合并）。
**相似检索**：Smart Search（CLIP 语义，**模型可换，换模型必须全量重跑**）+ 高级筛选（人/OCR 文字/标签/相机/路径/媒体类型/星级/是否在相册）。
**存储**：原文件不动 + thumbs/encoded-video 缓存目录；上传按 checksum 去重。
**UCGR 能抄什么**：距离滑条阈值；自动预选规则（简单、可解释、立刻能实现）；元数据合并方向（丢者→留者）；stack = UCGR「同 CG 变体合并」；任务系统每个 ML job 独立可重跑、进度可见；「换模型=全量重算」的教训 → **指纹列要带算法版本号**。
**局限**：面向照片（GPS/人脸为中心），CG 语义弱；多用户/共享/移动端对 UCGR 是纯负担；必须 Postgres。

## 4. Stash

**URL**：https://github.com/stashapp/stash
**活跃度**：13,450★，pushed 2026-11-15。
**核心功能**：自托管场景库（视频+图集 galleries），强标签、强筛选、scraper/插件生态。
**技术栈**：Go + React + SQLite + ffmpeg。
**去重**（deduplication 页核实）：**phash = 从场景固定点抽 25 帧拼接后做感知哈希**，扫描时或单独任务生成；Dupe checker 页按汉明距离阈值出疑似对，支持键盘快捷键、批量处理。指纹体系 **oshash/md5/phash 多指纹并存**，导入即算、可补算。
**标签/筛选**（browsing 页核实）：父子层级 tag + 别名 + 合并；查询语法（or / -排除 / 引号短语，大小写不敏感）+ filter 面板 + **saved filters（可设为页面默认）** + 正则修饰符。
**UI**：卡片尺寸缩放滑条、shift/ctrl 多选、批量编辑对话框、图集 lightbox、视频 scrubber 精灵图、Reveal in file manager。
**UCGR 能抄什么**：多指纹表结构（导入哈希+像素哈希+phash 并存各司其职）；saved filters + 查询语法解析器；批量编辑对话框；图集 lightbox；卡片缩放；「扫描时算 + 单独任务补算」的弹性指纹管线。
**局限**：语义面向视频（performers/studios/markers 对 CG 无用）；Web 服务形态（UCGR 是 Wails 桌面，交互可借鉴、实现要改）。

## 5. PhotoPrism

**URL**：https://github.com/photoprism/photoprism
**活跃度**：40,468★，pushed 2026-11-20。
**核心功能**：AI 相册：索引管道（SHA1 指纹、平均色、分类标签、人脸、位置）、目录监控、RAW 支持。
**技术栈**：Go + TensorFlow（本地推理）+ Vue；SQLite/MariaDB/PG。
**去重**：以导入指纹（checksum）合并为主；官方 release-notes 未见 duplicate finder 专页，**人工复核式去重工作流不是它的强项**（与 Immich/Hydrus 对比明显）。
**UCGR 能抄什么**：单二进制+本地推理的集成方式（Go 直调模型——与 UCGR 技术栈同款）；**索引时一次算全所有指纹**（导入顺路，无二次任务）；**平均色（chroma）做主色调筛选/分组**——配 galgame CG 浏览很贴；目录监控增量索引。
**局限**：内存开销大（TensorFlow 常驻）；无人工复核去重；Go+TF 维护重。

## 6. LibrePhotos

**URL**：https://github.com/LibrePhotos/librephotos
**活跃度**：8,181★，pushed 2026-11-21。
**核心功能**：AI 相册：人脸、CLIP 语义搜索、OCR、图说、时间线、去重、变体合并。
**技术栈**：Django + React + celery worker；Postgres。
**去重**（duplicate-detection 页核实）：双层——**精确（MD5 字节）+ 视觉（64-bit DCT pHash，扫描时从大缩略图算，汉明距离阈值）**，只有已算 phash 的图参与视觉比对（省 CPU 的关键设计）。
**变体**（stacks-and-file-variants 页核实）：**file variants**（同拍摄不同格式：RAW+JPEG 自动合并为时间线单条目挂多文件）vs **stacks**（不同拍摄聚合）。
**搜索**（search 页核实）：**Cmd+K Spotlight 命令面板**（人物/地点/物体/相册/全文 + 快速跳页）。
**UCGR 能抄什么**：pHash 从缩略图算（UCGR 缩略图管线顺路完成）；**variants = UCGR 同一 CG 多容器/多分辨率合并为单条目**；Cmd+K 面板是低成本高感知的 UI 提升；job 系统可视化。
**局限**：多用户 web 架构 + celery/redis 全家桶对个人桌面应用过重；模型迭代快质量参差。

## 7. TagStudio

**URL**：https://github.com/TagStudioDev/TagStudio
**活跃度**：7,309★，pushed 2026-11-14。
**核心功能**：tag-first 文件组织器（不搬不改文件，library 建在目录旁）；v9.5 起 library 从 ts_library.json 迁移到 **SQLite**（library-changes 页核实）。
**标签体系**（docs.tagstud.io/tags/ 核实，全文读过）：**shorthand 短名字段**（可搜索）；**aliases 别名**（搜别名也能命中被 tag 的条目）；**父标签消歧**——勾选后自动显示「Freddy (FNAF)」；**继承语义**（tag Shrek = 自动带上 Character / Movie Franchise / Dreamworks），设计三原则：Simplicity via Deduplication / Intuition via Substitution / Rediscovery via Linking；entry 上可挂任意类型 **fields**。
**UCGR 能抄什么**：fields-on-entry 数据模型（CG 条目挂 source_engine/cg_id/scene/角色等任意键值）；**别名+shorthand 让「Saber」「セイバー」「阿尔托莉雅」互相命中——galgame 角色名场景刚需**；父标签消歧显示；「库是数据库、文件不动」哲学（与 UCGR 一致）。
**局限**：无去重/相似检索能力；同为 Qt 桌面但栈不同；JSON 时代大库性能曾是痛点（已换 SQLite）。

## 8. Photoview

**URL**：https://github.com/photoview/photoview
**活跃度**：6,577★，pushed 2026-11-20（活跃但节奏慢）。
**核心功能**：目录即相册、扫描器产缩略图、分享链接（README：simple and user-friendly photo gallery… navigate directories, with thousands of high-resolution photos. The scanner automatically picks up your media and starts to generate thumbnails）。
**技术栈**：Go + GraphQL + React；SQLite/MySQL/PG。
**UCGR 能抄什么**：目录优先信息架构与 UCGR「游戏目录→事件」的映射思路同源；GraphQL 单端点 API 的简洁；极简扫描器设计。
**局限**：功能薄、ML 弱、迭代慢于 Immich。

## 9. 其他同类速览

- **PiGallery2**（https://github.com/bpatrik/pigallery2，2,330★，11-21）：目录优先、极低内存、face-api.js 人脸、关键词、地图。抄：低资源缩略图/预览策略。
- **Damselfly**（https://github.com/Webreaper/Damselfly，1,799★，09-29）：C#/.NET+Blazor，服务端 phash 相似搜索 + AI 物体/人脸检测。抄：服务端相似检索接口设计。
- **PhotoStructure**（https://photostructure.com，官网可开；**GitHub 主仓库未能确认公开**，API 404）：商业 source-available。去重 FAQ（已抓原文）确认其模型：**asset = primary + variants**；比对分层——先字节流相同不再入库 → 元数据标签比对 → 图像内容比对；浏览时只展示最佳变体。抄：「**底层存全部变体、浏览层只出 king**」的展示去重；分层比对漏斗。局限：闭源部分多、无人工复核工作流。

---

## 10. 六个关键问题横向对比

1. **重复检测策略**：共识是四层漏斗——字节哈希（人人都有）→ 感知哈希（Hydrus/Stash/LibrePhotos/Damselfly/PhotoStructure）→ 嵌入（Immich/LibrePhotos）→ **人工复核 UI 收尾（三家都做，没人敢全自动删）**。
2. **标签体系**：Hydrus（namespace+siblings+parents+多服务）＞ Stash（树+别名+saved filters）＞ TagStudio（别名+shorthand+消歧父标签+fields）＞ Immich（扁平+自动标签）。galgame 场景角色/作品名多写法 → 别名+shorthand 是刚需。
3. **相似图检索**：小库（几十万张内）暴力汉明距离扫描完全够（Stash/LibrePhotos 都这么干）；Hydrus 用 VP 树；Immich 用 pgvector/VectorChord 做嵌入检索。UCGR 的 CG 量级：SQLite 存 phash + 批量位运算即可，VP 树是优化项不是起点。
4. **浏览交互**：多选+批量编辑（Stash/Immich）、卡片缩放滑条（Stash）、对比（Hydrus 双栏同步缩放 / Immich review 页）、键盘流（Hydrus 全面键盘 / Stash 快捷键 / LibrePhotos Cmd+K）、默认视图可保存（Stash saved filter 设默认）。
5. **元数据/ML 管道**：清一色后台 worker + 任务可重跑（Immich 每个 ML job 独立重跑/进度可见）；指纹在导入时顺路算；LibrePhotos 从缩略图算 pHash 省 CPU；换模型 = 全量重算（Immich 明文警告）→ 指纹列带算法版本。
6. **存储布局**：两派——内容寻址（Hydrus 按哈希前缀目录，天然去重）vs 原地不动+缓存目录（Immich/PhotoStructure/Stash：thumbs、previews、generated）。UCGR 直读游戏目录的场景更配后者；导出缓存库可选内容寻址。

---

## 11. 对 UCGR 的灵感清单

### 值得抄（按优先级）
1. **三层去重漏斗**：字节哈希（已有）→ 64-bit DCT pHash（导入时从缩略图顺路算，LibrePhotos）→ CLIP 类嵌入（可选后置）；指纹列带算法版本号（Immich 教训）。
2. **去重 = 关系标注不是删除**：potential pair → {duplicate|alternate|none} + king（Hydrus）；决策入库、可查询、传递推理 A=B∧B=C⇒A=C。
3. **复核 UI**：双栏并排对比、缩放同步、键盘判定键、队列可再过滤（Hydrus duplicate filter）。
4. **自动预选 + 元数据合并**：文件更大者/元数据更多者自动预选为保留方；丢弃者的 tag/备注/相册并入保留者（Immich）。
5. **变体合并展示**：同一 CG 的多容器/多分辨率 → 条目级合并（variants），底层存全部、浏览只出最佳（Immich stack / LibrePhotos variants / PhotoStructure）。
6. **标签模型**：别名 + shorthand + 父标签消歧（TagStudio）+ 引擎自动 tag 与用户 tag 分离存储（Hydrus 多 service 的简化版）。
7. **筛选体系**：saved filters 可设默认 + 查询语法（or/-/引号）+ system: 谓词族（尺寸/大小/时长/有音频）（Stash/Hydrus）。
8. **自检升级**：把 UCGR 已有的批量导入有效性自检扩展成「去重自检报告」——本次导入多少 phash 冲突、多少待复核对。
9. **交互三件套**：Cmd+K 命令面板（LibrePhotos）、卡片缩放滑条 + 图集 lightbox + 批量编辑对话框（Stash）。
10. **任务系统**：扫描/指纹/缩略图/去重发现各自独立任务、可重跑、进度可见（Immich/Stash）。
11. **存储**：UCGR 导出缓存库按内容哈希存（Hydrus），原游戏目录永不写入（全员共识）。

### 值得简化
- **向量检索**：CG 量级先用文本元数据 + phash；要上嵌入也是单机 ONNX 小模型，不抄 Immich 的独立 ML 服务部署。
- **数据库**：SQLite 足够（Stash/TagStudio 双双证明），Postgres+VectorChord 不引入。
- **多用户/共享/权限/移动端**（Immich/LibrePhotos/Photoview 的主体复杂度）：个人单机工具全部砍掉。
- **Hydrus 的多 tag service/PTR/下载器/订阅**：UCGR 元数据来自游戏文件本身，不需要网络采集那一整套。
- **PhotoStructure 的自动复制重组库**：UCGR 保持不写原目录。

### 不必做
- **全库两两 pairwise 关系存储**——Hydrus 官方文档明说 game CG（几十上百个同源变体）会打出百万级关系、模型失败正在重构；UCGR 用「事件/场景 = 组」的一级结构从数据模型上直接绕开。
- **视频 25 帧拼接 phash**（Stash 为视频设计）；UCGR 是静态 CG，直接 DCT pHash。
- **地图/反向地理编码/GPS**：CG 无此维度。
- **人脸识别聚类**：galgame CG 的「角色」用 tag 体系达成更准；除非以后做角色自动聚类再考虑嵌入。
- **RAW 处理、云备份接入、scraper 生态、E2E 加密、跨设备同步**。

---
*核对注：所有文档页均 web_fetch 打开验证；star/pushed 经 GitHub API（代理通道）2026-11-21 取样；PhotoStructure GitHub 仓库未确认公开（已标注），其余仓库页均验证存在。*
