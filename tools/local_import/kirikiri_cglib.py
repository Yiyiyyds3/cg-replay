#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""把 KiriKiri 解包产物（raw/data/image/<role>/**）打包成 cg-index.json CG 库。

与 tyrano_cglib.py 同结构（schema=1 / engine=unity / kind=still），差别只在输入：
KiriKiri 解出来的图是 .png/.bmp/.tlg 混排，这里统一转 PNG：

  .png            -> 原样复制（不重编码，避免无谓损失）
  .bmp/.jpg/.webp -> PIL 转存 PNG
  .tlg            -> 交给 ucgr decode（项目自己的 internal/decode 解码器）；
                     TLG0 是「15 字节头 + 内嵌 TLG5 流」的封装，先剥壳再交给它

图片按名字分组：文件名里的【…】前缀就是原作自己的分类（【イベントCG】/【立ち絵】/
【背景】…），作为 category；CG id 用「role_文件名」清洗后去重，保证可复现。

用法：
  python kirikiri_cglib.py --src <解包目录> --out <库目录> [--game 名字] [--ucgr bin/ucgr.exe]
"""
import argparse, json, os, re, shutil, subprocess, sys, time

IMG_EXTS = ('.png', '.bmp', '.tlg', '.jpg', '.jpeg', '.webp')
CAT_RE = re.compile(r'^【([^】]+)】')
TLG0_MAGIC = b'TLG0.0' + bytes([0])   # 前 7 字节：TLG0.0 + NUL


def safe(s, limit=90):
    s = re.sub(r'[<>:"/\\|?*\x00-\x1f]', '_', s or '')
    s = s.strip(' .')
    return (s or 'unnamed')[:limit]


def category_of(name):
    m = CAT_RE.match(name)
    if m:
        return m.group(1)
    return '未分类'


def run_ucgr_decode(ucgr, src, dst):
    r = subprocess.run([ucgr, 'decode', src, '--out', dst],
                       capture_output=True, text=True, timeout=300,
                       encoding='utf-8', errors='replace')
    return r.returncode == 0 and os.path.exists(dst), ((r.stdout or '') + (r.stderr or '')).strip()


def convert(src, dst, ucgr, failures):
    ext = os.path.splitext(src)[1].lower()
    if ext == '.png':
        shutil.copyfile(src, dst)
        return True
    if ext == '.tlg':
        with open(src, 'rb') as f:
            head = f.read(15)
        if head[:7] == TLG0_MAGIC:
            # TLG0 = 11 字节头 + 4 字节偏移 + 内嵌 TLG5；剥出内层再说。
            with open(src, 'rb') as f:
                f.seek(15)
                inner = f.read()
            tmp = dst + '.inner.tlg'
            with open(tmp, 'wb') as f:
                f.write(inner)
            try:
                ok, why = run_ucgr_decode(ucgr, tmp, dst)
            finally:
                if os.path.exists(tmp):
                    os.remove(tmp)
            if not ok:
                failures.append((src, 'TLG0->TLG5: ' + why[:160]))
            return ok
        ok, why = run_ucgr_decode(ucgr, src, dst)
        if not ok:
            failures.append((src, why[:160]))
        return ok
    try:
        from PIL import Image
        with Image.open(src) as im:
            im.load()
            if im.mode not in ('RGB', 'RGBA', 'L', 'LA'):
                im = im.convert('RGBA')
            im.save(dst, 'PNG')
        return True
    except Exception as e:
        failures.append((src, 'PIL: %s' % e))
        return False


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument('--src', required=True)
    ap.add_argument('--out', required=True)
    ap.add_argument('--game', required=True)
    ap.add_argument('--ucgr', default=os.path.join(os.getcwd(), 'bin', 'ucgr.exe'))
    ap.add_argument('--image-root', default=os.path.join('raw', 'data', 'image'))
    args = ap.parse_args()

    root = os.path.join(args.src, args.image_root)
    if not os.path.isdir(root):
        print('NO_IMAGE_ROOT %s' % root)
        return 2

    files = []
    for dp, _, fns in os.walk(root):
        for fn in fns:
            if fn.lower().endswith(IMG_EXTS):
                files.append(os.path.join(dp, fn))
    files.sort()
    print('SOURCE_IMAGES=%d' % len(files))

    os.makedirs(os.path.join(args.out, 'cg'), exist_ok=True)
    cgs, seen, failures = [], set(), []
    per_cat, per_src_ext = {}, {}
    for p in files:
        rel = os.path.relpath(p, root).replace('\\', '/')
        role = rel.split('/')[0] if '/' in rel else 'misc'
        name = os.path.splitext(os.path.basename(p))[0]
        cat = category_of(name)
        base_id = safe(role + '_' + name)
        cid, n = base_id, 1
        while cid in seen:
            n += 1
            cid = '%s_%d' % (base_id, n)
        seen.add(cid)
        d = os.path.join(args.out, 'cg', cid)
        os.makedirs(d, exist_ok=True)
        fn = cid + '.png'
        if not convert(p, os.path.join(d, fn), args.ucgr, failures):
            shutil.rmtree(d, ignore_errors=True)
            continue
        per_cat[cat] = per_cat.get(cat, 0) + 1
        sx = os.path.splitext(p)[1].lower()
        per_src_ext[sx] = per_src_ext.get(sx, 0) + 1
        cgs.append({'id': cid, 'kind': 'still', 'category': cat, 'event_group': None,
                    'characters': [], 'files': {'png': [fn]}, 'source': rel})

    total = sum(os.path.getsize(os.path.join(dp, f))
                for dp, _, fs in os.walk(os.path.join(args.out, 'cg')) for f in fs)
    idx = {'schema': 1, 'engine': 'unity', 'kind': 'still', 'game': args.game,
           'generated_at': time.strftime('%Y-%m-%dT%H:%M:%S'),
           'source': {'unpack': args.src, 'images': len(files), 'by_ext': per_src_ext},
           'counts': {'cg': len(cgs), 'png': len(cgs), 'text': 0, 'spine': 0,
                      'frames': 0, 'still': len(cgs), 'orphan_png': 0, 'orphan_text': 0},
           'bytes': total, 'categories': per_cat, 'cgs': cgs,
           'notes': ['kirikiri_all_images: 图片按归档全量解（Options.AllImages），'
                     'tlg 走 ucgr decode（TLG0 先剥壳），bmp 走 PIL']}
    with open(os.path.join(args.out, 'cg-index.json'), 'w', encoding='utf-8') as g:
        json.dump(idx, g, ensure_ascii=False, indent=2)
    print('DONE cgs=%d bytes=%.1fMB categories=%d' % (len(cgs), total / 1048576.0, len(per_cat)))
    print('BY_EXT=%s' % json.dumps(per_src_ext, ensure_ascii=False, sort_keys=True))
    print('CATEGORIES=%s' % json.dumps(per_cat, ensure_ascii=False, sort_keys=True))
    if failures:
        print('CONVERT_FAILED=%d' % len(failures))
        for s, why in failures[:10]:
            print('  FAIL %s :: %s' % (os.path.basename(s), why))
    return 0


if __name__ == '__main__':
    sys.exit(main())
