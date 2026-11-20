#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""内容哈希去重 v2：真资源包用主包 SHA256；目录型游戏(rmmz/NW.js)用目录内容签名。"""
import json, os, sys, glob, hashlib, time

BASE = os.path.dirname(os.path.abspath(__file__))
REPO = os.path.abspath(os.path.join(BASE, "..", ".."))
INV  = os.path.join(REPO, "reports", "local_import", "data", "inventory.json")
OUT  = os.path.join(REPO, "reports", "local_import", "data", "dedup.json")
REAL_ARC_EXTS = {".xp3", ".arc", ".pac", ".pck", ".noa"}
NOISE = {"natives_blob.bin","v8_context_snapshot.bin","snapshot_blob.bin","icudtl.dat",
         "resources.pak","nw_100_percent.pak","nw_200_percent.pak","libegl.dll",
         "libglesv2.dll","ffmpeg.dll","node.dll"}

def sha256_file(path, buf=1<<20):
    h=hashlib.sha256()
    with open(path,"rb") as f:
        while True:
            b=f.read(buf)
            if not b: break
            h.update(b)
    return h.hexdigest()

def dir_signature(game_dir, max_files=400):
    """目录内容指纹：相对路径+大小，排序后哈希。排除运行时噪声。用于 rmmz/NW.js 目录型游戏。"""
    h=hashlib.sha256(); n=0
    for root, dirs, files in os.walk(game_dir):
        dirs[:]=[d for d in dirs if not d.startswith('.') and d.lower() not in ('node_modules','.git')]
        for f in sorted(files):
            if f.lower() in NOISE: continue
            p=os.path.join(root,f)
            try: sz=os.path.getsize(p)
            except Exception: sz=-1
            rel=os.path.relpath(p, game_dir).replace('\\','/')
            h.update((rel+'|'+str(sz)+'\n').encode('utf-8','replace'))
            n+=1
            if n>=max_files: break
        if n>=max_files: break
    return h.hexdigest(), n

def load_prior():
    m={}
    for p in glob.glob(os.path.join(REPO,"reports","audit","*.json")):
        try: data=json.load(open(p,encoding="utf-8"))
        except Exception: continue
        if isinstance(data,dict): data=data.get("games") or data.get("items") or []
        if not isinstance(data,list): continue
        for e in data:
            if not isinstance(e,dict) or not e.get("verdict"): continue
            for k in ("real_path","path"):
                if e.get(k): m.setdefault(os.path.normcase(os.path.normpath(e[k])), e["verdict"])
    return m

def load_registered():
    roots=set()
    gj=os.path.join(os.environ.get("LOCALAPPDATA",""),"cgreplay-ui","games.json")
    if os.path.isfile(gj):
        try:
            for g in json.load(open(gj,encoding="utf-8")).get("games",[]):
                if g.get("root"): roots.add(os.path.normcase(os.path.normpath(g["root"])))
        except Exception: pass
    return roots

def main():
    inv=json.load(open(INV,encoding="utf-8")); games=inv["games"]
    prior=load_prior(); registered=load_registered(); t0=time.time()
    for i,g in enumerate(games):
        ext=g["main_arc_ext"]; fn=os.path.basename(g["main_arc"]).lower()
        g["pkg_kind"]="real_package" if (ext in REAL_ARC_EXTS or (ext==".dat" and fn not in NOISE)) else "dir_game"
        key=os.path.normcase(os.path.normpath(g["path"]))
        g["prior_verdict"]=prior.get(key,"not_in_prior")
        g["registered"]=key in registered
        try:
            if g["pkg_kind"]=="real_package":
                g["content_hash"]=sha256_file(g["main_arc"]); g["hash_method"]="sha256(main_package)"
                g["hash_source"]=os.path.basename(g["main_arc"])
            else:
                g["content_hash"],g["hash_files"]=dir_signature(g["path"])
                g["hash_method"]="sha256(dir_signature)"; g["hash_source"]=g["path"]
            g["hash_error"]=""
        except Exception as e:
            g["content_hash"]=""; g["hash_method"]=""; g["hash_source"]=""; g["hash_error"]=str(e)
        if (i+1)%10==0: print("HASHED %d/%d (%.1fs)"%(i+1,len(games),time.time()-t0))
    by_hash={}
    for i,g in enumerate(games):
        if g["content_hash"]: by_hash.setdefault(g["content_hash"],[]).append(i)
    groups=[]; keep_idx=set(); dup_idx=set()
    for h,idxs in by_hash.items():
        if len(idxs)<2: continue
        # 保留：真资源包优先；再按目录层级更规整(路径短)；再按名
        srt=sorted(idxs,key=lambda i:(games[i]["pkg_kind"]!="real_package",len(games[i]["path"]),games[i]["path"].lower()))
        keep=srt[0]; keep_idx.add(keep)
        for i in idxs:
            if i!=keep: dup_idx.add(i)
        groups.append({"hash":h,"method":games[keep]["hash_method"],"count":len(idxs),
                       "keep":games[keep]["path"],"duplicates":[games[i]["path"] for i in idxs if i!=keep]})
    keep=[]; dup_existing=[]
    for i,g in enumerate(games):
        if g["registered"]:
            g["class"]="duplicate_existing"; dup_existing.append(g["path"])
        elif i in dup_idx:
            g["class"]="duplicate_internal"
        else:
            g["class"]="keep"; keep.append(g)
    res={"generated_at":time.strftime("%Y-%m-%d %H:%M:%S"),"hash_seconds":round(time.time()-t0,1),
         "counts":{"total":len(games),"keep":len(keep),"duplicate_internal":len(dup_idx),
                   "duplicate_existing":len(dup_existing),"internal_groups":len(groups),
                   "real_package":sum(1 for g in games if g["pkg_kind"]=="real_package"),
                   "dir_game":sum(1 for g in games if g["pkg_kind"]=="dir_game")},
         "internal_dup_groups":groups,"games":games}
    os.makedirs(os.path.dirname(OUT),exist_ok=True)
    json.dump(res,open(OUT,"w",encoding="utf-8"),ensure_ascii=False,indent=2)
    print("DONE %.1fs"%(time.time()-t0)); print(json.dumps(res["counts"],ensure_ascii=False,indent=2))
if __name__=="__main__": main()
