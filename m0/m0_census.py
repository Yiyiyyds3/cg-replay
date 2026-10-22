#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""m0_census.py —— M0 清点 v2a：46 个原始目录的 ①本体 / ②原归档 分类测量（只读）

口径（互斥，三分法 + 其它）：
  ①本体    ：.exe .dll .sys .ocx .cfg .ini .bat .cmd .manifest .dfn（可执行与运行时）
  ②原归档  ：.xp3 .pfs .int .kif .arc .nsa .alf .afs .bin .hcb .hzc .paz .pak
              .lpk .pck .afa .pac .npa .unity3d .assets .ress .bundle .dat
              （.dat/.bin 按引擎兼作归档与散数据，全局计入②，误差在报告中注明）
  其它      ：存档 / 补丁备份 / 散落影片 / 说明文本 / 安装包等（不计入三类）
另flag：源目录内的「解包输出/原始归档解包/_分类」重复解包树（避免与③重复计数）。

③解包产物取自 m0/engine_dist_verify.json（上一轮已测）。
灰色的有闲源目录已不在盘上 → ①②缺失，仅③。
"""
import json
import os
import sys

try:
    sys.stdout.reconfigure(encoding="utf-8")
    sys.stderr.reconfigure(encoding="utf-8")
except Exception:
    pass

G = os.environ.get("UCGR_GAME_ROOT", "")
HERE = os.path.dirname(os.path.abspath(__file__))
DIST_JSON = os.path.join(HERE, "engine_dist_verify.json")
GB = 1024.0 ** 3

# 47 款 ↔ 原始目录 映射（证据：各解包目录 .md / gar_tool 脚本路径 / 名称匹配）
MAP = {
    "9nine九次九日解包": r"[ぱれっと] 9-nine-九次九日九重色 汉化硬盘版",
    "9nine天色解包": r"9-nine-天色天歌天籁音",
    "9nine新章解包": r"102-13\9-nine- 新章汉化硬盘版",
    "9nine春色解包": r"9-nine-春色春恋春熙风（第三部）",
    "9nine雪色解包": r"9-nine-雪色雪花雪之后（雪色雪花雪余痕）",
    "BALDRSKY_DIVEX解包": "BALDRSKY DIVEX",
    "BLACK_SHEEP_TOWN解包": "BLACK.SHEEP.TOWN",
    "BaldrSky解包": "Baldr Sky",
    "CROSSCHANNEL解包": "CROSSCHANNEL",
    "Clover_Days解包": "Clover Day's",
    "GrisaiaAno1解包": "GrisaiaAno1",
    "G線上の魔王解包": "G線上の魔王",
    "HolyKnightRicca解包": "HolyKnightRicca_v116ku",
    "IO_revision2解包": "Ｉ／Ｏ‐revisionⅡ",
    "Monkeys!!解包": "Monkeys!!",
    "ORE_TUBA解包": "83660",
    "Remember11解包": "Remember11",
    "たねつみの歌解包": "たねつみの歌",
    "ディメンション凸ラバース解包": "PC_ディメンション凸ラバース",
    "五彩斑斓解包": r"74-9本篇 (2)\五彩斑斓的世界",
    "兰斯10解包": r"Rance10\兰斯10",
    "塔纳托斯解包": "塔纳托斯-Thanatos",
    "壳之少女解包": "壳之少女",
    "夏空彼方解包": "夏空彼方",
    "寒蝉鸣泣之时解包": "寒蝉鸣泣之时·解题篇（萌化版）",
    "封緘のグラセスタ解包": r"322\封緘のグラセスタ",
    "少女编织爱的画布解包": "少女编织爱的画布",
    "果实解包": "果实",
    "枯花解包": "枯れない世界と終わる花",
    "樱之刻解包": "sakuratoki",
    "樱之诗解包": "樱之诗",
    "樱花萌放解包": "樱花萌放",
    "海市蜃楼之馆解包": "PC_海市蜃楼之馆",
    "海猫鸣泣之时_出题篇解包": "海猫鸣泣之时 出题篇（萌化版）",
    "海猫鸣泣之时_散篇解包": "海猫鸣泣之时 散（萌化版）",
    "灰色的乐园FHD解包": "灰色的乐园FHD",
    "牵绊闪耀的恋之伊吕波解包": "牵绊闪耀的恋之伊吕波",
    "甜蜜女友+解包": "甜蜜女友+（GPT翻译）",
    "甜蜜女友2+解包": "甜蜜女友2+",
    "甜蜜女友2解包": "甜蜜女友2",
    "甜蜜女友3解包": "甜蜜女友3",
    "甜蜜女友PE解包": "甜蜜女友Perfect_Edition（GPT翻译）",
    "秽翼的尤斯蒂娅解包": r"160\秽翼的尤斯蒂娅",
    "美好的每一天解包": "美好的每一天～不连续的存在～ HD版",
    "重返蓝鲸岛解包": "重返蓝鲸岛",
    "鬼哭街重制版解包": "鬼哭街重制版",
    # 灰色的有闲解包：源目录已不在盘上（仅存 GrisaiaAno1/补丁&附件 里的汉化补丁 rar）
}

BODY_EXT = {".exe", ".dll", ".sys", ".ocx", ".cfg", ".ini", ".bat", ".cmd",
            ".manifest", ".dfn"}
ARC_EXT = {".xp3", ".pfs", ".int", ".kif", ".arc", ".nsa", ".alf", ".afs",
           ".bin", ".hcb", ".hzc", ".paz", ".pak", ".lpk", ".pck", ".afa",
           ".pac", ".npa", ".unity3d", ".assets", ".ress", ".bundle", ".dat"}
DUP_DIR_NAMES = {"解包输出", "原始归档解包", "_分类"}


def measure(src):
    body = arc = other = 0
    body_n = arc_n = other_n = 0
    dup_bytes = 0
    dup_note = set()
    for dp, dirs, fs in os.walk(src):
        rel = os.path.relpath(dp, src)
        parts = [] if rel == "." else rel.split(os.sep)
        # 重复解包树判定：相对源根目录的任一 component 是解包产物目录名
        is_dup = any(p in DUP_DIR_NAMES or "解包" in p for p in parts)
        if is_dup:
            dup_note.add(os.sep.join(parts[:2]) if len(parts) > 1 else parts[0])
        for fn in fs:
            ext = os.path.splitext(fn)[1].lower()
            try:
                sz = os.path.getsize(os.path.join(dp, fn))
            except OSError:
                sz = 0
            if is_dup:
                dup_bytes += sz
                continue
            if ext in BODY_EXT:
                body += sz; body_n += 1
            elif ext in ARC_EXT:
                arc += sz; arc_n += 1
            else:
                other += sz; other_n += 1
    return {"body": body, "body_n": body_n, "arc": arc, "arc_n": arc_n,
            "other": other, "other_n": other_n,
            "dup_bytes": dup_bytes, "dup": sorted(dup_note)}


def main():
    if not G:
        sys.exit("未设置环境变量 UCGR_GAME_ROOT（游戏库根目录）")
    if not os.path.isfile(DIST_JSON):
        sys.exit("缺少 m0/engine_dist_verify.json（本地生成，未入库）：请先运行 python tools/verify_engine_dist.py --root <解包根目录> --json m0/engine_dist_verify.json")
    dist = json.load(open(DIST_JSON, encoding="utf-8"))["games"]
    rows = []
    missing = []
    for game in sorted(dist):
        rel = MAP.get(game)
        if not rel:
            missing.append(game)
            rows.append((game, None))
            continue
        src = os.path.join(G, rel)
        if not os.path.isdir(src):
            missing.append(game + "（路径不存在: %s）" % rel)
            rows.append((game, None))
            continue
        rows.append((game, {"src": rel, **measure(src)}))

    print("| 游戏 | 引擎 | ①本体 GB | ②原归档 GB | 其它 GB | 源合计 GB | ③解包 GB | 源目录 |")
    print("|---|---|---:|---:|---:|---:|---:|---|")
    tb = ta = to = t3 = 0.0
    for game, rec in rows:
        d = dist[game]
        eng = {"KiriKiri": "KiriKiri", "CatSystem2": "CatSystem2", "BGI": "BGI",
               "NScripter": "NScripter", "UnityIL2CPP": "Unity",
               "Artemis": "Artemis", "GIGA_BALDR": "BALDR",
               "FVP": "FVP"}.get(d["engine"], d["engine"])
        b3 = d["bytes"] / GB
        t3 += b3
        if rec is None:
            print("| %s | %s | — | — | — | — | %.2f | **缺失** |" % (game, eng, b3))
            continue
        b1 = rec["body"] / GB; b2 = rec["arc"] / GB; bo = rec["other"] / GB
        tb += b1; ta += b2; to += bo
        note = rec["src"]
        if rec["dup"]:
            note += " ⚠️含解包副本%.2fGB" % (rec["dup_bytes"] / GB)
        print("| %s | %s | %.2f | %.2f | %.2f | %.2f | %.2f | %s |" % (
            game, eng, b1, b2, bo, b1 + b2 + bo, b3, note))
    print()
    print("合计：①本体 %.2f GB ｜ ②原归档 %.2f GB ｜ 其它 %.2f GB ｜ ③解包 %.2f GB"
          % (tb, ta, to, t3))
    print("①+②+③ = %.2f GB" % (tb + ta + t3))
    if missing:
        print("缺失/异常：%s" % "; ".join(missing))

    out = os.path.join(HERE, "m0_census.json")
    with open(out, "w", encoding="utf-8") as f:
        json.dump({"rows": {g: r for g, r in rows},
                   "totals": {"body": tb, "arc": ta, "other": to, "unpacked": t3}},
                  f, ensure_ascii=False, indent=1)
    print("JSON: %s" % out)


if __name__ == "__main__":
    main()
