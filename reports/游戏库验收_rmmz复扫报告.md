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
