#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""Godot PCK 探针：读文件表，打印路径样本与统计。标准库 only。"""
import struct, sys, os

def read_pck(path):
    f = open(path, 'rb')
    magic = f.read(4)
    assert magic == b'GDPC', 'not GDPC: %r' % magic
    pack_ver, major, minor, patch = struct.unpack('<4I', f.read(16))
    entries = []
    if pack_ver == 1:
        # 16 reserved u32
        f.read(64)
        (count,) = struct.unpack('<I', f.read(4))
        for _ in range(count):
            (plen,) = struct.unpack('<I', f.read(4))
            p = f.read(plen).rstrip(b'\x00').decode('utf-8', 'replace')
            off, size = struct.unpack('<QQ', f.read(16))
            md5 = f.read(16)
            entries.append((p, off, size))
    elif pack_ver == 2:
        pack_flags, = struct.unpack('<I', f.read(4))
        file_base, = struct.unpack('<Q', f.read(8))
        f.read(64)  # 16 reserved
        (count,) = struct.unpack('<I', f.read(4))
        for _ in range(count):
            (plen,) = struct.unpack('<I', f.read(4))
            p = f.read(plen).rstrip(b'\x00').decode('utf-8', 'replace')
            off, size = struct.unpack('<QQ', f.read(16))
            md5 = f.read(16)
            flags, = struct.unpack('<I', f.read(4))
            if pack_flags & 2:  # PACK_REL_FILEBASE? offset relative
                off += file_base
            entries.append((p, off, size, flags))
    f.close()
    return pack_ver, major, minor, patch, entries

def ext_stats(entries):
    from collections import Counter
    c = Counter()
    for p, off, size, *rest in entries:
        e = os.path.splitext(p)[1].lower()
        c[e] += 1
    return c.most_common(15)

def sample(entries, pred, n):
    out = []
    for p, off, size, *rest in entries:
        if pred(p):
            out.append((p, size))
            if len(out) >= n: break
    return out

if __name__ == '__main__':
    path = sys.argv[1]
    pv, ma, mi, pa, entries = read_pck(path)
    print('PCK %s: pack_ver=%d godot=%d.%d.%d entries=%d' % (os.path.basename(path), pv, ma, mi, pa, len(entries)))
    print('EXT:', ext_stats(entries))
    total = sum(e[2] for e in entries)
    print('TOTAL_BYTES=%.1fMB' % (total / 1048576))
    print('--- image-ish paths (first 30) ---')
    for p, size in sample(entries, lambda p: any(k in p.lower() for k in ('cg', 'image', 'event', 'hscene', 'picture')) and os.path.splitext(p)[1].lower() in ('.ctex', '.tex', '.png', '.webp'), 30):
        print('  %s %.1fKB' % (p, size / 1024))
    print('--- first 20 entries ---')
    for p, size in sample(entries, lambda p: True, 20):
        print('  %s %.1fKB' % (p, size / 1024))
