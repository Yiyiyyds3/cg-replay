# UCGR 同类项目调研 · 总报告（灵感清单）

调研时间：2026-11-21 ｜ 方式：4 个子智能体并行调研（B 方向迟交、lead 应急补写版已被完整版覆盖），全部项目经 web_fetch/web_search 验证存在。共 40+ 项目。

## 一、最重要的三条结论

1. **UCGR 的生态位是空的**。「外部 CG 回放器 / 事件→CG 序列回放」没有现成实现——GARbro、arc_unpacker、QuickBMS 全部止步于资源文件层；VNTextPatch 只做文本；Ren'Py/WebGAL 的回放只服务自家引擎。方向 A 的检索也证实「CG回廊」只有游戏内实现、无外部工具。连「A/B 对比模式」在全部 40+ 项目里都没人做。UCGR 做的语义层（事件→CG 序列回放）是真空地带。
2. **去重的完整工程答案在自托管媒体库圈已跑通**。四层漏斗：字节哈希 → 64-bit DCT pHash → 嵌入 → 人工复核（没人敢全自动删）。Hydrus 的关系标注模型（duplicate/alternate/king + 传递推理）+ 复核队列 UI 是最完整实现；其官方文档亲口承认 game CG 会打爆 pairwise 关系模型——UCGR 用「事件=组」的一级结构从数据模型上直接绕开。
3. **差分合成有现成原料可拼**。krkr 的 pimg/psb 容器拆开就是「散图 + 坐标层级 JSON」（FreeMote）；Ren'Py layeredimage 的 group/attribute/auto 是现成的规则 DSL 词汇表；文件名约定兜底。UCGR 的 krkr 解码管线接在 FreeMote 产物后面即可自动合成事件 CG。

## 二、四方向速览

| 方向 | 代表项目 | 对 UCGR 最有价值的一条 | 报告 |
|---|---|---|---|
| A 库管理&CG鉴赏 | KoiPage(151★)/GARbro/Playnite/create-all-cg-save | 扫描→识别→候选→人工确认的两段式入库流水线 + VNDB 元数据主源 | reports/research/A_galgame_manager_cg_viewer.md |
| B 解包生态 | GARbro(3.4k★)/arc_unpacker/RPGMakerDecrypter/XFileUnpacker | detect/list/extract 三态支持矩阵自动化生成 + known_games 密钥索引表 | reports/research/B_unpacking_ecosystem.md |
| C 媒体库UX | Hydrus/Immich/Stash/TagStudio/LibrePhotos | pHash 指纹（带算法版本）+ 去重=关系标注(king) + 复核队列 UI | reports/research/C_selfhosted_media_ux.md |
| D 立绘合成&事件回放 | FreeMote(530★)/Ren'Py/ReplayBook/VNTextPatch | 差分规则双层设计（文件名约定+overlay JSON）+ 事件时间轴回放交互 | reports/research/D_tachie_composite_event_replay.md |

## 三、合并灵感清单（跨方向去重）

### 值得抄 · TOP 15
1. **三层去重漏斗**（C）：字节哈希（已有）→ 64-bit DCT pHash（导入时从缩略图顺路算，列带算法版本号）→ 可选嵌入；phash 冲突并入批量导入自检报告。
2. **去重 = 关系标注不是删除**（C/Hydrus）：potential_pair → {duplicate|alternate|none} + king；决策入库、可查询、传递推理省一半人工。
3. **复核队列 UI**（C/Hydrus + Immich）：双栏并排、缩放同步、键盘判定；自动预选规则（字节更大/元数据更多者优先），丢弃者元数据并入保留者。
4. **事件=组的数据模型**（C 反面教材的直接教训）：CG 变体挂事件组内，不做全库两两 pairwise——Hydrus 官方承认 game CG 会百万级关系爆炸。
5. **detect/list/extract 三态自检矩阵**（B/XFileUnpacker）：自检报告从「成功/失败」升级为「识别成功/能列目录/能导出」三态 + 探测超时预算。
6. **known_games 密钥/参数索引表**（B/arc_unpacker GAMELIST + GARbro keytable.dat）：游戏名/别名→引擎、密钥、特殊参数、VNDB id；识别失败时交互补录。
7. **RPGMV 解密增强三件套**（B/RPGMakerDecrypter）：key 反推三来源（System.json→rpg_core.js→图片头）、无 key PNG 头重建、批处理进度/取消/超时跳过。
8. **每引擎一个 scenario adapter**（D）：统一输出 {event_id, cg_sequence[], meta}——事件映射自研，是 UCGR 的差异化核心。
9. **差分合成双层规则**（D）：默认文件名约定 + 可编辑 overlay JSON（基底 role + 层列表 x/y/z），词汇表对标 Ren'Py layeredimage；krkr 优先读 pimg/psb 内嵌 JSON（FreeMote 产物直接吃进来）。
10. **事件时间轴回放交互**（D/ReplayBook）：扫描→索引→筛选/标签/收藏→详情面板→时间轴跳转→回放。
11. **Gallery 状态机语义**（D/Ren'Py rooms.html）：网格缩略图 + locked 占位 + hover 边框 + 跨页 span + slideshow_delay 幻灯片。
12. **VNDB API 主源 + 两段式刮削**（A）：自动候选 top-N → 人工确认；tag 的 spoiler level 0/1/2 映射到 CG 剧透分级。
13. **标签别名/shorthand/namespace**（C/TagStudio + Hydrus）：「Saber/セイバー/阿尔托莉雅」互相命中，galgame 角色名刚需；引擎自动 tag 与用户 tag 分离存储。
14. **saved filters + 查询语法 + Cmd+K**（C/Stash + LibrePhotos）：可设默认的保存筛选器、or/-/引号查询语法、命令面板。
15. **导入流水线两段式**（A/KoiPage）+ **per-game action 模型**（A/Playnite）：扫描→识别→确认→入库；解包/回放/导出都挂到游戏上。

### 值得简化
- 向量检索：CG 量级 SQLite 存 phash + 批量位运算即可；嵌入用单机 ONNX 小模型，不抄 Immich 的独立 ML 服务。
- 数据库：SQLite 足够（Stash/TagStudio 双证明），不上 Postgres。
- 格式外置化（QuickBMS 脚本/Crass 插件）：编译期注册够用，引擎数 15+ 再考虑。
- 云同步降级为 JSON 快照导出；插件系统降级为适配器注册表扩展点。
- 多语言/主题框架/商店分发：有真实用户再上。

### 不必做
- 全库 pairwise 关系存储（Hydrus 自认失败正在重构）。
- 写方向能力（封包/reimport）——arc_unpacker 作者论证过投入产出比极低。
- 实时差分渲染引擎/Live2D/Spine、文本 hook/翻译、booru 图站下载、人脸聚类、地图/GPS、多用户/移动端、2900+ 格式全覆盖。

## 四、建议路线图（按依赖排序）

**P1 · 数据层（先动，其他都压在上面）**
- CG 条目加 phash 列（64-bit DCT，带算法版本）+ known_games 表 + 标签 alias/shorthand 字段 + 事件=组的变体挂载模型。
- 导入管线顺路算 pHash（从缩略图）；自检升级为 detect/list/extract 三态 + phash 冲突报告。

**P1 · 解密增强（立即可做）**
- RPGMV key 反推三来源 + 无 key PNG 头重建（对标 RPGMakerDecrypter，主人库里的 RPGMV 游戏直接受益）。

**P2 · GUI（差异化主战场）**
- 去重复核队列（双栏对比+键盘判定）；
- 事件时间轴回放（ReplayBook 交互）+ gallery 状态机（locked/幻灯片）；
- **对比模式**（A/B 双窗格）——全部调研项目都没做，独一份；
- krkr 差分合成管线接 FreeMote 产物。

**P2 · 语义层（核心壁垒）**
- 每引擎 scenario adapter 输出 {event_id, cg_sequence[]}；krkr 的 KAG 标签流（[image storage=…]）是第一刀。

**P3 · 元数据与长尾**
- VNDB API 主源 + 两段式刮削 + saved filters + Cmd+K；
- 覆盖不了的引擎输出全 CG 存档（create-all-cg-save 路线，状态注入互补）。

## 五、文件索引

- A：reports/research/A_galgame_manager_cg_viewer.md（14 项目，库管理器/CG 鉴赏/VNDB 生态）
- B：reports/research/B_unpacking_ecosystem.md（12 项目 + 3 生态补充，格式注册表/密钥表/自检矩阵）
- C：reports/research/C_selfhosted_media_ux.md（10 项目，Hydrus 深挖 + 去重漏斗/标签/存储对比）
- D：reports/research/D_tachie_composite_event_replay.md（12 项目，差分 5 种规则来源 + 事件映射结论）

*验证级别注：A/C/D 由子代理逐条 web_fetch 验证（含 star/API 取样时间）；B 子代理迟到交付完整版（GARbro supported.html、arc_conv formats.txt、XFileUnpacker doc/CURRENT.md 等一手文本），lead 应急补写版已覆盖。各报告内已逐项标注网络受限导致的缺口。*
