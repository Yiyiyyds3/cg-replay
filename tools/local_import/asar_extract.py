#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""Electron asar 提取器（标准库）。asar 头：8B pickle(u32=4, u32=header_size, u32=header_obj_size, u32=header_str_size) + JSON 文件表 + 数据区。
用法: asar_extract.py <asar路径> <输出目录> [--list]"""
import struct, os, sys, json

def read_header(f):
    f.seek(0)
    s1, s2, s3, s4 = struct.unpack('<4I', f.read(16))
    js = f.read(s4).decode('utf-8', 'replace')
    data_base = 8 + s2  # pickle(8) + header string size
    return json.loads(js.rstrip('\x00')), data_base

def walk(node, prefix, out, base):
    for name, meta in (node.get('files') or {}).items():
        p = prefix + '/' + name
        if 'files' in meta:
            walk(meta, p, out, base)
        elif 'offset' in meta:
            out.append((p, base + int(meta['offset']), meta['size']))

def main():
    src, dst = sys.argv[1], sys.argv[2]
    list_only = '--list' in sys.argv
    f = open(src, 'rb')
    hdr, base = read_header(f)
    files = []
    walk(hdr, '', files, base)
    print('FILES=%d data_base=%d' % (len(files), base))
    if list_only:
        for p, off, size in files[:80]:
            print('  %-60s %8d' % (p, size))
        return
    for p, off, size in files:
        rel = p.lstrip('/')
        dest = os.path.join(dst, rel.replace('/', os.sep))
        if size == 0:
            os.makedirs(dest, exist_ok=True)
            continue
        os.makedirs(os.path.dirname(dest), exist_ok=True)
        f.seek(off)
        remaining = size
        with open(dest, 'wb') as g:
            while remaining > 0:
                chunk = f.read(min(1 << 20, remaining))
                if not chunk: break
                g.write(chunk)
                remaining -= len(chunk)
    f.close()
    print('DONE -> ' + dst)

if __name__ == '__main__':
    main()
