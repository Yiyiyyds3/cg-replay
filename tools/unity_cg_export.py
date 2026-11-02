#!/usr/bin/env python3
"""unity_cg_export.py —— 把 Unity/Addressables 里的 Spine CG 导成播放器能读的普通文件。

为什么需要它
------------
播放器跑在 webview 里，只认 `PNG` + Spine 的 `.atlas` / `.json`。
游戏给的是 Unity 私有容器（UnityFS + LZ4 + 序列化对象 + 未压缩像素流），
webview 里没有 Unity 运行时，所以中间必须有一次「格式转换」。

它在整条链路里的位置，与 VN 游戏的解包器完全对应：

    VN:   .xp3  → 解包器 → raw/data/image/*.png + *.ks
    ACT:  .bundle → 本脚本 → cg/<CG名>/*.png + *.json + *.atlas

它做四件事：打开容器 → 按 `Art/CG/<CG名>/` 分组导出 → 读骨架统计动画/皮肤 →
写 `cg-index.json`。**不渲染、不播放、不改游戏、不碰存档。**

用法
----
    # 系统 Python 3.12（UnityPy 装在这里；仓库自带的 python3 没有 UnityPy）
    python tools/unity_cg_export.py --game "E:/.../v0.7.5 步兵班" --out "D:/out/cg"

    # 只看索引，不落盘贴图
    python tools/unity_cg_export.py --game "..." --out "..." --dry-run

    # 只导某一部
    python tools/unity_cg_export.py --game "..." --out "..." --only HCG02

依赖：UnityPy（pip install UnityPy），以及它自带的 Pillow。
"""

from __future__ import annotations

import argparse
import csv
import json
import os
import re
import subprocess
import sys
import time
from collections import defaultdict

try:
    import UnityPy
except ImportError:  # pragma: no cover - 环境问题，直接给可执行的提示
    sys.exit(
        "缺少 UnityPy。安装：\n"
        '  "C:/Users/<你>/AppData/Local/Programs/Python/Python312/python.exe" -m pip install '
        "-i https://pypi.tuna.tsinghua.edu.cn/simple UnityPy"
    )


# 分类规则：从 DialogueData 的组名 / CG 目录名推断。
# 这些名字是作者自己起的（HCGGoblin01 = 哥布林），我们只是把它读出来，不是猜。
def category_of(name: str) -> str:
    n = name.lower()
    if "goblin" in n:
        return "战斗H·哥布林"
    if "witch" in n:
        return "战斗H·女巫"
    if n.startswith("hcg"):
        return "战斗H"
    if "breastfeed" in n or "breastfeed" in n:
        return "特殊·授乳"
    if n.startswith("shop"):
        return "商店"
    if n.startswith("passage"):
        return "过场"
    if "remember" in n or "remeber" in n:  # 原作把 Remember 拼成 Remeber
        return "回想"
    if n.startswith("opening"):
        return "开场"
    if "boss" in n:
        return "Boss 事件"
    if "bed" in n or "sleep" in n:
        return "床"
    if n.startswith("npc"):
        return "NPC"
    return "未分类"


# 角色/敌人词表（贴图名里常直接写敌人名：bandit、goblin…）。
# 命中就把名字记进索引，作为「这部 CG 里有谁」的自动线索。
CHARACTER_WORDS = (
    "bandit", "goblin", "witch", "orc", "slime", "wolf", "knight", "soldier",
    "boss", "npc", "girl", "freya", "rabbit", "spider", "ghost", "demon",
)


def characters_of(names: list[str]) -> list[str]:
    hit: set[str] = set()
    for n in names:
        low = n.lower()
        for w in CHARACTER_WORDS:
            if w in low:
                hit.add(w)
    return sorted(hit)


def tokens(s: str) -> set[str]:
    """拆词用于事件组匹配：HCGGoblin01 → {hcg, goblin}。"""
    parts = re.findall(r"[a-z]+|\d+", s.lower())
    out = set()
    for p in parts:
        if p.isdigit():
            continue
        # camelCase 已被小写化，改按已知前缀/词表切
        out.add(p)
    for w in CHARACTER_WORDS:
        if w in s.lower():
            out.add(w)
    return out


def norm(s: str) -> str:
    """归一化：去掉大小写/下划线等差异。"""
    return re.sub(r"[^a-z0-9]", "", s.lower())


def match_score(cg: str, group: str) -> int:
    """CG 目录名与 DialogueData 组名的匹配分。

    宁可留空也不错配：只有「完全同名 / 编号一致 / 角色词一致」才算匹配，
    光共享一个 shop/passage 这种大类词不算（否则 ShopCG01 会全指向 ShopCG02）。
    """
    nc, ng = norm(cg), norm(group)
    if nc == ng:
        return 1000                  # 同名最可信（ShopCG03 ↔ ShopCG03）
    # 只有「共同的角色词」才算匹配（GoblinCG_01 ↔ HCGGoblin01、Witch01 ↔ WitchCG01）；
    # 光共享 hcg/shop/passage 这种大类词不算 —— 否则 HCG01 会撞上 BreastFeeding01（编号都是 01）。
    inter = set(characters_of([cg])) & set(characters_of([group]))
    if not inter:
        return 0
    sc = 100 * len(inter)
    dc, dg = re.findall(r"\d+", cg), re.findall(r"\d+", group)
    if dc and dg and int(dc[-1]) > 0 and int(dc[-1]) == int(dg[-1]):
        sc += 10                     # 角色词相同再看编号，用于同分裁决
    return sc


MATCH_MIN = 50  # 低于这个分就不认（留空，人工补）

# Windows 不允许的字符 + 路径分隔符。CG 名/资产名来自游戏数据，不能直接拼路径。
_BAD_NAME = re.compile(r'[<>:"/\\|?*\x00-\x1f]')


def safe_component(raw: str, fallback: str = "unnamed") -> str:
    """把游戏数据里的名字变成单个安全的路径分量。

    只取 basename，并替换 Windows 非法字符；这样 `../x`、`C:\\x`、`a/b`
    都只会落在当前目录，不会把文件写到 --out 之外。
    """
    s = os.path.basename(str(raw).replace("\\", "/"))
    s = _BAD_NAME.sub("_", s).strip().rstrip(". ")
    if not s or s in (".", ".."):
        return fallback
    return s


def natural_key(s: str):
    """自然排序键：`2.png` < `10.png`，`Witch01_2` < `Witch01_10`。"""
    return [
        (0, int(part)) if part.isdigit() else (1, part.lower())
        for part in re.split(r"(\d+)", s)
    ]


def within(root: str, path: str) -> bool:
    """path 是否在 root 子树内（用于落盘前兜底校验）。"""
    try:
        return os.path.commonpath([os.path.abspath(root), os.path.abspath(path)]) == os.path.abspath(root)
    except ValueError:  # Windows 不同盘符
        return False


def out_path(root: str, *parts: str) -> str:
    p = os.path.join(root, *parts)
    if not within(root, p):
        raise SystemExit(f"拒绝写出 --out 之外：{p}")
    return p


def repo_root_for(path: str) -> str:
    """返回 path 所在 git 仓库根；不是仓库/没有 git 时返回空串。"""
    p = os.path.abspath(path)
    while p and not os.path.isdir(p):
        parent = os.path.dirname(p)
        if parent == p:
            break
        p = parent
    try:
        r = subprocess.run(
            ["git", "-C", p, "rev-parse", "--show-toplevel"],
            capture_output=True, text=True, timeout=5,
        )
    except Exception:
        return ""
    return r.stdout.strip() if r.returncode == 0 else ""


def find_game_layout(game: str) -> tuple[str, str]:
    """在游戏根目录里找 catalog.json 与 bundle 目录。"""
    aa = os.path.join(game, "*_Data", "StreamingAssets", "aa")
    import glob

    cands = [p for p in glob.glob(aa) if os.path.isdir(p)]
    if not cands:
        raise SystemExit(f"没找到 StreamingAssets/aa：{aa}")
    aa_dir = cands[0]
    cat = os.path.join(aa_dir, "catalog.json")
    if not os.path.isfile(cat):
        raise SystemExit(f"没找到 catalog.json：{cat}")
    # 平台目录 = aa 下含 .bundle 最多的那个子目录（aa 里还有 AddressablesLink 之类的辅助目录）
    dirs = [p for p in glob.glob(os.path.join(aa_dir, "*")) if os.path.isdir(p)]
    scored = [(len(glob.glob(os.path.join(p, "*.bundle"))), p) for p in dirs]
    scored = [x for x in scored if x[0] > 0]
    if not scored:
        raise SystemExit(f"aa 下没有含 .bundle 的目录：{aa_dir}")
    scored.sort(reverse=True)
    return cat, scored[0][1]


def read_catalog(cat_path: str) -> tuple[dict[str, set[str]], list[str]]:
    """返回 (资产名 → CG 名集合, DialogueData 组名列表)。

    用集合而不是单值：同名贴图可能被多个 CG 目录复用，后写覆盖会让其中
    一部 CG 少图。导出时按集合逐个复制。
    """
    with open(cat_path, encoding="utf-8") as f:
        d = json.load(f)
    ids = d.get("m_InternalIds") or []

    asset_to_cg: dict[str, set[str]] = defaultdict(set)
    for i in ids:
        m = re.search(r"Art/CG/([^/]+)/([^/]+)$", i.replace("\\", "/"))
        if not m:
            continue
        cg = safe_component(m.group(1), "cg")
        fname = safe_component(m.group(2), "asset")
        asset_to_cg[os.path.splitext(fname)[0]].add(cg)

    groups: set[str] = set()
    for i in ids:
        m = re.search(r"Data/DialogueData/([^/]+?)(?:\.asset|/)", i.replace("\\", "/"))
        if m:
            groups.add(m.group(1))
    return asset_to_cg, sorted(groups)


# ---------------------------------------------------------------------------
# 非 Addressables 的两种真实布局
# ---------------------------------------------------------------------------
#
# 实测（2026-11，「一般」目录三款 Unity 游戏）：
#   1. 资源内嵌：CG 在 <游戏>_Data/resources.assets 或 sharedassetsN.assets 里，
#      旁边是 resources.assets.resS（像素流）；没有 catalog，也没有 *_all.bundle。
#   2. StreamingAssets 散装 bundle：aa/catalog.json 不存在（新版是 catalog.bin），
#      bundle 直接放在 StreamingAssets 下，且文件开头多 64 字节自定义 GUID 头，
#      「UnityFS」从 0x40 才开始。
# 两条路都不需要 catalog：CG 分组直接按资产名（骨架 JSON + 同名 .atlas + 同名贴图）。

def load_container(path: str, offset: str = "auto"):
    """读一个 Unity 容器，必要时跳过 UnityFS 之前的自定义头。

    注意：带外部 .resS 的 .assets 必须按路径加载（走 bytes 会丢 resS），
    所以只有确实存在头偏移时才降级为 bytes。
    """
    off = 0
    with open(path, "rb") as f:
        head = f.read(4096)
    if offset == "auto":
        i = head.find(b"UnityFS")
        if i > 0:
            off = i
    elif offset not in (None, "", "0"):
        off = int(offset)
    if off:
        with open(path, "rb") as f:
            f.seek(off)
            return UnityPy.load(f.read())
    return UnityPy.load(path)


def _obj_name(d) -> str:
    n = getattr(d, "m_Name", None)
    n = getattr(n, "string", n) if n is not None else None
    return str(n) if n is not None else ""


def _safe_name(name: str) -> str:
    """目录/文件名里只保留 Windows 允许的字符（贴图名可能带 / 等）。"""
    return re.sub(r'[\\/:*?"<>|]', "_", name)


def read_cg_csv(path: str) -> tuple[dict[str, str], list[tuple[str, list[str]]]]:
    """读游戏自带的 CG 表（实测列名：AssetID / Type / CGUrl / Variants）。

    返回 (Spine 骨架名 → CG id, [(CG id, [贴图名…]), …])。
    Type=spine 时 CGUrl 形如 SpinePrefab_ICG1；Type=pic 时形如 CG/reop1，
    Variants 里可能还有 reop2;reop3 —— 那是同一部 CG 的其余帧。
    """
    with open(path, "rb") as f:
        txt = f.read().decode("utf-8-sig", "replace")
    spine: dict[str, str] = {}
    stills: list[tuple[str, list[str]]] = []
    for row in csv.DictReader(txt.splitlines()):
        typ = (row.get("Type") or "").strip().lower()
        url = (row.get("CGUrl") or "").strip()
        aid = (row.get("AssetID") or "").strip()
        if typ == "spine" and url:
            spine[url.replace("SpinePrefab_", "")] = aid or url
        elif typ == "pic" and url:
            urls = [url] + [v for v in (row.get("Variants") or "").split(";") if v.strip()]
            texs: list[str] = []
            for u in urls:
                t = u.replace("\\", "/").split("/")[-1].strip()
                if t and t not in texs:
                    texs.append(t)
            stills.append((aid or texs[0], texs))
    return spine, stills


def export_embedded(paths: list[str], args, problems: list[str]) -> tuple[dict, dict, list[str], int, int]:
    """把「资源内嵌 / 散装 bundle」里的 CG 导出来。

    只认两种能被播放器直接消费的形态（与 Addressables 版一致）：
      spine  —— 骨架 JSON + .atlas + 贴图（渲染器接入后可播）
      still  —— 单张贴图（现在就能播）
    """
    textures: dict[str, object] = {}
    texts: dict[str, bytes] = {}
    sources: list[str] = []
    for p in paths:
        env = load_container(p, args.offset)
        sources.append(p)
        for o in env.objects:
            t = o.type.name
            if t not in ("Texture2D", "TextAsset"):
                continue
            try:
                d = o.read()
            except Exception as e:  # 个别对象坏掉不影响整体
                print(f"  ! 读对象失败 {os.path.basename(p)} {t}: {e}")
                continue
            n = _obj_name(d)
            if not n:
                continue
            if t == "Texture2D":
                textures.setdefault(n, d)
            else:
                raw = d.m_Script
                raw = bytes(raw) if isinstance(raw, (bytes, bytearray)) else str(raw).encode("utf-8", "ignore")
                texts.setdefault(n, raw)
        env = None  # 尽快释放容器索引

    skel_by_base: dict[str, bytes] = {}
    for n, raw in texts.items():
        if raw[:1] == b"{" and b'"skeleton"' in raw[:400]:
            base = n[:-5] if n.lower().endswith(".json") else n
            skel_by_base.setdefault(base, raw)
    atlas_by_base: dict[str, bytes] = {}
    for n, raw in texts.items():
        if n.lower().endswith(".atlas"):
            atlas_by_base.setdefault(n[:-6], raw)
    bases = sorted(set(skel_by_base) & set(atlas_by_base))

    csv_spine: dict[str, str] = {}
    csv_stills: list[tuple[str, str]] = []
    if args.cg_csv:
        csv_spine, csv_stills = read_cg_csv(args.cg_csv)

    if csv_spine:
        missing = sorted(set(csv_spine) - set(bases))
        if missing:
            print(f"  ! CG 表里有 {len(missing)} 个 Spine 在容器里没找到：{missing[:6]}")
        bases = [b for b in sorted(csv_spine) if b in set(bases)]
    elif args.spine_prefix:
        pre = tuple(x.strip() for x in args.spine_prefix.split(",") if x.strip())
        bases = [b for b in bases if b.startswith(pre)]

    cg_files: dict[str, dict[str, list[str]]] = defaultdict(lambda: {"png": [], "text": []})
    orphan = {"png": 0, "text": 0}
    used_tex: set[str] = set()
    png_bytes = text_bytes = 0
    dry = args.dry_run

    def save_png(tex_name: str, cg: str) -> bool:
        nonlocal png_bytes
        d = textures.get(tex_name)
        if d is None:
            return False
        fname = safe_component(tex_name, "texture") + ".png"
        dest_dir = out_path(args.out, "cg", cg)
        dest = out_path(args.out, "cg", cg, fname)
        if dry:
            return True
        os.makedirs(dest_dir, exist_ok=True)
        if not args.force and os.path.isfile(dest) and os.path.getsize(dest) > 0:
            png_bytes += os.path.getsize(dest)
            return True
        try:
            d.image.save(dest)
        except Exception as e:
            msg = f"{cg}/{fname}: 贴图导出失败：{e}"
            problems.append(msg)
            print(f"  ! {msg}")
            return False
        png_bytes += os.path.getsize(dest)
        return True

    for base in bases:
        cg = safe_component(csv_spine.get(base, base), "cg")
        if args.only and args.only not in (cg, base):
            continue
        dest_dir = out_path(args.out, "cg", cg)
        if not dry:
            os.makedirs(dest_dir, exist_ok=True)
        # 骨架 + 图集
        sk = skel_by_base[base]
        at = atlas_by_base[base]
        spine_name = safe_component(base, "spine")
        if not dry:
            try:
                with open(out_path(args.out, "cg", cg, spine_name + ".json"), "wb") as f:
                    f.write(sk)
                with open(out_path(args.out, "cg", cg, spine_name + ".atlas"), "wb") as f:
                    f.write(at)
            except Exception as e:
                msg = f"{cg}/{spine_name}: 骨架导出失败：{e}"
                problems.append(msg)
                print(f"  ! {msg}")
                continue
        cg_files[cg]["text"].extend([spine_name + ".json", spine_name + ".atlas"])
        text_bytes += len(sk) + len(at)
        # 同名贴图（含 <base>_2 这种变体）
        cand = [t for t in textures if t == base or t.startswith(base + "_")]
        if not cand:
            print(f"  ! {base}: 有骨架/图集但没有同名贴图")
        for tn in cand:
            if tn in used_tex:
                continue
            if save_png(tn, cg):
                used_tex.add(tn)
                cg_files[cg]["png"].append(safe_component(tn, "texture") + ".png")

    # 单图 CG：优先用游戏自带 CG 表；否则按前缀/正则挑孤立贴图。
    for cg_id, texs in csv_stills:
        cg = safe_component(cg_id, "cg")
        if args.only and args.only != cg:
            continue
        for tex in texs:
            if tex not in textures:
                print(f"  ! {cg}: CG 表指向的贴图 {tex} 不存在")
                continue
            if save_png(tex, cg):
                used_tex.add(tex)
                cg_files[cg]["png"].append(safe_component(tex, "texture") + ".png")

    prefixes = tuple(x.strip() for x in args.still_prefix.split(",") if x.strip())
    rx = re.compile(args.still_regex) if args.still_regex else None
    for tn in textures:
        if tn in used_tex:
            continue
        if not ((prefixes and tn.startswith(prefixes)) or (rx and rx.search(tn))):
            continue
        cg = safe_component(tn, "cg")
        if args.only and args.only != cg:
            continue
        if save_png(tn, cg):
            used_tex.add(tn)
            cg_files[cg]["png"].append(safe_component(tn, "texture") + ".png")

    orphan["png"] = len(textures) - len(used_tex)
    orphan["text"] = len(texts) - sum(len(v["text"]) for v in cg_files.values())
    return cg_files, orphan, sources, png_bytes, text_bytes


def spine_stats(path: str) -> dict:
    """从 Spine 的 skeleton JSON 里数出骨骼/插槽/皮肤/动画，并估算每段时长。

    整个函数都在 try 里：任何字段形状异常都返回 {"error": ...}，不能因为
    顶层是数组/字符串就抛出去，导致 PNG 已落盘而索引没写。
    """
    try:
        with open(path, encoding="utf-8") as f:
            d = json.load(f)
        if not isinstance(d, dict):
            return {"error": "骨架 JSON 顶层不是对象"}

        def anim_dur(a: dict) -> float:
            t = 0.0
            for key in ("bones", "slots", "deform", "drawOrder", "ik", "transform", "paths", "attachments"):
                node = a.get(key) or {}
                items = node.values() if isinstance(node, dict) else []
                for tl in items:
                    if not isinstance(tl, dict):
                        continue
                    for v in tl.values():
                        if not isinstance(v, list):
                            continue
                        for e in v:
                            if isinstance(e, dict) and isinstance(e.get("time"), (int, float)):
                                t = max(t, float(e["time"]))
            return round(t, 2)

        anims = d.get("animations") or {}
        if not isinstance(anims, dict):
            anims = {}
        skins = d.get("skins") or {}
        if isinstance(skins, dict):
            skin_names = list(skins.keys())
        elif isinstance(skins, list):
            skin_names = [s.get("name") for s in skins if isinstance(s, dict) and s.get("name")]
        else:
            skin_names = []
        skeleton = d.get("skeleton") or {}
        return {
            "version": skeleton.get("spine") if isinstance(skeleton, dict) else None,
            "bones": len(d.get("bones") or []),
            "slots": len(d.get("slots") or []),
            "skins": skin_names,
            "animations": [
                {"name": n, "loop": "loop" in n.lower(), "dur": anim_dur(anims[n])}
                for n in anims
                if isinstance(anims.get(n), dict)
            ],
        }
    except Exception as e:
        return {"error": str(e)}


def main() -> int:
    ap = argparse.ArgumentParser(description="导出 Unity/Addressables 里的 Spine CG")
    ap.add_argument("--game", default="", help="游戏根目录（含 *_Data）；用 --assets 时可不填")
    ap.add_argument("--out", required=True, help="输出目录")
    ap.add_argument("--bundle", default="cg_assets_all.bundle", help="CG 所在 bundle 文件名")
    ap.add_argument("--assets", action="append", default=[],
                    help="直接指定容器（.assets/.bundle/无扩展名 bundle），可重复；给了就不走 catalog")
    ap.add_argument("--offset", default="auto", help="容器头偏移；auto=自动找 UnityFS（默认）")
    ap.add_argument("--cg-csv", default="", help="游戏自带 CG 表 CSV（列 Type/CGUrl/AssetID）")
    ap.add_argument("--spine-prefix", default="", help="只导出这些前缀的 Spine（逗号分隔）")
    ap.add_argument("--still-prefix", default="", help="把名字前缀匹配的孤立贴图当单图 CG（逗号分隔）")
    ap.add_argument("--still-regex", default="", help="把名字正则匹配的孤立贴图当单图 CG")
    ap.add_argument("--only", default="", help="只导某个 CG（目录名）")
    ap.add_argument("--dry-run", action="store_true", help="只出索引，不写素材")
    ap.add_argument("--force", action="store_true", help="已存在的文件也重写")
    ap.add_argument(
        "--allow-repo-output",
        action="store_true",
        help="允许把产物写进 git 仓库（默认拒绝，避免 100+ MB PNG 被顺手提交）",
    )
    args = ap.parse_args()

    if not args.dry_run and not args.allow_repo_output:
        repo = repo_root_for(args.out)
        if repo and within(repo, args.out):
            raise SystemExit(
                f"--out 位于 git 仓库内：{args.out}\n"
                "导出产物很大，默认拒绝写进仓库；换到仓库外，或加 --allow-repo-output。"
            )

    t0 = time.time()
    cg_files: dict[str, dict[str, list[str]]] = defaultdict(lambda: {"png": [], "text": []})
    groups: list[str] = []
    png_bytes = text_bytes = 0
    problems: list[str] = []

    if args.assets:
        cg_files, orphan, sources, png_bytes, text_bytes = export_embedded(args.assets, args, problems)
        if not args.game:
            args.game = os.path.basename(os.path.dirname(os.path.abspath(args.assets[0])))
        print(f"容器：{len(args.assets)} 个 · 候选 CG：{len(cg_files)} 部")
    else:
        if not args.game:
            raise SystemExit("要么给 --game（Addressables 布局），要么给 --assets（内嵌/散装 bundle）")
        cat, bdir = find_game_layout(args.game)
        bundle = os.path.join(bdir, args.bundle)
        if not os.path.isfile(bundle):
            raise SystemExit(f"没找到 bundle：{bundle}")
        sources = [bundle]
        orphan = {"png": 0, "text": 0}

        asset_to_cg, groups = read_catalog(cat)
        print(f"清单：{len(asset_to_cg)} 个 CG 资产 · {len(groups)} 个 DialogueData 组")

        env = UnityPy.load(bundle)

        for o in env.objects:
            tname = o.type.name
            if tname == "Texture2D":
                d = o.read()
                name = safe_component(getattr(d.m_Name, "string", str(d.m_Name)), "texture")
                targets = asset_to_cg.get(name, set())
                if not targets:
                    orphan["png"] += 1
                    continue
                for cg in sorted(targets):
                    if args.only and cg != args.only:
                        continue
                    fname = name + ".png"
                    dest_dir = out_path(args.out, "cg", cg)
                    dest = out_path(args.out, "cg", cg, fname)
                    if args.dry_run:
                        cg_files[cg]["png"].append(fname)
                        continue
                    os.makedirs(dest_dir, exist_ok=True)
                    if not args.force and os.path.isfile(dest) and os.path.getsize(dest) > 0:
                        cg_files[cg]["png"].append(fname)
                        png_bytes += os.path.getsize(dest)
                        continue
                    try:
                        d.image.save(dest)
                    except Exception as e:
                        msg = f"{cg}/{fname}: 贴图导出失败：{e}"
                        problems.append(msg)
                        print(f"  ! {msg}")
                        continue
                    cg_files[cg]["png"].append(fname)
                    png_bytes += os.path.getsize(dest)
            elif tname == "TextAsset":
                d = o.read()
                name = safe_component(getattr(d.m_Name, "string", str(d.m_Name)), "text")
                stem = safe_component(os.path.splitext(name)[0], "text")
                targets = asset_to_cg.get(stem) or asset_to_cg.get(name) or set()
                if not targets:
                    orphan["text"] += 1
                    continue
                raw = d.m_Script
                raw = raw if isinstance(raw, (bytes, bytearray)) else str(raw).encode("utf-8", "ignore")
                # 名字多半已经带扩展名（X.atlas / X.json）；没有的话按内容判：骨架是 JSON，图集是文本
                if name.lower().endswith((".atlas", ".json")):
                    fname = name
                else:
                    fname = name + (".json" if raw[:1] == b"{" else ".atlas")
                fname = safe_component(fname, "text")
                for cg in sorted(targets):
                    if args.only and cg != args.only:
                        continue
                    if args.dry_run:
                        cg_files[cg]["text"].append(fname)
                        continue
                    dest_dir = out_path(args.out, "cg", cg)
                    os.makedirs(dest_dir, exist_ok=True)
                    dest = out_path(args.out, "cg", cg, fname)
                    try:
                        if args.force or not (os.path.isfile(dest) and os.path.getsize(dest) == len(raw)):
                            with open(dest, "wb") as f:
                                f.write(raw)
                    except Exception as e:
                        msg = f"{cg}/{fname}: 文本导出失败：{e}"
                        problems.append(msg)
                        print(f"  ! {msg}")
                        continue
                    cg_files[cg]["text"].append(fname)
                    text_bytes += len(raw)

    # 建索引（`--only` 时与旧索引合并：一次调试不能把整库索引删成只剩一部）
    cgs = []
    for cg in sorted(cg_files):
        files = cg_files[cg]
        cg_dir = out_path(args.out, "cg", cg)
        png_files = sorted(files["png"], key=natural_key)
        text_files = sorted(files["text"], key=natural_key)
        skeletons = [f for f in text_files if f.lower().endswith(".json")]
        stats = {}
        if skeletons and not args.dry_run:
            stats = spine_stats(os.path.join(cg_dir, skeletons[0]))
        # CG 目录名 → 最接近的 DialogueData 组（作者命名略有差异，用归一化匹配）
        match = ""
        best = 0
        for g in groups:
            sc = match_score(cg, g)
            if sc > best:
                best, match = sc, g
        if best < MATCH_MIN:
            match = ""
        keyname = match or cg
        size = 0
        if not args.dry_run:
            for f in files["png"] + files["text"]:
                p = os.path.join(cg_dir, f)
                if os.path.isfile(p):
                    size += os.path.getsize(p)
        # 两类做法混在一起：有骨架 JSON 的是 Spine 骨骼动画；
        # 只有一堆 png 的是「逐帧序列 + Unity AnimationClip」（如 Witch01 的 bandit 帧）。
        if skeletons:
            kind = "spine"
        elif len(files["png"]) > 1:
            kind = "frames"      # 逐帧序列（Unity AnimationClip 驱动）
        elif len(files["png"]) == 1:
            kind = "still"       # 单张静态 CG
        else:
            kind = "unknown"
        chars = characters_of(png_files + text_files)
        if stats.get("skins"):
            chars = sorted(set(chars) | set(characters_of(stats["skins"])))
        cgs.append(
            {
                "id": cg,
                "kind": kind,
                "category": category_of(keyname),
                "event_group": match or None,
                "characters": chars,
                "files": {"png": png_files, "text": text_files},
                "spine": stats if kind == "spine" else None,
                "frames": png_files if kind == "frames" else None,
                "bytes": size,
            }
        )

    if args.only:
        idx_path = out_path(args.out, "cg-index.json")
        if os.path.isfile(idx_path):
            try:
                with open(idx_path, encoding="utf-8") as f:
                    old = json.load(f)
                if not isinstance(old, dict) or not isinstance(old.get("cgs"), list):
                    raise ValueError("cgs 不是数组")
            except Exception as e:
                raise SystemExit(
                    f"--only 需要合并旧索引，但旧索引无法解析：{idx_path}（{e}）\n"
                    "请先备份/删除它，避免把整库索引覆盖成只剩一部。"
                )
            keep = [
                c for c in old["cgs"]
                if isinstance(c, dict) and c.get("id") and c["id"] != args.only
            ]
            cgs = keep + cgs
            old_groups = old.get("event_groups")
            if isinstance(old_groups, list):
                groups = sorted(set(groups) | {str(x) for x in old_groups})
        else:
            print("  ! 没有旧 cg-index.json：这次只写 --only 指定的 CG")
    cgs.sort(key=lambda c: str(c.get("id", "")))

    index = {
        "schema": 1,
        "engine": "unity",
        "game": os.path.basename(os.path.normpath(args.game)),
        "generated_at": time.strftime("%Y-%m-%dT%H:%M:%S"),
        "source": {"bundle": "、".join(os.path.basename(p) for p in sources), "bytes": sum(os.path.getsize(p) for p in sources)},
        "counts": {
            "cg": len(cgs),
            "png": sum(len((c.get("files") or {}).get("png") or []) for c in cgs),
            "text": sum(len((c.get("files") or {}).get("text") or []) for c in cgs),
            "spine": sum(1 for c in cgs if c.get("kind") == "spine"),
            "frames": sum(1 for c in cgs if c.get("kind") == "frames"),
            "still": sum(1 for c in cgs if c.get("kind") == "still"),
            "orphan_png": orphan["png"],
            "orphan_text": orphan["text"],
        },
        "bytes": 0,  # 下面覆盖
        "cgs": cgs,
        "event_groups": groups,
    }
    index["bytes"] = sum(int(c.get("bytes") or 0) for c in cgs)

    # 自检：spine 必须有骨架、frames 至少 2 帧、索引里的文件必须真在磁盘上
    check_problems: list[str] = []
    for c in cgs:
        cid = str(c.get("id") or "")
        files = c.get("files") or {}
        pngs = files.get("png") or []
        texts = files.get("text") or []
        cg_dir = out_path(args.out, "cg", cid)
        if c.get("kind") == "spine" and not texts:
            check_problems.append(f"{cid}: 标为 spine 但没有骨架文件")
        if c.get("kind") == "frames" and len(pngs) < 2:
            check_problems.append(f"{cid}: 标为 frames 但只有 {len(pngs)} 张图")
        if args.dry_run:
            continue
        for f in pngs + texts:
            if not os.path.isfile(out_path(cg_dir, f)):
                check_problems.append(f"{cid}: 索引里有 {f}，磁盘上没有")
    index["problems"] = problems + check_problems

    if not args.dry_run:
        os.makedirs(args.out, exist_ok=True)
        with open(out_path(args.out, "cg-index.json"), "w", encoding="utf-8") as f:
            json.dump(index, f, ensure_ascii=False, indent=2)
        # 约定文件：让桌面端「添加游戏」认出这是 unity 库，并把 cg/ 当剧本目录
        # （AddGame 会检查 conv.scenario_dir 是否存在，所以必须是导出后真实存在的目录）。
        with open(out_path(args.out, "ucgr-convention.json"), "w", encoding="utf-8") as f:
            json.dump({"engine_id": "unity", "scenario_dir": "cg"}, f, ensure_ascii=False, indent=2)

    print(
        f"完成：{len(cgs)} 部 CG · PNG {index['counts']['png']} 张（{png_bytes/1048576:.1f} MB）· "
        f"文本 {index['counts']['text']} 个（{text_bytes/1048576:.1f} MB）· "
        f"落盘 {index['bytes']/1048576:.1f} MB · {time.time()-t0:.1f}s"
    )
    if orphan["png"] or orphan["text"]:
        print(f"未归属到 CG 的对象：png {orphan['png']} · text {orphan['text']}（正常，bundle 里还有别的资源）")
    print(
        f"类型：spine {index['counts']['spine']} · frames {index['counts']['frames']} · "
        f"still {index['counts']['still']}"
    )
    all_problems = index["problems"]
    if all_problems:
        print(f"发现 {len(all_problems)} 个问题：")
        for x in all_problems[:8]:
            print("  -", x)
    else:
        print("自检通过：索引与磁盘一致")
    if not args.dry_run:
        print(f"索引：{out_path(args.out, 'cg-index.json')}")
    return 1 if all_problems else 0


if __name__ == "__main__":
    raise SystemExit(main())
