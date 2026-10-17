#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""ast_refs.py —— Artemis Engine 剧本解析 + 引用提取 + ④测算（P2 第二个引擎）

对应 05 文档 P2。复用 ks_refs 的匹配/保守保留/3-gram 索引。

Artemis 实测格式（2026-10-16，たねつみの歌/樱之刻/甜蜜女友3）：
  - 剧本 .ast 是【明文 Lua 表】：astver/astname + ast={ ["label"]={ {"cmd",k=v,...}, ... } }
  - 资源引用 = 命令里的 file="..."：
      {"bg",file="black",path=":bg/"}      背景
      {"fg",file="...",path=":fg/xx/z1/"}  立绘
      {"vo",file="man_hir_00001",ch="hir"} 语音
      {"bgm",file="bgm29"}                 BGM
      {"se",file="..."}                    音效
      {"evset",file="ev_cut_001_00",...}   事件 CG
      {"movie",file="..."}                 影片
  - 图片在 pfs 内就是 PNG/JPG（原格式），提取后位于 图片/<卷>/image/...；
    语音在 音频/<卷>/sound/...；影片在 视频/<卷>/...
  - 池口径：整树，排除前人派生文本（剧本_对话_*、资源调度/）与清单/说明文件

DR-16：BGM 全保留；E-mote（.psb/.psz/.lpb）按硬边界不复刻 → 剥出单独报告

用法：
  python ast_refs.py --all [--json out.json]
  python ast_refs.py <解包目录>
"""
import argparse
import glob
import json
import os
import re
import sys

try:
    sys.stdout.reconfigure(encoding="utf-8")
except Exception:
    pass

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
from ks_refs import (GB, match_all, dynamic_candidates, role_of, nfkc,
                     build_substr_index)
from cst_refs import EMOTE_EXTS, role_cs2

ROOT = os.path.dirname(HERE)
ARTEMIS_GAMES = ["樱之刻解包", "甜蜜女友3解包", "たねつみの歌解包"]

# 带 file= 资源引用的命令（实测词表）
RES_CMDS = ("vo", "bg", "fg", "fgf", "fgact", "se", "bgm", "evset", "movie",
            "excall", "cg", "cgdel")
CMD_BLOCK_RE = re.compile(r'\{"(%s)"([^{}]*)\}' % "|".join(RES_CMDS))
FILE_RE = re.compile(r'file="([^"]+)"')
PATH_PREFIX_RE = re.compile(r"^:[A-Za-z0-9_/]*/")   # :ani/ 、:fg/xx/z1/ 等路径别名前缀
# 池排除：前人派生文本 / 清单说明 / 脚本自身 / 松散 pfs 卷文件
# （E-mote 的 .psb/.psz/.lpb 【进池】但按硬边界从 ④ 剥出——与 CS2 口径一致，
#   这样可删量才看得见；实测甜蜜女友3 的 E-mote 达 4.78 GB）
POOL_EXCLUDE_DIRS = ("剧本_对话_中文", "剧本_对话_日文", "剧本_对话_英文", "资源调度")
POOL_EXCLUDE_EXTS = (".ast", ".md", ".txt", ".pfs")


def ast_files(game_dir):
    out = []
    for dp, dirs, fs in os.walk(game_dir):
        dirs[:] = [d for d in dirs if d not in POOL_EXCLUDE_DIRS]
        for fn in fs:
            if fn.lower().endswith(".ast"):
                out.append(os.path.join(dp, fn))
    return sorted(out)


def parse_ast(path):
    """返回该 .ast 的全部资源引用（file= 值，去重保序）。"""
    try:
        t = open(path, encoding="utf-8", errors="replace").read()
    except OSError:
        return []
    refs = []
    seen = set()
    for m in CMD_BLOCK_RE.finditer(t):
        fm = FILE_RE.search(m.group(2))
        if not fm:
            continue
        v = fm.group(1).strip()
        # 部分命令把路径别名写进 file=（如 file=":ani/kiss"）→ 剥掉 :别名/ 前缀
        v = PATH_PREFIX_RE.sub("", v)
        if v and v not in seen:
            seen.add(v)
            refs.append(v)
    return refs


def build_pool_ast(game_dir):
    """池 = 整树，排除派生文本/清单/脚本/E-mote 原卷。"""
    pool = {}
    for dp, dirs, fs in os.walk(game_dir):
        dirs[:] = [d for d in dirs if d not in POOL_EXCLUDE_DIRS]
        for fn in fs:
            if fn.lower().endswith(POOL_EXCLUDE_EXTS):
                continue
            ap = os.path.join(dp, fn)
            rel = nfkc(os.path.relpath(ap, game_dir).replace("\\", "/"))
            try:
                pool[rel] = (ap, os.path.getsize(ap))
            except OSError:
                pass
    return game_dir, pool


def run_game(game_dir, verbose=True):
    game = os.path.basename(game_dir.rstrip("/\\"))
    base, pool = build_pool_ast(game_dir)
    pool_by_name = {}
    for k in pool:
        pool_by_name.setdefault(k.rsplit("/", 1)[-1], []).append(k)

    files = ast_files(game_dir)
    script_names = {os.path.splitext(os.path.basename(p))[0] for p in files}
    all_refs = {}
    script_refs = set()
    for p in files:
        for r in parse_ast(p):
            if r in script_names:
                # excall/selback 等剧本间调用——按 DR-25 剧本全留，不算资产未命中
                script_refs.add(r)
                continue
            all_refs[r] = all_refs.get(r, 0) + 1

    matched = {}
    unmatched = {}
    for ref, n in all_refs.items():
        ks = match_all(ref, pool, pool_by_name)
        if ks:
            for k in ks:
                matched[k] = matched.get(k, 0) + n
        else:
            unmatched[ref] = n

    dynamic = {}
    substr_idx = build_substr_index(pool_by_name)
    for ref in sorted(unmatched):
        for k in dynamic_candidates(ref, pool_by_name, substr_idx):
            if k not in matched:
                matched[k] = 0
                dynamic[k] = [ref]
            elif k in dynamic and ref not in dynamic[k]:
                dynamic[k].append(ref)

    bgm_all = [k for k in pool if "bgm" in k]
    for k in bgm_all:
        matched.setdefault(k, 0)

    emote = {k: pool[k][1] for k in list(matched) if k.lower().endswith(EMOTE_EXTS)}
    for k in emote:
        matched.pop(k, None)

    mbytes = sum(pool[k][1] for k in matched)
    pool_bytes = sum(v[1] for v in pool.values())
    roles = {}
    for k in matched:
        roles.setdefault(role_cs2(k), [0, 0])
        roles[role_cs2(k)][0] += 1
        roles[role_cs2(k)][1] += pool[k][1]

    rec = {
        "game": game, "ast_files": len(files), "distinct_refs": len(all_refs),
        "script_refs": len(script_refs),
        "matched": len(matched), "unmatched": len(unmatched),
        "unmatched_samples": sorted(unmatched)[:15],
        "dynamic_retained": len(dynamic),
        "dynamic_bytes": sum(pool[k][1] for k in dynamic),
        "emote_excluded_n": len(emote),
        "emote_excluded_bytes": sum(emote.values()),
        "matched_bytes": mbytes, "pool_bytes": pool_bytes,
        "pool_files": len(pool), "bgm_all": len(bgm_all),
        "ratio": (mbytes / pool_bytes) if pool_bytes else 0.0, "roles": roles,
    }
    if verbose:
        print("== %s" % game)
        print("   ast=%d distinct资产引用=%d 剧本引用=%d 命中=%d 未命中=%d 保守保留=%d(%.2fGB)"
              % (len(files), len(all_refs), len(script_refs), len(matched),
                 len(unmatched), len(dynamic), rec["dynamic_bytes"] / GB))
        if unmatched:
            print("   未命中样例: %s" % ", ".join(sorted(unmatched)[:8]))
        print("   ④ %.2f GB（含BGM全留 %d 个）/ 池 %.2f GB（%d 文件） → 保留率 %.1f%%"
              % (mbytes / GB, len(bgm_all), pool_bytes / GB, len(pool), rec["ratio"] * 100))
        if emote:
            print("   E-mote 剥出 %d 个 / %.2f GB（不复刻，可删）"
                  % (len(emote), sum(emote.values()) / GB))
        for role, (c, b) in sorted(roles.items(), key=lambda kv: -kv[1][1]):
            print("      %-10s %5d 个 %8.2f GB" % (role, c, b / GB))
    return rec


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("target", nargs="?", help="解包目录")
    ap.add_argument("--all", action="store_true")
    ap.add_argument("--json", dest="json_path")
    args = ap.parse_args()
    targets = ([os.path.join(ROOT, g) for g in ARTEMIS_GAMES] if args.all
               else [args.target] if args.target else [])
    if not targets:
        ap.error("给目录或用 --all")
    recs = []
    for t in targets:
        if os.path.isdir(t):
            recs.append(run_game(t))
    print()
    tm = sum(r["matched_bytes"] for r in recs) / GB
    tp = sum(r["pool_bytes"] for r in recs) / GB
    te = sum(r["emote_excluded_bytes"] for r in recs) / GB
    print("合计（%d 款）: ④ %.2f GB / 池 %.2f GB → 保留率 %.1f%%（另 E-mote 剥出 %.2f GB）"
          % (len(recs), tm, tp, (tm / tp * 100) if tp else 0, te))
    if args.json_path:
        with open(args.json_path, "w", encoding="utf-8") as f:
            json.dump(recs, f, ensure_ascii=False, indent=1)
        print("JSON: %s" % args.json_path)


if __name__ == "__main__":
    main()
