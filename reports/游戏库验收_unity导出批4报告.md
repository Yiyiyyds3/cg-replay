# 游戏库验收 · Unity 导出批 4 报告

- 仓库基线：`main @ 2e73cc1`（批3 收尾提交；逐款提交 `e2ec523` LifeguardHolic / `0a4cb9c` NTRaholic / `8dfb018` LoveConfessions / `6459a28` MasyouSyoujo，本报告随收尾提交）
- 日期：2026-11-15
- 红线遵守：**全程只读游戏目录**；所有导出产物仅写 `D:\UCGR_Work\unitycg\<短名>\`，未写入/改动任何游戏目录。
- 工具链：`tools/unity_cg_export.py`（main 版：`--assets` / `--still-regex`，rx 语义 = `re.search`，全部 `^` 锚定）+ `cmd/spike-a -mode unity`（ScanUnity 数 events）+ UnityPy 只读 m_Name 探针（`D:\UCGR_Work\probe\probe_names.py`，命名族**先探后锚定**）；Python/UnityPy 环境与批1/批2/批3 同款。**单款预算放宽到 10 分钟**（批3 按 4 分钟红线留下的 4 款大款，本批全部实跑）。

## 一、选款与形态探明（只读测量）

批3 留下的 4 款 B 形态大款（批2 已实探 NTRaholic/Love Confessions 2 款，解密/魔性少女 本批补探确认）。E 形态 5 款（84865、HO(中上)、斯塔WIN(9)、棘罪最新版、洗脑APP大小姐）本批继续跳过。按体积升序：

解密→内层 Lifeguard Holic 5261MB（IL2CPP）、（牛）NTRaholic 5618MB（Mono+Live2D）、Love Confessions on the Adventure 5669MB（Mono）、魔性少女的诱惑同居 11371MB（Mono+Live2D）。

## 二、逐款导出与重扫结果（4 款）

| 游戏 | 组 | 顶层体积 | 容器形态 / 命令要点 | 导出结果 | events | 新 verdict | 报错/备注摘要 |
|---|---|---|---|---|---|---|---|
| 解密 → 内层 Lifeguard Holic | 未评级 | 5261 MB | IL2CPP；`Lifeguard Holic_Data\resources.assets`(280.5MB,1563 tex)+`sharedassets0.assets`(16.8MB,25 tex) 2 容器全量传参，.resS 齐全（resources.assets.resS 4396.6MB）；`--still-regex ^Screenshot_` | 导出 38.2s，cg=108（spine 100 + still 8）/ png=217（236.2 MB） | **100** | **pass**（unity_export_rescan） | Spine 骨架∩图集自动配对全中：youy_*（日期号场景族）、s#/s#_*n#_*、idle#、sex#/sex#p、yy#、kagura_*、grab_rub_*、group_sex_sisters_main 等 100 组 H 场景全部可回放；still 8 = Screenshot_1..8；swim_suit_#(60)/other_suit_#(46) 换装预览族、hair_#/body_# 等未锚定（保守）；重扫 events=100/scenes=100（与索引 108 部差额为扫描层合并） |
| （牛）NTRaholic | 未评级 | 5618 MB | Mono+Live2D；`Ntraholic_Data\resources.assets`(71.6MB,3359 tex)+sharedassets0..7.assets 共 9 容器全量传参，.resS 齐全（resources.assets.resS 4881.3MB）；`--still-regex ^(nakadashi \|ossan[0-9]+ deru \|main dashi \|mob dash )` | 导出 2.6s，cg=86（spine 6 + still 80）/ png=86（**1.0 MB**） | **86** | **pass**（unity_export_rescan） | ⚠ **量级存疑如实记录**：PNG 合计仅 1.0MB、单张约 12KB，疑为 H 场景部件帧/低分辨率纹理（KanzenChikan 17 部 0.0MB 同类先例，见 §四）；探针族计数 70+27+13+11=121，跨容器同名纹理 setdefault 去重后实导 80；重扫 {cgs:0, events:86, scenes:86}；Live2D 场景本体（Spring_CG# model/motion 族、replay_#）归 --auto/Live2D 批 |
| Love Confessions on the Adventure（一般）／冒險途中全員告白 | 一般 | 5669 MB | Mono；`冒險途中全員告白_Data\resources.assets`(83.1MB,1647 tex)+`sharedassets0.assets`(21.9MB,259 tex) 2 容器传参（sharedassets1..62 实测 0 tex/0 text 不传；resources.assets.resS 4490.2MB）；`--still-regex ^state_girls_photo` | 导出 67.2s，cg=103（spine 98 + still 5）/ png=138（579.9 MB） | **100** | **pass**（unity_export_rescan） | 5 女主 H 场景 Spine（BH1..5/MH1..5/GH1..5/WH1..5/IH1..5）+ BCG/GCG/MCG/ICG/WCG 族 + battle/wander/train 等小游戏 Spine 共 98 组；still 5 = state_girls_photo1..5（少女相册）；批2 预估「超 4 分钟红线概率高」实测 67.2s——元数据-only 扫描远快于体积直觉；重扫 events=100/scenes=100（与索引 103 部差额为扫描层合并） |
| 魔性少女的诱惑同居～禁忌缠绵每一天～（MasyouSyoujo） | 未评级 | 11371 MB | Mono+Live2D；`MasyouSyoujo_Data\resources.assets`(256.9MB,2672 tex,7912 text)+sharedassets0.assets(2 tex)+sharedassets1.assets(110 tex)+sharedassets2.assets(0) 4 容器全量传参，.resS 齐全（resources.assets.resS 8454.5MB，本批最大）；`--still-regex ^(gallery[0-9]+\|erobana_\|img_[0-9]+g$)` | 导出 22.1s，cg=425（still 425）/ png=425（37.1 MB） | **425** | **pass**（unity_export_rescan） | 该作无 Spine .atlas 文本对（全 Live2D：Face_#_#.motion#/up_Dakisime_Love#_/Furo#_/Kiss_Love#_ 等文本 7765 段未被 Spine 路径捕获）；still 命中 = gallery01..06（CG 鉴赏缩略）+ erobana_6..14（H 解锁标记）+ img_Ng 族 409 张（单张约 91KB，量级正常贴图层）；img_Ng 的「g」后缀语义未逐一人工验图，按命名族+可解码事实保守口径（同 KanzenChikan）；重扫 {cgs:0, events:425, scenes:425}；Live2D 交互 H 场景本体归 --auto/Live2D 批 |

**本批 pass 数：4**（LifeguardHolic 100、NTRaholic 86、LoveConfessions 100、MasyouSyoujo 425，合计 events=**711**）；**fail：0；timeout：0**（最长 67.2s）。判定口径与批1/批2/批3 一致：events>0 → pass（扫描层），**无编造通过**。导出库合计约 855 MB（236.2+1.9+579.9+37.1），全在仓库外。

## 三、Live2D 形态说明

NTRaholic、MasyouSyoujo 两款为 **Live2D 形态**（目录内 `.model` / `.motion` / `.cdi` / `.physics` 齐全）。main 版导出器只吃 Unity 贴图载体（Spine 骨架∩图集自动配对 + `--still-regex` 静图），**Live2D 场景本体（.model/.motion 交互 H 场景）本批不解析，归 --auto/Live2D 批**；本批判定只覆盖贴图层——NTRaholic still 80 + spine 6、MasyouSyoujo still 425——不夸大覆盖面。LoveConfessions 的 Live2D 无关（Mono 纯 Spine+still），LifeguardHolic（IL2CPP）主力载体是 Spine 配对，still 仅补 8 张截图族。

## 四、量级存疑备注（沿用 KanzenChikan 先例）

- **KanzenChikan（批3）**：17 部纯数字名 PNG 合计 0.0MB——量级存疑，如实记录在案。
- **NTRaholic（本批）**：86 部 PNG 合计仅 1.0MB、单张约 12KB——同款「量级存疑」，疑为 H 场景部件帧/低分辨率纹理。判定 events>0 → pass 属扫描层事实，量级备注不掩盖，可人工抽看 PNG 确认。
- **MasyouSyoujo（本批）**：img_Ng 409 张合计 37.1MB 量级正常，但「g」后缀语义未逐一验图，按命名族+可解码事实保守口径（同 KanzenChikan）。

## 五、对总进度的影响（算式闭合）

- pass：49 + 4 = **53**
- fail：22 − 4 = **18**
- A 档（现成工具可推）：28 − 4 = **24**
- 核对：53 + 6（partial）+ 18（fail）+ 40（skip）= **117** ✅；A 24 + B 9 + C 31 = **64** = partial 6 + fail 18 + skip 40 ✅
- 未通过收敛：68 款/58%（批3 后）→ **64 款/55%**
- 类①（Unity 缺 cg-index.json，31 款）现状：**pass 16**（批1 3 + 批2 3 + 批3 6 + 批4 4）+ **fail 10**（批1 4 + 批2 3 + 批3 3，待 --auto/图集切分/3D 渲染/视频批复试）+ **未动 5**（E 形态：84865、HO、斯塔WIN(9)、棘罪最新版、洗脑APP大小姐）

## 六、实测推翻的直觉（诚实记录）

批2/批3 按「全量内嵌导出超 4 分钟红线」留下这 4 款大款（5.3~11.4GB）；本批实测导出仅 **2.6~67.2s**——导出器是元数据扫描+按需解码 resS，不搬全量像素，本机 NVMe 上「超红线留下批」的体积直觉不成立。后续选款不再按体积预判红线，直接实跑。

## 七、下批继续（诚实清单）

1. **E 形态 5 款**（84865、HO、斯塔WIN(9)、棘罪最新版、洗脑APP大小姐）人工看目录后归形态；
2. **0 CG 复试 10 款**（批1 4 + 批2 3 + 批3 3）：Addressables aa / Luban 表 / sactx 图集 / 3D 渲染 / 视频载体，待 `--auto` 版导出器（feat/spine-skins-cover 已验证）合入 main 后重试；
3. **量级存疑抽验**：NTRaholic 1.0MB / KanzenChikan 0.0MB 人工抽看 PNG 确认是否占位图/部件帧；
4. **Live2D 批**：NTRaholic、MasyouSyoujo 的 .model/.motion 场景本体待 --auto/Live2D 批。

## 八、产物

- 判定 JSON：`reports/audit/unity-batch4/<短名>.json` × 4 + `summary.json`（已随逐款提交 e2ec523 / 0a4cb9c / 8dfb018 / 6459a28 落库）
- 导出库：`D:\UCGR_Work\unitycg\{LifeguardHolic, NTRaholic, LoveConfessions, MasyouSyoujo}`（pass 4 款合计约 855 MB，全在仓库外）
- 探针证据：`D:\UCGR_Work\probe\{LifeguardHolic, NTRaholic, LoveConfessions, MasyouSyoujo}.txt`
- 游戏目录零写入。
