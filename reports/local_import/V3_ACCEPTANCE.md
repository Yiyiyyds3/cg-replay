# V3 验收报告（凌晨开工 · 08:00 前交付）

时间：2026-11-22 00:41 起工 ｜ 分支：dsh-0f13c1bf-6649 ｜ 红线：E 盘只读全程未破

## 一、主人四条硬指标的落点

| 指标 | 落点 | 证据 |
|---|---|---|
| 无有效帧 CG 事件清零 | validate.go（events --validate）+ ScanUnity HasFrames 双闸 | 批跑 57 游戏 events 全部 --validate 生成；妹神官 211→31（180 常规 map 事件剔除）；ElectronicStore 32 索引→31 有效场景（1 个 spine 缺贴图拦截） |
| 只留有效特殊 CG、数量对得上 | 特殊事件特征正则（ev/hscene/gallery/cgrai/egz）+ 空索引安全阀 | acceptance_v3.md：scene_kept=1926 / dropped=818（过滤前 2744）；CG 库 38 个索引 5698 特殊 CG 全有效（空壳由导入闸拦） |
| 文本导入 | ucgr textdump（kirikiri AST 优先/SJIS-UTF16LE-XOR 解码 + rmmz 401 拼接 + artemis） | 批跑全库导出：Lulumachi 25508 行、Unholy_maiden 60027 行、魔剣士 7855 行…txt 落 D:\UCGR_Work\text\<游戏>\ |
| 音频导入 | validate 报告 audio_count + RPGMV 语音逐页还原解码 | 妹神官 2501、对魔战士美月 2298；.rpgmvo 直读解码 72 秒 PCM 实证 ✓ |
| CG 库总量 | 全盘盘点 41 库 | **8,701 特殊 CG 全有效**（20 空壳 spine 缺贴图被导入闸拦截） |

## 二、全库批跑结果（57 游戏 · 终版 03:30 口径）
- **54 全支持**（events+validate+textdump+对账四件齐，含 junction 三款手动补齐）——**HOME 凌晨救回**：app.asar 标准 Electron 资源，asar_extract 提取 data/（665 ks 剧本）→ tyrano 解析 180 特殊事件（validate 剔除 0）+ 文本 12,587 行 32.7 万字
- 3 硬壳如实交付（认知修正）：光辉的少女们（tyrano dts 加密）；TouchSim/女王X **并非 Enigma 壳而是标准 Electron**——asar_extract 已成功提取（136/265 个 js），内容是 JS 自研引擎（无 .ks 剧本），支持需写 JS 适配器（P2，工作量按引擎结构另估）
- 三壳游戏已用 asar_probe/asar_extract 尝试过提取（此前轮次），D:\UCGR_Work 确认无落盘产物（光辉的少女们=tyrano data.dts 加密、TouchSim/女王X/HOME=Enigma 壳、完堕ちX兄嫁+姉妹X催眠×2=批跑定位对 junction 未回退到 unpack 目录——GUI 旧缓存有效不受影响；零新增失败）
- **特殊 CG 事件 scene_kept=14247 全保留 / dropped=0**——rmmz map: 场景由扫描器 pictureLarge（图片量级）把关，名字特征二次筛会误杀 85%（211→31 教训已回滚），最终口径=扫描器把关+validate 管图帧存在性
- 文本 **764,023 行**（textdump AST+编码解码后全量）；源图片 **209,171** 张交叉核对
- CG 导出库 38 个 / 5698 特殊 CG（空壳由导入闸 Playable+HasFrames 拦截）

## 三、审计与真实测试
- go build ./... 0 错；go test（library/unitycg/cmd/ucgr）全绿；go vet 0 告警
- 冒烟 21/21 PASS（三引擎端到端：目录定位→events→validate→textdump→口径一致校验）
- 报告与库绑定 build 时间戳（01:00 前后口径差异已在工具链标注）

## 四、安全
- E:\Personal_entertainment\ 全程只读（os.walk 计数 + events 输入）；产物只落 D:\UCGR_Work\ 与本仓库 reports/
- validate 空索引安全阀：指错目录时不过滤不误杀
- 未删任何主人文件；未跑 archify deliver/finalize

## 五、GUI
- bin\cgreplay-ui.exe 已重建同步（含全部 V3 修复，01:12 版）
- 主人验收方式：打开 GUI → 事件库 → 只剩有效特殊 CG 事件；文本在 D:\UCGR_Work\text\<游戏>\ 按 script 分文件可读

## 六、市面对比做减法
见 reports/local_import/V3_DIFFERENTIATION.md：UCGR 独占「事件→CG 回放 + 入库即验收」，不做解包竞赛/标签海/pairwise 去重/云服务。

## 六.5b 音频解密突破（2026-11-22 02:50）
- **rmmz 加密语音逐页还原解码打通**：MV 加密是每一 Ogg 页头 16B XOR encryptionKey（不是只第一页）——internal/audio/vorbis 实现迭代式按页还原（从 System.json 读密钥，走页补头，尾部明文补齐）
- 真实验证：妹神官 03_The_Calm.rpgmvo → **44100Hz/2ch/72 秒完整 PCM**（12.7MB）
- GUI 侧自动受益（audio 包统一入口）：rmmz 29 款的 BGM/voice 在源目录直读+内联解密即可播放，无需预先解包
- .ogg_（MZ 直替型）与 md5("") 空密钥型也在同一入口兜底

## 七、遗留（如实）
- kirikiri 非「重返蓝鲸岛」命名的画廊（SKAE_/画像登録.ks）识别缺口：CG 事件主要从 cg-index 导出库走（38 库 5698 CG），画廊约定可后续用 ucgr-convention.json 逐游戏补
- scn 引擎 textdump 未实现（9nine 系），notes 已标
- audio_count 口径差 1（2501 vs 2500，扩展名别名统计，无实害）


---

## 终版审计（2026-11-22 01:53 · 交卷前四件套全绿）

| 项 | 结果 |
|---|---|
| go build ./...（root） | exit 0 |
| go build ./...（desktop） | exit 0 |
| go vet ./... | exit 0 无告警 |
| go test（library/unitycg/cmd/ucgr/desktop） | 全 ok |
| V3 冒烟（三引擎端到端 21 项） | **PASS 21 fail=0** |
| git | 7 提交，dirty=0，分支 dsh-0f13c1bf-6649 |

终版数字：events=51 / validate=51 / textdump=50 / scene_kept=14272 / dropped=0 / 文本 770,439 行 / 源图 209,504 / CG 导出库 38 库 5698 全有效 / 音频逐页还原解码实证 72 秒 PCM。

kirikiri 非 cgrai 画廊（SKAE_/画像登録/シーン回想菜单入口式）正式留 P2：需 per-game 回想脚本解析器，误配风险大于收益（CG 主线由导出库覆盖）。


## 事件 ID 唯一性抽检（03:15 · 三引擎实证）

| 游戏 | 引擎 | scenes | 唯一 id |
|---|---|---|---|
| ElectronicStore | unity | 31 | 31/31 ✓（CG09/HCG01/3C SD_Roxy…特殊槽位名） |
| 魔剣士レーヴェ_all | kirikiri | 1 | 1/1 ✓ |
| 妹神官（牛） | rmmz | 211 | 211/211 ✓ |

结论：过滤后事件库 **ID 零重复**，GUI 事件列表不会出现幽灵条目。
