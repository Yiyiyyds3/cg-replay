#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""批量导入包装：对 dedup.json 的 keep 组逐款调用项目自带入口 ucgr events。
只读游戏目录；产物写到仓库外 D:/UCGR_Work/local_import/<safe_name>/events.json。
单款超时 300s；超时/失败不中断，记入结果 JSON。禁止 archify deliver/finalize。
"""
import json, os, sys, subprocess, time, re

BASE=os.path.dirname(os.path.abspath(__file__))
REPO=os.path.abspath(os.path.join(BASE,"..",".."))
DEDUP=os.path.join(REPO,"reports","local_import","data","dedup.json")
RES=os.path.join(REPO,"reports","local_import","data","import_results.json")
UCGR=os.path.join(REPO,"bin","ucgr.exe")
OUTROOT=r"D:\UCGR_Work\local_import"
TIMEOUT=300

def safe_name(s):
    s=re.sub(r'[<>:"\\|?*\x00-\x1f]','_',s).strip().rstrip('. ')
    return (s or "unnamed")[:120]

def events_count(stdout):
    m=re.search(r'CG 事件\s*(\d+)\s*个',stdout)
    sc=re.search(r'场景事件\s*(\d+)\s*个',stdout)
    return (int(m.group(1)) if m else 0)+(int(sc.group(1)) if sc else 0)

def main():
    only=set(os.environ.get("UCGR_IMPORT_ONLY","").split(",")) - {""}
    limit=int(os.environ.get("UCGR_IMPORT_LIMIT","0"))
    d=json.load(open(DEDUP,encoding="utf-8"))
    keep=[g for g in d["games"] if g["class"]=="keep"]
    if only: keep=[g for g in keep if g["name"] in only]
    if limit>0: keep=keep[:limit]
    os.makedirs(OUTROOT,exist_ok=True)
    results=[]
    for i,g in enumerate(keep):
        name=g["name"]; root=g["path"]; od=os.path.join(OUTROOT,safe_name(name))
        os.makedirs(od,exist_ok=True)
        ev=os.path.join(od,"events.json")
        cmd=[UCGR,"events",root,"--out",ev]
        rec={"name":name,"path":root,"pkg_kind":g["pkg_kind"],
             "prior_verdict":g.get("prior_verdict",""),"out":ev,
             "cmd":" ".join([UCGR,"events","<game>","--out",ev])}
        t=time.time()
        try:
            p=subprocess.run(cmd,capture_output=True,text=True,timeout=TIMEOUT,encoding="utf-8",errors="replace")
            rec["elapsed_s"]=round(time.time()-t,1)
            rec["returncode"]=p.returncode
            rec["stdout"]=p.stdout[-4000:]
            rec["stderr"]=p.stderr[-4000:]
            rec["events"]=events_count(p.stdout)
            if p.returncode==0 and os.path.isfile(ev) and os.path.getsize(ev)>0:
                rec["status"]="import_ok"
            else:
                rec["status"]="import_fail"
        except subprocess.TimeoutExpired as e:
            rec["elapsed_s"]=round(time.time()-t,1)
            rec["status"]="import_timeout"
            rec["returncode"]=-999
            so=e.stdout or ""; se=e.stderr or ""
            rec["stdout"]=so.decode("utf-8","replace")[-2000:] if isinstance(so,bytes) else so[-2000:]
            rec["stderr"]=se.decode("utf-8","replace")[-2000:] if isinstance(se,bytes) else se[-2000:]
            rec["events"]=0
        results.append(rec)
        print("[%d/%d] %-40s %-14s ev=%-5d %.1fs"%(i+1,len(keep),name[:40],rec["status"],rec.get("events",0),rec["elapsed_s"]),flush=True)
    os.makedirs(os.path.dirname(RES),exist_ok=True)
    json.dump({"generated_at":time.strftime("%Y-%m-%d %H:%M:%S"),"total":len(results),
               "ok":sum(1 for r in results if r["status"]=="import_ok"),
               "fail":sum(1 for r in results if r["status"]=="import_fail"),
               "timeout":sum(1 for r in results if r["status"]=="import_timeout"),
               "results":results},open(RES,"w",encoding="utf-8"),ensure_ascii=False,indent=2)
    print("DONE ok=%d fail=%d timeout=%d"%(sum(1 for r in results if r["status"]=="import_ok"),
          sum(1 for r in results if r["status"]=="import_fail"),
          sum(1 for r in results if r["status"]=="import_timeout")))
if __name__=="__main__": main()
