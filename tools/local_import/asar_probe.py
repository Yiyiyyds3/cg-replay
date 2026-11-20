#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""Electron portable exe 内嵌 asar 探针：搜 {"files" JSON 目录特征。"""
import sys, os, json, struct

def probe(path):
    size = os.path.getsize(path)
    hits = []
    f = open(path, 'rb')
    # 分块搜 {"files": 与 "package.json" 字符串
    CH = 8 * 1024 * 1024
    overlap = 64
    pos = 0
    prev = b''
    names = 0
    while True:
        buf = f.read(CH)
        if not buf: break
        data = prev + buf
        base = pos - len(prev)
        i = data.find(b'{"files":{')
        if i >= 0:
            hits.append(base + i)
        j = data.find(b'"package.json"')
        if j >= 0:
            names += 1
        pos += len(buf)
        prev = buf[-overlap:]
    f.close()
    out = {'file': os.path.basename(path), 'size_mb': round(size/1048576,1),
           'files_json_hits': hits[:3], 'package_json_refs': names}
    if hits:
        f = open(path, 'rb')
        f.seek(hits[0] - 16)
        head = f.read(64)
        f.close()
        out['around_first_hit_hex'] = head.hex()
        # 尝试按 asar pickle 解析：hits[0]-16 处应有 u32 链
        try:
            b = head
            # 8字节对齐探测 pickle sizes
            out['maybe_pickle_sizes'] = [struct.unpack('<I', b[k:k+4])[0] for k in range(0, 16, 4) if k+4 <= len(b)]
        except Exception as e:
            out['pickle_err'] = str(e)
        # 抓 JSON 前缀
        f = open(path, 'rb')
        f.seek(hits[0])
        js = f.read(400)
        f.close()
        out['json_head'] = js.decode('utf-8', 'replace')[:300]
    return out

if __name__ == '__main__':
    for p in sys.argv[1:]:
        print(json.dumps(probe(p), ensure_ascii=False, indent=1))
