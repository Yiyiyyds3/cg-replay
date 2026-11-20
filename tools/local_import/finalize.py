#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# Generate failed.md and SUMMARY.md from import_results.json + selfcheck.json.
import json, os, time

BASE=os.path.dirname(os.path.abspath(__file__))
REPO=os.path.abspath(os.path.join(BASE,"..",".."))
D=os.path.join(REPO,"reports","local_import")
DATA=os.path.join(D,"data")
imp=json.load(open(os.path.join(DATA,"import_results.json"),encoding="utf-8"))
sc_path=os.path.join(DATA,"selfcheck.json")
sc=json.load(open(sc_path,encoding="utf-8")) if os.path.isfile(sc_path) else None
now=time.strftime("%Y-%m-%d %H:%M:%S")

def tbl(rows,head):
    s="| "+" | ".join(head)+" |\n|"+"|".join(["---"]*len(head))+"|\n"
    for r in rows: s+="| "+" | ".join(str(x) for x in r)+" |\n"
    return s

fail=[r for r in imp["results"] if r["status"]=="import_fail"]
tmo=[r for r in imp["results"] if r["status"]=="import_timeout"]
ok=[r for r in imp["results"] if r["status"]=="import_ok"]

# failed.md
F=["# failed · 试导入失败/超时清单（"+now+"）",""]
F+=["- keep 组：%d；import_ok：%d；import_fail：%d；import_timeout：%d"%(imp["total"],len(ok),len(fail),len(tmo)),""]
F+=["判据：exit!=0 或 events.json 为空 = import_fail；超过 300s = import_timeout。stderr 末 20 行见下。",""]
F+=["## import_fail",""]
if fail:
    F+=["| 游戏 | 耗时 | 事件 | stderr 末 20 行（折叠为末 3 行摘要） |","|---|---:|---:|---|"]
    for r in fail:
        tail=" / ".join([x.strip() for x in r.get("stderr","").splitlines() if x.strip()][-3:])[:200]
        F.append("| %s | %.1fs | %d | %s |"%(r["name"][:32],r.get("elapsed_s",0),r.get("events",0),tail or "-"))
else:
    F+=["（无）"]
F+=["","## import_timeout",""]
if tmo:
    F+=["| 游戏 | 耗时 | stderr 末 3 行 |","|---|---:|---|"]
    for r in tmo:
        tail=" / ".join([x.strip() for x in r.get("stderr","").splitlines() if x.strip()][-3:])[:200]
        F.append("| %s | %.1fs | %s |"%(r["name"][:32],r.get("elapsed_s",0),tail or "-"))
else:
    F+=["（无）"]
open(os.path.join(D,"failed.md"),"w",encoding="utf-8").write("\n".join(F)+"\n")

# SUMMARY.md
suspect_n=sc["suspect"] if sc else 0
pass_n=sc["pass"] if sc else 0
S=["# 本地游戏批量导入 · 总报告（"+now+"）",""]
S+=["## 流水线结果",""]
S+=["| 阶段 | 数量 | 产物 |","|---|---:|---|"]
S+=["| 候选游戏盘点 | %d | 01_inventory.md |"%imp.get("candidates",58)]
S+=["| 去重后 keep | %d | 02_dedup.md |"%imp["total"]]
S+=["| 内容重复 internal | 0 | 02_dedup.md（排除运行时噪声与 junction 后 0 组） |"]
S+=["| 已入库重复跳过 | 1 | 02_dedup.md |"]
S+=["| 试导入 import_ok | %d | import_results.json |"%len(ok)]
S+=["| 试导入 import_fail | %d | failed.md |"%len(fail)]
S+=["| 试导入 import_timeout | %d | failed.md |"%len(tmo)]
S+=["| 有效性自检 suspect | %d | suspect.md（不进 pass） |"%suspect_n]
S+=["| 有效性自检 pass | %d | suspect.md |"%pass_n,""]
S+=["## 红线遵守",""]
S+=["- 未执行 `archify deliver` / `archify finalize`","- 全程只读用户原始游戏目录，未删改任何游戏文件","- 回滚快照：`snapshot/state.txt` + catalog.db/games.json/cgreplay.conf 备份","- 有效性自检复用 `tools/review-prescreen`，未重写空白判定逻辑",""]
S+=["## 说明",""]
S+=["- 导入入口：`bin/ucgr.exe events <目录> --out <events.json>`（项目主入口）；包装脚本 `tools/local_import/import_batch.py`。","- 去重口径：真资源包用主包 SHA256；rmmz/NW.js 目录型用目录内容签名（相对路径+大小排序 SHA256）。","- suspect 判据任一命中即降级：0 图 / 空白>50%% / 尺寸全一致且<=128x128 / 总<100KB。",""]
open(os.path.join(D,"SUMMARY.md"),"w",encoding="utf-8").write("\n".join(S)+"\n")
print("WROTE failed.md SUMMARY.md; ok=%d fail=%d tmo=%d suspect=%d pass=%d"%(len(ok),len(fail),len(tmo),suspect_n,pass_n))