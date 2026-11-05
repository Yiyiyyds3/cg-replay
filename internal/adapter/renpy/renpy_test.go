package renpy

import (
	"bytes"
	"encoding/hex"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ucgr/internal/ir"
)

// writeTestPNG 写一个 w×h 的纯色 PNG。
func writeTestPNG(t *testing.T, path string, w, h int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(w/2, h/2, color.RGBA{255, 0, 0, 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

type testFile struct {
	name string
	data []byte
}

// buildRPA 手工构造一个 RPA-3.0 归档（含非零 XOR 密钥）。
//
// 布局：头行（RPA-3.0 <16hex 索引偏移> <8hex 密钥>\n）+ 引导填充 +
// pickle 索引 + 各文件数据。索引按给定顺序写：
//
//	PROTO 2, EMPTY_DICT, {encname: [(off, size, "")]}, STOP
func buildRPA(t *testing.T, files []testFile, key uint32) []byte {
	t.Helper()
	const hdrLen = len("RPA-3.0 0000000000000000 00000000\n") // 8+16+1+8+1 = 34
	junk := []byte("JUNK1234")

	// 每条目字节数：BINUNICODE(5+encLen) + EMPTY_LIST(1) +
	// MARK(1)+BININT4(5)+BININT4(5)+SHORT_BINUNICODE(2)+TUPLE(1) +
	// APPEND(1) + SETITEM(1)。头部：PROTO(2)+EMPTY_DICT(1)+STOP(1)。
	idxLen := 4
	for _, f := range files {
		idxLen += 5 + hex.EncodedLen(len(f.name)) + 1 + 14 + 2
	}
	// 头行的偏移字段指向索引区（紧跟 junk 之后）。
	indexStart := hdrLen + len(junk)

	var idx bytes.Buffer
	idx.WriteByte(0x80) // PROTO 2
	idx.WriteByte(0x02)
	idx.WriteByte(0x7d) // EMPTY_DICT
	var offAt []int
	for _, f := range files {
		enc := make([]byte, len(f.name))
		for i := 0; i < len(f.name); i++ {
			enc[i] = f.name[i] ^ byte(key>>((i%4)*8))
		}
		idx.WriteByte(0x58) // BINUNICODE
		encStr := hex.EncodeToString(enc)
		n := len(encStr)
		idx.Write([]byte{byte(n), byte(n >> 8), byte(n >> 16), byte(n >> 24)})
		idx.WriteString(encStr)
		idx.WriteByte(0x5d) // EMPTY_LIST
		idx.WriteByte(0x28) // MARK
		idx.WriteByte(0x4a) // BININT4 offset（回填）
		offAt = append(offAt, idx.Len())
		idx.Write([]byte{0, 0, 0, 0})
		idx.WriteByte(0x4a) // BININT4 size
		s := len(f.data)
		idx.Write([]byte{byte(s), byte(s >> 8), byte(s >> 16), byte(s >> 24)})
		idx.WriteByte(0x8c) // SHORT_BINUNICODE ""（prefix）
		idx.WriteByte(0)
		idx.WriteByte(0x74) // TUPLE
		idx.WriteByte(0x61) // APPEND
		idx.WriteByte(0x73) // SETITEM
	}
	idx.WriteByte(0x2e) // STOP
	index := idx.Bytes()
	if len(index) != idxLen {
		t.Fatalf("索引长度预估 %d ≠ 实际 %d", idxLen, len(index))
	}

	out := []byte("RPA-3.0 " + fmt16(indexStart) + " " + fmt8(key) + "\n")
	out = append(out, junk...)
	out = append(out, index...)
	idxBase := hdrLen + len(junk)
	for i, f := range files {
		dataOff := uint32(len(out)) // 即将追加的数据在归档里的偏移
		p := idxBase + offAt[i]
		out[p] = byte(dataOff)
		out[p+1] = byte(dataOff >> 8)
		out[p+2] = byte(dataOff >> 16)
		out[p+3] = byte(dataOff >> 24)
		out = append(out, f.data...)
	}
	return out
}

func fmt16(v int) string {
	const digits = "0123456789abcdef"
	s := make([]byte, 16)
	for i := 15; i >= 0; i-- {
		s[i] = digits[v&0xf]
		v >>= 4
	}
	return string(s)
}

func fmt8(v uint32) string {
	const digits = "0123456789abcdef"
	s := make([]byte, 8)
	for i := 7; i >= 0; i-- {
		s[i] = digits[v&0xf]
		v >>= 4
	}
	return string(s)
}

func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func writeGameDir(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	game := filepath.Join(root, "game")
	if err := os.MkdirAll(game, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestPNG(t, filepath.Join(game, "images", "cg01.png"), 400, 300)
	writeTestPNG(t, filepath.Join(game, "images", "bg_class.png"), 400, 300)
	writeTestPNG(t, filepath.Join(game, "images", "002.png"), 400, 300)
	script := `# 游戏主脚本
image cg special = "images/cg01.png"

label start:
    scene bg_class with dissolve
    "这里是旁白。"
    e "角色说的话。"
    show cg special
    pause 1.0
    scene 002
    return

label menu_screen:
    "没有图片演出的 label。"
    return
`
	if err := os.WriteFile(filepath.Join(game, "script.rpy"), []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestParseScript(t *testing.T) {
	root := writeGameDir(t)
	sc, err := ParseScriptFile(filepath.Join(root, "game", "script.rpy"))
	if err != nil {
		t.Fatal(err)
	}
	if len(sc.Labels) != 2 || sc.Labels[0] != "start" || sc.Labels[1] != "menu_screen" {
		t.Fatalf("labels = %v", sc.Labels)
	}
	if def, ok := sc.ImageDefs["cg special"]; !ok || def != "images/cg01.png" {
		t.Fatalf("image 定义解析错误：%v", sc.ImageDefs)
	}
	body := sc.Body["start"]
	if len(body) == 0 {
		t.Fatal("start 没有语句")
	}
	if body[0].Kind != "scene" || body[0].Name != "bg_class" {
		t.Fatalf("第一条语句错：%+v", body[0])
	}
	says := 0
	for _, st := range body {
		if st.Kind == "say" {
			says++
		}
	}
	if says != 2 {
		t.Fatalf("台词应有 2 条，实为 %d", says)
	}
}

func TestFindGameDir(t *testing.T) {
	root := writeGameDir(t)
	got, ok := FindGameDir(root)
	if !ok || got != "game" {
		t.Fatalf("标准布局找不到 game：%q %v", got, ok)
	}

	// 套壳布局：两层目录 + 大写 Game（兑现 实测形态）。
	gameAbs := filepath.Join(root, "game")
	nested := filepath.Join(root, "包装", "SomeGame-1.0-pc", "Game")
	if err := os.MkdirAll(filepath.Dir(nested), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(gameAbs, nested); err != nil {
		t.Fatal(err)
	}
	got, ok = FindGameDir(root)
	if !ok {
		t.Fatal("套壳布局找不到 game")
	}
	if !strings.EqualFold(got, filepath.ToSlash(filepath.Join("包装", "SomeGame-1.0-pc", "Game"))) {
		t.Fatalf("套壳布局路径不对：%q", got)
	}
}

func TestScanAndBuildShow(t *testing.T) {
	root := writeGameDir(t)
	res, err := Scan(root, "game")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Refs) != 1 || res.Refs[0].Key != "start" {
		t.Fatalf("Refs = %+v", res.Refs)
	}
	if res.Refs[0].Images != 3 { // bg_class + cg special + 002
		t.Fatalf("start 可解析图片数 = %d（应 3）", res.Refs[0].Images)
	}

	ev, err := BuildShow(root, "game", "start")
	if err != nil {
		t.Fatal(err)
	}
	shows, texts, waits := 0, 0, 0
	var firstShow *ir.ShowOp
	for i := range ev.Show {
		op := &ev.Show[i]
		switch op.Kind {
		case ir.ShowLayerShow:
			shows++
			if firstShow == nil {
				firstShow = op
			}
		case ir.ShowTextRun:
			texts++
		case ir.ShowWait:
			waits++
		}
	}
	if shows != 3 {
		t.Fatalf("layer_show 应 3 条，实为 %d", shows)
	}
	if texts != 2 {
		t.Fatalf("text 应 2 条，实为 %d", texts)
	}
	if waits == 0 {
		t.Fatal("with/pause 应产生 wait")
	}
	if firstShow == nil || firstShow.Ref != "game/images/bg_class.png" {
		t.Fatalf("首条 show 的 Ref 错：%+v", firstShow)
	}
	if ev.EndMs <= 0 {
		t.Fatal("EndMs 应大于 0")
	}
}

// TestRPAOpenAndExtract 非零密钥的合成归档：索引解析 + 按需解包进缓存。
func TestRPAOpenAndExtract(t *testing.T) {
	root := t.TempDir()
	game := filepath.Join(root, "game")
	if err := os.MkdirAll(game, 0o755); err != nil {
		t.Fatal(err)
	}
	const key = 0x11223344
	rpa := buildRPA(t, []testFile{
		{name: "CG/hidden_cg.png", data: pngBytes(t, 320, 240)},
	}, key)
	if err := os.WriteFile(filepath.Join(game, "archive.rpa"), rpa, 0o644); err != nil {
		t.Fatal(err)
	}
	script := "label rpa_cg:\n    scene cg hidden_cg\n    return\n"
	if err := os.WriteFile(filepath.Join(game, "rpa_script.rpy"), []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := Scan(root, "game")
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(res.Warnings) > 0 {
		t.Logf("警告：%v", res.Warnings)
	}
	if len(res.Refs) != 1 || res.Refs[0].Key != "rpa_cg" || res.Refs[0].Images != 1 {
		t.Fatalf("Refs = %+v（rpa 图像应可解析）", res.Refs)
	}
	if res.Archives != 1 {
		t.Fatalf("Archives = %d", res.Archives)
	}

	ev, err := BuildShow(root, "game", "rpa_cg")
	if err != nil {
		t.Fatal(err)
	}
	var show *ir.ShowOp
	for i := range ev.Show {
		if ev.Show[i].Kind == ir.ShowLayerShow {
			show = &ev.Show[i]
			break
		}
	}
	if show == nil {
		t.Fatal("rpa 图像没有产 layer_show")
	}
	want := filepath.ToSlash(filepath.Join("game", cacheDirName, "CG", "hidden_cg.png"))
	if show.Ref != want {
		t.Fatalf("Ref = %q，应为缓存路径 %q", show.Ref, want)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(want))); err != nil {
		t.Fatalf("缓存文件未落盘：%v", err)
	}
}
