#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""ks_reextract.py —— 从原始 XP3 归档重新提取 .ks 剧本（不做任何解密/变换）

用途（2026-10-16 实测）：G線上の魔王的「加密」是假象——
  归档里 .ks 就是 zlib 压缩的明文 SJIS；前人管线把压缩数据的高熵误判为加密，
  还整体涂了一层 XOR 0x23 写进解包目录（disk = zlib_blob ^ 0x23）。
  本脚本按归档真实结构提取：解压段 → 原样落地，不涂任何东西。

用法：
  python ks_reextract.py <游戏解包目录名> [归档根目录]
  例：python ks_reextract.py G線上の魔王解包
  输出：<游戏>_脚本修复/<卷名>/<归档内路径>（不改动原解包目录）
"""
import argparse
import os
import sys
import zlib

try:
    sys.stdout.reconfigure(encoding="utf-8")
    sys.stderr.reconfigure(encoding="utf-8")
except Exception:
    pass

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import probe_xp3 as P
from ks_refs import decode_ks, weak_ok

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
G = os.environ.get("UCGR_GAME_ROOT", "")


def entry_raw(f, e):
    raw = bytearray()
    for c, off, sz, pk in e["segs"]:
        f.seek(off)
        r = f.read(pk)
        raw += zlib.decompress(r) if c else r
    return bytes(raw)


def run_game(game, arc_base):
    gdir = os.path.join(ROOT, game)
    out_root = os.path.join(ROOT, game + "_脚本修复")
    os.makedirs(out_root, exist_ok=True)
    total = ok = weak = 0
    for arc in sorted(os.listdir(arc_base)):
        if not arc.lower().endswith(".xp3"):
            continue
        p = os.path.join(arc_base, arc)
        if open(p, "rb").read(3) != b"XP3":
            continue
        f = open(p, "rb")
        t, b = P.read_index(f)
        if t is None:
            continue
        _, es = P.parse(t, b)
        vol = arc[:-4]
        for e in es:
            if not e["name"].lower().endswith(".ks"):
                continue
            raw = entry_raw(f, e)
            total += 1
            txt, how = decode_ks(raw)
            if txt is None and weak_ok(raw):
                # iscript 重的文件：明文但结构分不够（如 firststart.ks），弱接受
                ok += 1
                weak += 1
            elif txt is not None:
                ok += 1
            else:
                print("  ✗ %s/%s 解不动 (%s)" % (vol, e["name"], raw[:12].hex(" ")))
                continue
            # 落地：同时按「卷/归档路径」写（ks_refs 的 basename 回退能找到）
            outp = os.path.join(out_root, vol, e["name"].replace("\\", "/"))
            os.makedirs(os.path.dirname(outp), exist_ok=True)
            with open(outp, "wb") as w:
                w.write(raw)
    print("%s: 提取 .ks %d 个，可解码 %d（其中弱接受 %d）→ %s"
          % (game, total, ok, weak, out_root))
    return total, ok


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("game", help="游戏解包目录名，如 G線上の魔王解包")
    ap.add_argument("--base", help="归档根目录（默认 UCGR_GAME_ROOT/<同名无解包后缀>）")
    args = ap.parse_args()
    if not args.base and not G:
        ap.error("未指定归档根目录：请用 --base 或设置环境变量 UCGR_GAME_ROOT")
    base = args.base or os.path.join(G, args.game.replace("解包", ""))
    if not os.path.isdir(base):
        print("归档目录不存在: %s" % base)
        return
    run_game(args.game, base)


if __name__ == "__main__":
    main()
