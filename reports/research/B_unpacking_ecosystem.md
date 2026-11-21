# UCGR 同类项目调研 · 方向 B：游戏资源解包/提取生态（UCGR 引擎适配层参考）

调研时间：2026-11-21 ｜ 调研员：子代理 5cfbfd98（方向 B）；本篇为完整版，取代 lead 应急补写版
调研方式：web_search + web_fetch 实证（GitHub 页面/README/格式清单/SourceForge/GalgameWiki/CSDN）。本会话网络受限：GitHub API 限流（star/日期来自首轮 API 成功调用与 shields.io 徽章）、raw.githubusercontent.com / jsdelivr CDN / r.jina.ai / web.archive.org 直连失败，文件内容改经 ghproxy.net 镜像读取；QuickBMS 官网（aluigi.altervista.org）与 watto.org/extract/info.html 打不开，已在条目标注。共核实 12 个真实项目 + 3 条生态补充。

---

## 1. GARbro（morkt）⭐ 生态核心，UCGR 最大参照物
- URL：https://github.com/morkt/GARbro ；支持格式表：https://morkt.github.io/GARbro/supported.html（已抓取验证）
- 活跃度：3,372★；仓库 push 2024-07-08（API），默认分支最后提交徽章显示 2023-10；未归档；MIT；C#/.NET 4.6（Win/WPF）
- 核心功能：视觉小说资源浏览器。资源管理器式 UI：Enter 把归档当作目录进入；F4 提取选中文件并**自动把图像/音频转成通用格式**；F6 单独转换；F3 可重建部分归档（写方向）；加密归档弹窗要求凭据或猜测的游戏名；文件类型识别错可手动 Assign file type；含简中/日/韩翻译
- 格式库维护（已实证目录结构）：ArcFormats 工程按**引擎命名目录**，抽出 200+ 目录：KiriKiri、NScripter、RealLive、RenPy、CatSystem、Artemis、Ethornell（=BGI/Buriko）、Majiro、Malie、QLIE、YU-RIS(YPF)、WillPlus、Eushully、Escude、Glib2、Nekopack、NitroPlus、Softpal、Leaf、LiveMaker、ShiinaRio、Cmvs、DjSystem…每目录内按 ArcXxx.cs（归档）/ ImageXxx.cs（图像）/ AudioXxx.cs（音频）分文件；公共件沉淀在工程根部：MultiFileArchive/SingleFileArchive/PackedStream 基类、LzssStream/Lzma/Lz4Stream/HuffmanCompression、RC4/Blowfish/MersenneTwister、CRC16/32/64、keytable.dat 已知游戏密钥表
- 支持矩阵：supported.html 列 = Files（扩展名）| Signature（魔数）| Create | Brand/Engine | Titles，约 168 行归档格式，例：*.xp3=XP3、*.tlg=TLG0.0/5.0/6.0、*.nsa/*.sar（NScripter）、*.rpa=RPA-3.0、*.ypf（YU-RIS）、*.pna=PNAP、*.war=WARC1.1–1.7
- 对 UCGR 的具体借鉴：①适配层目录法 formats/<engine>/ + 三类实现分离 + Go interface 基类；②探测=魔数优先、扩展名兜底，支持矩阵页与批量导入自检报告共用同一注册表数据源自动生成；③压缩/加密原语独立成包，新引擎只写差异；④加密包弹窗问游戏名 + 内置密钥表（keytable.dat → UCGR 内置 JSON 密钥库）；⑤提取即转码入库（PNG/OGG）
- 局限：近两年停更；WPF 单平台；无 CLI 批处理模式；写方向支持有限

## 2. arc_unpacker（vn-tools）
- URL：https://github.com/vn-tools/arc_unpacker
- 活跃度：671★；最后提交 2021-09（徽章）/push 2022-02；**archived=true**；C++11，Windows/Linux，无 GUI（作者在 README 明确论证过不做 GUI 的理由：跨平台成本、解码器选择与每游戏密钥难以在 GUI 表达）
- 核心功能：拖拽/CLI 解包，自动猜格式；--dec=engine/format 显式选解码器（如 --dec=team-shanghai-alice/pbgz）；--plugin 传每游戏解密插件（XP3 例：--dec=krkr/xp3 --plugin=fsn 即 Fate/stay night 解密）；GAMELIST.htm 是**结构化游戏数据库**：每游戏含 title/title_orig/dev/date/fmt（按资源类型分列的格式）/vndb id/CLI 调用参数/info；明确拒绝封包（投入产出比论证）
- 格式库：src/arc/fmt/<engine>/<format> 目录树 + 每格式 EntryBuilder 注册；覆盖 NScripter、KiriKiri、RGSSAD、RPA、QLIE、YPF、Majiro、BGI 等（registry.cc 在 master 404，目录结构由 README/GAMELIST 交叉验证）
- 对 UCGR 的借鉴：①游戏→密钥/参数/VNDB 索引表结构直接对齐 UCGR 游戏库，fmt 按资源类型分列可参考进事件元数据；②engine/format 命名空间 + 猜测冲突时人工指定的降级交互；③只读不写的定位与 UCGR 一致；④贡献模型（记录游戏/移植他人解码器）值得写进 UCGR CONTRIBUTING
- 局限：停更归档；大文件 std::bad_alloc（需 -t=1 --no-recurse）；无图像元数据输出

## 3. Crass / crage（w8m/Yuu）
- URL：无官方开源仓库（闭源、停更）；资料页 https://www.galgamewiki.com/?p=347 （已抓取）；工具集合帖 https://blog.csdn.net/luozhuang/article/details/45872757
- 活跃度：最终版 0.4.14.1，免费软件、简中、Win7+，状态已停止更新
- 核心功能：Crass = Crage（解包）+ Assage（封包）；CUI/AUI **脚本插件制**解析上百种私有归档，汉化组老牌万能解包利器
- 对 UCGR 的借鉴：新格式=新脚本而非改主程序的外置格式包思想（远期形态）；解包/封包职责分离的命名
- 局限：闭源、停更、无版本库可考古，现代引擎力不从心（社区共识）

## 4. KrkrExtract（X'moe 汉化组）
- URL：可访问镜像 https://github.com/unlimit999/KrkrExtract （24★，2016-04）；原仓库 github.com/xmoeproject/KrkrExtract 已 404（实测）
- 活跃度：停更；闭源（README 原话 won't be an open source project）
- 核心功能：krkr2/krkrz 的 XP3 解包+封包，IA32/AMD64 双版本，多语言 UI，支持特殊加密包（如サノバウィッチ）
- 对 UCGR 的借鉴：XP3 每游戏解密插件集合是 UCGR Kirikiri 模块的补强来源；特殊包逐游戏适配的实践路径
- 局限：闭源、原仓库消失——闭源工具生态脆弱的实证

## 5. arc_conv（w8m，与 Crass 同作者）
- URL：镜像 https://github.com/GameCodes/arc_conv （0★，2014-06）；原发布于俄文论坛
- 活跃度：无维护；纯 x86 ASM
- 核心功能：命令行 VN 工具箱，183 个 arc_*.asm 单文件格式模块：bgi、nsa、xp3、rgssad、rpa3、qlie、ypf、majiro、malie、will、softpal_ap/fh、eushully、escude、glib2、kid_lnk、dsc（Ethornell/BGI 系）…
- 格式库维护（已抓 formats.txt，454 行）：手工表，每行 = (格式代号) 已验证游戏名清单 (文件模式) (转换输出)，如 (kid_lnk) "Ai Yori Aoshi","Ever 17" (*.dat) (cps > tga)；通用格式归 find_common
- 对 UCGR 的借鉴：**游戏名锚定的格式验证表** = 批量导入自检的最佳数据模板（格式→样例游戏→期望文件名→转换输出）；一格式一文件的极端模块化
- 局限：ASM 无法移植；无元数据输出；文档在论坛生态里

## 6. QuickBMS（Luigi Auriemma）
- URL：http://aluigi.altervista.org/quickbms.htm —— **本轮官网直连失败**，事实来自 2026-08-28 更新的第三方指南 https://www.codegenes.net/blog/quickbms-download/ （已抓取）
- 活跃度：第三方指南称 2026 年仍是行业标准、持续更新（最新版本号未能核实）
- 核心功能：BMS 脚本语言驱动的通用提取/回写器，**2900+ 归档格式**脚本、数千游戏；reimport 改后资源回写归档；GUI+CLI
- 对 UCGR 的借鉴：格式知识外置成数据（脚本库）+ 程序只做执行器的分层；社区脚本分发对应 UCGR 未来冷门引擎格式包化
- 局限：脚本门槛高、质量参差；无 CG/立绘/语音语义分类输出

## 7. Game Extractor（Watto Studios）
- URL：https://www.watto.org/ （主站可达，/extract/info.html 404）；SourceForge 项目页 https://sourceforge.net/projects/gameextractor/ （已抓取）
- 活跃度：Last Update 2026-08-01，周下载 290；免费+付费双版本；Java
- 核心功能：GUI 打开/编辑 **5700+ 游戏**归档（插件式 FormatPlugin），预览/提取/添加文件
- 对 UCGR 的借鉴：插件接口形态（探测函数+目录解析函数+预览）；watto specs 站按引擎组织的格式情报库可当资料源
- 局限：Java 桌面巨物；galgame 专用图像/音频编解码弱于 GARbro；部分功能付费

## 8. Unrpa（Lattyware）
- URL：https://github.com/Lattyware/unrpa ；介绍页 https://www.lattyware.co.uk/projects/unrpa/
- 活跃度：772★；最后提交 2019-12（徽章）/push 2022-06；Python、MIT、PyPI 分发
- 核心功能：Ren'Py RPA-0/1/2/3 提取，README 明确 can also be used as a library
- 对 UCGR 的借鉴：单格式专注 + 库化（UCGR 解包核心保持可 import 的 Go package，GUI 只是壳）；pip 式低门槛分发
- 局限：只管 RPA 容器，不碰 Ren'Py 内部图像/音频

## 9. RPGMakerDecrypter（LynxShu）⭐ 与 UCGR 现有 RPGMV 模块直接对标
- URL：https://github.com/LynxShu/RPGMakerDecrypter
- 活跃度：45★；push 2026-10-26（非常活跃）；TypeScript/Node，MIT，Windows GUI+CLI 双形态，含简中/繁中/英/日与自定义语言包
- 核心功能：MV .rpgmvp/.rpgmvo/.rpgmvm 与 MZ .png_/.ogg_/.m4a_ 解密+再加密；**无密钥时重建 PNG 文件头恢复图片**；**密钥反推**（System.json / rpg_core.js / 加密图片头三来源）；多线程批处理：实时进度、可取消、单文件超时自动跳过、超大目录流式扫描+渐进渲染；导出为目录或 ZIP；CLI 带 --json、--dry-run、detect-key/decode-dir 子命令
- 对 UCGR 的借鉴：①key 反推三来源直接提升 UCGR RPGMV 解密成功率；②批处理工程细节（进度/取消/超时跳过/流式扫描）升级 UCGR 批量导入；③导出目录树 or 单 ZIP + --json 报告，正好挂有效性自检
- 局限：只覆盖 RPG Maker MV/MZ；无事件/CG 语义组织

## 10. Galgame_automation_processing_tool（YIWANG-sketch）
- URL：https://github.com/YIWANG-sketch/Galgame_automation_processing_tool
- 活跃度：54★；最后提交 2024-03；Python 3、GPLv3、CLI 引导式
- 核心功能：解包+封包自动化流水线：WillPlus/AdvHD（.arc 解/封、.ws2 编解码、.pna 复合图形）、Softpal（.pac、TEXT.DAT）；文本提取为 **JSON{name, message}** 与人名表；自动化汉化流程（原始.arc→汉化.arc）
- 对 UCGR 的借鉴：{name,message} JSON 结构可平移为事件→CG 序列元数据 schema；人名表=立绘归属线索（CG 按角色聚合）；分段式流水线 CLI 设计
- 局限：面向汉化而非回放；引擎覆盖窄；工程化程度一般

## 11. XFileUnpacker（horsicq，XUnpacker 线）⭐ 支持矩阵文档的最佳形态
- URL：https://github.com/horsicq/XFileUnpacker
- 活跃度：9★；最后提交 2026-11-21（当天仍在动）；Qt5/6 + C++17，跨平台 CLI+GUI，0.1.0 早期
- 核心功能：通用解包器：XArchive 原生引擎（ZIP/7z/RAR/CAB/TAR 免 7z.exe）；内建 Quake PAK、Doom WAD、**Godot PCK**、WBFS/RVZ 等；参考版 **485 种类型、21 张分类表**（其中 Game and engine containers 含 NScripter NSA/NS2/SAR、RPG Maker RGSSAD、RPG Maker MV resource、Ren'Py RPA、MPQ 等）
- 文档维护（已抓 doc/CURRENT.md）：完整支持矩阵，每格式标 **detect/list/extract 三态能力** + 所用 codec + 已知限制；xfileunpackerc -i 让二进制自打印支持清单；自动探测 20s 预算（--probe-timeout 可调）
- 对 UCGR 的借鉴：①自检报告升级为 detect/list/extract 三态矩阵（识别成功/能列目录/能导出）；②GUI 诊断页一键导出能力清单；③探测超时预算防批量扫描卡死；④Godot PCK 的第三方实现可对照 UCGR 现有实现
- 局限：galgame 引擎专精弱（游戏容器仅 9 种）；项目早期

## 12. UnRen-forall（Lurmel）
- URL：https://github.com/Lurmel/UnRen-forall
- 活跃度：52★；最后提交 2026-10-04；批处理启动器+脚本集，多语言
- 核心功能：Ren'Py 游戏一键工具包（unencrypt/提取 RPA/反编译 rpyc 全流程一键化）
- 对 UCGR 的借鉴：一键全流程交互（扫描→解包→就绪）；RenPy 游戏可组合 unrpa 逻辑进 UCGR 流水线
- 局限：Ren'Py 专用；bat+python 脚本质感

### 生态补充（快速过）
- AssetStudio https://github.com/Perfare/AssetStudio ：15,591★，2022-12 归档；Unity 资源浏览/导出，按类型（Texture2D/Sprite/AudioClip）+按容器双层目录组织导出——UCGR Unity CG 导出组织可参考；社区有活跃 fork
- Kirikiroid2 https://github.com/zeas2/Kirikiroid2 ：Android krkr2/krkrz 运行时，XP3 解密兼容面广，可当密钥情报源
- 中文工具索引页 https://github.com/aoikaze-unofficial/board-ng/blob/main/pages/nav/tools.md （已抓取）：GARbro/KrkrExtract/Crass 的社区口碑定位，可写进 UCGR 文档的工具对比章节

## 对 UCGR 的灵感清单

### 值得抄
1. **适配层目录法**（GARbro）：formats/<engine>/ 下 archive/image/audio 三类分文件；LZSS/LZMA/LZ4/Huffman/RC4/Blowfish/MT19937/CRC 等公共原语独立成 primitives 包，新引擎只写差异
2. **支持矩阵自动化**（GARbro supported.html + XFileUnpacker doc/CURRENT.md）：扩展名+魔数+引擎+样例游戏+detect/list/extract 三态，从代码注册表自动生成一页文档，并复用为批量导入自检报告格式
3. **游戏→密钥/参数索引**（arc_unpacker GAMELIST + GARbro keytable.dat）：UCGR 建 known_games 表（游戏名/别名→引擎、密钥、特殊参数、VNDB id），识别失败时交互补录
4. **RPGMV 增强三件套**（RPGMakerDecrypter）：key 反推（System.json→rpg_core.js→图片头）、无 key PNG 头重建、批处理进度/取消/单文件超时跳过+流式扫描
5. **导出标准化**（GARbro F4 + RPGMakerDecrypter）：提取即转 PNG/OGG 入库；输出目录树 or 单 ZIP 双形态；CLI --json 机器可读报告
6. **探测预算**（XFileUnpacker probe-timeout）：批量扫描每文件设时限防卡死
7. **库化分发**（unrpa）：解包核心保持可 import 的 Go package
8. **游戏名锚定的格式验证表**（arc_conv formats.txt）：格式→已验证游戏→期望文件名模式→转换输出，直接当 UCGR 自检用例库模板

### 值得简化
1. 通用容器泛化：只做游戏引擎容器一档，不碰磁盘镜像/文件系统/SFX 壳
2. 资源管理器式双面板 GUI（GARbro）：UCGR 主界面是事件回放，扫描期解包做成后台任务+进度条即可
3. 格式外置化（QuickBMS/Crass）：现阶段编译期注册够用，引擎数 15+ 再考虑格式包数据化
4. 多语言/主题框架：有真实用户再上
5. 引导式流水线 CLI：UCGR 有 GUI，CLI 只保留非交互参数式

### 不必做
1. **写方向能力**（封包/reimport）——arc_unpacker 作者已论证投入产出比极低，UCGR 是回放库更不需要
2. 2900+ 格式全覆盖路线：UCGR 聚焦 galgame 十余引擎
3. RAR/7z 通用压缩、磁盘镜像、安装器 SFX
4. Hook/OCR 运行时提取、Android 运行时
5. 闭源插件生态：KrkrExtract 原仓库已 404 的前车之鉴，UCGR 保持 MIT 开源

### 验证缺口备注
- GitHub API 本会话限流：个别 star 数未能补齐；已用首轮 API 数据 + shields.io 徽章交叉
- QuickBMS 官网、watto.org/extract/info.html、raw.githubusercontent.com 直连、web.archive.org 均打不开（已在对应条目标注），文件内容经 ghproxy.net 镜像读取，README/格式清单均为一手文本
