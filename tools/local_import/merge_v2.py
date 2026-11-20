#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""把 A 线解包重导与 Godot 导出结果并入导入台账（v2）。"""
import json, os, time
BASE=os.path.dirname(os.path.abspath(__file__))
REPO=os.path.abspath(os.path.join(BASE,"..",".."))
DATA=os.path.join(REPO,"reports","local_import","data")
imp=json.load(open(os.path.join(DATA,"import_results.json"),encoding="utf-8"))
by_name={r["name"]:r for r in imp["results"]}

def mark(name, status, events, note, out=""):
    r=by_name.get(name)
    if not r: return
    r["status"]=status
    r["events"]=events
    r["v2_note"]=note
    r["out"]=out
    r["stderr"]=""

additions=[
 # A 线：Kirikiri 解包重导（产物在 D:\UCGR_Work\unpack\<名>）
 ("共妻之村 (牛)","import_ok",38,"A线解包 data.xp3 -> D:\\UCGR_Work\\unpack\\共妻之村，38 场景事件"),
 ("无法诉说的关系(中等)","import_ok",7,"A线解包 -> 7 场景事件"),
 ("(牛)完堕ちX兄嫁 ～アンタのせいよ！ 私、この浮気チ●ポじゃなきゃダメなのっ！～","import_ok",70,"A线解包 scenario/append xp3 -> 70 场景+70 演出"),
 ("（一般）姉妹X催眠 ～絶対服従、俺がずっと王様～","import_ok",11,"A线解包 -> 11 事件（部分 CG 加密未解）"),
 ("（一般）姉妹X催眠2 ～絶対服従、セレブ妻と日焼けJKが堕ちる性感マッサージ～","import_ok",14,"A线解包 -> 14 场景事件"),
 # Godot 线
 ("SOTDQ_0.3.5.3v_WINDOWS（很牢）","import_ok",461,"Godot4.3 pck 导出 461 CG（274 张压缩纹理未切）-> D:\\UCGR_Work\\godot\\SOTDQ"),
 ("Phoenixes-v17.0.1","import_ok",2596,"Godot3.6 pck 1700.pck 全量 2596 CG 413MB -> D:\\UCGR_Work\\godot\\Phoenixes"),
]
for name,st,ev,note in additions:
    mark(name,st,ev,note)

# 未解决 4 款标注引擎定性
notes={
 "光辉的少女们":"TyranoBuilder 私有容器（SRTS/SDTS 魔数），需容器级逆向，本轮未解决",
 "（一般）Touch Simulation Game":"Electron portable 单文件（asar 内嵌），resources 目录空，需 exe 级提取，本轮未解决",
 "（一般）女王游戏X - 和妖精一起玩卡牌 クイーンズX - 妖精":"Electron portable 单文件，本轮未解决（卡牌游戏可能无 CG 演出）",
 "HOME":"Electron portable + Tyrano（sav 名含 tyrano_data），本轮未解决",
}
for name,note in notes.items():
    for k,r in by_name.items():
        if k.startswith(name[:12]):
            r["v2_note"]=note

imp["v2_updated_at"]=time.strftime("%Y-%m-%d %H:%M:%S")
imp["v2_ok"]=sum(1 for r in imp["results"] if r["status"]=="import_ok")
imp["v2_fail"]=sum(1 for r in imp["results"] if r["status"]=="import_fail")
json.dump(imp,open(os.path.join(DATA,"import_results_v2.json"),"w",encoding="utf-8"),ensure_ascii=False,indent=2)
print("V2 ok=%d fail=%d total=%d"%(imp["v2_ok"],imp["v2_fail"],imp["total"]))
for r in imp["results"]:
    if r["status"]=="import_fail":
        print("  STILL FAIL: %s | %s"%(r["name"][:30], r.get("v2_note","")[:60]))
