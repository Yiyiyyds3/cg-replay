#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""verify_engine_dist.py —— 复核 47 个解包目录的引擎分布（只读，不删不改任何游戏文件）

背景：删除 3 个「旧_单字节XOR已证伪」目录后，需重新确认 DR-21「先做 KiriKiri」的选型前提
（款数最多）是否仍成立；同时顺带产出 M0 清点需要的「解包产物」口径体积与文件数。

判定依据（解包目录内的是解包产物，按内容签名识别，不依赖原始安装目录）：
  KiriKiri   : .ks/.tjs/.sli/.func/.scn/.sinfo 脚本系 / .tlg 图像 / .xp3 归档
  Artemis    : .ast 剧本 / .psb/.psz/.lpb E-mote / .lua / .pfs
  CatSystem2 : .cst / .hg3 / .int / .kif
  BGI        : .bs / .sgcg / .arc(BURIKO)
  NScripter  : .nsa / .nss
  FVP        : .hcb / .hzc
  Eushully   : .alf
  Unity      : global-metadata.dat / Managed 目录
  Kid(AFS)   : .afs / .bip / .t2p
  Siglus(Aug): 以 exe/dll 线索在原始目录才有；解包产物按 .arc(SIG) 由人工复核

用法（解包根目录取自 --root，缺省读取环境变量 UCGR_UNPACK_ROOT）：
  python verify_engine_dist.py --root <解包根目录>                 # 扫描并打印汇总
  python verify_engine_dist.py --root <解包根目录> --json out.json # 同时落盘 JSON
"""
import argparse
import json
import os
import sys
from collections import Counter

try:
    sys.stdout.reconfigure(encoding="utf-8")
    sys.stderr.reconfigure(encoding="utf-8")
except Exception:
    pass

# 引擎 → 签名扩展名（强证据）
SIG_EXT = {
    "KiriKiri":   {".ks", ".tjs", ".sli", ".func", ".scn", ".sinfo", ".tlg", ".xp3"},
    "Artemis":    {".ast", ".psb", ".psz", ".lpb", ".pfs", ".lua"},
    "CatSystem2": {".cst", ".hg3", ".int", ".kif"},
    "BGI":        {".bs", ".sgcg"},
    "NScripter":  {".nsa", ".nss"},
    "FVP":        {".hcb", ".hzc"},
    "Eushully":   {".alf"},
    "KidAFS":     {".afs", ".bip", ".t2p"},
}
# 引擎 → 脚本类扩展（用于"有剧本"判定，区分"有脚本证据"与"只有图像证据"）
SCRIPT_EXT = {
    "KiriKiri":   {".ks", ".tjs", ".sli", ".func", ".scn", ".sinfo"},
    "Artemis":    {".ast", ".lua"},
    "CatSystem2": {".cst"},
    "BGI":        {".bs"},
    "NScripter":  {".nss"},
    "FVP":        {".hcb"},
}
ARCHIVE_EXT = {".xp3", ".pfs", ".int", ".kif", ".arc", ".dat", ".nsa", ".alf",
               ".afs", ".bin", ".hcb", ".hzc", ".paz", ".pak", ".lpk", ".pck"}
BODY_EXT = {".exe", ".dll", ".sys", ".ocx", ".cfg", ".ini", ".manifest", ".bat", ".cmd"}

GB = 1024.0 ** 3


def scan_game(gdir):
    files = 0
    total = 0
    ext_cnt = Counter()
    sig_cnt = Counter()        # 引擎 → 命中文件数
    script_cnt = Counter()     # 引擎 → 脚本文件数
    arch_cnt = Counter()       # 归档扩展名 → 数
    arch_bytes = 0
    body_cnt = Counter()
    body_bytes = 0
    unity_hit = False
    for dp, dirs, fs in os.walk(gdir):
        # Unity IL2CPP 特征
        if "global-metadata.dat" in fs or os.path.basename(dp).lower() == "managed":
            unity_hit = True
        for fn in fs:
            files += 1
            try:
                sz = os.path.getsize(os.path.join(dp, fn))
            except OSError:
                sz = 0
            total += sz
            ext = os.path.splitext(fn)[1].lower()
            ext_cnt[ext] += 1
            for eng, exts in SIG_EXT.items():
                if ext in exts:
                    sig_cnt[eng] += 1
            for eng, exts in SCRIPT_EXT.items():
                if ext in exts:
                    script_cnt[eng] += 1
            if ext in ARCHIVE_EXT:
                arch_cnt[ext] += 1
                arch_bytes += sz
            if ext in BODY_EXT:
                body_cnt[ext] += 1
                body_bytes += sz
    return {
        "files": files, "bytes": total,
        "ext_top": ext_cnt.most_common(12),
        "sig": dict(sig_cnt), "script": dict(script_cnt),
        "archives": dict(arch_cnt), "archive_bytes": arch_bytes,
        "body": dict(body_cnt), "body_bytes": body_bytes,
        "unity": unity_hit,
    }


def decide(rec):
    """按证据强度定引擎：脚本证据 > 图像/归档证据；Unity 特征优先。"""
    if rec["unity"]:
        return "UnityIL2CPP", "unity 特征(global-metadata/Managed)"
    # 脚本证据
    script_hits = sorted(rec["script"].items(), key=lambda kv: -kv[1])
    sig_hits = sorted(rec["sig"].items(), key=lambda kv: -kv[1])
    if script_hits and script_hits[0][1] > 0:
        eng, n = script_hits[0]
        # Artemis 的 .ast 与 KiriKiri 的 .ks 不会同时大量出现；平手时报歧义
        tied = [e for e, c in script_hits if c == n]
        if len(tied) == 1:
            return eng, "脚本 %s=%d" % (eng, n)
        return "AMBIGUOUS", "脚本平手 %s" % tied
    if sig_hits and sig_hits[0][1] > 0:
        eng, n = sig_hits[0]
        return eng, "签名 %s=%d（无脚本证据）" % (eng, n)
    return "UNKNOWN", "无签名命中"


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--root", default=os.environ.get("UCGR_UNPACK_ROOT"),
                    help="解包根目录（其下为 *解包 子目录）；缺省读取环境变量 UCGR_UNPACK_ROOT")
    ap.add_argument("--json", dest="json_path", default=None)
    args = ap.parse_args()
    if not args.root:
        ap.error("未指定解包根目录：请用 --root 或设置环境变量 UCGR_UNPACK_ROOT")
    root = args.root

    names = sorted(d for d in os.listdir(root)
                   if os.path.isdir(os.path.join(root, d)) and d.endswith("解包"))
    print("解包目录数: %d" % len(names))
    print()

    rows = []
    dist = Counter()
    amb = []
    for name in names:
        gdir = os.path.join(root, name)
        rec = scan_game(gdir)
        eng, why = decide(rec)
        rec["engine"] = eng
        rec["why"] = why
        rows.append((name, rec))
        dist[eng] += 1
        if eng in ("AMBIGUOUS", "UNKNOWN"):
            amb.append(name)

    # 明细表
    print("%-34s %9s %8s %6s  %-12s %s" % ("游戏", "体积GB", "文件数", "归档", "引擎", "依据"))
    print("-" * 110)
    tot_b = 0
    tot_f = 0
    for name, rec in rows:
        tot_b += rec["bytes"]
        tot_f += rec["files"]
        na = sum(rec["archives"].values())
        print("%-34s %9.2f %8d %6d  %-12s %s" % (
            name[:34], rec["bytes"] / GB, rec["files"], na, rec["engine"], rec["why"]))
    print("-" * 110)
    print("%-34s %9.2f %8d" % ("合计", tot_b / GB, tot_f))
    print()
    print("引擎分布:")
    for eng, n in dist.most_common():
        gb = sum(r["bytes"] for nm, r in rows if r["engine"] == eng) / GB
        print("  %-14s %2d 款   %8.2f GB" % (eng, n, gb))
    if amb:
        print()
        print("需人工复核: %s" % ", ".join(amb))

    if args.json_path:
        out = {
            "root": root,
            "games": {nm: {k: v for k, v in rec.items()} for nm, rec in rows},
            "distribution": dict(dist),
        }
        with open(args.json_path, "w", encoding="utf-8") as f:
            json.dump(out, f, ensure_ascii=False, indent=1)
        print()
        print("JSON 已写入: %s" % args.json_path)


if __name__ == "__main__":
    main()
