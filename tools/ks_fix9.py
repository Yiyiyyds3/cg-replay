#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""ks_fix9.py v2 —— 修复 9nine 五章中「解密不完整」的 .ks 脚本（只读原归档，输出到新目录）

2026-10-15 实证结论：
  * 前人管线按「bo 分段常量 XOR」解密了大部分 .ks，但文本无例外点修复、
    尾段密钥常选错（maplist.ks 正确 kt=0x02，前人选了 0x27 → bo 后全文乱码）。
  * peel.py 的 CxEncryption VM（mod_all.json 方案表）是 ground truth：
    用 MOD(518,77) 的 VM 密钥流可完美解开春/天色的大多数顽固文件
    （freecamera/sysmenu/version/maplist 全部 0 坏字节）。
  * 新章/雪色参数 (0x204,1211) 在 MOD 中无条目 → 用「结构分」常量模型
    （头/尾密钥都以 KAG 结构分选取，避开坏字数在半角假名上的失效）。

策略（按序）：
  1. 解码器直接能读 → 跳过
  2. VM 密钥流（章节有 MOD 匹配时）→ 解码器能读即采用
  3. 常量模型（结构分求 kh/kt）+ SJIS 例外点修复 → 解码器能读即采用
  4. 同名多条目（汉化补丁层）逐一尝试，取最优
  5. 都失败 → 留待人工/逆向（按安全原则该游戏保守保留）

输出：<游戏>_脚本修复/<原始归档解包内的相对路径>（不改动原解包目录）
"""
import argparse
import json
import os
import sys
import zlib

try:
    sys.stdout.reconfigure(encoding="utf-8")
except Exception:
    pass

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import probe_xp3 as P
from ks_refs import decode_ks, _kag_score, _sjis_repair, weak_ok
from peel import Layer, M32

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
G = r"<GAMES_ROOT>"
HERE = os.path.dirname(os.path.abspath(__file__))

# 游戏 → (归档根, MASK, OFFSET, 卷, MOD匹配标题或None)
CHAPTERS = {
    "9nine九次九日解包": (r"[ぱれっと] 9-nine-九次九日九重色 汉化硬盘版", 0x206, 77,
                    ["data.xp3", "evimage.xp3", "fgimage.xp3", "video.xp3", "voice.xp3"],
                    "9 -Nine- Kokonotsu Kokonoka Kokonoiro"),
    "9nine天色解包": (r"9-nine-天色天歌天籁音", 0x206, 77,
                  ["data.xp3", "evimage.xp3", "fgimage.xp3", "video.xp3", "voice.xp3"],
                  "9 -Nine- Kokonotsu Kokonoka Kokonoiro"),
    "9nine春色解包": (r"9-nine-春色春恋春熙风（第三部）", 0x206, 77,
                  ["data.xp3", "evimage.xp3", "fgimage.xp3", "video.xp3", "voice.xp3"],
                  "9 -Nine- Kokonotsu Kokonoka Kokonoiro"),
    "9nine新章解包": (r"102-13\9-nine- 新章汉化硬盘版", 0x204, 1211,
                  ["data.xp3", "evimage.xp3", "fgimage.xp3", "video.xp3", "voice.xp3"], None),
    "9nine雪色解包": (r"9-nine-雪色雪花雪之后（雪色雪花雪余痕）", 0x204, 1211,
                  ["data.xp3", "evimage.xp3", "fgimage.xp3", "video.xp3", "voice.xp3"], None),
}

XT = [bytes(((i ^ k) & 0xFF) for i in range(256)) for k in range(256)]
TAGSTART = __import__("re").compile(r"\[[A-Za-z]")


def struct_score(t):
    """KAG 结构分：标签/属性/ASCII 结构字符密度（对尾段比坏字数可靠）。"""
    s = 3 * len(TAGSTART.findall(t)) + 2 * t.count("storage=") + 2 * t.count("text=")
    s += t.count("[") + t.count("]") + t.count("=") + t.count('"')
    s += t.count("\r") + t.count("\n") + t.count("\t")
    s += sum(1 for c in t if c.isascii() and c.isalpha()) // 20
    return s


_MOD_CACHE = {}


def get_layer(title):
    if title in _MOD_CACHE:
        return _MOD_CACHE[title]
    mod = json.load(open(os.path.join(HERE, "mod_all.json"), encoding="utf-8"))
    ent = [m for m in mod if m["title"] == title]
    lb = None
    if ent:
        e = ent[0]
        lb = Layer([v & M32 for v in e["ControlBlock"][:1024]], e["PrologOrder"],
                   e["OddBranchOrder"], e["EvenBranchOrder"], e["m_mask"], e["m_offset"])
    _MOD_CACHE[title] = lb
    return lb


def read_entry(f, e):
    raw = bytearray()
    for comp, off, sz, pk in e["segs"]:
        f.seek(off)
        r = f.read(pk)
        raw += zlib.decompress(r) if comp else r
    return bytes(raw)


def load_archive(base, arcs):
    """name -> [ (entry, fh), ... ]（保留同名多条目：汉化补丁层）"""
    ents = {}
    for arc in arcs:
        path = os.path.join(G, base, arc)
        if not os.path.exists(path):
            continue
        f = open(path, "rb")
        t, b = P.read_index(f)
        _, es = P.parse(t, b)
        for e in es:
            ents.setdefault(e["name"].replace("\\", "/"), []).append((e, f))
    return ents


def try_vm(raw, h, layer):
    if layer is None:
        return None
    ks = layer.stream(h, len(raw))
    return bytes(a ^ b for a, b in zip(raw, ks))


def try_const(raw, h, mask, offset):
    n = len(raw)
    bo = (h & mask) + offset
    if bo <= 0 or bo >= n:
        # 整文件单密钥。小文件（<512B）结构分不可靠，加首字节启发式：
        # KAG 文本通常以 *label / ;注释 / [tag / @macro 开头
        cands = []
        for k in range(256):
            D = raw.translate(XT[k])
            sc = struct_score(D.decode("shift_jis", errors="replace"))
            if n < 512 and D and D[0] in (0x2A, 0x3B, 0x5B, 0x40):
                t = D.decode("shift_jis", errors="replace")
                if t.count("\ufffd") <= 1 and ("\r\n" in t or "\n" in t):
                    sc += 1000
            cands.append((sc, k))
        best = max(cands)
        D = raw.translate(XT[best[1]])
    else:
        head = raw[:bo]
        tail = raw[bo:]
        kh = max(((struct_score(head.translate(XT[k]).decode("shift_jis", "replace")), k)
                  for k in range(256)))[1]
        kt = max(((struct_score(tail.translate(XT[k]).decode("shift_jis", "replace")), k)
                  for k in range(256)))[1]
        D = head.translate(XT[kh]) + tail.translate(XT[kt])
    rep, nf = _sjis_repair(D)
    return rep


def best_decrypt(raw, h, mask, offset, layer):
    """依次尝试 VM / 常量模型，返回 (结果, 方式) 或 (None, 原因)"""
    cands = []
    D = try_vm(raw, h, layer)
    if D is not None:
        cands.append(("vm", D))
    D2 = try_const(raw, h, mask, offset)
    cands.append(("const", D2))
    best = None
    for how, D in cands:
        t, dec = decode_ks(D)
        if t is not None:
            sc = _kag_score(t)
            if best is None or sc > best[0]:
                best = (sc, D, how)
        elif weak_ok(D):
            if best is None:
                best = (0, D, how + "+weak")
    if best:
        return best[1], best[2]
    return None, "all-failed"


def run_game(game):
    base, mask, offset, arcs, mod_title = CHAPTERS[game]
    layer = get_layer(mod_title) if mod_title else None
    gdir = os.path.join(ROOT, game)
    out_root = os.path.join(ROOT, game + "_脚本修复")
    ents = load_archive(base, arcs)
    by_base = {}
    for name in ents:
        by_base.setdefault(os.path.basename(name).lower(), []).append(name)

    src_root = os.path.join(gdir, "原始归档解包")
    walk_root = src_root if os.path.isdir(src_root) else gdir
    targets = []
    for dp, dirs, fs in os.walk(walk_root):
        dirs[:] = [d for d in dirs if d not in ("_分类", "解包输出")]
        for fn in fs:
            if fn.lower().endswith(".ks"):
                p = os.path.join(dp, fn)
                if decode_ks(open(p, "rb").read())[0] is None:
                    targets.append(p)
    targets.sort()

    fixed = failed = 0
    for p in targets:
        rel = os.path.relpath(p, walk_root)
        parts = rel.replace("\\", "/").split("/")
        cand_names = []
        if len(parts) > 1:
            suffix = "/".join(parts[1:])
            if suffix in ents:
                cand_names.append(suffix)
        cand_names += by_base.get(os.path.basename(p).lower(), [])
        if not cand_names:
            failed += 1
            print("  ✗ %s（归档中找不到条目）" % rel)
            continue
        best = None
        for name in cand_names:
            for e, f in ents[name]:
                raw = read_entry(f, e)
                D, how = best_decrypt(raw, e["hash"], mask, offset, layer)
                if D is not None:
                    t, _ = decode_ks(D)
                    sc = _kag_score(t) if t is not None else 0
                    if best is None or sc > best[0]:
                        best = (sc, D, how, name)
        if best is None:
            failed += 1
            print("  ✗ %s（VM/常量模型都解不出）" % rel)
            continue
        _, D, how, name = best
        # 输出路径与解包目录结构对齐（相对游戏根，含 原始归档解包/ 或 raw/ 前缀）
        rel_g = os.path.relpath(p, gdir)
        outp = os.path.join(out_root, rel_g)
        os.makedirs(os.path.dirname(outp), exist_ok=True)
        with open(outp, "wb") as w:
            w.write(D)
        fixed += 1
    print("%s: 解不动 %d → 修复 %d，失败 %d；输出 %s"
          % (game, len(targets), fixed, failed, out_root))
    return fixed, failed


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("game", nargs="?", help="只跑某一章")
    args = ap.parse_args()
    games = [args.game] if args.game else list(CHAPTERS)
    tf = tl = 0
    for g in games:
        if g not in CHAPTERS:
            print("未知游戏: %s" % g)
            continue
        fx, fl = run_game(g)
        tf += fx
        tl += fl
    print("合计: 修复 %d，失败 %d" % (tf, tl))


if __name__ == "__main__":
    main()
