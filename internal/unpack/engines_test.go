package unpack

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
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
		"data.xp3":  "kirikiri",
		"data.pfs":  "artemis",
		"pack.arc":  "bgi",
		"scene.int": "catsystem2", // CS2 的归档是 .int/.kif；.cst 是剧本不是归档
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

// ---------------------------------------------------------------
// CatSystem2（.cst）夹具
// ---------------------------------------------------------------

// cstEntry .cst 解压后的一个条目（0x01 + 类型 + 内容 + \0）。
type cstEntry struct {
	typ  byte // 0x20 对白 / 0x21 角色名 / 0x30 命令
	body string
}

// buildCST 造一份最小 CatScene 文件（魔数 + u32 压缩长 + 16 字节头 + zlib）。
func buildCST(t *testing.T, entries []cstEntry) []byte {
	t.Helper()
	var raw []byte
	for _, e := range entries {
		raw = append(raw, 0x01, e.typ)
		raw = append(raw, []byte(e.body)...)
		raw = append(raw, 0)
	}
	var zbuf bytes.Buffer
	zw := zlib.NewWriter(&zbuf)
	if _, err := zw.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	comp := zbuf.Bytes()
	head := make([]byte, 16)
	copy(head, "CatScene")
	binary.LittleEndian.PutUint32(head[8:12], uint32(len(comp)))
	return append(head, comp...)
}

// TestCatSystem2ScriptRefs CS2 接进解包后：.cst 能解析、引用能提取。
//
// 修正说明：此前 unpack 层认为 CS2「没有脚本解析」而拒绝解包 —— 实为误判，
// 适配器早有 ParseScript/ExtractRefs。本用例守住这条能力不再回退。
func TestCatSystem2ScriptRefs(t *testing.T) {
	deps, ok := engineFor("catsystem2")
	if !ok {
		t.Fatal("catsystem2 引擎未注册")
	}
	cst := buildCST(t, []cstEntry{
		{0x30, "bg 0 bg01"},
		{0x30, "pcm voice001"},
		{0x21, "名前"},
		{0x20, "一行目"},
	})
	info, err := deps.ParseScript(cst, "01.cst")
	if err != nil {
		t.Fatalf("ParseScript(.cst): %v", err)
	}
	if len(info.Labels) != 1 {
		t.Fatalf("事件数 = %d, want 1（一脚本一事件）", len(info.Labels))
	}
	has := func(want string) bool {
		for _, r := range info.Refs {
			if r == want {
				return true
			}
		}
		return false
	}
	for _, want := range []string{"bg01", "voice001"} {
		if !has(want) {
			t.Fatalf("引用里缺 %q（实际 %v）", want, info.Refs)
		}
	}
}

// TestCatSystem2DetectByArchiveExt：CS2 归档后缀能被认出来。
func TestCatSystem2DetectByArchiveExt(t *testing.T) {
	for _, name := range []string{"scene.int", "update01.kif"} {
		if got := detectEngine(name); got != "catsystem2" {
			t.Errorf("detectEngine(%q) = %q, want catsystem2", name, got)
		}
	}
}

// TestUnpackExplainsWrongContainer 扩展名骗人时要说清「它到底是什么」。
//
// 实测踩到：ambitious_mission/update.xp3 其实是 ZIP，Monkeys!!_cn.xp3 其实是
// EVB（E-mote 产物）。解析器拒绝是对的，但必须给人一句可用的话。
func TestUnpackExplainsWrongContainer(t *testing.T) {
	cases := []struct {
		name string
		head []byte
		want string
	}{
		{"update.xp3", []byte("PK\x03\x04fake zip body"), "ZIP"},
		{"Monkeys_cn.xp3", []byte("EVB\x00\x40\x00\x00\x00rest"), "EVB"},
	}
	for _, c := range cases {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, c.name), c.head, 0o644); err != nil {
			t.Fatal(err)
		}
		plan, err := Preview(dir)
		if err != nil {
			t.Fatalf("%s: Preview 应返回计划而不是错误: %v", c.name, err)
		}
		if !plan.Unsupported {
			t.Fatalf("%s: 应为不支持（%+v）", c.name, plan)
		}
		if !strings.Contains(plan.Reason, c.want) {
			t.Fatalf("%s: 原因里应提到 %s，实际 %q", c.name, c.want, plan.Reason)
		}
	}
}

// TestUnpackSkipVideo 「不提取影片」开关：影片条目不落盘，其余照常。
//
// 动机：单个 OP/ED 就 100~300 MB，全解出来很占盘；很多用户只想看剧情 CG。
func TestUnpackSkipVideo(t *testing.T) {
	makeArchive := func(t *testing.T) string {
		dir := t.TempDir()
		xp3 := filepath.Join(dir, "data.xp3")
		ks := "[movie storage=\"op_demo.mpg\"]\n[wait time=100]\n[image storage=\"cg001_a.png\"]\n"
		writeXP3(t, xp3, []xp3File{
			{path: "data/scenario/01_movie.ks", data: []byte(ks)},
			{path: "data/image/cg001_a.png", data: []byte("PNG-1")},
			{path: "data/movie/op_demo.mpg", data: []byte("MPG-DATA")},
		})
		return xp3
	}

	// 预览要能算出「影片有几个、多大」，用户据此决定。
	if plan, err := Preview(makeArchive(t)); err != nil {
		t.Fatalf("Preview: %v", err)
	} else if plan.Videos != 1 || plan.VideoBytes <= 0 {
		t.Fatalf("预览应报出影片数与体积，实际 videos=%d bytes=%d", plan.Videos, plan.VideoBytes)
	}

	for _, skip := range []bool{true, false} {
		xp3 := makeArchive(t)
		dest := t.TempDir()
		res, err := ExtractWith(xp3, dest, Options{Scope: ScopeAll, SkipVideo: skip})
		if err != nil {
			t.Fatalf("ExtractWith(skip=%v): %v", skip, err)
		}
		movie := filepath.Join(dest, "raw", "data", "image", "op_demo.mpg")
		_, statErr := os.Stat(movie)
		if skip {
			if statErr == nil {
				t.Fatal("skip=true 时影片不应落盘")
			}
			if res.SkippedVideo == 0 {
				t.Fatal("应统计出被跳过的影片数")
			}
		} else {
			if statErr != nil {
				t.Fatalf("skip=false 时影片应落盘：%v", statErr)
			}
			if res.SkippedVideo != 0 {
				t.Fatalf("skip=false 时不该有跳过计数（%d）", res.SkippedVideo)
			}
		}
	}
}
