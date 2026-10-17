#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""ks_refs.py —— P0 原型：KiriKiri KAG 脚本解析 + 资源引用提取 + ④体积测算（只读）

目标（对应 05 文档 P0-4/P0-7 的最小可跑版本）：
  从解包目录的 .ks 剧本中提取 storage=/file=/src= 资源引用，
  匹配到解包树真实文件，得出「CG 演出实际引用资源」并集体积（M0 第④项），
  与 ③ 全量解包对比得到裁剪率预估。

解码探测（按 KAG 特征打分选最优）：
  1) UTF-16LE（带 BOM）          夏空彼方 / 枯花
  2) 单字节 XOR k 后成 UTF-16LE   海市蜃楼之馆（k=0xA4）
  3) UTF-8 明文                  重返蓝鲸岛
  4) Shift_JIS 明文              9nine 全系
  5) 都不行 → 标记 unrecognized（保守保留整游戏，宁多勿缺）

用法：
  python ks_refs.py <解包目录> [--json out.json]
  python ks_refs.py --all          # 跑全部 11 款 KiriKiri
"""
import argparse
import glob
import json
import os
import re
import sys
import unicodedata

try:
    sys.stdout.reconfigure(encoding="utf-8")
except Exception:
    pass

GB = 1024.0 ** 3
KIRIKIRI_GAMES = [
    "9nine九次九日解包", "9nine天色解包", "9nine新章解包", "9nine春色解包", "9nine雪色解包",
    "G線上の魔王解包", "Monkeys!!解包", "夏空彼方解包", "枯花解包", "海市蜃楼之馆解包",
    "重返蓝鲸岛解包",
]

TAG_RE = re.compile(r"[@\[]([A-Za-z_][A-Za-z0-9_]*)((?:\s+[^\]]*?)?)[\]]", re.S)
ATTR_RE = re.compile(r"([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(\"[^\"]*\"|'[^']*'|[^\s\]@]+)")
TAGSTART_RE = re.compile(r"[@\[]([A-Za-z])")
KAG_HINT = re.compile(r"\[(macro|endmacro|macro|name|text|storage|eval|end\b|if|else|elsif|endif|jump|return|call|wait|ws|bg|cg|chara|playbgm|stopbgm|vo|movie|trans|sel|graph|position|layer|clearlayers|history|sysmovie|linemode|init|endmacro)")

# 资源类属性（保守：全部采集，宁多勿缺）
# 注意：不含 layer —— layer= 是图层名（message0/stage），不是资源
RES_ATTRS = ("storage", "file", "src", "sound")
# 这些标签的 storage 是【脚本装载目标】，不是资产
SCRIPT_TAGS = {"jump", "call", "addsysscript", "addsyshook"}
NUMERIC_RE = re.compile(r"^[0-9]+$")
# 无括号 @宏 调用（G線 实测：@chr c=… 这种行尾即止、没有 ] 的形态）
AT_CALL_RE = re.compile(r"^@([A-Za-z_][A-Za-z0-9_]*)\s*(.*)$")
# 按标签扩展的资源属性（游戏自定义宏集，2026-10-16 G線 实测）：
#   @chr c/l/r/ll/rr = 立绘 中/左/右/次左/次右；@mface name = 差分；[nm s=] = 语音
TAG_EXTRA_ATTRS = {
    "chr": ("c", "l", "r", "ll", "rr"),
    "mface": ("name",),
    "nm": ("s",),
}
# 常见资源扩展名（用于无扩展名引用的补全尝试；含 KiriKiri 与 CatSystem2 两套）
RES_EXTS = [".tlg", ".png", ".jpg", ".jpeg", ".bmp", ".ogg", ".wav", ".mp3",
            ".mpg", ".mpeg", ".wmv", ".ogv", ".asf", ".avi", ".mp4", ".csv",
            ".txt", ".tjs", ".asd", ".ks", ".ma", ".amv", ".sli", ".func",
            ".scn", ".sinfo", ".pimg", ".fdr", ".psb", ".dat",
            ".hg3", ".hg2", ".fes", ".kcs", ".kx2", ".mot", ".ptcl", ".lpb",
            ".psz", ".zt", ".anm", ".mov"]

ROLE_RULES = [
    ("evimage", "CG事件图"), ("cg/", "CG事件图"), ("cgi/", "CG事件图"),
    ("fgimage", "立绘差分"), ("chara", "立绘差分"),
    ("bgimage", "背景"), ("bg/", "背景"),
    ("voice", "语音"), ("vo/", "语音"),
    ("bgm", "BGM"), ("sound", "音效/BGM"), ("se", "音效/BGM"),
    ("video", "事件视频"), ("movie", "事件视频"),
]


def _kag_score(t):
    """结构分：真实 KAG 脚本有大量 [tag 起始与 storage=/text=；随机字节里极罕见。"""
    return (3 * len(TAGSTART_RE.findall(t)) + 2 * t.count("storage=")
            + t.count("text=") + len(KAG_HINT.findall(t)))


def _sjis_repair(raw):
    """修复 Shift_JIS 里的孤立非法字节（9nine CxEncryption 每文件 2 个例外点，
    前人解密时未修文本）。返回 (修复后字节, 修了几处)。"""
    out = bytearray(raw)
    fixed = 0
    i = 0
    n = len(out)
    while i < n:
        x = out[i]
        if x < 0x80:
            i += 1
            continue
        lead = 0x81 <= x <= 0x9F or 0xE0 <= x <= 0xEF or 0xF0 <= x <= 0xFC
        if lead and i + 1 < n and (0x40 <= out[i + 1] <= 0x7E or 0x80 <= out[i + 1] <= 0xFC):
            i += 2
            continue
        # 非法序列起点：爆破该字节，取能让本位置合法且下一字节也合理的值
        best = None
        for v in range(256):
            out[i] = v
            vlead = 0x81 <= v <= 0x9F or 0xE0 <= v <= 0xEF or 0xF0 <= v <= 0xFC
            if vlead:
                if i + 1 < n and (0x40 <= out[i + 1] <= 0x7E or 0x80 <= out[i + 1] <= 0xFC):
                    best = v
                    break
            elif v < 0x80:
                # 单字节 ASCII：仅当下一字节能独立合法时才算候选
                nx = out[i + 1] if i + 1 < n else 0x20
                if nx < 0x80 or (0x81 <= nx <= 0x9F or 0xE0 <= nx <= 0xEF):
                    best = v
                    break
        if best is None:
            out[i] = 0x20  # 兜底空格
            best = 0x20
        out[i] = best
        fixed += 1
        i += 1 if best < 0x80 or not (0x81 <= best <= 0x9F or 0xE0 <= best <= 0xEF or 0xF0 <= best <= 0xFC) else 2
    return bytes(out), fixed


def weak_ok(D):
    """弱接受：解密结果干净（坏字符≤1%）且含 KAG 标记。
    用于 dummy_scene.ks 这类无 [tag] 的零引用存根（结构分打不够但确实解对了）。
    只应作用于 VM/常量模型等可信来源的解密结果，不用于裸磁盘文件。"""
    t = D.decode("shift_jis", errors="replace")
    if not t:
        return False
    bad = t.count("\ufffd")
    if bad > max(2, 0.01 * len(t)):
        return False
    return any(m in t for m in ("[", "@", "*", ";")) and ("\r\n" in t or "\n" in t)


def decode_ks(raw):
    """返回 (text, how)；无法解码返回 (None, reason)。"""
    # 1) UTF-16LE BOM
    if raw[:2] == b"\xff\xfe":
        try:
            return raw[2:].decode("utf-16-le", errors="replace"), "utf16le-bom"
        except Exception:
            pass
    # 2) 单字节 XOR 出 BOM
    for k in range(1, 256):
        if bytes(b ^ k for b in raw[:2]) == b"\xff\xfe":
            try:
                d = bytes(b ^ k for b in raw)
                return d[2:].decode("utf-16-le", errors="replace"), "xor-0x%02x+utf16le" % k
            except Exception:
                break
    # 3) UTF-8 / SJIS 明文（允许替换符：9nine 解密文本带 2 个未修例外点）；
    #    4) 无 BOM UTF-16LE（KiriKiri 常见）
    best = (0, None, None)
    for enc in ("utf-8", "shift_jis", "cp932", "utf-16-le"):
        try:
            t = raw.decode(enc, errors="replace")
        except Exception:
            continue
        sc = _kag_score(t)
        if enc in ("shift_jis", "cp932"):
            rep, nf = _sjis_repair(raw)
            if nf:
                t2 = rep.decode(enc, errors="replace")
                sc2 = _kag_score(t2)
                if sc2 > sc:
                    sc, t = sc2, t2
        # UTF-16LE 需额外证据：高位字节大量为 0x00/0x30（ASCII/假名）
        if enc == "utf-16-le" and len(raw) >= 8:
            hi = raw[1::2]
            if sum(1 for b in hi if b in (0x00, 0x30)) < len(hi) * 0.5:
                sc = 0
        # 守卫按【候选】应用：坏字符超 5% 的候选直接出局
        # （否则 SJIS 字节被 UTF-8 解成乱码时 ASCII 结构分持平、凭顺序胜出，再把好候选挤掉）
        if t.count("\ufffd") > 0.05 * max(1, len(t)):
            sc = 0
        if sc > best[0]:
            best = (sc, t, "plain-" + enc)
    if best[1] is not None and best[0] >= 10:
        return best[1], best[2]
    return None, "undecodable"


ISCRIPT_RE = re.compile(r"\[iscript\][\s\S]*?\[endscript\]", re.I)
ENDSCRIPT_ONLY_RE = re.compile(r"\[iscript\][\s\S]*", re.I)


def strip_code_blocks(text):
    """去掉 [iscript]..[endscript] 的 JS 块，避免把 JS 代码当属性提取。"""
    t = ISCRIPT_RE.sub("", text)
    t = ENDSCRIPT_ONLY_RE.sub("", t)
    return t


def nfkc(s):
    """全角/半角归一，用于引用匹配。
    注意保护 ／(U+FF0F)：NFKC 会把它转成 /，而它是合法文件名字符
    （Windows 文件名可含 ／），转成 / 会伪造出路径分隔符导致匹配失败。"""
    s = s.replace("\uff0f", "\x00")
    s = unicodedata.normalize("NFKC", s).lower()
    return s.replace("\x00", "\uff0f")


def _collect(body, tag, refs):
    """从一段标签体里采集资源引用。tag 决定可用哪些扩展属性。"""
    extra = TAG_EXTRA_ATTRS.get(tag, ())
    for am in ATTR_RE.finditer(body):
        k = am.group(1).lower()
        if k not in RES_ATTRS and k not in extra:
            continue
        v = am.group(2).strip("\"'")
        if not v:
            continue
        if v[0] in "%&$":
            # %param|default：取默认值部分（param 部分是动态变量名）
            if "|" in v:
                v = v.split("|", 1)[1]
                if not v or v[0] in "%&$":
                    continue
            else:
                continue
        if NUMERIC_RE.match(v):
            continue
        if k == "storage" and tag in SCRIPT_TAGS:
            continue
        refs.append(v.replace("\\", "/"))


def parse_refs(text):
    """从 KAG 文本提取资源引用。先剥掉 iscript JS 块。

    两种标签形态都要处理：
      1. 有括号  [tag k=v ...] / @tag k=v ...]   （TAG_RE）
      2. 无括号  @tag k=v ...                    （AT_CALL_RE，G線 的 @chr c=… 形态）
    噪声过滤（2026-10-15/16 实测驱动）：
      - 注释行（首个非空白字符为 ;）整行跳过——里面是被作者注掉的旧代码
      - 纯数字值跳过——[se storage="5000"] 是音效 ID 不是文件
      - SCRIPT_TAGS 的 storage 跳过——[jump storage=x.ks] 是脚本目标
      - %参数|默认值 语法取 | 后的默认值（KAG 宏默认参数，常是真资源）
    """
    refs = []
    for line in strip_code_blocks(text).splitlines():
        if line.lstrip().startswith(";"):
            continue
        # 无括号 @宏 调用（G線：@chr c=eiichi_… / @mface name=… / @bg storage=…）
        m = AT_CALL_RE.match(line.strip())
        if m:
            _collect(m.group(2), m.group(1).lower(), refs)
        # 有括号标签（含 @tag ...] 形式）
        for tm in TAG_RE.finditer(line):
            _collect(tm.group(2) or "", tm.group(1).lower(), refs)
    return refs


def match_all(ref, pool, pool_by_name):
    """match_ref 的多文件版：返回该引用对应的全部池内文件。

    与 match_ref 的差别只在最后一层：basename 命中多个时（如基础卷与汉化
    覆盖卷同名文件、image00/image01 同图），match_ref 因歧义放弃，
    本函数把同名文件全部返回——它们是同一资产的多个版本，都算被引用。
    """
    k = match_ref(ref, pool, pool_by_name)
    if k:
        return [k]
    r = nfkc(ref.lower().lstrip("./"))
    if not os.path.splitext(r)[1]:
        for ext in RES_EXTS:
            hits = pool_by_name.get(r + ext)
            if hits:
                return sorted(set(hits))
        cand = [k for k in pool if k.rsplit("/", 1)[-1].startswith(r + ".")]
        if len(cand) == 1:
            return cand
    else:
        hits = pool_by_name.get(r)
        if hits:
            return sorted(set(hits))
    return []


def build_substr_index(pool_by_name):
    """3-gram 倒排索引：保留「文件名含 token」的原子串语义，但查询 O(1)。

    大池（10万+文件）下逐个引用扫全池太慢（--all 会被拖死），
    故建一次倒排：每个文件 basename 的所有 3-gram → [pool keys]。
    """
    idx = {}
    for name, paths in pool_by_name.items():
        base = os.path.splitext(name)[0].lower()
        seen = set()
        for i in range(len(base) - 2):
            g = base[i:i + 3]
            if g not in seen:
                seen.add(g)
                idx.setdefault(g, []).extend(paths)
    return idx


def dynamic_candidates(ref, pool_by_name, substr_index=None):
    """未命中引用的【保守候选】：防误裁。

    现象：事件 CG 只经映射 ID 引用（枯花 [ev file=ev01a] → evimage/Haru_01a.png，
    真实文件名从不出现在脚本里）。映射表在引擎/TJS 里，静态不可解。
    做法：剥常见前缀（ev/e/large_/bg_/cg_）与取尾部数字字母核心 token，
    把「文件名含该 token」的池内文件全部列为候选——宁多勿缺（安全方向）。
    token 需 ≥3 字符且含字母，避免 'op'/'0' 之类滥匹配。
    substr_index 提供时走倒排（语义不变、速度快）。
    """
    r = nfkc(ref.lower())
    tokens = {r}
    for pre in ("ev", "e", "large_", "bg_", "cg_"):
        if r.startswith(pre) and len(r) > len(pre) + 2:
            tokens.add(r[len(pre):])
    m = re.search(r"([0-9]+[a-z][0-9a-z_]*)$", r)
    if m:
        tokens.add(m.group(1))
    hits = []
    for tok in tokens:
        if len(tok) < 3 or not any(c.isalpha() for c in tok):
            continue
        if substr_index is not None:
            grams = [tok[i:i + 3] for i in range(len(tok) - 2)]
            postings = [substr_index.get(g) for g in grams]
            if any(p is None for p in postings):
                continue
            postings.sort(key=len)
            cand, rest = postings[0], postings[1:]
            for k in cand:
                if all(k in p for p in rest):
                    hits.append(k)
        else:
            for name, paths in pool_by_name.items():
                base = os.path.splitext(name)[0]
                if tok in base:
                    hits.extend(paths)
    return sorted(set(hits))


def build_pool(game_dir):
    """资产池 = raw/ 优先，否则整树（排除 解包输出 副本树与 .ks）。"""
    raw_dir = os.path.join(game_dir, "raw")
    base = raw_dir if os.path.isdir(raw_dir) else game_dir
    pool = {}          # NFKC 归一相对路径(小写) -> (abspath, size)
    for dp, dirs, fs in os.walk(base):
        dirs[:] = [d for d in dirs if d not in ("解包输出", "_分类")]
        for fn in fs:
            if fn.lower().endswith(".ks"):
                continue
            ap = os.path.join(dp, fn)
            rel = nfkc(os.path.relpath(ap, base).replace("\\", "/"))
            try:
                pool[rel] = (ap, os.path.getsize(ap))
            except OSError:
                pass
    return base, pool


def match_ref(ref, pool, pool_by_name):
    """三级匹配：精确 → 补全扩展名 → 同名文件（唯一时）。引用侧与池侧均 NFKC 归一。"""
    r = nfkc(ref.lstrip("./"))
    if r in pool:
        return r
    for ext in RES_EXTS:
        if r + ext in pool:
            return r + ext
    if not os.path.splitext(r)[1]:
        cand = [k for k in pool if k.startswith(r + ".")]
        if len(cand) == 1:
            return cand[0]
    # 去掉可能的前缀目录再试（storage 常相对卷根）
    name = r.rsplit("/", 1)[-1]
    cand = pool_by_name.get(name)
    if cand and len(cand) == 1:
        return cand[0]
    for ext in RES_EXTS:
        cand = pool_by_name.get(name + ext)
        if cand and len(cand) == 1:
            return cand[0]
    return None


def role_of(path):
    p = path.lower()
    for key, role in ROLE_RULES:
        if key in p:
            return role
    return "其它"


def run_game(game_dir, verbose=True):
    base, pool = build_pool(game_dir)
    pool_by_name = {}
    for k in pool:
        pool_by_name.setdefault(k.rsplit("/", 1)[-1], []).append(k)

    # 修复目录（ks_fix9.py 的产出）：解不动的脚本回退到这里读
    fix_root = os.path.join(os.path.dirname(game_dir.rstrip("/\\")),
                            os.path.basename(game_dir.rstrip("/\\")) + "_脚本修复")

    ks_files = []
    for dp, dirs, fs in os.walk(base):
        dirs[:] = [d for d in dirs if d not in ("解包输出", "_分类")]
        for fn in fs:
            if fn.lower().endswith(".ks"):
                ks_files.append(os.path.join(dp, fn))
    ks_files.sort()

    all_refs = {}        # 规范化引用 -> 次数
    script_refs = set()  # 指向剧本（.ks/.tjs）的引用：剧本按 DR-25 全留，不算资产未命中
    bad_scripts = []     # 解不动的脚本
    total_script_bytes = 0
    for p in ks_files:
        try:
            raw = open(p, "rb").read()
        except OSError:
            continue
        total_script_bytes += len(raw)
        text, how = decode_ks(raw)
        if text is None and os.path.isdir(fix_root):
            # 回退到 ks_fix9.py 的修复副本（兼容两种 rel 约定）
            rel = os.path.relpath(p, base)
            cands = [os.path.join(fix_root, rel)]
            parts = rel.replace("\\", "/").split("/")
            if len(parts) > 1:
                cands.append(os.path.join(fix_root, *parts[1:]))
            import glob as _gl
            cands += _gl.glob(os.path.join(fix_root, "**", os.path.basename(p)), recursive=True)
            for fp in cands:
                if os.path.isfile(fp):
                    try:
                        fx = open(fp, "rb").read()
                    except OSError:
                        continue
                    t2, how2 = decode_ks(fx)
                    if t2 is None and weak_ok(fx):
                        # 修复副本是可信来源：弱接受（如 dummy_scene 零引用存根）
                        t2, how2 = fx.decode("shift_jis", "replace"), "weak"
                    if t2 is not None:
                        text, how = t2, how2 + "+fixed"
                        break
        if text is None:
            bad_scripts.append(os.path.relpath(p, base))
            continue
        for r in parse_refs(text):
            if r.lower().endswith(".ks") or r.lower().endswith(".tjs"):
                script_refs.add(r.lower())
                continue
            if r in all_refs:
                all_refs[r] += 1
            else:
                all_refs[r] = 1

    matched = {}        # pool key -> 引用次数
    unmatched = {}
    for ref, n in all_refs.items():
        k = match_ref(ref, pool, pool_by_name)
        if k:
            matched[k] = matched.get(k, 0) + n
        else:
            unmatched[ref] = n

    # 未命中引用的保守保留（防误裁）：映射 ID 类引用把候选文件一并计入 ④
    dynamic = {}        # pool key -> [触发它的未命中引用]
    substr_idx = build_substr_index(pool_by_name)
    for ref in sorted(unmatched):
        for k in dynamic_candidates(ref, pool_by_name, substr_idx):
            if k not in matched:
                matched[k] = 0
                dynamic[k] = [ref]
            elif k in dynamic and ref not in dynamic[k]:
                dynamic[k].append(ref)

    # DR-16：BGM 全部保留 → ④ = 引用命中 ∪ 池内全部 BGM
    bgm_all = [k for k in pool if "bgm" in k]
    for k in bgm_all:
        matched.setdefault(k, 0)

    mbytes = sum(pool[k][1] for k in matched)
    pool_bytes = sum(v[1] for v in pool.values())
    roles = {}
    for k in matched:
        roles.setdefault(role_of(k), [0, 0])
        roles[role_of(k)][0] += 1
        roles[role_of(k)][1] += pool[k][1]

    rec = {
        "game": os.path.basename(game_dir.rstrip("/\\")),
        "ks_files": len(ks_files),
        "ks_bytes": total_script_bytes,
        "undecodable_ks": bad_scripts,
        "distinct_refs": len(all_refs),
        "script_refs": len(script_refs),
        "matched": len(matched),
        "unmatched": len(unmatched),
        "unmatched_samples": sorted(unmatched)[:15],
        "dynamic_retained": len(dynamic),
        "dynamic_bytes": sum(pool[k][1] for k in dynamic),
        "matched_bytes": mbytes,
        "pool_bytes": pool_bytes,
        "pool_files": len(pool),
        "bgm_all": len(bgm_all),
        "ratio": (mbytes / pool_bytes) if pool_bytes else 0.0,
        "roles": roles,
    }
    if verbose:
        print("== %s" % rec["game"])
        print("   ks=%d(%.1f MB) 解不动=%d  distinct资产引用=%d 剧本引用=%d 命中=%d 未命中=%d 保守保留=%d(%.2fGB)"
              % (rec["ks_files"], rec["ks_bytes"] / 1048576, len(bad_scripts),
                 rec["distinct_refs"], rec["script_refs"], rec["matched"], rec["unmatched"],
                 rec["dynamic_retained"], rec["dynamic_bytes"] / GB))
        if bad_scripts:
            print("   ⚠️ 解不动脚本 %d 个 → 按安全原则整游戏保守保留" % len(bad_scripts))
        if rec["unmatched_samples"]:
            print("   未命中样例: %s" % ", ".join(rec["unmatched_samples"][:8]))
        print("   ④ %.2f GB（含BGM全留 %d 个）/ 池 %.2f GB（%d 文件） → 保留率 %.1f%%"
              % (mbytes / GB, rec["bgm_all"], pool_bytes / GB, rec["pool_files"], rec["ratio"] * 100))
        for role, (c, b) in sorted(roles.items(), key=lambda kv: -kv[1][1]):
            print("      %-8s %5d 个 %8.2f GB" % (role, c, b / GB))
    return rec


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("target", nargs="?", help="解包目录")
    ap.add_argument("--all", action="store_true", help="跑全部 11 款 KiriKiri")
    ap.add_argument("--json", dest="json_path")
    args = ap.parse_args()

    here = os.path.dirname(os.path.abspath(__file__))
    root = os.path.dirname(here)
    targets = []
    if args.all:
        targets = [os.path.join(root, g) for g in KIRIKIRI_GAMES]
    elif args.target:
        targets = [args.target]
    else:
        ap.error("给目录或用 --all")

    recs = []
    for t in targets:
        if not os.path.isdir(t):
            print("跳过（不存在）: %s" % t)
            continue
        recs.append(run_game(t))
    print()
    tm = sum(r["matched_bytes"] for r in recs) / GB
    tp = sum(r["pool_bytes"] for r in recs) / GB
    print("合计（%d 款）: 命中 %.2f GB / 池 %.2f GB → 引用率 %.1f%%"
          % (len(recs), tm, tp, (tm / tp * 100) if tp else 0))
    if args.json_path:
        with open(args.json_path, "w", encoding="utf-8") as f:
            json.dump(recs, f, ensure_ascii=False, indent=1)
        print("JSON: %s" % args.json_path)


if __name__ == "__main__":
    main()
