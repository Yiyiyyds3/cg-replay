#!/usr/bin/env python3
# -*- coding: utf-8 -*-
r"""分层立绘导出器：RPG Maker MV 插件驱动的立绘（img/portraits/ 树，脚本里无图片字面量）-> cg-index.json（unity 链路）。

背景：wuxian 的 H 画面由 ER_PortraitManager 插件在运行时拼名加载 ——
  ImageManager.loadBitmap('img/portraits/${filePath}/', fileName)
事件脚本里没有图片名字面量，所以 ucgr 的静态 Script 图片提取对它无效；
但立绘图实际平铺在 www/img/portraits/ 子树下。本工具按目录枚举 + 插件常量解析把它们捞出来，
整理成「每张图一个 cg/<id>/ 单图」的 CG 库（cg-index.json schema=1 engine=unity），
再交给 bin/ucgr.exe events 走 unitycg 链路验证。

红线：只读游戏目录（加密图解密只在产物目录落盘，绝不写回源目录、绝不删除源文件）；
      产物只落 --out（默认 D:\UCGR_Work\portraits\<游戏名>）。

用法：
  python tools/local_import/portrait_export.py --game "E:\...\wuxian"
  python tools/local_import/portrait_export.py --game <目录> --dry-run
  python tools/local_import/portrait_export.py --game <目录> --out D:\UCGR_Work\portraits\wuxian --no-verify
"""
import argparse, json, os, re, struct, subprocess, sys, time, zlib

BASE = os.path.dirname(os.path.abspath(__file__))
REPO = os.path.abspath(os.path.join(BASE, "..", ".."))
DATA = os.path.join(REPO, "reports", "local_import", "data")
DEFAULT_GAME = r"E:\Personal_entertainment\game\ero\egame\20260426\RPG\未评级\wuxian"
OUT_ROOT = "D:" + os.sep + "UCGR_Work" + os.sep + "portraits"
UCGR = os.path.join(REPO, "bin", "ucgr.exe")

PNG_SIG = b"\x89PNG\r\n\x1a\n"
IMG_EXT = (".png", ".jpg", ".jpeg", ".webp")
ENC_EXT = (".rpgmvp", ".png_")
BAD_CHARS = re.compile(r'[<>:"/\\|?*\x00-\x1f]')
BT = chr(96)                      # 反引号
QUOTES = "\"'" + BT               # JS 里的三种字符串定界符

MAX_IMAGES = 2000      # 超过就按源字节数取最大的 KEEP_IMAGES 张
KEEP_IMAGES = 1000


# ---------- 通用 ----------

def safe_id(rel):
    """相对路径 -> 安全化 id（子目录_文件名）：body/lower/anal/sex/normal.png -> body_lower_anal_sex_normal"""
    stem = os.path.splitext(rel.replace(os.sep, "/"))[0]
    parts = [p for p in stem.split("/") if p]
    return BAD_CHARS.sub("_", "_".join(parts)).strip().rstrip(". ") or "img"


def walk_images(root):
    """枚举 root 下全部图片与加密图：返回 [{rel, abs, ext, size}]。"""
    out = []
    for dp, dns, fns in os.walk(root):
        dns.sort()
        for fn in sorted(fns):
            low = fn.lower()
            if not (low.endswith(IMG_EXT) or low.endswith(ENC_EXT)):
                continue
            ap = os.path.join(dp, fn)
            try:
                size = os.path.getsize(ap)
            except OSError:
                continue
            out.append({"rel": os.path.relpath(ap, root), "abs": ap,
                        "ext": os.path.splitext(fn)[1].lower(), "size": size})
    out.sort(key=lambda r: r["rel"].lower())
    return out


def find_portrait_root(game):
    """img/portraits 优先；否则 img/ 下带 portrait 关键词的子树；都没有退回整棵 img/。"""
    for c in (os.path.join(game, "www", "img", "portraits"),
              os.path.join(game, "img", "portraits")):
        if os.path.isdir(c):
            return c, "img/portraits"
    img = os.path.join(game, "www", "img")
    if not os.path.isdir(img):
        img = os.path.join(game, "img")
    if os.path.isdir(img):
        for name in sorted(os.listdir(img)):
            p = os.path.join(img, name)
            if os.path.isdir(p) and "portrait" in name.lower():
                return p, "img/" + name
        return img, "img/(未找到 portrait 关键词子树，退回整棵 img/)"
    return None, ""


# ---------- PNG 头校验（明文图用） ----------

def ihdr_ok(path):
    try:
        with open(path, "rb") as f:
            b = f.read(33)
    except OSError as e:
        return False, "IO: %s" % e
    if len(b) < 33 or b[:8] != PNG_SIG:
        return False, "非 PNG"
    if struct.unpack(">I", b[8:12])[0] != 13 or b[12:16] != b"IHDR":
        return False, "IHDR 块头不符"
    w, h = struct.unpack(">II", b[16:24])
    if not (0 < w <= 100000 and 0 < h <= 100000):
        return False, "尺寸非法"
    if (zlib.crc32(b[12:29]) & 0xFFFFFFFF) != struct.unpack(">I", b[29:33])[0]:
        return False, "IHDR CRC 不符"
    return True, "%dx%d" % (w, h)


def load_decryptor():
    """复用 tools/local_import/rpg_decrypt.py 的解密逻辑（import 失败就明确报出来）。"""
    if BASE not in sys.path:
        sys.path.insert(0, BASE)
    try:
        import rpg_decrypt as rd
        return rd
    except Exception as e:
        print("WARN 无法 import rpg_decrypt（%s），加密图将跳过" % e)
        return None


# ---------- 插件参数 / 常量解析 ----------

def read_text(path):
    for enc in ("utf-8-sig", "utf-8", "cp936", "latin-1"):
        try:
            with open(path, encoding=enc) as f:
                return f.read()
        except UnicodeDecodeError:
            continue
        except OSError:
            return ""
    return ""


def parse_plugins_js(game):
    """读 www/js/plugins.js：插件总数、Portrait 相关参数条目、插件源码目录清单。"""
    p = os.path.join(game, "www", "js", "plugins.js")
    for alt in (os.path.join(game, "www", "plugins.js"),
                os.path.join(game, "www", "js", "plugins", "plugins.js")):
        if not os.path.isfile(p) and os.path.isfile(alt):
            p = alt
    plugins_dir = os.path.join(game, "www", "js", "plugins")
    js_files = sorted(n for n in os.listdir(plugins_dir) if n.lower().endswith(".js")) \
        if os.path.isdir(plugins_dir) else []
    res = {"plugins_js": p if os.path.isfile(p) else "",
           "plugins_dir": plugins_dir if os.path.isdir(plugins_dir) else "",
           "plugin_files_total": len(js_files), "names": [], "portrait_entries": [],
           "portrait_plugin_files": [n for n in js_files if "portrait" in n.lower()]}
    if not res["plugins_js"]:
        return res
    s = read_text(res["plugins_js"])
    for m in re.finditer(r'\{"name":"(.*?)","status":(true|false),"description":"(.*?)","parameters":\{(.*?)\}\}', s, re.S):
        name, status, desc, params = m.group(1), m.group(2) == "true", m.group(3), m.group(4)
        res["names"].append(name)
        if "portrait" in name.lower() or "Portrait" in desc:
            kv = {}
            for pm in re.finditer(r'"(.*?)":"(.*?)"', params):
                kv[pm.group(1)] = pm.group(2)
            res["portrait_entries"].append({"name": name, "status": status,
                                            "description": desc[:200], "parameters": kv,
                                            "parameters_count": len(kv)})
    return res


def _skip_string(s, i):
    q = s[i]
    j = i + 1
    while j < len(s):
        if s[j] == "\\":
            j += 2
            continue
        if s[j] == q:
            return j + 1
        j += 1
    return j


def _match_paren(s, i):
    open_ch = s[i]
    close_ch = {"(": ")", "[": "]", "{": "}"}[open_ch]
    depth = 0
    j = i
    while j < len(s):
        c = s[j]
        if c in QUOTES:
            j = _skip_string(s, j)
            continue
        if c == open_ch:
            depth += 1
        elif c == close_ch:
            depth -= 1
            if depth == 0:
                return j
        j += 1
    return -1


def _split_args(inner):
    args, cur, depth, i = [], [], 0, 0
    while i < len(inner):
        c = inner[i]
        if c in QUOTES:
            j = _skip_string(inner, i)
            cur.append(inner[i:j])
            i = j
            continue
        if c in "([{":
            depth += 1
        elif c in ")]}":
            depth -= 1
        if c == "," and depth == 0:
            args.append("".join(cur).strip())
            cur = []
        else:
            cur.append(c)
        i += 1
    if "".join(cur).strip():
        args.append("".join(cur).strip())
    return args


def _pool(expr):
    """条目里所有「不带点」的字符串字面量 = 模板变量候选池（anal / vagina / slap / memory ...）。"""
    vals, seen = [], set()
    for m in re.finditer(r"""['"]([^'"\n]*)['"]""", expr):
        v = m.group(1)
        if v and "." not in v and len(v) < 40 and v not in seen:
            seen.add(v)
            vals.append(v)
    return vals


def _suffix_expand(base, max1, max2, dflt=6):
    """genPath 的编号后缀：.i / .i.j（静态读不出上界时按 1..dflt 兜底）。"""
    if max2:
        return ["%s.%d.%d" % (base, a, b)
                for a in range(1, (max1 or dflt) + 1) for b in range(1, max2 + 1)]
    if max1:
        return ["%s.%d" % (base, a) for a in range(1, max1 + 1)]
    return [base]


def _expand_template(tpl, max1, max2, pool, cap):
    """把模板字面量（face.eye.${mood}.${num1}.${num2}.active）展开成具体路径。"""
    parts, i = [], 0
    while i < len(tpl):
        if tpl.startswith("${", i):
            j = tpl.find("}", i)
            if j < 0:
                break
            parts.append(("var", tpl[i + 2:j].strip()))
            i = j + 1
            continue
        parts.append(("lit", tpl[i]))
        i += 1
    merged = []
    for kind, val in parts:
        if kind == "lit" and merged and merged[-1][0] == "lit":
            merged[-1] = ("lit", merged[-1][1] + val)
        else:
            merged.append((kind, val))
    numeric = [v for k, v in merged if k == "var" and re.match(r"^num\d*$", v)]
    others = [v for k, v in merged if k == "var" and not re.match(r"^num\d*$", v)]
    num_ranges = []
    for idx in range(len(numeric)):
        if idx == 0 and max1:
            num_ranges.append(list(range(1, max1 + 1)))
        elif idx == 1 and max2:
            num_ranges.append(list(range(1, max2 + 1)))
        else:
            num_ranges.append(list(range(1, 7)))      # 静态读不出上界，1..6 兜底
    if not others:
        other_combos = [()]
    elif not pool:
        return []
    else:
        combos = [()]
        for _ in others:
            combos = [c + (v,) for c in combos for v in pool]
            if len(combos) > cap:
                combos = combos[:cap]
        other_combos = combos
    # 先把非数字占位符逐个组合固化成字面量；纯数字占位按范围枚举；
    # 模板里根本没有数字占位时，按 genPath 语义补 .num 后缀（prefix 是已插值的普通字符串）。
    out = []
    for combo in other_combos:
        sub = []
        k = 0
        for kind, val in merged:
            if kind == "var" and not re.match(r"^num\d*$", val):
                sub.append(("lit", combo[k] if k < len(combo) else ""))
                k += 1
            else:
                sub.append((kind, val))

        def emit(pos, acc):
            if len(out) >= cap:
                return
            if pos >= len(sub):
                out.append(acc)
                return
            kk, vv = sub[pos]
            if kk == "lit":
                emit(pos + 1, acc + vv)
                return
            for n in num_ranges[numeric.index(vv)]:
                emit(pos + 1, acc + str(n))

        if numeric:
            emit(0, "")
        else:
            base = "".join(v for _k, v in sub)
            for p in _suffix_expand(base, max1, max2):
                if len(out) >= cap:
                    break
                out.append(p)
        if len(out) >= cap:
            break
    return out


def _genpath_expansions(expr, cap=20000):
    """从条目表达式里抽出全部具体路径（裸字面量 + genPath 展开）。"""
    paths, eaten = [], []
    for m in re.finditer(r"\$utils\.genPath\s*\(", expr):
        op = m.end() - 1
        cl = _match_paren(expr, op)
        if cl < 0:
            continue
        eaten.append((m.start(), cl + 1))
        args = _split_args(expr[op + 1:cl])
        if not args:
            continue
        prefix = args[0]

        def _num(a):
            a = (a or "").strip()
            return int(a) if re.fullmatch(r"\d+", a) else 0

        max1 = _num(args[1]) if len(args) > 1 else 0
        max2 = _num(args[2]) if len(args) > 2 else 0
        pool = _pool(expr)
        if re.match(r"^['\"]", prefix):
            paths.extend(_suffix_expand(prefix[1:-1], max1, max2))
        else:
            tm = re.search("[" + BT + "]([^" + BT + "]*)" + "[" + BT + "]", prefix)
            if not tm:
                continue
            paths.extend(_expand_template(tm.group(1), max1, max2, pool, cap))
        if len(paths) > cap:
            break
    mask = list(expr)
    for a, b in eaten:
        for k in range(a, b):
            mask[k] = " "
    for m in re.finditer(r"""['"]([^'"\n]+)['"]""", "".join(mask)):
        v = m.group(1)
        if "." in v and re.match(r"^[A-Za-z0-9_.\-]+$", v):
            paths.append(v)
    seen, out = set(), []
    for p in paths:
        if p not in seen:
            seen.add(p)
            out.append(p)
    return out


def parse_portrait_consts(game, rel_index):
    """解析 ER_Consts.js 的 CATEGORIES / PORTRAITS：类别 -> 实际存在的图（尽力而为）。"""
    js_dir = os.path.join(game, "www", "js", "plugins")
    consts = os.path.join(js_dir, "ER_Consts.js")
    res = {"consts_file": consts if os.path.isfile(consts) else "", "categories": [],
           "layer_slots_declared": 0, "layer_paths_matched": 0, "categories_with_files": 0,
           "categories_detail": [], "uncovered_files": [], "uncovered_total": 0,
           "rel_total": len(rel_index), "note": []}
    if not res["consts_file"]:
        res["note"].append("未找到 ER_Consts.js，跳过类别常量解析")
        return res
    s = read_text(consts)
    m = re.search(r"const CATEGORIES = \{", s)
    if m:
        op = m.end() - 1
        cl = _match_paren(s, op)
        body = s[op + 1:cl] if cl > 0 else ""
        for km in re.finditer(r"^\s*(\w+)\s*:\s*'(\w+)'", body, re.M):
            res["categories"].append(km.group(1))
    m = re.search(r"const PORTRAITS = \{", s)
    if not m:
        res["note"].append("未找到 PORTRAITS 常量")
        return res
    op = m.end() - 1
    cl = _match_paren(s, op)
    body = s[op + 1:cl] if cl > 0 else ""
    heads = [(hm.start(), hm.end(), hm.group(1))
             for hm in re.finditer(r"\[\s*this\.CATEGORIES\.(\w+)\s*\]\s*:", body)]
    per_cat = {}
    for k, (a, b, cat) in enumerate(heads):
        end = heads[k + 1][0] if k + 1 < len(heads) else len(body)
        slots = _genpath_expansions(body[b:end])
        res["layer_slots_declared"] += len(slots)
        per_cat[cat] = slots
    covered = set()
    for cat in res["categories"]:
        slots = per_cat.get(cat, [])
        hit = set(p.replace(".", "/") for p in slots if p.replace(".", "/") in rel_index)
        covered |= hit
        if hit:
            res["categories_with_files"] += 1
        res["layer_paths_matched"] += len(hit)
        res["categories_detail"].append({"category": cat, "declared": len(slots),
                                         "matched_files": len(hit),
                                         "sample": sorted(hit)[:3]})
    res["note"].append(
        "layer_slots_declared 是静态展开的上界（同一模板里的多个变量按候选池做笛卡尔积，"
        "含插件运行时不会取到的组合）；layer_paths_matched 才是磁盘上真实存在的文件数")
    res["uncovered_files"] = sorted(rel_index - covered)[:80]
    res["uncovered_total"] = len(rel_index - covered)
    return res


def parse_actor_map(game, plugins):
    """尽力找 actorId -> 立绘前缀 映射；找不到就如实记为什么找不到。"""
    out = {"found": False, "map": {}, "source": "", "note": []}
    js_dir = os.path.join(game, "www", "js", "plugins")
    targets = list(plugins.get("portrait_plugin_files") or [])
    if not targets and os.path.isdir(js_dir):
        targets = [n for n in sorted(os.listdir(js_dir)) if "portrait" in n.lower()]
    for name in targets:
        p = os.path.join(js_dir, name)
        s = read_text(p)
        if not s:
            continue
        out["source"] = os.path.relpath(p, game)
        for m in re.finditer(r"(?:ACTOR_PREFIX|PORTRAIT_PREFIX|actorPrefix\w*|portraitPrefix\w*)\s*=\s*\{([^}]*)\}", s):
            for km in re.finditer(r"(\d+)\s*:\s*['\"]([^'\"]+)['\"]", m.group(1)):
                out["map"][km.group(1)] = km.group(2)
        if out["map"]:
            out["found"] = True
            out["note"].append("从 %s 的显式常量表解析出 actor->前缀" % name)
            continue
        has_actor_key = bool(re.search(r"this\.portraits\s*\[\s*actorId\s*\]", s))
        hm = re.search(r"loadBitmap\(\s*[" + QUOTES + r"]([^" + QUOTES + r"]*)[" + QUOTES + r"]", s)
        load_dir = hm.group(1) if hm else ""
        out["load_dir_template"] = load_dir
        out["actor_indexed_state"] = has_actor_key
        if load_dir:
            tpl_vars = re.findall(r"\$\{([^}]+)\}", load_dir)
            out["load_dir_vars"] = tpl_vars
            if tpl_vars and not any("actor" in v.lower() for v in tpl_vars):
                out["note"].append(
                    "未找到 actor->前缀映射：%s 用 ImageManager.loadBitmap('%s') 拼名，"
                    "目录模板变量 %s 与 actorId 无关；portraits[actorId] 只存每个 actor 的部件状态"
                    "（defaultPortraits 键 1..4），部件路径由 $consts.PORTRAITS 的类别清单决定 —— "
                    "即全部 actor 共用同一套分层图，没有 actor 专属前缀。" % (name, load_dir, tpl_vars))
        for pm in plugins.get("portrait_entries", []):
            if not pm["parameters"]:
                out["note"].append("plugins.js 里 %s 的 parameters 是空对象（没有配置参数可解析）" % pm["name"])
    if not out["note"]:
        out["note"].append("未找到任何 Portrait 相关插件源码，无法解析 actor 映射")
    return out


# ---------- 导出 ----------

def _decrypt_into(src, dst, kind, keys):
    """调用 rpg_decrypt.decrypt_one（成功时返回 4 元组，个别失败分支返回 3 元组）。"""
    rd = load_decryptor()
    if not rd:
        return "fail", "无解密器", "", ""
    res = tuple(rd.decrypt_one(src, dst, kind, keys))
    if len(res) < 4:
        res = res + ("",)
    return res[0], res[1], res[2], res[3]


def export_library(items, out_dir, game_name, notes):
    os.makedirs(os.path.join(out_dir, "cg"), exist_ok=True)
    cgs, seen, fail = [], set(), []
    total = 0
    for it in items:
        cid = safe_id(it["rel"])
        base, n = cid, 1
        while cid in seen:
            n += 1
            cid = "%s_%d" % (base, n)
        seen.add(cid)
        d = os.path.join(out_dir, "cg", cid)
        os.makedirs(d, exist_ok=True)
        ext_out = it["ext_out"]
        dst = os.path.join(d, cid + ext_out)
        try:
            if it.get("decrypted_from"):
                st, why, _label, written = _decrypt_into(it["abs"], dst, it["decrypted_from"], it.get("keys") or [])
                if st not in ("ok", "plain"):
                    fail.append("%s: %s" % (it["rel"], why))
                    continue
                fn = os.path.basename(written)
                ext_out = os.path.splitext(fn)[1] or ext_out
            else:
                with open(it["abs"], "rb") as fi, open(dst, "wb") as fo:
                    while True:
                        b = fi.read(1 << 20)
                        if not b:
                            break
                        fo.write(b)
                ok, why = ihdr_ok(dst) if ext_out == ".png" else (True, "")
                if not ok:
                    fail.append("%s: %s" % (it["rel"], why))
                    try:
                        os.remove(dst)
                    except OSError:
                        pass
                    continue
                fn = os.path.basename(dst)
            sz = os.path.getsize(os.path.join(d, fn))
            total += sz
            cgs.append({"id": cid, "kind": "still", "category": it["group"],
                        "event_group": None, "characters": [],
                        "files": {"png": [fn]}, "bytes": sz, "source_rel": it["rel"]})
        except OSError as e:
            fail.append("%s: IO %s" % (it["rel"], e))
    idx = {
        "schema": 1, "engine": "unity", "game": game_name,
        "generated_at": time.strftime("%Y-%m-%dT%H:%M:%S"),
        "source": {"kind": "rpg-maker-mv portraits (plugin-driven layers)",
                   "portrait_root": "img/portraits"},
        "counts": {"cg": len(cgs), "png": len(cgs), "text": 0, "spine": 0,
                   "frames": 0, "still": len(cgs), "orphan_png": 0, "orphan_text": 0},
        "bytes": total,
        "cgs": cgs,
        "notes": notes,
    }
    with open(os.path.join(out_dir, "cg-index.json"), "w", encoding="utf-8") as f:
        json.dump(idx, f, ensure_ascii=False, indent=2)
    return cgs, total, fail


# ---------- 验证 ----------

def verify_events(out_dir):
    """bin/ucgr.exe events <dir>：走 unitycg 链路，读「CG 事件 N 个」。"""
    res = {"cmd": "", "ok": False, "cg_events": 0, "scene_events": 0, "chapters": 0,
           "total_events": 0, "stdout_tail": "", "error": "", "exit_code": None,
           "note": "unitycg 链路把「一部 CG」记成一个场景事件，CG 画廊位（CG 事件）恒为 0，"
                   "这是该链路的正常语义；events>0 以 total_events 判定"}
    if not os.path.isfile(UCGR):
        res["error"] = "bin/ucgr.exe 不存在：%s" % UCGR
        return res
    res["cmd"] = "%s events %s" % (UCGR, out_dir)
    try:
        p = subprocess.run([UCGR, "events", out_dir], capture_output=True, timeout=600)
    except Exception as e:
        res["error"] = "执行失败: %s" % e
        return res
    out = (p.stdout or b"").decode("utf-8", "replace") + (p.stderr or b"").decode("utf-8", "replace")
    res["stdout_tail"] = out[-4000:]
    res["exit_code"] = p.returncode
    m = re.search(r"CG 事件 (\d+) 个[^\n]*?场景事件 (\d+) 个[^\n]*?演出段落 (\d+) 个", out)
    if m:
        res["cg_events"] = int(m.group(1))
        res["scene_events"] = int(m.group(2))
        res["chapters"] = int(m.group(3))
        res["total_events"] = res["cg_events"] + res["scene_events"] + res["chapters"]
        res["ok"] = (p.returncode == 0 and res["total_events"] > 0)
    else:
        res["error"] = "输出里没有事件统计行（exit=%s）" % p.returncode
    return res


# ---------- 主流程 ----------

def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--game", default=DEFAULT_GAME)
    ap.add_argument("--out", default="")
    ap.add_argument("--report", default="")
    ap.add_argument("--max-images", type=int, default=MAX_IMAGES)
    ap.add_argument("--keep", type=int, default=KEEP_IMAGES)
    ap.add_argument("--dry-run", action="store_true")
    ap.add_argument("--no-verify", action="store_true")
    a = ap.parse_args()

    t0 = time.time()
    game = os.path.abspath(a.game)
    name = os.path.basename(game.rstrip("\\/")) or "game"
    out_dir = a.out or os.path.join(OUT_ROOT, name)
    report = a.report or os.path.join(DATA, "portrait_%s.json" % re.sub(r"[^0-9A-Za-z_\-]", "_", name))

    if not os.path.isdir(game):
        raise SystemExit("游戏目录不存在：%s" % game)
    root, root_kind = find_portrait_root(game)
    if not root:
        raise SystemExit("没有找到 img/portraits 或 img/ 子树：%s" % game)
    if os.path.abspath(out_dir).lower().startswith(os.path.abspath(game).lower() + os.sep):
        raise SystemExit("输出目录落在游戏目录内，拒绝执行：%s" % out_dir)

    files = walk_images(root)
    plain, enc = {}, {}
    for f in files:
        key = os.path.splitext(f["rel"])[0].replace(os.sep, "/").lower()
        if f["ext"] in IMG_EXT:
            plain.setdefault(key, f)
        else:
            enc.setdefault(key, f)

    raw_groups = {}          # 磁盘文件数（含 .rpgmvp 重复）
    for f in files:
        rel = f["rel"].replace(os.sep, "/")
        g = rel.split("/")[0] if "/" in rel else "(root)"
        raw_groups[g] = raw_groups.get(g, 0) + 1

    rd = load_decryptor()
    keys = rd.game_keys(game) if rd else []

    items = []
    for key in sorted(set(plain) | set(enc)):
        rel = (plain.get(key) or enc.get(key))["rel"].replace(os.sep, "/")
        group = rel.split("/")[0] if "/" in rel else "(root)"
        if key in plain:
            f = plain[key]
            items.append({"rel": rel, "abs": f["abs"], "bytes_src": f["size"],
                          "group": group, "ext_out": f["ext"], "decrypted_from": ""})
        else:
            f = enc[key]
            items.append({"rel": rel, "abs": f["abs"], "bytes_src": f["size"],
                          "group": group, "ext_out": ".png",
                          "decrypted_from": "rpgmvp" if f["ext"] == ".rpgmvp" else "png_",
                          "keys": keys})

    groups = {}              # 位点数（同一张图的 .png / .rpgmvp 只算一个）
    for it in items:
        groups[it["group"]] = groups.get(it["group"], 0) + 1

    partial_note = ""
    total_slots = len(items)
    if total_slots > a.max_images:
        items.sort(key=lambda r: r["bytes_src"], reverse=True)
        dropped = total_slots - a.keep
        items = items[:a.keep]
        items.sort(key=lambda r: r["rel"].lower())
        partial_note = ("partial: 立绘 %d 张 > %d 上限，按源字节数取最大 %d 张导出，余 %d 张未导出"
                        % (total_slots, a.max_images, len(items), dropped))

    notes = ["portrait_export: RPG Maker MV 插件驱动立绘（ER_PortraitManager 运行时拼名，事件脚本里无图片字面量）",
             "枚举源目录 %s（%s）；明文图直拷，加密图经 rpg_decrypt 解密后落产物目录，源目录只读"
             % (root_kind, root)]
    if partial_note:
        notes.append(partial_note)
    if enc and all((k in plain) for k in enc):
        notes.append("加密图（%d 张 .rpgmvp）与明文 .png 逐一同名成对，故全部走明文直拷；"
                     "解密分支（rpg_decrypt.decrypt_one）只在「只有加密图」的游戏上启用" % len(enc))

    plugins = parse_plugins_js(game)
    rel_index = set(os.path.splitext(f["rel"].replace(os.sep, "/"))[0] for f in files)
    consts = parse_portrait_consts(game, rel_index)
    actor = parse_actor_map(game, plugins)

    print("GAME   %s" % game)
    print("ROOT   %s (%s)" % (root, root_kind))
    print("FILES  明图 %d · 加密图 %d · 位点 %d · 分组 %d" % (len(plain), len(enc), total_slots, len(groups)))
    for g in sorted(groups):
        print("       %-16s 位点 %-5d 磁盘文件 %d" % (g, groups[g], raw_groups.get(g, 0)))
    print("PLUGIN plugins.js=%s · Portrait 条目 %d · portrait 插件源码 %s"
          % (bool(plugins["plugins_js"]), len(plugins["portrait_entries"]),
             ",".join(plugins["portrait_plugin_files"]) or "-"))
    print("CONSTS 类别 %d · 声明槽位 %d · 命中文件 %d / 目录内 %d"
          % (len(consts["categories"]), consts["layer_slots_declared"],
             consts["layer_paths_matched"], len(rel_index)))
    print("ACTORMAP found=%s" % actor["found"])

    if a.dry_run:
        print("DRY-RUN 不写产物；预计导出 %d 张 -> %s" % (len(items), out_dir))
        return

    cgs, total_bytes, fail = export_library(items, out_dir, name, notes)
    print("EXPORT cg=%d bytes=%.2fMB fail=%d -> %s"
          % (len(cgs), total_bytes / 1048576.0, len(fail), out_dir))

    if a.no_verify:
        ver = {"ok": False, "cg_events": 0, "error": "skipped(--no-verify)"}
    else:
        ver = verify_events(out_dir)
        print("VERIFY events=%d（CG %d + 场景 %d + 段落 %d）ok=%s %s"
              % (ver.get("total_events", 0), ver.get("cg_events", 0), ver.get("scene_events", 0),
                 ver.get("chapters", 0), ver.get("ok"), ver.get("error", "")))

    group_detail = []
    for g in sorted(groups):
        gi = [it for it in items if it["group"] == g]
        group_detail.append({"group": g, "files_on_disk": raw_groups.get(g, 0),
                             "layer_slots": len(gi),
                             "bytes_source": sum(it["bytes_src"] for it in gi),
                             "exported": len([c for c in cgs if c["category"] == g])})
    rep = {
        "generated_at": time.strftime("%Y-%m-%d %H:%M:%S"),
        "game": name, "game_path": game,
        "portrait_root": root, "portrait_root_kind": root_kind, "out_dir": out_dir,
        "dry_run": bool(a.dry_run),
        "enumerate": {
            "files_total": len(files),
            "plain_images": len(plain),
            "encrypted_images": len(enc),
            "slots": total_slots,
            "to_export": len(items),
            "bytes_source": sum(it["bytes_src"] for it in items),
            "groups": len(groups),
            "group_detail": group_detail,
            "files": [{"rel": it["rel"], "bytes": it["bytes_src"],
                       "from": it["decrypted_from"] or "plain"} for it in items],
        },
        "partial": partial_note,
        "export": {
            "cg_count": len(cgs), "bytes_written": total_bytes, "failed": len(fail),
            "fail_samples": fail[:10],
            "cg_index": os.path.join(out_dir, "cg-index.json"),
            "id_rule": "相对路径安全化（子目录_文件名），每图一个 cg/<id>/ 单图目录",
        },
        "plugins": {
            "plugins_js": plugins["plugins_js"], "plugins_dir": plugins["plugins_dir"],
            "plugin_files_total": plugins["plugin_files_total"],
            "plugin_count": len(plugins["names"]),
            "portrait_entries": plugins["portrait_entries"],
            "portrait_plugin_files": plugins["portrait_plugin_files"],
        },
        "portrait_consts": consts,
        "actor_prefix_map": actor,
        "verify": ver,
        "summary": {
            "portraits_total": len(files),
            "portrait_slots": total_slots,
            "groups": len(groups),
            "groups_detail": {g: groups[g] for g in sorted(groups)},
            "exported_cg": len(cgs),
            "exported_bytes": total_bytes,
            "events_total": ver.get("total_events", 0),
            "events_ok": bool(ver.get("ok")),
            "layer_categories": len(consts["categories"]),
            "layer_files_matched": consts["layer_paths_matched"],
            "actor_prefix_map_found": bool(actor["found"]),
        },
        "elapsed_sec": round(time.time() - t0, 1),
    }
    os.makedirs(os.path.dirname(os.path.abspath(report)), exist_ok=True)
    with open(report, "w", encoding="utf-8") as f:
        json.dump(rep, f, ensure_ascii=False, indent=2)
    print("REPORT %s" % os.path.abspath(report))


if __name__ == "__main__":
    main()
