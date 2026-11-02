#!/usr/bin/env python3
"""unity_cg_export.py 的离线单测（不需要 UnityPy / 游戏素材）。

UnityPy 在导入期就会被检查，这里先塞一个空模块，避免测试环境装不了依赖。
"""
from __future__ import annotations

import json
import os
import sys
import tempfile
import types
import unittest

sys.modules.setdefault("UnityPy", types.ModuleType("UnityPy"))
sys.path.insert(0, os.path.dirname(__file__))

import unity_cg_export as u  # noqa: E402


class UnityCgExportTest(unittest.TestCase):
    def test_natural_key_orders_frames(self):
        names = ["Witch01_10.png", "Witch01_2.png", "Witch01_1.png"]
        self.assertEqual(
            sorted(names, key=u.natural_key),
            ["Witch01_1.png", "Witch01_2.png", "Witch01_10.png"],
        )

    def test_safe_component_blocks_traversal(self):
        for raw in ("../evil", r"..\evil", "a/b", r"C:\tmp\x", ".."):
            got = u.safe_component(raw)
            self.assertNotIn("/", got)
            self.assertNotIn("\\", got)
            self.assertNotIn(":", got)
            self.assertNotEqual(got, "..")

    def test_spine_stats_bad_shape_returns_error(self):
        with tempfile.TemporaryDirectory() as td:
            p = os.path.join(td, "bad.json")
            with open(p, "w", encoding="utf-8") as f:
                json.dump([1, 2, 3], f)
            stats = u.spine_stats(p)
            self.assertIn("error", stats)
            self.assertIn("顶层不是对象", stats["error"])

    def test_spine_stats_ok(self):
        with tempfile.TemporaryDirectory() as td:
            p = os.path.join(td, "ok.json")
            with open(p, "w", encoding="utf-8") as f:
                json.dump(
                    {
                        "skeleton": {"spine": "4.1"},
                        "bones": [{"name": "root"}],
                        "slots": [],
                        "skins": {"default": {}},
                        "animations": {"idle": {"bones": {"root": {"rotate": [{"time": 1.5}]}}}},
                    },
                    f,
                )
            stats = u.spine_stats(p)
            self.assertNotIn("error", stats)
            self.assertEqual(stats["version"], "4.1")
            self.assertEqual(stats["bones"], 1)
            self.assertEqual(stats["animations"][0]["dur"], 1.5)

    def test_read_catalog_sanitizes_cg(self):
        with tempfile.TemporaryDirectory() as td:
            p = os.path.join(td, "catalog.json")
            with open(p, "w", encoding="utf-8") as f:
                json.dump(
                    {"m_InternalIds": ["Art/CG/../X.png", "Art/CG/HCG01/Y.json"]},
                    f,
                )
            assets, _ = u.read_catalog(p)
            self.assertIn("HCG01", assets.get("Y", set()))
            self.assertEqual(assets.get("X"), {"cg"})


if __name__ == "__main__":
    unittest.main()
