#!/usr/bin/env python3
# -*- coding: utf-8 -*-
r"""reconcile_v3.py —— V3 验收对账：events(+validate) / textdump / 解包源素材 三方对齐。

输入（只读）：
  D:\UCGR_Work\local_import_v3\<游戏>\events.json                 run_all_v3.ps1 的产物
  D:\UCGR_Work\local_import_v3\<游戏>\events.json.validate.json    ucgr events --validate 的过滤报告
  D:\UCGR_Work\text\<游戏>\_textdump.json                         ucgr textdump 的统计报告
  解包目录（unpack / decrypt / unitycg / unitycg_auto / godot / tyrano 之一）递归数图与音频
输出（只写这两处）：
  reports/local_import/data/acceptance_v3.json    逐游戏记录 + counts 汇总
  reports/local_import/data/acceptance_v3.md      按 engine 分组的表格 + 未覆盖游戏清单

口径（与 internal/library/validate.go 对齐，否则两边数对不上）：
  源图片 = png/jpg/jpeg/webp/bmp/tlg/ctex/avif
  源音频 = ogg/wav/mp3/m4a/ogg_/rpgmvo
  images_ratio = kept CG 事件 pages 并集去重数 / 源图片数（源图 0 时为 null）
  缺失的输入一律记 null（不填 0：没跑过和跑了是 0 必须分得清）。

只用标准库；不写 BOM；路径全走 os.path。
"""
import argparse
import json
import os
import re
import sys
import time
from collections import Counter, OrderedDict

HERE = os.path.dirname(os.path.abspath(__file__))
REPO = os.path.abspath(os.path.join(HERE, "..", ".."))
DATA_DIR = os.path.join(REPO, "reports", "local_import", "data")

DEFAULT_GAMES = os.path.join(DATA_DIR, "import_results_v2.json")
DEFAULT_JSON = os.path.join(DATA_DIR, "acceptance_v3.json")
DEFAULT_MD = os.path.join(DATA_DIR, "acceptance_v3.md")

DEFAULT_LIB_ROOT = r"D:\UCGR_Work\local_import_v3"
DEFAULT_TEXT_ROOT = r"D:\UCGR_Work\text"

# 解包/镜像产物的五个落脚点（据 V3_PLAN 与磁盘实况实测）：
#   kirikiri / 通用解包 -> D:\UCGR_Work\unpack\<名>          （含 raw\ 与 ucgr-convention.json）
#   rmmz/tv 加密镜像    -> D:\UCGR_Work\decrypt\<名>         （含 www\；prescreen_manifest_v2 的 source_real_path 指这里）
#   unity               -> D:\UCGR_Work\unitycg(_auto)\<名>  （含 cg\ 与 cg-index.json）
#   godot               -> D:\UCGR_Work\godot\<名>
#   tyrano              -> D:\UCGR_Work\tyrano\<名>
UNPACK_ROOTS = [
    (r"D:\UCGR_Work\unpack", "unpack"),
    (r"D:\UCGR_Work\decrypt", "decrypt"),
    (r"D:\UCGR_Work\unitycg", "unitycg"),
    (r"D:\UCGR_Work\unitycg_auto", "unitycg_auto"),
    (r"D:\UCGR_Work\godot", "godot"),
    (r"D:\UCGR_Work\tyrano", "tyrano"),
]

IMAGE_EXTS = {".png", ".jpg", ".jpeg", ".webp", ".bmp", ".tlg", ".ctex", ".avif"}
AUDIO_EXTS = {".ogg", ".wav", ".mp3", ".m4a", ".ogg_", ".rpgmvo"}

# 判定「这是个解包目录」的标记，任中其一即认
MARKER_FILES = ("ucgr-convention.json", "cg-index.json", ".ucgr-source.json", "_carve_manifest.json")
# img/audio 是 rmmz 解密镜像的常见落点（decrypt\<游戏>\www 里就分 img 与 audio，
# 也有直接把 img/audio 摊在根上的），认它才能把镜像当解包目录。
MARKER_DIRS = ("raw", "www", "data", "cg", "img")

# 历史遗留目录名后缀（local_import 里出现过 _unpacked / _cglib / _all / _godot）
NAME_SUFFIXES = ("_unpacked", "_cglib", "_all", "_godot", "_base")

VALIDATE_SUFFIX = ".validate.json"


def log(msg):
    sys.stderr.write(str(msg) + "\n")
    sys.stderr.flush()


def safe_name(s):
    """与 tools/local_import/import_batch.py 同口径：只替换 Windows 非法字符。"""
    out = []
    for ch in str(s):
        if ch in '<>:"/\\|?*' or ord(ch) < 0x20:
            out.append("_")
        else:
            out.append(ch)
    s = "".join(out).strip().rstrip(". ")
    return (s or "unnamed")[:120]


def read_json(path):
    try:
        with open(path, "r", encoding="utf-8-sig") as f:
            return json.load(f)
    except Exception as e:
        log("  ! JSON 读取失败 %s: %s" % (path, e))
        return None


def load_games(path):
    """兼容四种清单形状：
       import_results_v2.json -> {results:[{name,path,...}]}
       inventory.json         -> {games:[{name,path,rel,...}]}
       dedup.json             -> {games:[...]}
       裸数组                 -> [{name,...}, ...]
    """
    d = read_json(path)
    if d is None:
        return []
    if isinstance(d, list):
        rows = d
    elif isinstance(d, dict):
        rows = None
        for key in ("results", "games", "items", "verdicts"):
            v = d.get(key)
            if isinstance(v, list) and v:
                rows = v
                break
        if rows is None:
            rows = []
    else:
        rows = []
    games = []
    for r in rows:
        if not isinstance(r, dict):
            continue
        name = r.get("name") or r.get("game") or ""
        if not name:
            continue
        games.append({
            "name": name,
            "path": r.get("path") or r.get("source_real_path") or r.get("source_dir") or "",
            "engine": str(r.get("engine") or r.get("engine_id") or "").lower(),
            "pkg_kind": r.get("pkg_kind") or "",
            "status": r.get("status") or r.get("verdict") or "",
        })
    seen, uniq = set(), []
    for g in games:
        if g["name"] in seen:
            continue
        seen.add(g["name"])
        uniq.append(g)
    return uniq


def looks_like_unpack(path):
    if not os.path.isdir(path):
        return False
    try:
        names = os.listdir(path)
    except OSError:
        return False
    low = {n.lower() for n in names}
    if any(m.lower() in low for m in MARKER_FILES):
        return True
    return any(n.lower() in MARKER_DIRS for n in names)


def name_variants(name):
    """目录名候选：原样 -> 去后缀 -> 加后缀（local_import 里出现过 _unpacked/_cglib/_all/_godot）。"""
    cands = [name]
    for suf in NAME_SUFFIXES:
        if name.lower().endswith(suf):
            cands.append(name[: -len(suf)])
    for suf in NAME_SUFFIXES:
        if not name.lower().endswith(suf):
            cands.append(name + suf)
    out, seen = [], set()
    for c in cands:
        if c and c not in seen:
            seen.add(c)
            out.append(c)
    return out


def name_key(s):
    """名字归一：去掉空格/括号/标点，只留字母数字与汉字。
    用来兜住「共妻之村 (牛)」这种清单名 vs「共妻之村」这种解包目录名。"""
    return re.sub(r"[^0-9a-z\u4e00-\u9fff]", "", str(s).lower())


def fuzzy_dirs(root, name, limit=3):
    """同名/前缀关系兜底，返回最多 limit 个候选目录（按匹配长度降序）。"""
    key = name_key(name)
    if len(key) < 3:
        return []
    try:
        entries = os.listdir(root)
    except OSError:
        return []
    hits = []
    for e in entries:
        p = os.path.join(root, e)
        if not os.path.isdir(p):
            continue
        dk = name_key(e)
        if len(dk) < 3:
            continue
        if dk == key:
            hits.append((1000 + len(dk), p))
        elif dk.startswith(key) or key.startswith(dk):
            hits.append((min(len(dk), len(key)), p))
    hits.sort(key=lambda x: (-x[0], x[1]))
    return [p for _score, p in hits[:limit]]


def locate_unpack(name, roots=None):
    """按「原样名 -> 去后缀 -> 加后缀」三档顺序在六个 root 里找解包目录。
    返回 (path, kind) 或 (None, None)。junction 不用特判：os.path.isdir 会跟随链接，
    realpath 只是让日志里显示真实落点。"""
    roots = roots or UNPACK_ROOTS
    cands = name_variants(name)
    for root, kind in roots:
        if not os.path.isdir(root):
            continue
        for cand in cands:
            p = os.path.join(root, cand)
            if looks_like_unpack(p):
                return os.path.realpath(p), kind
    # 模糊兜底（清单名常带「(牛)」这类评级后缀）
    for root, kind in roots:
        for p in fuzzy_dirs(root, name):
            if looks_like_unpack(p):
                return os.path.realpath(p), kind
    return None, None


def guess_engine_from_dir(d):
    """结构推断：convention 优先，其次看目录长相。"""
    conv_path = os.path.join(d, "ucgr-convention.json")
    if os.path.isfile(conv_path):
        conv = read_json(conv_path)
        if isinstance(conv, dict) and conv.get("engine_id"):
            return str(conv["engine_id"]).lower()
    try:
        names = {n.lower() for n in os.listdir(d)}
    except OSError:
        names = set()
    if "raw" in names and os.path.isdir(os.path.join(d, "raw", "data")):
        return "kirikiri"
    if "www" in names or "img" in names:
        return "rmmz"
    if "cg-index.json" in names:
        return "unity"
    return ""


def count_media(root):
    """递归数源图片与音频；不打开文件。返回 (images, audio, bytes_total, read_errors)。"""
    imgs = audio = total = errs = 0
    for dirpath, _dirnames, filenames in os.walk(root):
        for fn in filenames:
            ext = os.path.splitext(fn)[1].lower()
            if ext in IMAGE_EXTS:
                imgs += 1
            elif ext in AUDIO_EXTS:
                audio += 1
            else:
                continue
            try:
                total += os.path.getsize(os.path.join(dirpath, fn))
            except OSError:
                errs += 1
    return imgs, audio, total, errs


def script_dir_candidates(name, roots):
    """所有可能承载素材的目录（按 root 顺序、名字三档展开）。"""
    out, seen = [], set()
    for root, kind in roots:
        if not os.path.isdir(root):
            continue
        for cand in name_variants(name):
            p = os.path.join(root, cand)
            if os.path.isdir(p):
                rp = os.path.realpath(p)
                if rp not in seen:
                    seen.add(rp)
                    out.append((rp, kind))
        for p in fuzzy_dirs(root, name):
            rp = os.path.realpath(p)
            if rp not in seen:
                seen.add(rp)
                out.append((rp, kind))
    return out


def pick_asset_dir(name, run_dir, roots):
    r"""选「数源图/源音频」用的目录，返回 (chosen, first)。

    优先 events.json 里的 root（它记录这次到底扫的哪个目录），其次按 root 顺序找
    第一个「真的有图」的目录。为什么要换目录：rmmz 的剧本在原始游戏目录，而图片
    解出来后落在 D:\UCGR_Work\decrypt\<游戏>（只有 www\img，没有脚本），照着扫
    会得到 source_images=0。chosen = 第一个有图的；first = 候选里第一个（兜底）。
    """
    cands = []
    if run_dir and os.path.isdir(run_dir):
        cands.append((os.path.realpath(run_dir), "events_root"))
    for item in script_dir_candidates(name, roots):
        if item[0] not in [c[0] for c in cands]:
            cands.append(item)
    best = None
    for d, kind in cands:
        imgs, audio, nbytes, errs = count_media(d)
        info = (d, kind, imgs, audio, nbytes, errs)
        # 取「图最多」的那个，而不是「第一个非 0」：原始游戏目录里常常只有几张
        # 图标/封面（3 张），真正的素材在 decrypt 镜像里（上千张）。
        if best is None or imgs > best[2]:
            best = info
    return best, best


def pages_union(cgs):
    """kept CG 事件 pages 并集：按 basename 小写去重（各家 storage 写法不一，
    validate.go 的命中判定也是 rel/basename/stem 三套键兜底）。"""
    pages = set()
    per_event = {}
    for cg in cgs:
        if not isinstance(cg, dict):
            continue
        names = []
        for p in (cg.get("pages") or []):
            if not isinstance(p, str):
                continue
            base = os.path.basename(p.replace("\\", "/")).lower()
            if base:
                pages.add(base)
                names.append(base)
        if names:
            per_event[str(cg.get("id") or cg.get("label") or "?")] = len(set(names))
    return pages, per_event


def build_record(game, lib_root, text_root, roots):
    name = game["name"]
    safe = safe_name(name)
    rec = OrderedDict()
    # —— 硬口径字段（顺序即 md 列顺序）——
    rec["game"] = name
    rec["engine"] = None
    rec["cg_kept"] = None
    rec["cg_dropped_empty"] = None
    rec["scene_kept"] = None
    rec["scene_dropped"] = None
    rec["audio_count"] = None
    rec["text_scripts"] = None
    rec["text_lines"] = None
    rec["source_images"] = None
    rec["images_ratio"] = None
    # —— 对账辅助字段 ——
    rec["source_audio"] = None
    rec["pages_distinct"] = None
    rec["cg_events"] = None
    rec["scene_events"] = None
    rec["validate_present"] = False
    rec["text_present"] = False
    rec["unpack_dir"] = None
    rec["unpack_kind"] = None
    rec["asset_dir"] = None
    rec["asset_kind"] = None
    rec["notes"] = []
    rec["error"] = None

    ev_path = os.path.join(lib_root, safe, "events.json")
    ev = read_json(ev_path) if os.path.isfile(ev_path) else None
    va_path = ev_path + VALIDATE_SUFFIX
    va = read_json(va_path) if os.path.isfile(va_path) else None
    td_path = os.path.join(text_root, safe, "_textdump.json")
    td = read_json(td_path) if os.path.isfile(td_path) else None

    unpack, kind = locate_unpack(name, roots)
    if unpack:
        rec["unpack_dir"] = unpack
        rec["unpack_kind"] = kind

    # 引擎：events.json 是真跑出来的，优先；再 validate；再目录长相；再清单
    engine = ""
    if isinstance(ev, dict) and ev.get("engine_id"):
        engine = str(ev["engine_id"]).lower()
    elif isinstance(va, dict) and va.get("engine_id"):
        engine = str(va["engine_id"]).lower()
    elif unpack:
        engine = guess_engine_from_dir(unpack)
    elif game.get("engine"):
        engine = game["engine"]
    rec["engine"] = engine or "unknown"

    # events / validate
    if isinstance(ev, dict):
        cgs = ev.get("cgs") or []
        scenes = ev.get("scenes") or []
        rec["cg_events"] = len(cgs)
        rec["scene_events"] = len(scenes)
        if ev.get("warnings"):
            rec["notes"].append("events.warnings=%d" % len(ev["warnings"]))
        pages, per_event = pages_union(cgs)
        rec["pages_distinct"] = len(pages)
        if per_event and len(per_event) < rec["cg_events"]:
            rec["notes"].append("%d 个 CG 事件没有 pages" % (rec["cg_events"] - len(per_event)))
    else:
        rec["notes"].append("events.json 缺失（未跑或跑挂）")

    va_text = (None, None)
    if isinstance(va, dict):
        rec["validate_present"] = True
        for k in ("cg_kept", "cg_dropped_empty", "scene_kept", "scene_dropped", "audio_count"):
            v = va.get(k)
            rec[k] = int(v) if isinstance(v, (int, float)) else None
        va_text = (va.get("text_scripts"), va.get("text_lines"))
        if isinstance(ev, dict):
            # 落盘 events.json 只有「留下」的事件，所以只能拿 kept 比对；
            # scene_dropped 是过滤前被删的条数，加不进落盘数里。
            if rec["cg_kept"] is not None and rec["cg_kept"] != rec["cg_events"]:
                rec["notes"].append("cg_kept(%s) != 落盘 cgs(%s)" % (rec["cg_kept"], rec["cg_events"]))
            if rec["scene_kept"] is not None and rec["scene_kept"] != rec["scene_events"]:
                rec["notes"].append("scene_kept(%s) != 落盘 scenes(%s)" % (rec["scene_kept"], rec["scene_events"]))
    elif isinstance(ev, dict):
        rec["notes"].append("events.json.validate.json 缺失（--validate 未跑）")

    # textdump
    if isinstance(td, dict):
        rec["text_present"] = True
        ts, tl = td.get("total_scripts"), td.get("total_lines")
        rec["text_scripts"] = int(ts) if isinstance(ts, (int, float)) else None
        rec["text_lines"] = int(tl) if isinstance(tl, (int, float)) else None
    elif isinstance(ev, dict):
        rec["notes"].append("_textdump.json 缺失（textdump 未跑）")
    if rec["text_lines"] is None and va_text[1]:
        rec["text_scripts"], rec["text_lines"] = va_text
        rec["notes"].append("text 取 validate 兜底值")

    # 源素材：优先按 events.json 的 root 数；那儿没图（rmmz 原始目录是加密图）再换解密镜像
    run_dir = None
    if isinstance(ev, dict) and isinstance(ev.get("root"), str) and os.path.isdir(ev["root"]):
        run_dir = ev["root"]
    chosen, first = pick_asset_dir(name, run_dir, roots)
    if chosen:
        asset_dir, asset_kind, imgs, audio, _nbytes, errs = chosen
        rec["asset_dir"] = asset_dir
        rec["asset_kind"] = asset_kind
        rec["source_images"] = imgs
        rec["source_audio"] = audio
        if errs:
            rec["notes"].append("扫描时 %d 个文件读不到大小" % errs)
        if rec["audio_count"] is not None and audio != rec["audio_count"]:
            rec["notes"].append("audio_count(%s) != 源音频(%s)" % (rec["audio_count"], audio))
        if imgs == 0:
            rec["notes"].append("素材目录无源图片（壳或素材未解出）")
        if first and first[0] != asset_dir:
            rec["notes"].append("扫描用的素材目录与首个候选不同：%s -> %s" % (
                os.path.basename(first[0]) or first[0], asset_kind))
        if run_dir and os.path.realpath(run_dir) != asset_dir and asset_kind != "events_root":
            rec["notes"].append("剧本目录(%s)与素材目录(%s)不在同一处" % (run_dir, asset_dir))
    else:
        rec["notes"].append("未找到解包/素材目录（不参与 images_ratio）")

    # 引擎兜底：材料目录定下来后再猜一次（rmmz 解密镜像只有 img\ 也能认出来）
    if rec["engine"] in (None, "", "unknown") and rec["asset_dir"]:
        guessed = guess_engine_from_dir(rec["asset_dir"])
        if guessed:
            rec["engine"] = guessed
            rec["notes"].append("引擎按素材目录推断为 %s" % guessed)

    # images_ratio：kept 事件 pages 并集 / 源图片
    if rec["pages_distinct"] is not None and rec["source_images"]:
        rec["images_ratio"] = round(rec["pages_distinct"] / float(rec["source_images"]), 4)
    elif rec["pages_distinct"] == 0 and rec["source_images"]:
        rec["images_ratio"] = 0.0

    return rec


def summarise(records, games, games_file):
    def s(key):
        return sum(r[key] for r in records if isinstance(r.get(key), (int, float)))

    counts = OrderedDict()
    counts["games_in_list"] = len(games)
    counts["games_processed"] = len(records)
    counts["with_events"] = sum(1 for r in records if r.get("cg_events") is not None)
    counts["with_validate"] = sum(1 for r in records if r.get("validate_present"))
    counts["with_textdump"] = sum(1 for r in records if r.get("text_present"))
    counts["with_unpack"] = sum(1 for r in records if r.get("unpack_dir"))
    counts["no_events"] = sum(1 for r in records if r.get("cg_events") is None)
    counts["cg_kept_sum"] = s("cg_kept")
    counts["cg_dropped_empty_sum"] = s("cg_dropped_empty")
    counts["scene_kept_sum"] = s("scene_kept")
    counts["scene_dropped_sum"] = s("scene_dropped")
    counts["audio_count_sum"] = s("audio_count")
    counts["source_images_sum"] = s("source_images")
    counts["source_audio_sum"] = s("source_audio")
    counts["pages_distinct_sum"] = s("pages_distinct")
    counts["text_scripts_sum"] = s("text_scripts")
    counts["text_lines_sum"] = s("text_lines")
    by_engine = Counter()
    for r in records:
        by_engine[r.get("engine") or "unknown"] += 1
    counts["by_engine"] = OrderedDict(sorted(by_engine.items()))
    ratios = [r["images_ratio"] for r in records if isinstance(r.get("images_ratio"), (int, float))]
    counts["images_ratio_avg"] = round(sum(ratios) / len(ratios), 4) if ratios else None
    counts["games_file"] = games_file
    return counts


def fmt(v, dash="-"):
    if v is None:
        return dash
    if isinstance(v, float):
        return "%.2f" % v
    return str(v)


def write_md(path, counts, records, games, games_file):
    bt = chr(96)
    def q(s):
        return bt + str(s) + bt
    L = []
    L.append("# V3 验收对账（acceptance_v3）")
    L.append("")
    L.append("生成时间：" + time.strftime("%Y-%m-%d %H:%M:%S") + "  ")
    L.append("清单来源：" + q(games_file) + "（%d 款）  " % counts["games_in_list"])
    L.append("数据来源：" + q(r"D:\UCGR_Work\local_import_v3\<游戏>\events.json(.validate.json)") + "、"
             + q(r"D:\UCGR_Work\text\<游戏>\_textdump.json") + "、解包目录递归计数")
    L.append("")
    L.append("## 汇总")
    L.append("")
    L.append("| 指标 | 值 |")
    L.append("| --- | --- |")
    for k in ("games_processed", "with_events", "with_validate", "with_textdump", "with_unpack",
              "no_events", "cg_kept_sum", "cg_dropped_empty_sum", "scene_kept_sum",
              "scene_dropped_sum", "audio_count_sum", "source_audio_sum", "pages_distinct_sum",
              "source_images_sum", "text_scripts_sum", "text_lines_sum", "images_ratio_avg"):
        L.append("| %s | %s |" % (k, fmt(counts.get(k))))
    L.append("")
    L.append("按引擎分布：" + "、".join("%s=%d" % (k, v) for k, v in counts["by_engine"].items()))
    L.append("")

    groups = OrderedDict()
    for r in records:
        groups.setdefault(r.get("engine") or "unknown", []).append(r)

    L.append("## 逐游戏（按 engine 分组）")
    L.append("")
    for engine in sorted(groups, key=lambda e: (e == "unknown", e)):
        rows = sorted(groups[engine], key=lambda r: r.get("game") or "")
        L.append("### %s（%d 款）" % (engine, len(rows)))
        L.append("")
        L.append("| 游戏 | cg_kept | cg空壳 | scene_kept | scene_dropped | audio | 文本脚本 | 文本行 | 源图 | images_ratio |")
        L.append("| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |")
        for r in rows:
            L.append("| %s | %s | %s | %s | %s | %s | %s | %s | %s | %s |" % (
                str(r.get("game")).replace("|", "\\|"),
                fmt(r.get("cg_kept")), fmt(r.get("cg_dropped_empty")),
                fmt(r.get("scene_kept")), fmt(r.get("scene_dropped")),
                fmt(r.get("audio_count")), fmt(r.get("text_scripts")), fmt(r.get("text_lines")),
                fmt(r.get("source_images")), fmt(r.get("images_ratio"))))
        L.append("")
        miss = [r for r in rows if r.get("cg_events") is None]
        if miss:
            L.append("该组未跑出 events：" + "、".join(str(r.get("game")) for r in miss))
            L.append("")

    covered = {r.get("game") for r in records if r.get("cg_events") is not None}
    uncovered = [g for g in games if g["name"] not in covered]
    L.append("## 未覆盖游戏（清单内但没有 events.json）")
    L.append("")
    if uncovered:
        L.append("共 %d 款：" % len(uncovered))
        L.append("")
        for g in uncovered:
            why = g.get("status") or "清单无状态字段"
            L.append("- %s（%s）" % (g["name"], why))
    else:
        L.append("无——清单内每款都有 events.json。")
    L.append("")
    L.append("## 口径说明")
    L.append("")
    L.append("- " + q("source_images") + " / " + q("source_audio") + "：解包目录递归计数，扩展名表与 "
             + q("internal/library/validate.go") + " 一致（图 png/jpg/jpeg/webp/bmp/tlg/ctex/avif；"
             "音 ogg/wav/mp3/m4a/ogg_/rpgmvo）。")
    L.append("- " + q("images_ratio") + " = kept CG 事件 " + q("pages") + " 并集（按 basename 小写去重）/ "
             + q("source_images") + "；源图为 0 或 events 缺失时为 " + q("null") + "。")
    L.append("- " + q("cg_kept") + " / " + q("cg_dropped_empty") + " / " + q("scene_kept") + " / "
             + q("scene_dropped") + " / " + q("audio_count") + " 全部来自 " + q("*.validate.json") + "；报告缺失为 " + q("null") + "。")
    L.append("- " + q("text_scripts") + " / " + q("text_lines") + " 来自 " + q("_textdump.json")
             + " 的 " + q("total_scripts") + " / " + q("total_lines") + "；缺失为 " + q("null") + "。")
    L.append("- 解包目录定位顺序：unpack -> decrypt -> unitycg -> unitycg_auto -> godot -> tyrano，"
             "名字按「原样 / 去后缀(_unpacked _cglib _all _godot) / 加后缀」三档试。")
    L.append("- " + q("asset_dir") + "：实际数源图的目录。先用 events.json 里的 " + q("root")
             + "；那儿数出 0 张图时退到同名解密镜像（rmmz 的剧本在原始目录、图在 decrypt 镜像）。")
    L.append("")
    with open(path, "w", encoding="utf-8", newline="\n") as f:
        f.write("\n".join(L))


def main(argv=None):
    ap = argparse.ArgumentParser(description="V3 验收对账（events/validate/textdump/源素材 三方对齐）")
    ap.add_argument("--games-file", default=DEFAULT_GAMES, help="游戏清单（缺省 import_results_v2.json）")
    ap.add_argument("--lib-root", default=DEFAULT_LIB_ROOT, help="events 产物根目录")
    ap.add_argument("--text-root", default=DEFAULT_TEXT_ROOT, help="textdump 产物根目录")
    ap.add_argument("--out-json", default=DEFAULT_JSON)
    ap.add_argument("--out-md", default=DEFAULT_MD)
    ap.add_argument("--only", default="", help="只处理这些游戏名（逗号分隔）")
    ap.add_argument("--limit", type=int, default=0, help="只处理前 N 款（调试）")
    args = ap.parse_args(argv)

    games = load_games(args.games_file)
    if not games:
        log("清单为空或读不了：" + args.games_file)
        return 2
    only = {s.strip() for s in args.only.split(",") if s.strip()}
    if only:
        games = [g for g in games if g["name"] in only]
    if args.limit > 0:
        games = games[: args.limit]
    log("清单 %d 款 -> 对账开始（lib=%s text=%s）" % (len(games), args.lib_root, args.text_root))

    records = []
    for i, g in enumerate(games, 1):
        try:
            rec = build_record(g, args.lib_root, args.text_root, UNPACK_ROOTS)
        except Exception as e:  # 单款炸了不中断整体
            rec = OrderedDict([("game", g["name"]), ("engine", "unknown"), ("error", repr(e))])
            for k in ("cg_kept", "cg_dropped_empty", "scene_kept", "scene_dropped", "audio_count",
                      "text_scripts", "text_lines", "source_images", "images_ratio"):
                rec.setdefault(k, None)
            rec.setdefault("notes", ["脚本异常：" + repr(e)])
            log("  [%d/%d] %s 异常 %r" % (i, len(games), g["name"], e))
        records.append(rec)
        log("  [%d/%d] %-44s engine=%-9s cg=%-5s src=%-5s txt=%s" % (
            i, len(games), str(rec.get("game"))[:44], rec.get("engine"),
            fmt(rec.get("cg_kept")), fmt(rec.get("source_images")), fmt(rec.get("text_lines"))))

    counts = summarise(records, games, args.games_file)
    payload = OrderedDict()
    payload["generated_at"] = time.strftime("%Y-%m-%d %H:%M:%S")
    payload["games_file"] = args.games_file
    payload["lib_root"] = args.lib_root
    payload["text_root"] = args.text_root
    payload["counts"] = counts
    payload["games"] = records

    for p in (args.out_json, args.out_md):
        d = os.path.dirname(p)
        if d and not os.path.isdir(d):
            os.makedirs(d, exist_ok=True)
    with open(args.out_json, "w", encoding="utf-8", newline="\n") as f:
        json.dump(payload, f, ensure_ascii=False, indent=2)
        f.write("\n")
    write_md(args.out_md, counts, records, games, args.games_file)

    out = []
    out.append("OK games=%d events=%d validate=%d textdump=%d unpack=%d" % (
        counts["games_processed"], counts["with_events"], counts["with_validate"],
        counts["with_textdump"], counts["with_unpack"]))
    out.append("   cg_kept=%d cg_dropped_empty=%d scene_kept=%d scene_dropped=%d" % (
        counts["cg_kept_sum"], counts["cg_dropped_empty_sum"],
        counts["scene_kept_sum"], counts["scene_dropped_sum"]))
    out.append("   pages=%d source_images=%d ratio_avg=%s text_lines=%d" % (
        counts["pages_distinct_sum"], counts["source_images_sum"],
        fmt(counts["images_ratio_avg"]), counts["text_lines_sum"]))
    out.append("   -> " + args.out_json)
    out.append("   -> " + args.out_md)
    print("\n".join(out))
    return 0


if __name__ == "__main__":
    sys.exit(main())
