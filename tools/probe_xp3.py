# -*- coding: utf-8 -*-
"""
XP3 探查器（只读，不写文件）
============================
结构依据 GARbro 的 ArcXP3.cs（本目录 ArcXP3_dl.cs），不是猜的：

    dir_offset = base + int64(base + 0x0B)
    if u32(dir_offset) == 0x80:                 # 重定向层
        dir_offset = base + int64(dir_offset + 9)

    header_type = u8(dir_offset)
      0 -> 明文索引: header_size = int64(+1);   index = 原始字节 [+9, +9+size)
      1 -> zlib索引: packed = int64(+1);        index = zlib解压 [+17, +17+packed)

    index = 若干 (u32 magic, i64 size, payload)
      "File"(0x656C6946) 内再分 section：
        "info"(0x6f666e69): u32 IsEncrypted, i64 file_size, i64 packed_size,
                            u16 name_len, name(name_len*2, UTF-16LE)
        "segm"(0x6d676573): 每条 28 字节 = i32 compressed + i64 off + i64 size + i64 packed
        "adlr"(0x726c6461): u32 hash
      "hnfn"(0x6E666E68):   u32 hash + u16 name_len + name(UTF-16LE)

用法:
    python gar_tool/probe_xp3.py <xxx.xp3> [--samples 8] [--head 32]
"""
import argparse
import os
import struct
import sys
import zlib
from collections import Counter

try:
    sys.stdout.reconfigure(encoding="utf-8")
except Exception:
    pass

FILE_SIG = 0x656C6946
INFO_SIG = 0x6F666E69
SEGM_SIG = 0x6D676573
ADLR_SIG = 0x726C6461
HNFN_SIG = 0x6E666E68

SIGS = [
    ("png", b"\x89PNG\x0d\x0a\x1a\x0a"),
    ("ogg", b"OggS"),
    ("tlg5", b"TLG5.0\x00raw\x1a"),
    ("tlg6", b"TLG6.0\x00raw\x1a"),
    ("tlg0", b"TLG0.0\x00sds\x1a"),
    ("ttf", b"\x00\x01\x00\x00"),
    ("otf", b"OTTO"),
    ("mpg", b"\x00\x00\x01\xba"),
    ("wav", b"RIFF"),
]

TEXT_EXT = {"ks", "tjs", "scn", "asd", "sli", "func", "txt", "ini", "csv", "stand", "xml"}


def is_zlib(b):
    return len(b) >= 2 and (b[0] & 0x0F) == 8 and ((b[0] << 8 | b[1]) % 31 == 0)


def read_index(f):
    """返回 (index_bytes, 说明) 或 (None, 错误)"""
    f.seek(0)
    head = f.read(0x20)
    if head[:3] != b"XP3":
        if head[:2] == b"MZ":
            return None, "PE 头（exe 内嵌 xp3），本脚本未处理，请用 GARbro"
        return None, "不是 XP3"
    dir_offset = struct.unpack_from("<q", head, 0x0B)[0]
    fsize = os.path.getsize(f.name)
    if dir_offset < 0x13 or dir_offset >= fsize:
        return None, "dir_offset 越界: %d" % dir_offset
    f.seek(dir_offset)
    if struct.unpack("<I", f.read(4))[0] == 0x80:
        f.seek(dir_offset + 9)
        dir_offset = struct.unpack("<q", f.read(8))[0]
        if dir_offset < 0x13 or dir_offset >= fsize:
            return None, "重定向后 dir_offset 越界: %d" % dir_offset
        redirected = True
    else:
        redirected = False
    f.seek(dir_offset)
    htype = f.read(1)[0]
    if htype == 0:
        hsize = struct.unpack("<q", f.read(8))[0]
        f.seek(dir_offset + 9)
        return f.read(hsize), "明文索引 %d 字节%s" % (hsize, "（有重定向层）" if redirected else "")
    if htype == 1:
        packed = struct.unpack("<q", f.read(8))[0]
        hsize = struct.unpack("<q", f.read(8))[0]
        f.seek(dir_offset + 17)
        raw = f.read(packed)
        table = zlib.decompress(raw)
        return table, "zlib 索引 %d→%d 字节%s" % (packed, hsize, "（有重定向层）" if redirected else "")
    return None, "未知 header_type=%d" % htype


def parse(table, base=0):
    hnfn, entries = {}, []
    p, n = 0, len(table)
    while p + 12 <= n:
        magic = struct.unpack_from("<I", table, p)[0]
        size = struct.unpack_from("<q", table, p + 4)[0]
        p += 12
        if size < 0 or p + size > n:
            break
        pay = table[p:p + size]
        p += size
        if magic == HNFN_SIG and len(pay) >= 6:
            h = struct.unpack_from("<I", pay, 0)[0]
            nl = struct.unpack_from("<H", pay, 4)[0]
            hnfn[h] = pay[6:6 + nl * 2].decode("utf-16-le", "replace")
            continue
        if magic != FILE_SIG:
            continue
        q, left = 0, size
        name = None
        isenc = False
        usize = psize = 0
        hashv = None
        segs = []
        while q + 12 <= size:
            sec = struct.unpack_from("<I", pay, q)[0]
            ssize = struct.unpack_from("<q", pay, q + 4)[0]
            q += 12
            if ssize < 0 or q + ssize > size:
                break
            sp = pay[q:q + ssize]
            if sec == INFO_SIG and len(sp) >= 22:
                isenc = struct.unpack_from("<I", sp, 0)[0] != 0
                usize, psize = struct.unpack_from("<qq", sp, 4)
                nl = struct.unpack_from("<H", sp, 20)[0]
                name = sp[22:22 + nl * 2].decode("utf-16-le", "replace")
            elif sec == SEGM_SIG:
                o = 0
                while o + 28 <= ssize:
                    comp = struct.unpack_from("<i", pay, q + o)[0]
                    off, sz, pk = struct.unpack_from("<qqq", pay, q + o + 4)
                    segs.append((comp, off, sz, pk))
                    o += 28
            elif sec == ADLR_SIG and len(sp) >= 4:
                hashv = struct.unpack_from("<I", sp, 0)[0]
            q += ssize
        if name:
            if hashv is not None and hashv in hnfn:
                name = hnfn[hashv]
            entries.append({
                "name": name, "enc": isenc, "usize": usize, "psize": psize,
                "segs": segs, "hash": hashv,
            })
    return hnfn, entries


def looks_text(b):
    if not b:
        return 0.0
    good = sum(1 for x in b if 0x09 <= x <= 0x0D or 0x20 <= x < 0x7F or x >= 0xA0)
    return good / len(b)


def classify(raw, ext):
    head = raw[:24]
    for nm, sig in SIGS:
        if head[:len(sig)] == sig:
            return "明文", "直接命中 %s 签名" % nm
    if is_zlib(head):
        return "zlib（无 XOR）", "CMF/FLG 校验通过"
    for nm, sig in SIGS:
        k = len(sig)
        for key in range(256):
            if all(head[i] ^ key == sig[i] for i in range(min(k, len(head)))):
                return "单字节 XOR", "key=0x%02x → %s" % (key, nm)
    for nm, sig in SIGS:
        k = min(len(sig), len(head))
        c = Counter(head[i] ^ sig[i] for i in range(k))
        if c:
            key, cnt = c.most_common(1)[0]
            if cnt >= k - 1:
                return "单字节 XOR（头部变体）", "key≈0x%02x（%d/%d 命中 %s）" % (key, cnt, k, nm)
    if ext and ext.lower() in TEXT_EXT:
        best = max(((looks_text(bytes(b ^ k for b in head)), k) for k in range(256)))
        if best[0] > 0.85:
            return "单字节 XOR（文本）", "key=0x%02x 可读率 %.2f" % (best[1], best[0])
        return "疑似 CxEncryption", "文本可读率最高仅 %.2f" % best[0]
    return "疑似 CxEncryption", "前16字节 " + head[:16].hex(" ")


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("xp3")
    ap.add_argument("--samples", type=int, default=8)
    ap.add_argument("--head", type=int, default=24)
    a = ap.parse_args()

    print("文件: %s  (%.1f MB)" % (os.path.basename(a.xp3), os.path.getsize(a.xp3) / 1048576))
    with open(a.xp3, "rb") as f:
        table, info = read_index(f)
        if table is None:
            print("错误:", info)
            return
        print("索引: %s" % info)
        hnfn, ents = parse(table)
        print("条目: %d 个（hnfn 映射 %d 条）" % (len(ents), len(hnfn)))
        exts = Counter(e["name"].rsplit(".", 1)[-1].lower() if "." in e["name"] else "(无后缀)"
                       for e in ents)
        print("扩展名分布: %s" % dict(exts.most_common(15)))
        print("标记 IsEncrypted 的条目: %d / %d" % (sum(1 for e in ents if e["enc"]), len(ents)))

        picked, seen_ext = [], set()
        for w in ["png", "ogg", "tlg", "ks", "scn", "tjs", "asd", "sli", "func", "csv"]:
            for e in ents:
                if e["name"].lower().endswith("." + w) and e["segs"]:
                    picked.append(e)
                    seen_ext.add(w)
                    break
            if len(picked) >= a.samples:
                break
        if not picked:
            picked = [e for e in ents if e["segs"]][:a.samples]

        print("\n--- 样本探测（读前 4KB 判断加密形态）---")
        for e in picked:
            comp, off, sz, pk = e["segs"][0]
            f.seek(off)
            raw = f.read(min(pk, 4096))
            ext = e["name"].rsplit(".", 1)[-1].lower() if "." in e["name"] else ""
            kind, detail = classify(raw, ext)
            print("%-40s %-22s %s" % (e["name"][-40:], kind, detail))
            print("     off=%d size=%d packed=%d comp=%d  head=%s" % (
                off, sz, pk, comp, raw[:a.head].hex(" ")))


if __name__ == "__main__":
    main()
