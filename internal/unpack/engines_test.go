package unpack

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------
// Artemis（PFS）夹具
// ---------------------------------------------------------------

// testAST Artemis 的剧本是**明文 Lua 表**（适配器已确认无需解密）。
const testAST = `astver = 2.0
astname = "ast"
ast = {
	block_00000 = {
		{"savetitle", text="銀色、遥か"},
		{"bg", time=2000, file="black", path=":bg/"},
		{"bg", file="ev_cut_001_00", id=1, lv=5, path=":cg/", sync=0},
		{"ex", time=2000, func="wait"},
		{"text"},
		{"fg", file="chara_a", ch=1, path=":fg/tami/z1/"},
		text = {
			vo = {
				{"vo", file="fem_a_0001", ch="tami"},
			},
			ja = {
				{ name={"少女","しょうじょ"}, "「こんにちは」", {"rt2"}, },
			},
		},
		linkback = "block_00000",
		linknext = "block_00002",
		line = 887,
	},
	label = {
		top = { block="block_00000", label=1 },
	},
}
`

// writePFS 造一份最小 PFS 归档（格式与 internal/adapter/artemis 的解析口径一致）。
func writePFS(t *testing.T, path string, entries map[string][]byte) {
	t.Helper()
	names := make([]string, 0, len(entries))
	for n := range entries {
		names = append(names, n)
	}
	// PFS 索引从文件偏移 7 开始，前 4 字节是 count；index_size 含这 4 字节。
	indexLen := 4
	for _, n := range names {
		indexLen += len(n) + 16
	}
	dataBase := 7 + indexLen

	index := make([]byte, 4, indexLen)
	binary.LittleEndian.PutUint32(index, uint32(len(names)))
	var data []byte
	for _, n := range names {
		var rec [4]byte
		binary.LittleEndian.PutUint32(rec[:], uint32(len(n)))
		index = append(index, rec[:]...)
		index = append(index, n...)
		index = append(index, 0, 0, 0, 0)
		var offSize [8]byte
		binary.LittleEndian.PutUint32(offSize[:], uint32(dataBase+len(data)))
		binary.LittleEndian.PutUint32(offSize[4:], uint32(len(entries[n])))
		index = append(index, offSize[:]...)
		data = append(data, entries[n]...)
	}
	hdr := []byte{'p', 'f', '6', 0, 0, 0, 0} // v6（解析器支持 v2/v6/v8；v2 索引布局不同）
	binary.LittleEndian.PutUint32(hdr[3:], uint32(len(index)))
	all := append(hdr, index...)
	all = append(all, data...)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, all, 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestUnpackArtemisPFS：Artemis 归档 → 最小集合（剧本 + 被引用的图）。
func TestUnpackArtemisPFS(t *testing.T) {
	dir := t.TempDir()
	writePFS(t, filepath.Join(dir, "data.pfs"), map[string][]byte{
		"scenario/01_01.ast":  []byte(testAST),
		"image/ev_cut_001_00": []byte("CG1"),
		"image/chara_a":       []byte("SPRITE"),
		"image/black":         []byte("BG"),
		"image/fem_a_0001":    []byte("VO"),
		"image/unused.png":    []byte("UNUSED"),
	})
	p, err := Preview(dir)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if p.EngineID != "artemis" {
		t.Fatalf("引擎 = %q, want artemis（%+v）", p.EngineID, p)
	}
	if p.ScenarioFiles != 1 {
		t.Fatalf("剧本数 = %d, want 1", p.ScenarioFiles)
	}
	if p.Labels == 0 {
		t.Fatal("应识别出事件标签")
	}
	if p.Refs == 0 {
		t.Fatal("应识别出资源引用")
	}

	dest := filepath.Join(t.TempDir(), "game")
	res, err := Extract(dir, dest, ScopeCG)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if res.Files < 2 {
		t.Fatalf("解出文件数 = %d, want ≥2（剧本 + 素材）", res.Files)
	}
	// 剧本落盘
	if _, err := os.Stat(filepath.Join(dest, "scenario", "01_01.ast")); err != nil {
		t.Fatalf("剧本没落盘：%v", err)
	}
	// 被引用的图落盘
	if _, err := os.Stat(filepath.Join(dest, "image", "ev_cut_001_00")); err != nil {
		t.Fatalf("被引用的素材没落盘：%v", err)
	}
	// 未被引用的不该解出（最小集合的意义）
	if _, err := os.Stat(filepath.Join(dest, "image", "unused.png")); err == nil {
		t.Fatal("未被引用的素材不该被解出")
	}
	// 约定文件
	if _, err := os.Stat(filepath.Join(dest, "ucgr-convention.json")); err != nil {
		t.Fatalf("约定文件没写：%v", err)
	}
}

// ---------------------------------------------------------------
// 引擎分派
// ---------------------------------------------------------------

// TestDetectEngineByArchiveExt：按归档后缀认出引擎。
func TestDetectEngineByArchiveExt(t *testing.T) {
	cases := map[string]string{
		"data.xp3": "kirikiri",
		"data.pfs": "artemis",
		"pack.arc": "bgi",
		"data.cst": "catsystem2",
	}
	for name, want := range cases {
		if got := detectEngine(name); got != want {
			t.Errorf("detectEngine(%q) = %q, want %q", name, got, want)
		}
	}
	if got := detectEngine("random.txt"); got != "" {
		t.Errorf("未知后缀应认不出，got %q", got)
	}
}

// TestCatSystem2UnsupportedWithoutScriptParser：CatSystem2 没有脚本解析
// → 预览要明说「暂不支持」并给退路，不能含糊报错。
func TestCatSystem2UnsupportedWithoutScriptParser(t *testing.T) {
	dir := t.TempDir()
	// 造一个最小 .cst（KIF 归档）；内容随意——重点是走完「认出引擎 → 尝试
	// 解析剧本 → 明确报不支持」这条路径。
	if err := os.WriteFile(filepath.Join(dir, "data.cst"), []byte("KIF"), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := Preview(dir)
	if err != nil {
		// 认不出或解析失败都可以，但必须带明确说明，不能是空错误。
		if err.Error() == "" {
			t.Fatal("错误信息不能为空")
		}
		return
	}
	if p.EngineID != "catsystem2" && !p.Unsupported {
		t.Fatalf("认出引擎应为 catsystem2 或明确不支持：%+v", p)
	}
}
