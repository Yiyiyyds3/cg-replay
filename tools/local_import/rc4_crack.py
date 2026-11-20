#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""RC4 key 穷举：keystream[0:8]=0d f6 2d b7 c9 74 5e 00（PNG magic XOR 载荷头）。
候选：game.exe/LAA_BAK 里的 ASCII 串、UTF-16LE 串、二进制滑窗（可限长）。"""
import sys, os, struct, re

KS = bytes.fromhex('0df62db7c9745e00')  # 8 bytes known keystream

def rc4_prga_head(key, n=8):
    S = list(range(256))
    j = 0
    for i in range(256):
        j = (j + S[i] + key[i % len(key)]) & 0xFF
        S[i], S[j] = S[j], S[i]
    out = bytearray()
    i = j = 0
    for _ in range(n):
        i = (i + 1) & 0xFF
        j = (j + S[i]) & 0xFF
        S[i], S[j] = S[j], S[i]
        out.append(S[(S[i] + S[j]) & 0xFF])
    return bytes(out)

def candidates(buf):
    # ASCII printable runs >= 4
    for m in re.finditer(rb'[\x20-\x7e]{4,80}', buf):
        yield m.group()
        # strip trailing/leading common quotes
    # UTF-16LE runs: printable char + \x00 pattern
    for m in re.finditer(rb'(?:[\x20-\x7e]\x00){4,40}', buf):
        yield m.group().decode('utf-16le', 'replace').encode('utf-8')

def try_all(keys, label):
    n = 0
    for k in keys:
        if not (1 <= len(k) <= 128):
            continue
        n += 1
        if rc4_prga_head(k) == KS:
            print('HIT key=%r (len=%d) from %s' % (k, len(k), label))
            return k
    print('%s: tried %d candidates, no hit' % (label, n))
    return None

if __name__ == '__main__':
    exe = sys.argv[1]
    buf = open(exe, 'rb').read()
    hit = try_all(candidates(buf), os.path.basename(exe) + ':ascii/utf16')
    if not hit and '--deep' in sys.argv:
        # 二进制滑窗 8..32 字节（慢）——只扫非全零区
        found = False
        for L in (8, 16, 32):
            for off in range(0, len(buf) - L, 1):
                w = buf[off:off + L]
                if w.count(0) > L // 2:
                    continue
                if rc4_prga_head(w) == KS:
                    print('HIT sliding key=%s at off=0x%x len=%d' % (w.hex(), off, L))
                    found = True
                    break
            if found: break
            print('sliding len=%d done, no hit' % L)
