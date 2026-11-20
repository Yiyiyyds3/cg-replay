#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""盘点本地游戏源目录，按任务规则识别候选游戏（只读，不写游戏目录）。
规则：文件夹自身同时含 .exe 与 (.xp3/.arc/.dat/.pac/.pck/.noa/.bin) 之一 => 候选。
深度默认 1，可用 --depth 调到 2（厂商/游戏名两级）。这里主库布局为
<root>/<日期>/<类型>/<评价>/<游戏名>，故用 --base-depth 4 定位游戏层。
"""
import json, os, sys, fnmatch, argparse

ARCS = {".xp3", ".arc", ".dat", ".pac", ".pck", ".noa", ".bin"}
EXCLUDE_DIRS = {".git", "node_modules", "_todelete", "_待删除", "$recycle.bin",
                "system volume information", "_游戏库"}

def excluded(path):
    for pt in path.replace("/", os.sep).split(os.sep):
        low = pt.lower()
        if low in EXCLUDE_DIRS: return True
        if fnmatch.fnmatch(low, "*.tmp"): return True
        if pt.startswith(".") and low not in (".",): return True
    return False

def scan(root, base_depth, child_depth):
    out = []
    root = os.path.abspath(root)
    max_depth = base_depth + child_depth
    for dirpath, dirnames, filenames in os.walk(root):
        rel = os.path.relpath(dirpath, root)
        depth = 0 if rel == "." else len(rel.split(os.sep))
        if depth >= max_depth:
            dirnames[:] = []
        dirnames[:] = [d for d in dirnames
                       if d.lower() not in EXCLUDE_DIRS
                       and not d.startswith(".")
                       and not fnmatch.fnmatch(d.lower(), "*.tmp")]
        if excluded(dirpath): continue
        if not (base_depth <= depth <= max_depth):
            continue
        # only THIS folder's own files
        exts = [(f, os.path.splitext(f)[1].lower()) for f in filenames]
        if not any(e == ".exe" for _, e in exts): continue
        arc_files = [(os.path.join(dirpath, f), os.path.getsize(os.path.join(dirpath, f)))
                     for f, e in exts if e in ARCS and os.path.isfile(os.path.join(dirpath, f))]
        if not arc_files: continue
        # ignore tiny engine runtime dat (icudtl.dat / snapshot_blob.bin etc.)
        meaningful = [(p, s) for p, s in arc_files
                      if os.path.basename(p).lower() not in
                      ("icudtl.dat", "snapshot_blob.bin", "resources.pak", "nw_100_percent.pak")]
        if not meaningful: continue
        meaningful.sort(key=lambda x: -x[1])
        out.append({
            "name": os.path.basename(dirpath), "path": dirpath, "rel": rel, "depth": depth,
            "main_arc": meaningful[0][0],
            "main_arc_ext": os.path.splitext(meaningful[0][0])[1].lower(),
            "main_arc_bytes": meaningful[0][1],
            "arc_count": len(meaningful),
            "arc_total_bytes": sum(s for _, s in meaningful),
            "exe_count": sum(1 for _, e in exts if e == ".exe"),
        })
    out.sort(key=lambda x: x["path"].lower())
    return out

if __name__ == "__main__":
    ap = argparse.ArgumentParser()
    ap.add_argument("--root", default=os.environ.get("UCGR_GAME_ROOT", ""), help="游戏根目录，或设 UCGR_GAME_ROOT 环境变量")
    ap.add_argument("--base-depth", type=int, default=4)
    ap.add_argument("--child-depth", type=int, default=1)
    ap.add_argument("out", nargs="?", default="reports/local_import/data/inventory.json")
    a = ap.parse_args()
    if not a.root:
        raise SystemExit("需要 --root <游戏根目录> 或设置 UCGR_GAME_ROOT 环境变量（仓库不写本机路径）")
    data = scan(a.root, a.base_depth, a.child_depth)
    os.makedirs(os.path.dirname(a.out), exist_ok=True)
    with open(a.out, "w", encoding="utf-8") as f:
        json.dump({"root": a.root, "base_depth": a.base_depth, "child_depth": a.child_depth,
                   "count": len(data), "games": data}, f, ensure_ascii=False, indent=2)
    print("CANDIDATES=%d" % len(data)); print("WROTE=" + os.path.abspath(a.out))
