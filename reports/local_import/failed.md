# failed · 试导入失败清单（v2，2026-11-21 01:36:05）

- keep 组：57；import_ok：52；import_fail：5；timeout 0

## 剩余失败（含引擎定性）

| 游戏 | 定性 | 本轮结论 |
|---|---|---|
| 光辉的少女们 | TyranoBuilder 私有容器（SRTS/SDTS 魔数），需容器级逆向，本轮未解决 | 需容器级逆向/插件解析，列后续工单 |
| wuxian | 错误: 图片由插件/脚本驱动：Show Picture 命令仅 2 条、检测到脚本函数 $utils.shakeScre | 需容器级逆向/插件解析，列后续工单 |
| （一般）Touch Simulation Game | Electron portable 单文件（asar 内嵌），resources 目录空，需 exe 级提取，本轮未解决 | 需容器级逆向/插件解析，列后续工单 |
| （一般）女王游戏X - 和妖精一起玩卡牌 クイーンズX - 妖精さん | Electron portable 单文件，本轮未解决（卡牌游戏可能无 CG 演出） | 需容器级逆向/插件解析，列后续工单 |
| HOME | Electron portable + Tyrano（sav 名含 tyrano_data），本轮未解决 | 需容器级逆向/插件解析，列后续工单 |

## 本轮已从 fail 清除的款

| 游戏 | 新状态 | events | 方式 |
|---|---|---:|---|
| 共妻之村 (牛) | import_ok | 38 | A线解包 data.xp3 -> D:\UCGR_Work\unpack\共妻之村，38 场景事件 |
| SOTDQ_0.3.5.3v_WINDOWS（很牢） | import_ok | 461 | Godot4.3 pck 导出 461 CG（274 张压缩纹理未切）-> D:\UCGR_Work\godot\SOTDQ |
| 无法诉说的关系(中等) | import_ok | 7 | A线解包 -> 7 场景事件 |
| Phoenixes-v17.0.1 | import_ok | 2596 | Godot3.6 pck 1700.pck 全量 2596 CG 413MB -> D:\UCGR_Work\godot\Phoenixes |
| (牛)完堕ちX兄嫁 ～アンタのせいよ！ 私、この浮気チ●ポじゃなきゃ | import_ok | 70 | A线解包 scenario/append xp3 -> 70 场景+70 演出 |
| （一般）姉妹X催眠 ～絶対服従、俺がずっと王様～ | import_ok | 11 | A线解包 -> 11 事件（部分 CG 加密未解） |
| （一般）姉妹X催眠2 ～絶対服従、セレブ妻と日焼けJKが堕ちる性感マ | import_ok | 14 | A线解包 -> 14 场景事件 |
| 绝对合格 | import_ok | 28 | D线ScanRmmz Script图片提取(1121名)修好，28场景事件 |
