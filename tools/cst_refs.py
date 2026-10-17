#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""cst_refs.py —— CatSystem2 剧本解析 + 引用提取 + ④测算（P2 第一个引擎）

对应 05 文档 P2。复用 ks_refs 的匹配/保守保留逻辑，CS2 差异只在：
  1. 剧本是 .cst（CatScene 魔数 + zlib；条目 01 <type> <bytes> 00，
     type 0x20 对白 / 0x21 角色名 / 0x30 脚本命令）——提取器复用
     extract_cst_text4.decompress_cst / raw_entries
  2. 引用语法（2026-10-16 实测于甜蜜女友2/灰色的乐园）：
       bg|cg|fg|eg|fw|pl|mpl  <槽位|[*]>  <图片名>[ ,位置参数...]
       pcm  <语音名>                se  <buf> <音效名>
       movie <影片名>
  3. 池口径：原始归档解包（HG3 原样/fes/psz…）∪ 音频（语音/BGM/音效）
     ∪ 视频；排除「图片」目录（HG3→PNG 的派生副本）与 _分类/解包输出

DR-16：BGM 全保留 → ④ = 引用命中 ∪ 池内全部 BGM（路径含 bgm 的）
E-mote（.psb/.psz/.lpb）按硬边界不复刻 → 不计入 ④

用法：
  python cst_refs.py --all [--json out.json]
  python cst_refs.py <解包目录>
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
from ks_refs import (GB, build_pool, match_ref, dynamic_candidates, role_of,
                     nfkc, decode_ks, _kag_score, build_substr_index)
from extract_cst_text4 import decompress_cst, raw_entries

ROOT = os.path.dirname(HERE)
CAT2_GAMES = [
    "甜蜜女友PE解包", "甜蜜女友2解包", "甜蜜女友2+解包", "甜蜜女友+解包",
    "灰色的乐园FHD解包", "果实解包", "GrisaiaAno1解包", "灰色的有闲解包",
]

LAYERS = ("bg", "cg", "fg", "eg", "fw", "pl", "mpl")
IMG_RE = re.compile(r"^(%s)\s+(\[[^\]]*\]|\S+)\s+([A-Za-z][A-Za-z0-9_-]*)"
                    % "|".join(LAYERS))
PCM_RE = re.compile(r"^pcm\s+([A-Za-z0-9_.\-]+)")
SE_RE = re.compile(r"^se\s+\S+\s+([A-Za-z0-9_.\-]+)")
MOVIE_RE = re.compile(r"^movie\s+([A-Za-z0-9_.\-]+)")
NUMERIC_RE = re.compile(r"^[0-9]+$")
# 层命令的第二参数是关键字而非图片名的常见情形（匹配阶段也会过滤，这里只是提速）
IMG_KEYWORDS = {"blend", "pos", "blur", "tone", "stop", "mode", "attr_def",
                "disp", "scale", "fade", "arc", "wait", "quake", "anti",
                "mode", "clear", "copy", "tone", "effect"}


def build_group_index(pool_by_name):
    """CS2 多层 HG3 组索引：引用 Tama01f → Tama01f_0001..N.hg3 整组。

    解包工具把多图层 HG3 展开为 <name>_<序号>.<ext>（一层一文件），
    剧本里的裸名指向整组——少一层都渲染不出来，必须整组保留。
    """
    idx = {}
    for name, paths in pool_by_name.items():
        base = os.path.splitext(name)[0]
        m = re.match(r"^(.*)_\d+$", base)
        idx.setdefault(m.group(1) if m else base, []).extend(paths)
    return idx


def match_group(ref, pool, pool_by_name, group_index):
    """返回该引用对应的全部池内文件（单文件或图层组）。"""
    k = match_ref(ref, pool, pool_by_name)
    if k:
        return [k]
    r = nfkc(ref.lower())
    if r in group_index:
        return sorted(set(group_index[r]))
    return []


CS2_ROLE_RULES = [
    ("image", "图片"), ("bgm", "BGM"), ("语音", "语音"), ("voice", "语音"),
    ("音效", "音效"), ("fes", "事件视频"), ("视频", "事件视频"), ("movie", "事件视频"),
    ("psz", "E-mote(不复刻)"), ("psb", "E-mote(不复刻)"), ("lpb", "E-mote(不复刻)"),
]


def role_cs2(path):
    p = path.lower()
    for key, role in CS2_ROLE_RULES:
        if key in p:
            return role
    return role_of(path)


def cst_commands(path):
    """返回该 .cst 的全部 0x30 脚本命令（UTF-8 优先，失败退 cp932）。"""
    raw = decompress_cst(path)
    if raw is None:
        return []
    out = []
    for t, b in raw_entries(raw):
        if t != 0x30:
            continue
        try:
            s = b.decode("utf-8")
        except Exception:
            try:
                s = b.decode("cp932")
            except Exception:
                continue
        if s and not NUMERIC_RE.match(s):
            out.append(s.strip())
    return out


def parse_refs_cs2(cmds):
    """从命令流提取资源引用。"""
    refs = []
    for s in cmds:
        m = IMG_RE.match(s)
        if m:
            name = m.group(3)
            if name not in IMG_KEYWORDS and not NUMERIC_RE.match(name):
                refs.append(name)
            continue
        m = PCM_RE.match(s)
        if m:
            v = m.group(1)
            refs.append(v)
            # PE 实测：引用带 take 后缀（A_19_sayuki_ex01_022_1），
            # 文件名不带（A_19_sayuki_ex01_022.ogg）→ 同时登记去后缀变体
            m2 = re.match(r"^(.*)_\d+$", v)
            if m2 and len(m2.group(1)) >= 6:
                refs.append(m2.group(1))
            continue
        m = SE_RE.match(s)
        if m:
            refs.append(m.group(1))
            continue
        m = MOVIE_RE.match(s)
        if m:
            refs.append(m.group(1))
            continue
    return refs


# E-mote 硬边界（FINAL §4.4 / §11）：动态立绘与口型数据不复刻，可整块删除
EMOTE_EXTS = (".psb", ".psz", ".lpb")


def build_pool_cs2(game_dir):
    """CS2 池：原始归档解包 ∪ 音频 ∪ 视频；排除 图片（派生）/_分类/解包输出。"""
    pool = {}
    bases = []
    for name in ("原始归档解包", "音频", "视频"):
        p = os.path.join(game_dir, name)
        if os.path.isdir(p):
            bases.append(p)
    if not bases:
        bases = [game_dir]
    for base in bases:
        for dp, dirs, fs in os.walk(base):
            dirs[:] = [d for d in dirs if d not in ("_分类", "解包输出", "图片")]
            for fn in fs:
                if fn.lower().endswith(".cst"):
                    continue
                ap = os.path.join(dp, fn)
                rel = nfkc(os.path.relpath(ap, bases[0]).replace("\\", "/")) \
                    if base == bases[0] else \
                    nfkc((os.path.basename(base) + "/" +
                          os.path.relpath(ap, base)).replace("\\", "/"))
                try:
                    pool[rel] = (ap, os.path.getsize(ap))
                except OSError:
                    pass
    return bases[0], pool


def run_game(game_dir, verbose=True):
    game = os.path.basename(game_dir.rstrip("/\\"))
    base, pool = build_pool_cs2(game_dir)
    pool_by_name = {}
    for k in pool:
        pool_by_name.setdefault(k.rsplit("/", 1)[-1], []).append(k)

    cst_files = []
    for b in ([os.path.join(game_dir, "原始归档解包")]
              if os.path.isdir(os.path.join(game_dir, "原始归档解包")) else [game_dir]):
        for dp, dirs, fs in os.walk(b):
            dirs[:] = [d for d in dirs if d not in ("_分类", "解包输出")]
            for fn in fs:
                if fn.lower().endswith(".cst"):
                    cst_files.append(os.path.join(dp, fn))
    cst_files.sort()

    all_refs = {}
    n_cmd = 0
    for p in cst_files:
        for r in parse_refs_cs2(cst_commands(p)):
            all_refs[r] = all_refs.get(r, 0) + 1
            n_cmd += 1

    matched = {}
    unmatched = {}
    group_idx = build_group_index(pool_by_name)
    for ref, n in all_refs.items():
        ks = match_group(ref, pool, pool_by_name, group_idx)
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

    # E-mote 硬边界：.psb/.psz/.lpb 不复刻 → 从 ④ 剥出，单独计为可删量
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
        "game": game, "cst_files": len(cst_files), "cmds": n_cmd,
        "distinct_refs": len(all_refs), "matched": len(matched),
        "unmatched": len(unmatched), "unmatched_samples": sorted(unmatched)[:15],
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
        print("   cst=%d 命令引用=%d distinct=%d 命中=%d 未命中=%d 保守保留=%d(%.2fGB)"
              % (len(cst_files), n_cmd, len(all_refs), len(matched),
                 len(unmatched), len(dynamic), rec["dynamic_bytes"] / GB))
        if unmatched:
            print("   未命中样例: %s" % ", ".join(sorted(unmatched)[:8]))
        print("   ④ %.2f GB（含BGM全留 %d 个）/ 池 %.2f GB（%d 文件） → 保留率 %.1f%%"
              % (mbytes / GB, len(bgm_all), pool_bytes / GB, len(pool), rec["ratio"] * 100))
        if emote:
            print("   E-mote 剥出 %d 个 / %.2f GB（不复刻，可删）"
                  % (len(emote), sum(emote.values()) / GB))
        for role, (c, b) in sorted(roles.items(), key=lambda kv: -kv[1][1]):
            print("      %-8s %5d 个 %8.2f GB" % (role, c, b / GB))
    return rec


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("target", nargs="?", help="解包目录")
    ap.add_argument("--all", action="store_true")
    ap.add_argument("--json", dest="json_path")
    args = ap.parse_args()
    targets = ([os.path.join(ROOT, g) for g in CAT2_GAMES] if args.all
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
    print("合计（%d 款）: ④ %.2f GB / 池 %.2f GB → 保留率 %.1f%%"
          % (len(recs), tm, tp, (tm / tp * 100) if tp else 0))
    if args.json_path:
        with open(args.json_path, "w", encoding="utf-8") as f:
            json.dump(recs, f, ensure_ascii=False, indent=1)
        print("JSON: %s" % args.json_path)


if __name__ == "__main__":
    main()
