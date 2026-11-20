#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""Godot pck CG 导出器：GDPC v1/v2 文件表 -> .stex/.ctex 切 PNG/WebP -> cg-index.json（unity 导出器同构）。
只读 pck；产物写 --out。标准库 only。"""
import struct, os, sys, json, hashlib, re, argparse

PNG_SIG = b'\x89PNG\r\n\x1a\n'
WEBP_SIG = b'RIFF'

def png_length(buf, pos):
    """从 pos(PNG 签名) 起按 chunk 走到 IEND，返回结束偏移；失败返回 -1。"""
    p = pos + 8
    total = len(buf)
    for _ in range(4096):
        if p + 8 > total: return -1
        (ln,) = struct.unpack('>I', buf[p:p+4])
        typ = buf[p+4:p+8]
        p += 8 + ln + 4
        if typ == b'IEND':
            return p
        if ln > total:
            return -1
    return -1

def webp_length(buf, pos):
    if buf[pos+8:pos+12] != b'WEBP': return -1
    (ln,) = struct.unpack('<I', buf[pos+4:pos+8])
    return pos + 8 + ln

def extract_image(buf):
    """在 buffer 里找 PNG/WebP payload，返回 (ext, data)。"""
    png = buf.find(PNG_SIG)
    if png >= 0:
        end = png_length(buf, png)
        if end > 0:
            return '.png', buf[png:end]
    wp = buf.find(WEBP_SIG)
    if wp >= 0 and len(buf) > wp+12:
        end = webp_length(buf, wp)
        if end > 0:
            return '.webp', buf[wp:end]
    return None, None

def read_pck(path):
    f = open(path, 'rb')
    assert f.read(4) == b'GDPC'
    pack_ver, major, minor, patch = struct.unpack('<4I', f.read(16))
    entries = []
    if pack_ver == 1:
        f.read(64)
        (count,) = struct.unpack('<I', f.read(4))
        for _ in range(count):
            (plen,) = struct.unpack('<I', f.read(4))
            p = f.read(plen).rstrip(b'\x00').decode('utf-8','replace')
            off, size = struct.unpack('<QQ', f.read(16))
            f.read(16)
            entries.append((p, off, size))
    elif pack_ver == 2:
        pack_flags, = struct.unpack('<I', f.read(4))
        file_base, = struct.unpack('<Q', f.read(8))
        f.read(64)
        (count,) = struct.unpack('<I', f.read(4))
        for _ in range(count):
            (plen,) = struct.unpack('<I', f.read(4))
            p = f.read(plen).rstrip(b'\x00').decode('utf-8','replace')
            off, size = struct.unpack('<QQ', f.read(16))
            f.read(16)
            flags, = struct.unpack('<I', f.read(4))
            if flags & 1:
                continue  # encrypted, skip
            if pack_flags & 2:
                off += file_base
            entries.append((p, off, size))
    else:
        raise SystemExit('unsupported pack version %d' % pack_ver)
    f.close()
    return entries

# 导入产物名: res://.godot/imported/<orig>.png-<md5>.ctex  或  res://.import/<orig>.png-<md5>.stex
NAME_RE = re.compile(r'/(?:\.godot/imported|\.import)/(.+)\.(png|jpg|jpeg|webp)-[0-9a-f]{16,32}\.(ctex|stex)$')

def main():
    ap = argparse.ArgumentParser()
    ap.add_argument('pck', nargs='+')
    ap.add_argument('--out', required=True)
    ap.add_argument('--game', required=True)
    ap.add_argument('--prefix', default='', help='只导名字以该前缀开头的图（对原始 png 名匹配，逗号分隔可多个）')
    ap.add_argument('--list-only', action='store_true')
    a = ap.parse_args()
    prefixes = [p for p in a.prefix.split(',') if p] if a.prefix else []
    picked = {}
    for pk in a.pck:
        for p, off, size in read_pck(pk):
            m = NAME_RE.search(p)
            if not m: continue
            orig = m.group(1)
            stem = os.path.splitext(orig)[0]
            if prefixes and not any(stem.lower().startswith(x.lower()) for x in prefixes):
                continue
            picked.setdefault((pk, orig), (off, size, m.group(3)))
    print('PICKED=%d' % len(picked))
    if a.list_only:
        names = sorted(set(o for (_, o), _v in picked.items()))
        for n in names[:200]:
            print('  ' + n)
        print('TOTAL_UNIQUE=%d' % len(names))
        return
    os.makedirs(os.path.join(a.out, 'cg'), exist_ok=True)
    cgs = []
    fail = 0
    seen = set()
    for (pk, orig), (off, size, kind) in sorted(picked.items()):
        f = open(pk, 'rb')
        f.seek(off)
        buf = f.read(size)
        f.close()
        ext, data = extract_image(buf)
        if not ext:
            fail += 1
            continue
        stem = os.path.splitext(orig)[0]
        cid = stem
        n = 1
        while cid in seen:
            n += 1
            cid = '%s_%d' % (stem, n)
        seen.add(cid)
        d = os.path.join(a.out, 'cg', cid)
        os.makedirs(d, exist_ok=True)
        fn = cid + ext
        with open(os.path.join(d, fn), 'wb') as g:
            g.write(data)
        cgs.append({'id': cid, 'kind': 'still', 'category': '未分类', 'event_group': None,
                    'characters': [], 'files': {'png': [fn]}})
    total_bytes = sum(os.path.getsize(os.path.join(dp, fn)) for dp, _, fns in os.walk(os.path.join(a.out, 'cg')) for fn in fns)
    idx = {
        'schema': 1, 'engine': 'unity', 'game': a.game,
        'generated_at': __import__('time').strftime('%Y-%m-%dT%H:%M:%S'),
        'source': {'pck': [os.path.basename(x) for x in a.pck]},
        'counts': {'cg': len(cgs), 'png': len(cgs), 'text': 0, 'spine': 0, 'frames': 0, 'still': len(cgs), 'orphan_png': 0, 'orphan_text': 0},
        'bytes': total_bytes, 'cgs': cgs,
        'notes': ['godot_cg_export: magic-slice PNG/WebP from .ctex/.stex; fail=%d' % fail],
    }
    with open(os.path.join(a.out, 'cg-index.json'), 'w', encoding='utf-8') as g:
        json.dump(idx, g, ensure_ascii=False, indent=2)
    print('DONE cgs=%d fail=%d bytes=%.1fMB' % (len(cgs), fail, total_bytes/1048576))

if __name__ == '__main__':
    main()
