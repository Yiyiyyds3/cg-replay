#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""把 _carved 的 PNG/JPEG 打包成 cg-index.json CG 库（unity 形态）并验证。"""
import json, os, sys, time, shutil, hashlib

SRC = r"D:\UCGR_Work\tyrano\光辉的少女们\_carved"
MAN = r"D:\UCGR_Work\tyrano\光辉的少女们\_carve_manifest.json"
OUT = r"D:\UCGR_Work\tyrano\cglib"
GAME = "光辉的少女们"

def safe(s):
    import re
    s = re.sub(r'[<>:"/\\|?*\x00-\x1f]', '_', s or '')
    return (s or 'unnamed')[:80]

def main():
    files = []
    for dp, _, fns in os.walk(SRC):
        for fn in fns:
            if fn.lower().endswith(('.png', '.jpg', '.jpeg', '.webp')):
                files.append(os.path.join(dp, fn))
    print('CARVED_IMAGES=%d' % len(files))
    os.makedirs(os.path.join(OUT, 'cg'), exist_ok=True)
    cgs, seen = [], set()
    for p in sorted(files):
        rel = os.path.relpath(p, SRC).replace('/', '_').replace('\\', '_')
        cid = safe(os.path.splitext(rel)[0])
        n = 1
        base = cid
        while cid in seen:
            n += 1
            cid = base + '_' + str(n)
        seen.add(cid)
        ext = os.path.splitext(p)[1].lower()
        d = os.path.join(OUT, 'cg', cid)
        os.makedirs(d, exist_ok=True)
        fn = cid + ext
        shutil.copyfile(p, os.path.join(d, fn))
        cgs.append({'id': cid, 'kind': 'still', 'category': '未分类', 'event_group': None,
                    'characters': [], 'files': {'png': [fn]}})
    total = sum(os.path.getsize(os.path.join(dp, f)) for dp, _, fs in os.walk(os.path.join(OUT, 'cg')) for f in fs)
    idx = {'schema': 1, 'engine': 'unity', 'game': GAME,
           'generated_at': time.strftime('%Y-%m-%dT%H:%M:%S'),
           'source': {'carved': 'runtime.rts SRTS container carve'},
           'counts': {'cg': len(cgs), 'png': len(cgs), 'text': 0, 'spine': 0, 'frames': 0,
                      'still': len(cgs), 'orphan_png': 0, 'orphan_text': 0},
           'bytes': total, 'cgs': cgs,
           'notes': ['tyrano_srts_carve: 剧本层 data.dts 流密码未破，本库为 carved CG 图库']}
    with open(os.path.join(OUT, 'cg-index.json'), 'w', encoding='utf-8') as g:
        json.dump(idx, g, ensure_ascii=False, indent=2)
    print('DONE cgs=%d bytes=%.1fMB' % (len(cgs), total / 1048576))

if __name__ == '__main__':
    main()
