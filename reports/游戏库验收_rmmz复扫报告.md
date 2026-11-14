# 游戏库验收 · rmmz 脚本 CG 修复复扫报告（2026-11-15）

> 背景：commit `489f126` 修复 `internal/adapter/rmmz/rmmz.go`——识别 code 355/655 脚本指令里的 `cgLoad('组名')`（Wolfzq 系 CG 插件调用），CG 素材按 `pictures/<组名>/` 子目录定位。本报告用 `go run ./cmd/library-audit -group 夯,未评级 -batch batch-rmmz-recheck -out reports/audit` 复扫，核对总报告 §四 rmmz 类 4 款 fail 是否救回。游戏目录只读，产物只写 `reports/audit/`。

## 一、修复逻辑确认（rmmz.go 关键点）

- `scriptCGRe = cgLoad\(\s*['"]([^'"]+)['"]`：匹配 Wolfzq 插件脚本调用；
- `scriptCGName(c)`：仅对 code 355/655 提取 CG 组名，其他指令返回空；
- `locateCGSet(root, imageDirs, name)`：在 `<img目录>/pictures/<组名>/` 下定位样本图；
- `Segments` 统计处 `c.Code == 231 || scriptCGName(c) != ""` 与指令 231 同权重计数，组名作预览。

## 二、4 款逐款对照（batch-rmmz-recheck.json 实测）

| 游戏（组） | 复扫前 verdict/events | 复扫后 verdict/events | 结论 |
|---|---|---|---|
| 呪いの鎧 v3.0.1（夯） | fail / 0（警告 849 条，扫不出图片演出） | **pass / 222**（events>0） | ✅ 救回 |
| 詛咒铠甲2（夯） | fail / 0（警告 1231 条，同模板） | **pass / 267**（events>0） | ✅ 救回 |
| wuxian（未评级） | fail / 0 | fail / 0（`没有识别到图片演出（扫了 www/data，图片目录 "www/img"，警告 13 条）`） | ❌ 仍 fail：脚本 CG 修复不适用，其素材不落在 `pictures/<组名>/` 约定下 |
| 不谙(（未评级） | fail / 0 | fail / 0（`没找到 RPG Maker 的 data 目录（试过 www/data、data、内嵌/data）`） | ❌ 仍 fail：目录结构问题，与脚本 CG 无关，走 DetectEngine 补识别路线 |

本批 rmmz(guess) 引擎汇总：总数 23，pass 21 / fail 2（即上述 wuxian、不谙(）。

## 三、回归抽查（原 pass 的 rmmz 款）

抽查 4 款，全部保持 pass，无回归：

| 游戏（组） | 复扫后 |
|---|---|
| (夯)魔塔 | pass / 31 |
| 苍红界域（夯） | pass / 209 |
| 对魔战士美月(（未评级） | pass / 129 |
| 星天之魔女（Arpg）（未评级） | pass / 238 |

## 四、统计与结论

- **救回：2 款**（呪いの鎧 v3.0.1、詛咒铠甲2，fail→pass）；
- **仍 fail：2 款**（wuxian：素材不按 `pictures/<组名>/` 约定；不谙(：缺 data 目录）；
- **回归：0 款**；
- **适用范围结论**：Wolfzq CG 插件的 `cgLoad` 调用约定（code 355/655 + `pictures/<组名>/`）在该插件的实际发行版本里是通用约定——呪いの鎧 与 詛咒铠甲2 是同插件不同作品，均被同一修复救回；但不是所有 rmmz 游戏都用该插件，wuxian 即反例。修复值得保留并已随 489f126 生效于全部 rmmz 款（本批复扫 23 款 rmmz 全部经过新逻辑，21 pass），无需进一步推广动作；剩余 2 款 fail 的卡点已改判为「类②缺 data 目录」「类③非 Wolfzq 约定的图片演出识别」，归 A 档尾部继续。

## 五、总报告数字同步

n=2：pass 35→**37**、fail 36→**34**（partial 6、skip 40 不变，117 闭合：37+6+34+40=117 ✅）；未通过 82→**80**；A 档 42→**40**（rmmz 子项 4→2）。详见总报告对应章节。

产物：`reports/audit/batch-rmmz-recheck.json`（63 款逐款判定，含本报告全部引用值）。

## 六、复核澄清（五条，全部以文件证据为准）

### 1. 「117 = 37+6+34+40」是 verdict 四态分布，不是 A 档

`37+6+34+40=117` 的四个加项是 verdict 分布 = **pass 37 / partial 6 / fail 34 / skip 40**，四态互斥，每款必落其一（audit.go 的 verdict 判定：events>0 → pass；scan_error → fail；可解包但 scan_error → partial；main 无分派 → skip）。**不是 A 档**。A/B/C 是另一个维度：把「未通过的 80 款（partial 6 + fail 34 + skip 40）」按修复路径分成 **A 40 / B 9 / C 31**，A+B+C=80。两个维度不要混加：

```
verdict 维度： pass 37 │ partial 6 + fail 34 + skip 40 = 未通过 80
                                              ↓ 按修复路径切
修复维度：                        A 40（现成工具可推）+ B 9（待补适配器）+ C 31（真卡点）= 80
```

正确算式是两条：`37+6+34+40=117`（verdict）与 `40+9+31=80`（A/B/C）。不存在「117=A+B+C」的算式。

### 2. A 档 42→40 的 −2 来源

复扫救回 2 款 fail→pass（呪いの鎧 v3.0.1、詛咒铠甲2，见 §二对照表），未通过总数 82→80。这 2 款原本在 A 档（A 42 = fail 36 + partial 6，其中 rmmz 子项 4 款），救回后已 pass、不再需要修复，故 A 档 42−2=**40**（rmmz 子项 4→2，剩 不谙( 与 wuxian）。B/C 不变（9/31）。核对：**40+9+31=80 = partial 6 + fail 34 + skip 40** ✅。

### 3. 「23 款」口径

`reports/audit/batch-rmmz-recheck.json` 是 `-group 夯,未评级` 的**全量重扫**（63 款逐款判定，不是只扫 4 款）。其中 engine_resolved=rmmz 共 **23 款：pass 21 / fail 2**（fail=不谙(、wuxian）。§二的「4 款逐款对照」是从这 23 款里挑出的重点对象（原总报告 §四 rmmz 类 4 款 fail），23 款是全集、4 款是子集，两个数字不矛盾。

### 4. 救回 2 款的证据摘要（batch-rmmz-recheck.json 原始字段）

- **呪いの鎧 v3.0.1（夯）**：`"engine_resolved": "rmmz"`, `"events": 222`, `"errors": []`, `"verdict": "pass"`, `"reason": "events>0"`
- **詛咒铠甲2（夯）**：`"engine_resolved": "rmmz"`, `"events": 267`, `"errors": []`, `"verdict": "pass"`, `"reason": "events>0"`

复扫前两者均为 fail / events=0（错误模板：`没有识别到图片演出（扫了 www/data … 警告 849/1231 条）`）。修复点 = commit `489f126`（`internal/adapter/rmmz/rmmz.go`）：新增 `scriptCGName`（识别 code 355/655 脚本指令里的 `cgLoad('组名')`，正则 `scriptCGRe = cgLoad\(\s*['"]([^'"]+)['"]`）与 `locateCGSet`（按 `pictures/<组名>/` 子目录定位差分套图），`addSegments` 里脚本 CG 与指令 231 同权重计数。两款是 Wolfzq 同插件不同作品，被同一修复救回。

### 5. git status 澄清（实测输出）

- `git fetch origin` 后 `git status --short`：本地 main 与 origin/main 同指 `fb58fb3`，无 ahead/behind。
- 当前 `git status --short` 显示 63 个 `M`（已跟踪、已修改）文件：`summary.json` × 1 + `夯_01..19.json` × 19 + `未评级_01..44.json` × 44 —— 这些是 batch-rmmz-recheck 复扫**覆盖重写了同名的已提交逐款 JSON**（`-out reports/audit` 按分组序号覆写），属于复扫产物的正常更新，已在 fb58fb3 提交中包含旧版本，本次将提交新版本。
- `git status --porcelain --untracked-files=all`：**无任何 `??` 未跟踪条目**，没有未跟踪旧批次 JSON。结论：工作区不「干净」的部分只有上述 63 个已跟踪修改文件，无遗留未跟踪文件。

## 七、全量回归（复核方法学要求，已采纳执行）

命令：`go run ./cmd/library-audit -batch batch-rmmz-regression -out reports/audit`（不带 -group，**全库 117 款**，非抽样）。stdout 全文存 `reports/audit/batch-rmmz-regression.stdout.txt`，逐款存 `batch-rmmz-regression.json`。用时 557.4s。

stdout 尾部汇总（原文）：

```
[汇总] 总数=117 pass=37 partial=6 fail=34 skip=40 用时=557.4s
  引擎 rmmz(guess): 总数=39 pass=37 partial=0 fail=2 skip=0
```

- **rmmz(guess) 覆盖 39 款 = 全库 rmmz 全集**（39/39 = 100% 覆盖，远超复核要求的 30%），其中 **pass 37 / fail 2**（不谙( 缺 data 目录、wuxian 非 Wolfzq 约定图片演出，与复扫结论一致）。
- 全部 37 款 pass（verdict | 组 | 款名 | events）：
  pass|大雷|女主角|44；pass|夯|(夯)魔塔|31；pass|夯|（夯）直到这个魅魔的心灵崩坏|50；pass|夯|苍红界域|209；pass|夯|希尔丝大冒险 1.06|101；pass|夯|呪いの鎧 v3.0.1|222；pass|夯|詛咒铠甲2|267；pass|夯|ayuureiCN|132；pass|上等|（上等）哈尔法斯的恶魔|68；pass|上等|（上等）NTRエロトラップダンジョン|97；pass|上等|气息|348；pass|不赖|忍耐色诱的告白游戏 V2.0|478；pass|创意|人柱阿拉卡 v230216|86；pass|未评级|(牛)RJ01618221|203；pass|未评级|(牛)RJ01680223|1025；pass|未评级|（牛）间谍任务|716；pass|未评级|对魔战士美月(|129；pass|未评级|妹神官（牛）|211；pass|未评级|魔法学園見聞録|523；pass|未评级|女大学生 DX(8)|70；pass|未评级|星天之魔女（Arpg）|238；pass|未评级|Chronicles of Clams Island|161；pass|未评级|RA26037409(|198；pass|未评级|Restore(|338；pass|未评级|Risty and the Village of Bocchino v1.0.0|25；pass|未评级|Scars of Summer After v1.01（牛）|1350；pass|未评级|Unholy_maiden_Ver.1.0.10|843；pass|一般|（一般）冒険者リザシア|137；pass|一般|（一般）命に代えてもお守りします!|187；pass|一般|（一般）Shitsuke-ai_dlsite|47；pass|一般|裏垢JKは露出でバズりたい|25；pass|一般|栩的人生已经没救了|95；pass|中等|(中等)魔法是无所不能|12；pass|中等|(中等)The Forbidden Tomes of Olipos|336；pass|中等|（中等）NTR Hunter|216；pass|中等|时间勇者|125；pass|中上|魔法少女木叶 v2.04|22。
- **回归比对**：修复前 pass=35、修复后 pass=37；逐款比对全量回归结果与 batch-1..7 + recheck 的基线，**除救回的 2 款（呪いの鎧 v3.0.1、詛咒铠甲2）外，没有任何原本 pass 的 rmmz 款变 fail/partial/skip——回归 0 款**。全库 verdict 分布与总报告完全一致：**全量回归确认 37/34/40/6 不变**（pass 37 / fail 34 / skip 40 / partial 6），总报告与 docs/15 数字无需修改。
