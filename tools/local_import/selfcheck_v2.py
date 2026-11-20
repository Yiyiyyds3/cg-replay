#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""自检 v2：加密图款先做 MV/MZ 解密镜像（rpg_decrypt.py），再复用 tools/review-prescreen
源目录模式重跑 4 条 suspect 判据，输出升级前后对比。

判定逻辑与 tools/local_import/selfcheck.py 完全一致（不重写空白/尺寸/体积判定，全部来自 prescreen）。
输出：reports/local_import/suspect_v2.md、reports/local_import/data/selfcheck_v2.json
"""
import argparse, json, os, re, subprocess, sys, time

BASE = os.path.dirname(os.path.abspath(__file__))
REPO = os.path.abspath(os.path.join(BASE, "..", ".."))
D = os.path.join(REPO, "reports", "local_import")
DATA = os.path.join(D, "data")
RES = os.path.join(DATA, "import_results.json")
SC1 = os.path.join(DATA, "selfcheck.json")
DEC = os.path.join(DATA, "decrypt_report.json")
PRE = os.path.join(REPO, "bin", "review-prescreen.exe")
MAN = os.path.join(DATA, "_prescreen_manifest_v2.json")
OUTDIR = os.path.join(DATA, "prescreen_v2")
MD = os.path.join(D, "suspect_v2.md")
JS2 = os.path.join(DATA, "selfcheck_v2.json")
DECRYPT_ROOT = "D:" + os.sep + "UCGR_Work" + os.sep + "decrypt"

def dims_le_128(mode_wh):
    if not mode_wh: return False
    m = re.match(r"(\d+)\s*[xX]\s*(\d+)", mode_wh)
    if not m: return False
    return int(m.group(1)) <= 128 and int(m.group(2)) <= 128

MV_MZ = ("rmmz", "rmmv")

def suspect_remark(v):
    """给仍 suspect 的款补一句可执行的下一步，避免笼统写"图数=0"。"""
    eng = (v.get("engine") or "").lower()
    if v.get("images", 0) == 0:
        if eng and eng not in MV_MZ:
            return "非 MV/MZ 引擎（%s），无 img 子树可读，需先解包资源（如 xp3/unity 包），不在本次解密范围" % eng
        if v.get("v1_encrypted", 0) > 0:
            return "加密图未解密成功，见解密失败表"
        return "导出后无图，需人工确认导出步骤"
    return "有图但命中判据，需人工翻图"

def judge(name, r, g):
    """selfcheck.py 的 4 条判据，原样搬用。"""
    images = g.get("images", 0) or 0
    blank = g.get("blank", 0) or 0
    decoded = g.get("decoded", 0) or 0
    avgkb = g.get("avg_kb", 0.0) or 0.0
    mode_wh = g.get("mode_wh", "") or ""
    mode_count = g.get("mode_count", 0) or 0
    encrypted = g.get("encrypted", 0) or 0
    total_kb = avgkb * images
    reasons = []
    if images == 0: reasons.append("导出图数=0" + ("（含%d张加密图，机器不可读像素）" % encrypted if encrypted else ""))
    if decoded > 0 and (blank / decoded) > 0.5: reasons.append("空白图占比>50%% (%d/%d)" % (blank, decoded))
    if images > 0 and mode_count == images and dims_le_128(mode_wh): reasons.append("所有图尺寸一致且<=128x128 (%s)" % mode_wh)
    if images > 0 and total_kb < 100: reasons.append("总导出体积<100KB (%.1fKB)" % total_kb)
    return {"name": name, "images": images, "blank": blank, "decoded": decoded,
            "sampled": g.get("sampled", 0) or 0, "skipped_big": g.get("skipped_big", 0) or 0,
            "avg_kb": round(avgkb, 1), "total_kb": round(total_kb, 1), "mode_wh": mode_wh,
            "mode_count": mode_count, "encrypted": encrypted, "severity": g.get("severity", 0),
            "events": r.get("events", 0), "reasons": reasons,
            "verdict": "suspect" if reasons else "pass"}

def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--reuse", action="store_true",
                    help="复用已存在的 prescreen_v2 指标文件，不重跑 prescreen")
    args = ap.parse_args()
    imp = json.load(open(RES, encoding="utf-8"))
    ok = [r for r in imp["results"] if r["status"] == "import_ok"]
    v1 = {}
    if os.path.exists(SC1):
        s1 = json.load(open(SC1, encoding="utf-8"))
        v1 = {v["name"]: v for v in s1.get("verdicts", [])}
    dec = {}
    if os.path.exists(DEC):
        rep0 = json.load(open(DEC, encoding="utf-8"))
        for g in rep0.get("games", []):
            dec[g["name"]] = g

    man, src_mode, mirror_info = [], {}, {}
    for i, r in enumerate(ok, 1):
        name = r["name"]
        d = dec.get(name)
        use_mirror = bool(d and d.get("out_dir") and os.path.isdir(d["out_dir"])
                          and d.get("status") in ("ok", "partial")
                          and (d.get("decrypted", 0) + d.get("plain_copy", 0)) > 0)
        path = d["out_dir"] if use_mirror else r["path"]
        engine = ""
        try:
            engine = json.load(open(r.get("out", ""), encoding="utf-8")).get("engine_id", "")
        except Exception:
            pass
        man.append({"id": i, "name": name, "engine": engine, "cg_count": 0,
                    "source_dir": "", "source_real_path": path,
                    "source_dir_exists": os.path.isdir(path)})
        src_mode[name] = "decrypt-mirror" if use_mirror else "raw"
        if use_mirror:
            mirror_info[name] = d
    json.dump(man, open(MAN, "w", encoding="utf-8"), ensure_ascii=False, indent=2)
    n_mirror = sum(1 for v in src_mode.values() if v == "decrypt-mirror")
    print("MANIFEST games=%d  decrypt-mirror=%d  raw=%d" % (len(man), n_mirror, len(man) - n_mirror))

    os.makedirs(OUTDIR, exist_ok=True)
    cands = [f for f in os.listdir(OUTDIR) if f.startswith("预筛指标") and f.endswith(".json")]
    if args.reuse and cands:
        print("PRESCREEN skipped (--reuse, 复用 %s)" % cands[0])
    else:
        cmd = [PRE, "-from-manifest", MAN, "-out", OUTDIR, "-quiet"]
        p = subprocess.run(cmd, capture_output=True, text=True, encoding="utf-8", errors="replace", timeout=7200)
        print("PRESCREEN exit=%d" % p.returncode)
        if p.returncode != 0:
            print("STDERR:", (p.stderr or "")[-3000:])
        cands = [f for f in os.listdir(OUTDIR) if f.startswith("预筛指标") and f.endswith(".json")]
    if not cands:
        raise SystemExit("prescreen produced no metrics json in %s" % OUTDIR)
    rep = json.load(open(os.path.join(OUTDIR, cands[0]), encoding="utf-8"))
    met = {g["name"]: g for g in rep["games"]}
    print("METRICS %s games=%d" % (cands[0], len(met)))

    verdicts, suspects = [], []
    for r in ok:
        name = r["name"]
        rec = judge(name, r, met.get(name, {}))
        rec["source_mode"] = src_mode[name]
        rec["engine"] = (met.get(name) or {}).get("engine", "") or ""
        old = v1.get(name)
        rec["v1_verdict"] = old["verdict"] if old else "(未在 selfcheck.json)"
        rec["v1_encrypted"] = (old or {}).get("encrypted", 0)
        rec["v1_reasons"] = (old or {}).get("reasons", [])
        rec["changed"] = bool(old) and old["verdict"] != rec["verdict"]
        verdicts.append(rec)
        if rec["verdict"] == "suspect":
            suspects.append(rec)

    fail_games = []
    for g in dec.values():
        st = g.get("status", "")
        if st in ("ok", "partial") and (g.get("decrypted", 0) + g.get("plain_copy", 0)) > 0:
            continue
        fail_games.append({"name": g["name"], "status": st,
                           "failed": g.get("failed", 0),
                           "notes": g.get("note", []),
                           "reasons": g.get("fail_reasons", {}),
                           "src_path": g.get("src_path", "")})

    partial_fail = [{"name": g["name"], "status": "partial (部分图失败)",
                      "failed": g.get("failed", 0), "reasons": g.get("fail_reasons", {}),
                      "notes": [n for n in g.get("note", []) if n.startswith("FAIL")]}
                    for g in dec.values()
                    if g.get("failed", 0) > 0 and g.get("status") in ("ok", "partial")]

    p1 = sum(1 for v in v1.values() if v.get("verdict") == "pass")
    s1n = sum(1 for v in v1.values() if v.get("verdict") == "suspect")
    p2 = sum(1 for v in verdicts if v["verdict"] == "pass")
    upgraded = [v for v in verdicts if v["v1_verdict"] == "suspect" and v["verdict"] == "pass"]
    still = [v for v in verdicts if v["v1_verdict"] == "suspect" and v["verdict"] == "suspect"]
    downgraded = [v for v in verdicts if v["v1_verdict"] == "pass" and v["verdict"] == "suspect"]
    dec_ok = [g for g in dec.values() if g.get("status") in ("ok", "partial")
              and (g.get("decrypted", 0) + g.get("plain_copy", 0)) > 0]
    dec_files = sum((g.get("decrypted", 0) + g.get("plain_copy", 0)) for g in dec_ok)
    dec_gb = sum(g.get("bytes_written", 0) for g in dec_ok) / 1024 ** 3

    L = []
    L += ["# suspect v2 · 加密图解镜像后复检（RPG Maker MV/MZ）", ""]
    L += ["## 一句话结论", "",
          "- 升级前 pass %d / %d；升级后 pass **%d** / %d（净增 %+d）"
          % (p1, len(verdicts), p2, len(verdicts), p2 - p1),
          "- 加密图款 %d 款，解密镜像 %d 张、%.2f GB（输出全在 %s，源目录零写入）"
          % (len(dec_ok), dec_files, dec_gb, DECRYPT_ROOT),
          "- 升级为 pass：%d 款；仍 suspect：%d 款；解密全失败/未解：%d 款；单图失败 %d 款"
          % (len(upgraded), len(still), len(fail_games), len(partial_fail)), ""]
    L += ["## 判据（与 selfcheck v1 完全一致，判定逻辑未改一行）", "",
          "1. 导出图数 = 0",
          "2. 空白图占比 > 50%（复用 tools/review-prescreen：32x32 网格灰度方差 < 25）",
          "3. 所有图尺寸完全一致且 ≤ 128×128",
          "4. 总导出体积 < 100KB", "",
          "- 唯一变量是**输入**：v2 把解密成功的款，manifest 的 source_real_path 换成 "
          + DECRYPT_ROOT + os.sep + "<游戏名> 镜像（保留 www/img 或 img 子树），prescreen 第一次能读到真实像素。", ""]
    L += ["## 前后对比总表（suspect 在前）", "",
          "| 游戏 | 输入源 | v1 判定 | v2 判定 | 加密图(v1) | v2 图数 | 空白/抽样 | 总KB | 众数尺寸 | v2 命中判据 |",
          "|---|---|---|---|---:|---:|---:|---:|---|---|"]
    order = sorted(verdicts, key=lambda v: (v["verdict"] != "suspect", v["name"].lower()))
    for v in order:
        L.append("| %s | %s | %s | **%s** | %d | %d | %d/%d | %.1f | %s | %s |" % (
            v["name"][:34].replace("|", "/"), "decrypt" if v["source_mode"] == "decrypt-mirror" else "raw",
            v["v1_verdict"], v["verdict"], v["v1_encrypted"], v["images"],
            v["blank"], v["decoded"], v["total_kb"], v["mode_wh"] or "-",
            "; ".join(v["reasons"]) or "-"))
    L += ["", "## 升级后仍 suspect 的款（逐款原因）", "",
          "| 游戏 | 引擎 | 图数 | 空白/抽样 | 总KB | 众数尺寸 | 命中判据 | 备注 |",
          "|---|---|---:|---:|---:|---|---|---|"]
    if still:
        for v in still:
            L.append("| %s | %s | %d | %d/%d | %.1f | %s | %s | %s |" % (
                v["name"][:34].replace("|", "/"), v["engine"] or "-", v["images"],
                v["blank"], v["decoded"], v["total_kb"], v["mode_wh"] or "-",
                "; ".join(v["reasons"]), suspect_remark(v)))
    else:
        L.append("| - | - | - | - | - | - | - | 无 |")
    L += ["", "## 像素未参与抽样的款（图过大被 -max-pixels 跳过，判据 2/3 实际未触发）", "",
          "| 游戏 | 图数 | 抽样 | 已解码 | 跳过(过大) | 众数尺寸 | 说明 |",
          "|---|---:|---:|---:|---:|---|---|"]
    noblank = [v for v in verdicts if v["images"] > 0 and v["decoded"] == 0]
    if noblank:
        for v in noblank:
            L.append("| %s | %d | %d | %d | %d | %s | 仍按导出图数/体积判据通过；像素级空白判定未覆盖 |" % (
                v["name"][:34].replace("|", "/"), v["images"], v["sampled"], v["decoded"],
                v["skipped_big"], v["mode_wh"] or "-"))
    else:
        L.append("| - | - | - | - | - | - | 无 |")
    L += ["", "## 解密失败 / 未解密的款（如实记录）", "",
          "| 游戏 | 状态 | 失败张数 | 原因 |", "|---|---|---:|---|"]
    all_fail = fail_games + partial_fail
    if all_fail:
        for g in all_fail:
            why = "; ".join("%s ×%d" % (k, c) for k, c in (g["reasons"] or {}).items()) \
                  or "; ".join(g["notes"]) or "-"
            L.append("| %s | %s | %d | %s |" % (g["name"][:34].replace("|", "/"),
                                                g["status"], g["failed"], why.replace("|", "/")[:300]))
    else:
        L.append("| - | - | - | 无失败款 |")
    L += ["", "## 说明", "",
          "- 解密只写 " + DECRYPT_ROOT + " 镜像；游戏源目录零写入、源文件零删除。",
          "- review-prescreen.exe 未改一行（Go 代码零改动），v2 只换了 manifest 的 source_real_path。",
          "- 生成时间：%s" % time.strftime("%Y-%m-%d %H:%M:%S"), ""]
    open(MD, "w", encoding="utf-8").write("\n".join(L) + "\n")

    json.dump({"generated_at": time.strftime("%Y-%m-%d %H:%M:%S"),
               "import_ok": len(verdicts),
               "pass": p2, "suspect": len(suspects),   # 兼容 gen_v2_reports.py 读取的旧键名
               "pass_v1": p1, "pass_v2": p2,
               "suspect_v1": s1n, "suspect_v2": len(suspects),
               "upgraded": [v["name"] for v in upgraded],
               "still_suspect": [v["name"] for v in still],
               "downgraded": [{"name": v["name"], "reasons": v["reasons"]} for v in downgraded],
               "decrypt_failed_games": fail_games,
               "decrypt_partial_failures": partial_fail,
               "decrypt_stats": {"games": len(dec_ok), "files": dec_files, "gb": round(dec_gb, 2)},
               "verdicts": verdicts},
              open(JS2, "w", encoding="utf-8"), ensure_ascii=False, indent=2)

    print("")
    print("%-46s %-14s %-8s %-8s %8s %9s %8s" % ("游戏", "输入源", "v1", "v2", "图数", "空白/抽样", "总KB"))
    for v in order:
        print("%-46s %-14s %-8s %-8s %8d %4d/%-4d %8.1f"
              % (v["name"][:46], v["source_mode"], v["v1_verdict"], v["verdict"],
                 v["images"], v["blank"], v["decoded"], v["total_kb"]))
    print("")
    print("DONE pass %d -> %d (%+d)  suspect %d -> %d  upgraded=%d still=%d fail_games=%d"
          % (p1, p2, p2 - p1, s1n, len(suspects), len(upgraded), len(still), len(fail_games)))
    print("MD %s" % MD)
    print("JSON %s" % JS2)

if __name__ == "__main__":
    main()
