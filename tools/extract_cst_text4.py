# -*- coding: utf-8 -*-
"""
CatSystem2 .cst 文本提取器（v4：UTF-8 / GB18030 / cp932 三向判别 + 转义还原）
条目结构: 01 <type> <text bytes> 00
  type 0x20 = 对话正文 / 0x21 = 角色名 / 0x30 = 脚本命令

编码判别：
  1) 先试 UTF-8（整文件成功率 >= 0.9 即采用）
  2) 否则用 cp932 解码，比较「半角片假名占比」（GBK 被误读为 cp932 的典型特征）
     与「平假名占比」（真日文特征）—— 半角片假名占优则判为 GB18030
  3) 其余判为 cp932

转义还原：
  部分汉化/机翻流水线把字符写成 `\\$<十进制码点>;`（另有 &#NNN; / &#xHHHH; 形式），
  提取时统一还原为真实字符。

用法: python extract_cst_text4.py <inDir> <outDir> [--cmd]
"""
import sys, os, re, struct, zlib

TYPE_TEXT = 0x20
TYPE_NAME = 0x21
TYPE_CMD = 0x30
VALID_TYPES = (TYPE_TEXT, TYPE_NAME, TYPE_CMD)

ESC_DEC = re.compile(r'\\\$\s*(\d{1,7})\s*;')
ESC_ENT_DEC = re.compile(r'&#(\d{1,7});')
ESC_ENT_HEX = re.compile(r'&#[xX]([0-9A-Fa-f]{1,6});')


def unescape(s):
    """还原 \\$<十进制码点>; —— BMP 直接转字符；成对的 UTF-16 代理项合并为增补平面字符。"""
    out = []
    i = 0
    while True:
        m = ESC_DEC.search(s, i)
        if not m:
            out.append(s[i:])
            break
        out.append(s[i:m.start()])
        v = int(m.group(1))
        end = m.end()
        if 0xD800 <= v <= 0xDBFF:
            m2 = ESC_DEC.match(s, end)
            v2 = int(m2.group(1)) if m2 else -1
            if 0xDC00 <= v2 <= 0xDFFF:
                out.append(chr(0x10000 + ((v - 0xD800) << 10) + (v2 - 0xDC00)))
                i = m2.end()
                continue
            out.append(m.group(0))
        elif 0xDC00 <= v <= 0xDFFF or v > 0x10FFFF:
            out.append(m.group(0))
        else:
            out.append(chr(v))
        i = end
    s = ''.join(out)
    s = ESC_ENT_DEC.sub(lambda m: chr(int(m.group(1))), s)
    s = ESC_ENT_HEX.sub(lambda m: chr(int(m.group(1), 16)), s)
    return s


def decompress_cst(path):
    with open(path, 'rb') as fp:
        d = fp.read()
    if len(d) < 16 or d[:8] != b'CatScene':
        return None
    comp = struct.unpack_from('<I', d, 8)[0]
    body = d[16:16 + comp] if comp and comp <= len(d) - 16 else d[16:]
    try:
        return zlib.decompress(body)
    except Exception:
        return None


def raw_entries(raw):
    out = []
    start = 0
    n = len(raw)
    for i in range(n):
        if raw[i] != 0:
            continue
        run = raw[start:i]
        start = i + 1
        if len(run) < 3 or run[0] != 0x01 or run[1] not in VALID_TYPES:
            continue
        out.append((run[1], run[2:]))
    return out


def bad_char(s):
    return '\ufffd' in s or any(ord(c) < 0x20 for c in s)


def pick_encoding(payloads):
    good = 0
    for b in payloads:
        try:
            s = b.decode('utf-8')
        except Exception:
            continue
        if not bad_char(s):
            good += 1
    if payloads and good / len(payloads) >= 0.9:
        return 'utf-8'

    half = hira = total = 0
    for b in payloads:
        s = b.decode('cp932', 'ignore')
        for c in s:
            o = ord(c)
            total += 1
            if 0xFF61 <= o <= 0xFF9F:
                half += 1
            elif 0x3040 <= o <= 0x309F:
                hira += 1
    if total:
        hr, jr = half / total, hira / total
        if hr > 0.02 and hr >= jr:
            return 'gb18030'
    return 'cp932'


def parse(entries, enc):
    out = []
    for t, b in entries:
        try:
            s = b.decode(enc)
        except Exception:
            s = b.decode(enc, 'replace')
        if bad_char(s):
            continue
        out.append((t, unescape(s)))
    return out


def write_txt(entries, dst, src_name, raw_len, with_cmd, enc):
    lines = ['# %s' % src_name,
             '# 解压后大小: %d 字节   条目数: %d   编码: %s' % (raw_len, len(entries), enc),
             '']
    for t, s in entries:
        s = s.replace('\r', ' ').replace('\n', ' ')
        if t == TYPE_TEXT:
            lines.append('[正文] ' + s)
        elif t == TYPE_NAME:
            lines.append('[名字] ' + s)
        elif t == TYPE_CMD and with_cmd:
            lines.append('[命令] ' + s)
    with open(dst, 'w', encoding='utf-8-sig', newline='\r\n') as fp:
        fp.write('\n'.join(lines))


def main():
    if len(sys.argv) < 3:
        print(__doc__)
        return 1
    src_dir, out_dir = sys.argv[1], sys.argv[2]
    with_cmd = '--cmd' in sys.argv
    os.makedirs(out_dir, exist_ok=True)

    files = []
    for root, _dirs, names in os.walk(src_dir):
        for nm in names:
            if nm.lower().endswith('.cst'):
                files.append(os.path.join(root, nm))
    files.sort()

    ok = fail = 0
    enc_stat = {}
    tot_entries = tot_text = 0
    for i, f in enumerate(files, 1):
        raw = decompress_cst(f)
        if raw is None:
            fail += 1
            continue
        re_ = raw_entries(raw)
        enc = pick_encoding([b for _t, b in re_])
        enc_stat[enc] = enc_stat.get(enc, 0) + 1
        entries = parse(re_, enc)
        rel = os.path.relpath(f, src_dir)
        dst = os.path.join(out_dir, os.path.splitext(rel)[0] + '.txt')
        os.makedirs(os.path.dirname(dst), exist_ok=True)
        write_txt(entries, dst, os.path.basename(f), len(raw), with_cmd, enc)
        ok += 1
        tot_entries += len(entries)
        tot_text += sum(1 for t, _ in entries if t == TYPE_TEXT)
        if i % 500 == 0:
            print('  %d/%d  ok=%d fail=%d' % (i, len(files), ok, fail))
    print('DONE files=%d ok=%d fail=%d entries=%d dialogue=%d enc=%s' %
          (len(files), ok, fail, tot_entries, tot_text, enc_stat))
    return 0


if __name__ == '__main__':
    sys.exit(main())
