#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""RPG Maker MV/MZ 加密图（.rpgmvp / .png_）解密镜像工具（纯标准库）。

只读源目录，输出一律镜像到 D:\\UCGR_Work\\decrypt\\<游戏名>\\<相对路径>（扩展名还原为 .png）。
绝不写入游戏源目录、绝不删除任何源文件。

格式（MV/MZ 加密图只有前 16 字节受影响）：
  .rpgmvp : 头 16 字节是固定签名（RPGMV\\0\\0\\0\\x00\\x03\\x01...），
            其后 16 字节 = 原 PNG 前 16 字节 XOR 密钥；密钥 = md5(System.json 的 encryptionKey)。
            由于任何 PNG 的前 16 字节恒为 89 50 4E 47 0D 0A 1A 0A 00 00 00 0D 49 48 44 52，
            密钥也可以直接从文件本身反解出来（密钥源第 4 顺位）。
  .png_   : 头 16 字节直接替换为上述 16 字节 PNG 头，其余原样。

用法：
  python tools/local_import/rpg_decrypt.py --dry-run
  python tools/local_import/rpg_decrypt.py --games ALL
  python tools/local_import/rpg_decrypt.py --games "Lulumachi,时勇者" --max-bytes-per-game 25000000000
"""
import argparse, json, os, re, struct, sys, time, shutil, hashlib, zlib

BASE = os.path.dirname(os.path.abspath(__file__))
REPO = os.path.abspath(os.path.join(BASE, "..", ".."))
DATA = os.path.join(REPO, "reports", "local_import", "data")
SELFCHECK = os.path.join(DATA, "selfcheck.json")
IMPORT = os.path.join(DATA, "import_results.json")
DECRYPT_ROOT = "D:" + os.sep + "UCGR_Work" + os.sep + "decrypt"
DEFAULT_REPORT = os.path.join(DATA, "decrypt_report.json")

PNG_SIG = b"\x89PNG\r\n\x1a\n"
PNG_HDR16 = PNG_SIG + b"\x00\x00\x00\x0dIHDR"
JPEG_SOI = b"\xff\xd8\xff"                       # 部分游戏把 .jpg 也按同样方式加密进 .rpgmvp
CHUNK = 1 << 20
SAMPLE_LIMIT = 3000
BIG_GAME_BYTES = 25 * 1024 ** 3
ENC_EXTS = (".rpgmvp", ".png_")
MAX_HDR_MATCH = "PNG sig+IHDR 16B"

BAD_CHARS = re.compile(r'[<>:"/\\|?*\x00-\x1f]')

def safe_name(n):
    s = BAD_CHARS.sub("_", n).strip().rstrip(". ")
    return s or "unnamed"

def out_rel_name(rel):
    d, f = os.path.split(rel)
    low = f.lower()
    if low.endswith(".rpgmvp"):
        f = f[:-7] + ".png"
    elif low.endswith(".png_"):
        f = f[:-1]
    return os.path.join(d, f) if d else f

def xor(a, b):
    return bytes(x ^ y for x, y in zip(a, b))

# ---------- 密钥发现 ----------

def find_system_json(src_root):
    cands = ["www" + os.sep + "System.json", "System.json",
             "www" + os.sep + "data" + os.sep + "System.json",
             "data" + os.sep + "System.json"]
    for c in cands:
        p = os.path.join(src_root, c)
        if os.path.isfile(p):
            return p
    for dp, dns, fns in os.walk(src_root):
        if "System.json" in fns and len(dp) - len(src_root) < 120:
            return os.path.join(dp, "System.json")
    return None

def game_keys(src_root):
    """返回 [(key_bytes, 短标签), ...]，按可信度排序。"""
    out = []
    sp = find_system_json(src_root)
    if sp:
        try:
            cfg = json.load(open(sp, encoding="utf-8-sig"))
            ek = cfg.get("encryptionKey", "") or ""
            if isinstance(ek, str) and ek:
                if re.fullmatch(r"[0-9a-fA-F]{32}", ek):
                    out.append((bytes.fromhex(ek), "system-hex"))
                out.append((hashlib.md5(ek.encode("utf-8")).digest(), "system-md5"))
        except Exception:
            pass
    out.append((hashlib.md5(b"").digest(), "default-md5"))
    return out

def per_file_key(head32):
    """由文件自身反解密钥：raw[16:32] XOR 标准 PNG 前 16 字节。"""
    return xor(head32[16:32], PNG_HDR16)

# ---------- 单文件解密 ----------

def ihdr_check(path):
    """校验输出前 33 字节：PNG 签名 + IHDR 长度/类型/尺寸/CRC 全部自洽。"""
    try:
        with open(path, "rb") as f:
            b = f.read(33)
    except OSError as e:
        return False, "IO: %s" % e
    if len(b) < 33:
        return False, "文件<33B"
    if b[:8] != PNG_SIG:
        return False, "签名不符"
    if struct.unpack(">I", b[8:12])[0] != 13 or b[12:16] != b"IHDR":
        return False, "IHDR 块头不符"
    w, h = struct.unpack(">II", b[16:24])
    if not (0 < w <= 100000 and 0 < h <= 100000):
        return False, "尺寸非法 %dx%d" % (w, h)
    if (zlib.crc32(b[12:29]) & 0xFFFFFFFF) != struct.unpack(">I", b[29:33])[0]:
        return False, "IHDR CRC 不符"
    return True, "%dx%d" % (w, h)

def decrypt_one(src, dst, kind, keys):
    """返回 (状态, 说明, 用了哪个变体)。状态: ok / plain / fail。

    变体顺序（每个都靠 IHDR CRC 严格校验，失败就换下一个）：
      head16 : 头 16 字节替换为 PNG 头，其余原样（.png_ 直替型）
      head32 : 头 16 字节替换为 PNG 头，跳过紧跟的 16 字节密文头（MV 型 .png_）
      mvkey  : 头 16 字节 = 密钥 XOR 紧随 16 字节（.rpgmvp 规范路径）
    """
    try:
        size = os.path.getsize(src)
        with open(src, "rb") as f:
            head = f.read(48)
    except OSError as e:
        return "fail", "IO: %s" % e, ""
    if size < 16:
        return "fail", "文件<16B", ""

    variants = []
    if kind == "png_":
        variants.append(("head16", PNG_HDR16, 16))
        if size >= 32:
            variants.append(("head32", PNG_HDR16, 32))
    for k, tag in keys:
        if size >= 32:
            variants.append(("mvkey:" + tag, xor(k, head[16:32]), 32))
    if size >= 32:
        variants.append(("mvkey:derived", PNG_HDR16, 32))
        if kind == "rpgmvp":
            variants.append(("head16", PNG_HDR16, 16))

    tried = []
    for label, hdr16, skip in variants:
        fmt = "png" if hdr16[:8] == PNG_SIG else ("jpg" if hdr16[:3] == JPEG_SOI else None)
        if not fmt:
            tried.append("%s:sig" % label)
            continue
        st, why, out = _write(src, dst, hdr16, skip, fmt)
        if st == "ok":
            return "ok", why, label, out
        tried.append("%s:%s" % (label, why))

    # 兜底：源本身就是明文 PNG
    if head[:8] == PNG_SIG:
        st, why = _copy_plain(src, dst)
        return st, why, "源文件已是明文", dst
    return ("fail",
            "变体全失败（%s；载荷前 4 字节=%s，非 PNG/JPEG）"
            % ("; ".join(tried[:4]), head[16:20].hex()), "", "")

def _copy_plain(src, dst):
    os.makedirs(os.path.dirname(dst), exist_ok=True)
    shutil.copyfile(src, dst)
    ok, info = ihdr_check(dst)
    if not ok:
        os.remove(dst)
        return "fail", "明文直通后校验失败(%s)" % info
    return "plain", "源文件已是明文 PNG"

def _write(src, dst, header16, skip, fmt):
    """按认定的格式写出：PNG 校验 IHDR/CRC，JPEG 校验 SOI+EOI。返回 (状态, 说明, 实际输出路径)。"""
    if fmt == "jpg":
        dst = os.path.splitext(dst)[0] + ".jpg"
    os.makedirs(os.path.dirname(dst), exist_ok=True)
    tmp = dst + ".part"
    try:
        with open(src, "rb") as fi, open(tmp, "wb") as fo:
            fo.write(header16)
            fi.seek(skip)
            while True:
                buf = fi.read(CHUNK)
                if not buf:
                    break
                fo.write(buf)
        if fmt == "jpg":
            ok, info = jpeg_check(tmp)
        else:
            ok, info = ihdr_check(tmp)
        if not ok:
            os.remove(tmp)
            return "fail", info, dst
        os.replace(tmp, dst)
        return "ok", info, dst
    except OSError as e:
        try:
            if os.path.exists(tmp):
                os.remove(tmp)
        except OSError:
            pass
        return "fail", "IO: %s" % e

def jpeg_check(path):
    """JPEG 兜底校验：SOI 开头 + EOI 结尾。"""
    try:
        with open(path, "rb") as f:
            f.seek(0, os.SEEK_END)
            n = f.tell()
            f.seek(0)
            head = f.read(3)
            f.seek(max(0, n - 2))
            tail = f.read(2)
    except OSError as e:
        return False, "IO: %s" % e
    if head != JPEG_SOI:
        return False, "JPEG SOI 不符"
    if tail != b"\xff\xd9":
        return False, "JPEG EOI 缺失"
    return True, "jpeg %dB" % n

# ---------- 扫描 ----------

def collect(root):
    a, b = [], []
    for dp, dns, fns in os.walk(root):
        for fn in fns:
            low = fn.lower()
            if not low.endswith(ENC_EXTS):
                continue
            ap = os.path.join(dp, fn)
            try:
                sz = os.path.getsize(ap)
            except OSError:
                continue
            rel = os.path.relpath(ap, root)
            (a if low.endswith(".rpgmvp") else b).append((rel, ap, sz))
    key = lambda t: t[0].lower()
    a.sort(key=key); b.sort(key=key)
    return a, b

def sample_even(items, limit):
    if len(items) <= limit:
        return list(items)
    if limit <= 0:
        return []
    step = len(items) / float(limit)
    return [items[int(i * step)] for i in range(limit)]

def load_targets(games_arg):
    sc = json.load(open(SELFCHECK, encoding="utf-8"))
    ir = json.load(open(IMPORT, encoding="utf-8"))
    paths = {}
    for r in ir.get("results", []):
        paths[r.get("name", "")] = r.get("path", "")
    sus = [v for v in sc.get("verdicts", [])
           if v.get("verdict") == "suspect" and (v.get("encrypted") or 0) > 0]
    if games_arg and games_arg.strip().upper() != "ALL":
        want = [g.strip() for g in games_arg.split(",") if g.strip()]
        sel = []
        for w in want:
            hit = [v for v in sus if v["name"] == w] or [v for v in sus if w.lower() in v["name"].lower()]
            if not hit:
                print("WARN 名单内的款不在 suspect+encrypted 集合里：%s" % w)
            for h in hit:
                if h not in sel:
                    sel.append(h)
        sus = sel
    return [{"name": v["name"], "src_path": paths.get(v["name"], ""),
             "encrypted_prescreen": v.get("encrypted") or 0} for v in sus]

def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--dry-run", action="store_true")
    ap.add_argument("--games", default="ALL")
    ap.add_argument("--max-bytes-per-game", type=int, default=BIG_GAME_BYTES)
    ap.add_argument("--report", default=DEFAULT_REPORT)
    args = ap.parse_args()

    targets = load_targets(args.games)
    if not targets:
        raise SystemExit("没有可处理的款（suspect 且 encrypted>0）")
    print("TARGET games=%d  mode=%s  root=%s  max_bytes_per_game=%.2fGB"
          % (len(targets), "dry-run" if args.dry_run else "decrypt", DECRYPT_ROOT,
             args.max_bytes_per_game / 1024 ** 3), flush=True)

    games = []
    for t in targets:
        name, src = t["name"], t["src_path"]
        rec = {"name": name, "src_path": src, "out_dir": "", "note": [],
               "rpgmvp": 0, "png_": 0, "files_total": 0, "est_bytes": 0,
               "selected_files": 0, "decrypted": 0, "plain_copy": 0,
               "failed": 0, "fail_reasons": {}, "bytes_written": 0,
               "keys_used": {}, "jpg_out": 0, "elapsed_sec": 0.0, "status": ""}
        if not src or not os.path.isdir(src):
            rec["status"] = "src_missing"
            rec["note"].append("源目录不存在: %s" % (src or "(import_results 无此款)"))
            games.append(rec); continue
        mv, pn = collect(src)
        rec["rpgmvp"], rec["png_"] = len(mv), len(pn)
        rec["files_total"] = len(mv) + len(pn)
        est = sum(sz - 16 for _, _, sz in mv if sz >= 16) + sum(sz for _, _, sz in pn)
        rec["est_bytes"] = est
        if rec["files_total"] == 0:
            rec["status"] = "no_encrypted_files"
            rec["note"].append("源目录下没有 .rpgmvp/.png_（可能未加密或已解包）")
            games.append(rec); continue
        selected = [(r, a, s, "rpgmvp") for r, a, s in mv] + [(r, a, s, "png_") for r, a, s in pn]
        selected.sort(key=lambda x: x[0].lower())
        if est > BIG_GAME_BYTES:
            selected = sample_even(selected, SAMPLE_LIMIT)
            rec["note"].append("partial-decrypt: 预计输出 %.2fGB > 25GB，按文件名排序均匀抽样 %d/%d 张"
                               % (est / 1024 ** 3, len(selected), rec["files_total"]))
        rec["selected_files"] = len(selected)
        outdir = os.path.join(DECRYPT_ROOT, safe_name(name))
        rec["out_dir"] = outdir
        if os.path.abspath(outdir).lower().startswith(os.path.abspath(src).lower() + os.sep):
            rec["status"] = "refused_unsafe_outdir"
            rec["note"].append("输出目录落在源目录内，拒绝执行")
            games.append(rec); continue
        rec["status"] = "dry-run" if args.dry_run else "running"
        games.append(rec)
        if args.dry_run:
            continue
        keys = game_keys(src)
        rec["note"].append("密钥候选: " + " | ".join(l for _, l in keys))
        t0 = time.time()
        budget = args.max_bytes_per_game
        for rel, ap_, sz, kind in selected:
            dst = os.path.join(outdir, out_rel_name(rel))
            want = max(0, sz - 16) if kind == "rpgmvp" else sz
            if rec["bytes_written"] + want > budget:
                left = rec["selected_files"] - rec["decrypted"] - rec["plain_copy"] - rec["failed"]
                rec["note"].append("max-bytes-per-game 触顶：已写 %.2fGB，剩余 %d 张未解"
                                   % (rec["bytes_written"] / 1024 ** 3, left))
                break
            st, why, used, out = decrypt_one(ap_, dst, kind, keys)
            if st in ("ok", "plain"):
                if st == "ok":
                    rec["decrypted"] += 1
                    if out.lower().endswith(".jpg"):
                        rec["jpg_out"] += 1
                else:
                    rec["plain_copy"] += 1
                rec["bytes_written"] += want
                rec["keys_used"][used] = rec["keys_used"].get(used, 0) + 1
            else:
                rec["failed"] += 1
                rec["fail_reasons"][why] = rec["fail_reasons"].get(why, 0) + 1
                if rec["failed"] <= 3:
                    rec["note"].append("FAIL %s -> %s" % (rel, why))
        rec["elapsed_sec"] = round(time.time() - t0, 1)
        done = rec["decrypted"] + rec["plain_copy"]
        rec["status"] = "ok" if done == rec["selected_files"] else ("partial" if done else "failed")
        print("  %-40s 选中%-6d 解出%-6d (jpg %-4d) 明文%-4d 失败%-4d %7.2fGB %6.1fs"
              % (name[:40], rec["selected_files"], rec["decrypted"], rec["jpg_out"],
                 rec["plain_copy"], rec["failed"], rec["bytes_written"] / 1024 ** 3,
                 rec["elapsed_sec"]), flush=True)

    rep = {"generated_at": time.strftime("%Y-%m-%d %H:%M:%S"),
           "dry_run": bool(args.dry_run), "decrypt_root": DECRYPT_ROOT,
           "max_bytes_per_game": args.max_bytes_per_game, "games": games,
           "_summary": {"games": len(games),
                        "files_total": sum(g["files_total"] for g in games),
                        "files_selected": sum(g["selected_files"] for g in games),
                        "decrypted": sum(g["decrypted"] for g in games),
                        "plain_copy": sum(g["plain_copy"] for g in games),
                        "jpg_out": sum(g["jpg_out"] for g in games),
                        "failed": sum(g["failed"] for g in games),
                        "est_gb": round(sum(g["est_bytes"] for g in games) / 1024 ** 3, 2),
                        "written_gb": round(sum(g["bytes_written"] for g in games) / 1024 ** 3, 2)}}
    os.makedirs(os.path.dirname(os.path.abspath(args.report)), exist_ok=True)
    json.dump(rep, open(args.report, "w", encoding="utf-8"), ensure_ascii=False, indent=2)

    print("")
    print("%-40s %8s %8s %8s %8s %9s %s" % ("游戏", "加密图", "选中", "解出", "失败", "预计GB", "状态"))
    for g in games:
        print("%-40s %8d %8d %8d %8d %9.2f %s"
              % (g["name"][:40], g["files_total"], g["selected_files"],
                 g["decrypted"] + g["plain_copy"], g["failed"], g["est_bytes"] / 1024 ** 3, g["status"]))
    s = rep["_summary"]
    print("")
    print("SUMMARY games=%d files=%d selected=%d decrypted=%d plain=%d failed=%d est=%.2fGB written=%.2fGB"
          % (s["games"], s["files_total"], s["files_selected"], s["decrypted"],
             s["plain_copy"], s["failed"], s["est_gb"], s["written_gb"]))
    print("REPORT %s" % os.path.abspath(args.report))

if __name__ == "__main__":
    main()
