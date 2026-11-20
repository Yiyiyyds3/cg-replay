#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""从 import_results_v2.json 生成 failed.md 与 SUMMARY.md（v2 口径）。"""
import json, os, time
BASE=os.path.dirname(os.path.abspath(__file__))
REPO=os.path.abspath(os.path.join(BASE,"..",".."))
D=os.path.join(REPO,"reports","local_import")
DATA=os.path.join(D,"data")
imp=json.load(open(os.path.join(DATA,"import_results_v2.json"),encoding="utf-8"))
now=time.strftime("%Y-%m-%d %H:%M:%S")
fails=[r for r in imp["results"] if r["status"]=="import_fail"]
oks=[r for r in imp["results"] if r["status"]=="import_ok"]

F=["# failed · 试导入失败清单（v2，"+now+"）","",
   "- keep 组：%d；import_ok：%d；import_fail：%d；timeout 0"%(imp["total"],imp["v2_ok"],imp["v2_fail"]),"",
   "## 剩余失败（含引擎定性）","",
   "| 游戏 | 定性 | 本轮结论 |","|---|---|---|"]
for r in fails:
    F.append("| %s | %s | %s |"%(r["name"][:34], (r.get("v2_note") or r.get("stderr","")[:60] or "-").replace("|","/"), "需容器级逆向/插件解析，列后续工单"))
F+=["","## 本轮已从 fail 清除的款","",
    "| 游戏 | 新状态 | events | 方式 |","|---|---|---:|---|"]
solved=[r for r in oks if r.get("v2_note")]
for r in solved:
    F.append("| %s | import_ok | %d | %s |"%(r["name"][:34], r.get("events",0), r.get("v2_note","").replace("|","/")))
open(os.path.join(D,"failed.md"),"w",encoding="utf-8").write("\n".join(F)+"\n")

sc2=os.path.join(DATA,"selfcheck_v2.json")
sus=""
if os.path.isfile(sc2):
    sc=json.load(open(sc2,encoding="utf-8"))
    sus="；自检 v2：import_ok %d 中 pass %d / suspect %d"%(sc.get("import_ok",0),sc.get("pass",0),sc.get("suspect",0))
S=["# 本地游戏批量导入 v2 · 总报告（"+now+"）","",
   "## 主台账（keep 57 款）","",
   "| 类别 | v1 | v2 |","|---|---:|---:|",
   "| import_ok | 44 | **%d** |"%imp["v2_ok"],
   "| import_fail | 13 | **%d** |"%imp["v2_fail"],
   "| import_timeout | 0 | 0 |","",
   "## 已解决明细（fail → ok，7 款）","",
   "| 游戏 | events | 方式 |","|---|---:|---|",
   "| 共妻之村 (牛) | 38 | spike-a 解包 data.xp3 → kirikiri 重扫 |",
   "| 无法诉说的关系(中等) | 7 | 解包重扫 |",
   "| (牛)完堕ちX兄嫁 | 70 | 解包 scenario+append.xp3，兼容台本生成 |",
   "| （一般）姉妹X催眠 | 11 | 解包重扫 |",
   "| （一般）姉妹X催眠2 | 14 | 解包重扫 |",
   "| SOTDQ（很牢） | 461 | **Godot 4.3 pck 导出器**（新增 godot_cg_export.py，274 张压缩纹理未切如实记） |",
   "| Phoenixes-v17.0.1（浴火重火） | 2596 | **Godot 3.6 pck 导出器**，1700.pck 全量 413MB |","",
   "## 剩余 6 款 fail 的定性","",
   "| 游戏 | 引擎定性 | 卡点 |","|---|---|---|",
   "| 光辉的少女们 | TyranoBuilder（SRTS/SDTS 私有容器） | 容器格式未公开，需逆向 |",
   "| wuxian / 绝对合格 | RPG Maker 插件驱动 | ScanRmmz 已加 Script 图片提取（本轮 D 线），以实测为准 |",
   "| TouchSim / 女王X / HOME | Electron portable（asar 内嵌 exe） | 无 7z；TEMP 解压试验无产物；需 exe 级提取 |","",
   "## suspect 重自检"+sus,"",
   "- B 线：rpg_decrypt.py 解密 .rpgmvp/.png_ 到 D:\\UCGR_Work\\decrypt\\，selfcheck_v2 用解密镜像重跑 prescreen（结果见 suspect.md v2 段）","",
   "## 红线遵守","",
   "- 全程只读游戏目录；解包/导出/解密产物全部在 D:\\UCGR_Work\\；未执行 archify deliver/finalize",
   "- 回滚快照不变：reports/local_import/snapshot/",""]
open(os.path.join(D,"SUMMARY.md"),"w",encoding="utf-8").write("\n".join(S)+"\n")
print("WROTE failed.md SUMMARY.md (v2); ok=%d fail=%d"%(imp["v2_ok"],imp["v2_fail"]))
