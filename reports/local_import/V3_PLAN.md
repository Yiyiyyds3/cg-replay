# V3 验收口径（2026-11-22 凌晨开工，08:00 交卷）

## 验收硬指标（主人原话转译）
1. egame 全部游戏可支持（import_ok 54 款 + fail 3 款硬壳如实在案；人工测试主人来做）
2. 文本、音频必须导入好
3. CG 库不再出现「无有效帧的 CG 事件」；只保留有效 CG，数量必须与原游戏 CG 对得上
4. CG 口径 = 特殊 CG（事件/ gallery CG），不含常规立绘/背景/map 事件
5. 项目做减法，不引入繁杂功能；审计与真实测试到位

## 实现切面（做减法）
- internal/library/validate.go：Library 后处理过滤（图帧存在性 + 特殊事件特征），cmd events --validate 输出 <out>.validate.json
- cmd/ucgr/textdump.go：每游戏剧本文本导出 txt + _textdump.json（kirikiri/rmmz 优先）
- tools/local_import/reconcile_v3.py + run_all_v3.ps1：全库批跑 + 对账表 acceptance_v3.{json,md}
- GUI 无需改：消费过滤后的 events.json 自动只显示有效特殊 CG

## 有效帧判定（validate 规则）
- CGEvent：Pages 非空且 ≥1 帧命中解包目录真实图片文件（相对路径→basename 兜底）
- SceneEvent：特殊特征正则（ev/hscene/recall/gallery/cgrai…）；rmmz map: 事件未命中特征即剔除
- Chapters：指向被删场景的连带删除

## 对账口径
- cg_kept（有效特殊 CG 事件数）↔ kept 事件引用 distinct 图片数 ↔ 解包目录源图片数
- audio_count ↔ 解包目录音频文件数
- text_lines ↔ textdump 总行数

## 时间表（00:41 起）
- 00:41-01:10 盘点+规则+发车 ✓
- 01:10-02:30 三线编码（validate/textdump/工具链）
- 02:30-03:10 代码审查 + go build/test + 修缝
- 03:10-05:30 全库批跑（54 游戏 events+textdump）
- 05:30-06:40 对账表 + 审计（selfcheck 三态口径复核）+ 修复
- 06:40-07:30 wails 重建 GUI + bin 同步 + 抽样真实回放
- 07:30-08:00 验收报告 + 中文提交

## 安全红线
- E:\Personal_entertainment\ 只读
- 写入仅限：本仓库 worktree、D:\UCGR_Work\、reports/
- 不跑 archify deliver/finalize；不删主人源文件
