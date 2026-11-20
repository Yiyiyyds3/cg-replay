# 00 · 导入入口定位（2026-11-21 00:52 +08:00）

## 找到的项目自带入口

| 入口 | 类型 | 命令 | 说明 |
|---|---|---|---|
| 事件扫描/导入 | Go CLI | `bin/ucgr.exe events <游戏目录> [--out 事件库.json]` | 项目主入口。只读扫描游戏目录，产出原作自己的事件表（CG+场景），`--out` 落盘。**本次采用**。见 `cmd/ucgr/main.go:cmdEvents` |
| Unity CG 批量导出 | Python | `python tools/unity_cg_export.py --game <目录> --out <目录> --auto` | Unity/Addressables 的 `--auto` 批量导出器（catalog/bundle 自动发现），产出 `cg-index.json`+`cg/` |
| 桌面库登记 | Wails GUI | `App.AddGame(root)`（`desktop/app.go:394`） | 把目录登记进 `%LOCALAPPDATA%/cgreplay-ui`；GUI 绑定，不便脚本化 |
| 体积盘点 | Go CLI | `bin/ucgr.exe scan <目录>` | S0 盘点，辅助 |

## 实际采用的包装脚本

- `tools/local_import/import_batch.py`：对 keep 组逐款调用 `bin/ucgr.exe events <目录> --out D:\UCGR_Work\local_import\<名>\events.json`；单款超时 300s（`subprocess.run(timeout=300)`），超时标 `import_timeout` 继续；三态 import_ok / import_fail / import_timeout；只读游戏目录，产物在仓库外。
- `tools/local_import/scan_inventory.py`：盘点候选游戏。
- `tools/local_import/dedup.py`：SHA256 内容去重。
- 有效性自检：复用 `tools/review-prescreen`（从 agent/p0-untracked-import 引入源码编译运行），不重写空白判定。

## 红线

- 禁止 `archify deliver`/`archify finalize`（E: 盘必挂）——本流程**未执行**。
- 只做导入（events 扫描）与 render/预筛校验，不删改任何用户原始游戏文件。
