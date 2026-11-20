#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# Validity self-check: reuse tools/review-prescreen metrics (no reimplementation of blank detection).
import json, os, subprocess, sys, time, re

BASE=os.path.dirname(os.path.abspath(__file__))
REPO=os.path.abspath(os.path.join(BASE,"..",".."))
D=os.path.join(REPO,"reports","local_import")
DATA=os.path.join(D,"data")
RES=os.path.join(DATA,"import_results.json")
PRE=os.path.join(REPO,"bin","review-prescreen.exe")
MAN=os.path.join(DATA,"_prescreen_manifest.json")
OUTDIR=os.path.join(DATA,"prescreen")
SUSPECT=os.path.join(D,"suspect.md")

def dims_le_128(mode_wh):
    if not mode_wh: return False
    m=re.match(r"(\d+)\s*[xX]\s*(\d+)",mode_wh)
    if not m: return False
    return int(m.group(1))<=128 and int(m.group(2))<=128

def main():
    imp=json.load(open(RES,encoding="utf-8"))
    ok=[r for r in imp["results"] if r["status"]=="import_ok"]
    man=[]
    for i,r in enumerate(ok,1):
        ev=r.get("out","")
        engine=""
        try:
            engine=json.load(open(ev,encoding="utf-8")).get("engine_id","")
        except Exception:
            pass
        man.append({"id":i,"name":r["name"],"engine":engine,"cg_count":0,
                    "source_dir":"","source_real_path":r["path"],"source_dir_exists":True})
    json.dump(man,open(MAN,"w",encoding="utf-8"),ensure_ascii=False,indent=2)
    print("MANIFEST games=%d"%len(man))
    if not man:
        open(SUSPECT,"w",encoding="utf-8").write("# suspect\n\n(no import_ok games)\n")
        return
    os.makedirs(OUTDIR,exist_ok=True)
    cmd=[PRE,"-from-manifest",MAN,"-out",OUTDIR,"-quiet"]
    p=subprocess.run(cmd,capture_output=True,text=True,encoding="utf-8",errors="replace",timeout=1200)
    print("PRESCREEN exit=%d"%p.returncode)
    if p.returncode!=0: print("STDERR:",p.stderr[-2000:])
    cands=[f for f in os.listdir(OUTDIR) if f.startswith("预筛指标") and f.endswith(".json")]
    if not cands: raise SystemExit("prescreen produced no metrics json")
    rep=json.load(open(os.path.join(OUTDIR,cands[0]),encoding="utf-8"))
    met={g["name"]:g for g in rep["games"]}
    suspects=[]; verdicts=[]
    for r in ok:
        g=met.get(r["name"],{})
        images=g.get("images",0) or 0
        blank=g.get("blank",0) or 0
        decoded=g.get("decoded",0) or 0
        avgkb=g.get("avg_kb",0.0) or 0.0
        mode_wh=g.get("mode_wh","") or ""
        mode_count=g.get("mode_count",0) or 0
        encrypted=g.get("encrypted",0) or 0
        total_kb=avgkb*images
        reasons=[]
        if images==0: reasons.append("导出图数=0" + ("（含%d张加密图，机器不可读像素）"%encrypted if encrypted else ""))
        if decoded>0 and (blank/decoded)>0.5: reasons.append("空白图占比>50%% (%d/%d)"%(blank,decoded))
        if images>0 and mode_count==images and dims_le_128(mode_wh): reasons.append("所有图尺寸一致且<=128x128 (%s)"%mode_wh)
        if images>0 and total_kb<100: reasons.append("总导出体积<100KB (%.1fKB)"%total_kb)
        rec={"name":r["name"],"images":images,"blank":blank,"decoded":decoded,
             "avg_kb":round(avgkb,1),"total_kb":round(total_kb,1),"mode_wh":mode_wh,
             "mode_count":mode_count,"encrypted":encrypted,"severity":g.get("severity",0),
             "events":r.get("events",0),"reasons":reasons,
             "verdict":"suspect" if reasons else "pass"}
        verdicts.append(rec)
        if reasons: suspects.append(rec)
    L=["# suspect · 有效性自检降级清单（需人工翻图）",""]
    L+=["- 判据（任一命中即 suspect，不进主报告 pass 数）：",
        "  1. 导出图数 = 0",
        "  2. 空白图占比 > 50%%（复用 tools/review-prescreen 空白判定：32x32 网格灰度方差 < 25）",
        "  3. 所有图尺寸完全一致且 ≤ 128×128（图标/占位图特征）",
        "  4. 总导出体积 < 100KB",""]
    L+=["- 自检来源：tools/review-prescreen 源目录模式（未重写判定逻辑）",
        "- import_ok 款数：%d；suspect：%d；pass：%d"%(len(verdicts),len(suspects),len(verdicts)-len(suspects)),"",
        "## suspect 清单","",
        "| 游戏 | 图数 | 空白/抽样 | 均KB | 总KB | 众数尺寸 | 事件 | 命中判据 |",
        "|---|---:|---:|---:|---:|---|---:|---|"]
    for s in suspects:
        L.append("| %s | %d | %d/%d | %.1f | %.1f | %s | %d | %s |"%(
            s["name"][:34],s["images"],s["blank"],s["decoded"],s["avg_kb"],s["total_kb"],
            s["mode_wh"] or "-",s["events"],"; ".join(s["reasons"])))
    L+=["","## 全部 import_ok 判定","",
        "| 游戏 | 图数 | 加密图 | 空白/抽样 | 总KB | 众数尺寸 | sev | 判定 |",
        "|---|---:|---:|---:|---:|---|---:|---|"]
    for v in verdicts:
        L.append("| %s | %d | %d | %d/%d | %.1f | %s | %d | %s |"%(
            v["name"][:34],v["images"],v["encrypted"],v["blank"],v["decoded"],v["total_kb"],
            v["mode_wh"] or "-",v["severity"],v["verdict"]))
    open(SUSPECT,"w",encoding="utf-8").write("\n".join(L)+"\n")
    json.dump({"generated_at":time.strftime("%Y-%m-%d %H:%M:%S"),
               "import_ok":len(verdicts),"suspect":len(suspects),"pass":len(verdicts)-len(suspects),
               "verdicts":verdicts},open(os.path.join(DATA,"selfcheck.json"),"w",encoding="utf-8"),
              ensure_ascii=False,indent=2)
    print("DONE ok=%d suspect=%d pass=%d"%(len(verdicts),len(suspects),len(verdicts)-len(suspects)))
if __name__=="__main__": main()