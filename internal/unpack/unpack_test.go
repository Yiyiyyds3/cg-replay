package unpack

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"unicode/utf16"
)

// ---------------------------------------------------------------
// 合成 XP3 夹具（按适配器解析的格式写：头 19 字节 + 索引表 + 数据段）
// ---------------------------------------------------------------

// xp3File 一个待写入归档的文件。
type xp3File struct {
	path string
	data []byte
	enc  bool   // 写 adlr 块（真实 XP3 里 adlr = 条目名的 Adler32 校验和）
	hash uint32 // adlr 里的哈希值（默认按名字算 Adler32）
}

// writeXP3 写一个最小可解析的 XP3 归档。
//
// 格式（与 internal/adapter/kirikiri 的解析口径一致）：
//
//	头：11 字节魔数 "XP3\r\n \n\x1a\x8bg\x01" + int64 LE 索引偏移（0x0B 处）
//	索引：1 字节类型（0=明文）+ int64 LE 表长 + 表
//	表：若干块（u32 魔数 + u64 长度 + 负载）；File 块里再分
//	    info（u32 加密标记 + u64 原始大小 + u16 名长 + UTF-16LE 名）
//	    与 segm（每段 28 字节：u32 压缩标记 + u64 偏移 + u64 原始大小 + u64 压缩大小）
func writeXP3(t *testing.T, path string, files []xp3File) {
	t.Helper()

	// 建索引表：段偏移取决于「数据区起点」，而数据区起点又取决于表长 →
	// 两遍构建（表长与偏移无关，固定宽度）。
	build := func(dataStart int64) (bytes.Buffer, []byte) {
		var table bytes.Buffer
		var data bytes.Buffer
		for _, f := range files {
			var fileChunk bytes.Buffer
			// info 子块
			name16, nameLen := utf16le(f.path)
			var info bytes.Buffer
			enc := uint32(0)
			if f.enc {
				enc = 1
			}
			binary.Write(&info, binary.LittleEndian, enc)
			binary.Write(&info, binary.LittleEndian, uint64(len(f.data)))
			binary.Write(&info, binary.LittleEndian, uint64(0)) // 保留位（解析器在偏移 20 读名长）
			binary.Write(&info, binary.LittleEndian, uint16(nameLen))
			info.Write(name16)
			writeChunk(&fileChunk, sigInfo, info.Bytes())

			// segm 子块：这段数据不压缩，直接存
			off := dataStart + int64(data.Len())
			data.Write(f.data)
			var segm bytes.Buffer
			binary.Write(&segm, binary.LittleEndian, uint32(0)) // 不压缩
			binary.Write(&segm, binary.LittleEndian, off)
			binary.Write(&segm, binary.LittleEndian, uint64(len(f.data)))
			binary.Write(&segm, binary.LittleEndian, uint64(len(f.data)))
			writeChunk(&fileChunk, sigSegm, segm.Bytes())
			if f.enc {
				// adlr 块：u32 哈希（真实 XP3 里是条目名的 Adler32）。
				h := f.hash
				if h == 0 {
					h = adler32(f.path)
				}
				var adlr bytes.Buffer
				binary.Write(&adlr, binary.LittleEndian, h)
				writeChunk(&fileChunk, sigAdlr, adlr.Bytes())
			}
			writeChunk(&table, sigFile, fileChunk.Bytes())
		}
		return table, data.Bytes()
	}

	probe, _ := build(0)
	dataStart := int64(19 + 9 + probe.Len()) // 文件头 19 + 索引头 9 + 索引表
	table, data := build(dataStart)

	// 头
	var head bytes.Buffer
	head.WriteString("XP3\r\n \n\x1a\x8bg\x01")
	// dir_off 指向「索引头」起点（紧跟 19 字节文件头）：索引头 = 1 字节类型 +
	// 8 字节长度，索引表在 dir_off+9 处。
	dirOff := int64(19)
	binary.Write(&head, binary.LittleEndian, dirOff)
	if head.Len() != 19 {
		t.Fatalf("头长度应为 19，实际 %d", head.Len())
	}

	var out bytes.Buffer
	out.Write(head.Bytes())
	out.WriteByte(0) // 索引类型：明文
	binary.Write(&out, binary.LittleEndian, uint64(table.Len()))
	out.Write(table.Bytes())
	out.Write(data)

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, out.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

// XP3 块魔数（与适配器一致）。
const (
	sigFile = 0x656C6946 // "File"
	sigInfo = 0x6F666E69 // "info"
	sigSegm = 0x6D676573 // "segm"
	sigAdlr = 0x726C6461 // "adlr"
)

// adler32 计算条目名的 Adler32（XP3 的 adlr 块内容）。
func adler32(s string) uint32 {
	const mod = 65521
	var a, b uint32 = 1, 0
	for i := 0; i < len(s); i++ {
		a = (a + uint32(s[i])) % mod
		b = (b + a) % mod
	}
	return (b << 16) | a
}

// xorBytes 用单字节密钥 XOR（造加密夹具用）。
func xorBytes(data []byte, key byte) []byte {
	out := make([]byte, len(data))
	for i := range data {
		out[i] = data[i] ^ key
	}
	return out
}

func writeChunk(buf *bytes.Buffer, magic uint32, payload []byte) {
	binary.Write(buf, binary.LittleEndian, magic)
	binary.Write(buf, binary.LittleEndian, uint64(len(payload)))
	buf.Write(payload)
}

// utf16le UTF-16LE 编码（XP3 文件名用）；返回码元数与字节。
//
// 注意：XP3 的 info 块里「名长」字段存的是 **UTF-16 码元数**（解析器会 ×2
// 取字节数），所以这里必须按 rune 计数，不能用字节长度。
func utf16le(s string) ([]byte, int) {
	units := utf16.Encode([]rune(s))
	var b bytes.Buffer
	for _, u := range units {
		binary.Write(&b, binary.LittleEndian, u)
	}
	return b.Bytes(), len(units)
}

// zlibBytes 造一段 zlib 压缩数据（测多段拼接用）。
func zlibBytes(t *testing.T, data []byte) []byte {
	t.Helper()
	var b bytes.Buffer
	zw := zlib.NewWriter(&b)
	if _, err := zw.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// ---------------------------------------------------------------
// 夹具内容
// ---------------------------------------------------------------

// galleryKS 一个画廊脚本：sysbutton 定义槽位 + *cgNN 段引用图片。
const galleryKS = "[wait time=100]\r\n" +
	"*cgrai\r\n" +
	"[sysbutton graphic=\"cg001\" target=\"*cg001\"]\r\n" +
	"[sysbutton graphic=\"cg002\" target=\"*cg002\"]\r\n" +
	"*cg001\r\n" +
	"[image storage=\"cg001_a.png\" time=100]\r\n" +
	"[wait time=100]\r\n" +
	"*cg002\r\n" +
	"[image storage=\"cg002_a.png\" time=100]\r\n" +
	"[wait time=100]\r\n"

// sceneKS 一个场景脚本。
const sceneKS = "[wait time=100]\r\n" +
	"*label01|开场\r\n" +
	"[r]\r\n" +
	"[cm]\r\n" +
	"*label02|中段\r\n" +
	"[r]\r\n"

func TestUnpackXP3MinimalSet(t *testing.T) {
	dir := t.TempDir()
	xp3 := filepath.Join(dir, "data.xp3")
	writeXP3(t, xp3, []xp3File{
		{path: "data/scenario/cgrai00.ks", data: []byte(galleryKS)},
		{path: "data/scenario/01kuukou.ks", data: []byte(sceneKS)},
		{path: "data/image/cg001_a.png", data: []byte("PNG-1")},
		{path: "data/image/cg002_a.png", data: []byte("PNG-2")},
		{path: "data/image/bg_unused.jpg", data: []byte("JPG-UNUSED")},
	})

	// 预览：应认出 2 个剧本、2 个 CG 事件、2 个素材引用。
	p, err := Preview(dir)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if p.Kind != "archive" || p.EngineID != "kirikiri" {
		t.Fatalf("Plan = %+v", p)
	}
	if p.ScenarioFiles != 2 {
		t.Fatalf("剧本数 = %d, want 2", p.ScenarioFiles)
	}
	if p.Labels != 2 {
		t.Fatalf("CG 事件数 = %d, want 2", p.Labels)
	}
	if p.Refs != 2 {
		t.Fatalf("引用素材数 = %d, want 2（只算 CG 事件引用的）", p.Refs)
	}

	// 解包。
	dest := filepath.Join(t.TempDir(), "game")
	res, err := Extract(dir, dest, ScopeCG)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if res.Events != 2 {
		t.Fatalf("解出事件数 = %d, want 2", res.Events)
	}
	// 2 个剧本 + 2 张图 = 4 个文件；**没用到的 bg_unused.jpg 不该被解出**。
	if res.Files != 4 {
		t.Fatalf("解出文件数 = %d, want 4（2 剧本 + 2 图）：%+v", res.Files, res)
	}
	if len(res.Missing) != 0 {
		t.Fatalf("缺失素材 = %v（应为空）", res.Missing)
	}
	// 落盘布局要与加载器一致。
	for _, rel := range []string{
		"raw/data/scenario/cgrai00.ks",
		"raw/data/scenario/01kuukou.ks",
		"raw/data/image/cg001_a.png",
		"raw/data/image/cg002_a.png",
		"ucgr-convention.json",
	} {
		if _, err := os.Stat(filepath.Join(dest, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("缺 %s：%v", rel, err)
		}
	}
	// 没被引用的图不该出现。
	if _, err := os.Stat(filepath.Join(dest, "raw", "data", "image", "bg_unused.jpg")); err == nil {
		t.Fatal("未被引用的素材不该被解出（最小集合的意义就在这）")
	}
	// 剧本内容应逐字节一致。
	got, err := os.ReadFile(filepath.Join(dest, "raw", "data", "scenario", "cgrai00.ks"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, []byte(galleryKS)) {
		t.Fatalf("剧本内容不一致")
	}
}

func TestUnpackScopeAllIncludesSceneRefs(t *testing.T) {
	dir := t.TempDir()
	xp3 := filepath.Join(dir, "data.xp3")
	writeXP3(t, xp3, []xp3File{
		{path: "data/scenario/cgrai00.ks", data: []byte(galleryKS)},
		{path: "data/scenario/01kuukou.ks", data: []byte("[image storage=\"bg_scene.jpg\" time=100]\r\n[wait time=100]\r\n")},
		{path: "data/image/cg001_a.png", data: []byte("PNG-1")},
		{path: "data/image/bg_scene.jpg", data: []byte("JPG")},
	})
	dest := filepath.Join(t.TempDir(), "game")
	res, err := Extract(dir, dest, ScopeAll)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if res.Files == 0 {
		t.Fatal("ScopeAll 应解出文件")
	}
	if _, err := os.Stat(filepath.Join(dest, "raw", "data", "image", "bg_scene.jpg")); err != nil {
		t.Fatalf("ScopeAll 应解出场景引用的背景：%v", err)
	}
}

func TestUnpackAdlrEntryTreatedAsPlaintext(t *testing.T) {
	dir := t.TempDir()
	xp3 := filepath.Join(dir, "data.xp3")
	writeXP3(t, xp3, []xp3File{
		{path: "data/scenario/cgrai00.ks", data: []byte(galleryKS)},
		{path: "data/image/cg001_a.png", data: []byte("PNG-1"), enc: true},
		{path: "data/image/cg002_a.png", data: []byte("PNG-2")},
	})
	// 预览要报出带 adlr 的条目数（它是校验和，不等于加密）。
	p, err := Preview(dir)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if p.Encrypted != 1 {
		t.Fatalf("带 adlr 的条目数 = %d, want 1", p.Encrypted)
	}
	// 默认（不选方案）：带 adlr 的条目**照常按明文解出**（不能因为有 adlr 就跳过）。
	dest := filepath.Join(t.TempDir(), "game")
	res, err := Extract(dir, dest, ScopeCG)
	if err != nil {
		t.Fatalf("解包不应失败：%v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "raw", "data", "image", "cg001_a.png")); err != nil {
		t.Fatalf("带 adlr 的条目应照常解出（adlr 只是校验和）：%v", err)
	}
	// 但要记下来提示「若解出的是乱码可选解密方案」。
	if len(res.Encrypted) != 1 {
		t.Fatalf("应记录 1 个带 adlr 的条目：%+v", res)
	}
}

// TestUnpackDecryptScheme 选对方案时，加密条目能解出正确内容。
func TestUnpackDecryptScheme(t *testing.T) {
	dir := t.TempDir()
	xp3 := filepath.Join(dir, "data.xp3")
	// 剧本被「用 adlr 哈希派生的单字节密钥」加密（HashCrypt 家族的做法）。
	hash := adler32("data/scenario/cgrai00.ks")
	writeXP3(t, xp3, []xp3File{
		{path: "data/scenario/cgrai00.ks", data: xorBytes([]byte(galleryKS), byte(hash)), enc: true, hash: hash},
		{path: "data/image/cg001_a.png", data: []byte("PNG-1")},
	})

	// 不选方案：解析失败（乱码），要有告警而不是静默。
	destPlain := filepath.Join(t.TempDir(), "plain")
	resPlain, err := Extract(dir, destPlain, ScopeCG)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(resPlain.Warnings) == 0 {
		t.Fatal("未解密时应告警（脚本解析失败），不能静默")
	}

	// 选 hash 方案：解出正确内容，事件数与明文一致。
	scheme, err := SchemeFromString("hash")
	if err != nil {
		t.Fatalf("SchemeFromString: %v", err)
	}
	dest := filepath.Join(t.TempDir(), "dec")
	res, err := ExtractWith(dir, dest, Options{Scope: ScopeCG, Decrypt: scheme})
	if err != nil {
		t.Fatalf("ExtractWith: %v", err)
	}
	if res.Events != 2 {
		t.Fatalf("解密后事件数 = %d, want 2：%+v", res.Events, res)
	}
	if len(res.Warnings) != 0 {
		t.Fatalf("解密后不应有告警：%+v", res.Warnings)
	}
	got, err := os.ReadFile(filepath.Join(dest, "raw", "data", "scenario", "cgrai00.ks"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, []byte(galleryKS)) {
		t.Fatalf("解密后的剧本内容与明文不一致")
	}
}

func TestUnpackLooseDirNeedsNoExtraction(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "raw", "data", "scenario"), 0o755); err != nil {
		t.Fatal(err)
	}
	p, err := Preview(dir)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if p.Kind != "loose" {
		t.Fatalf("Kind = %q, want loose", p.Kind)
	}
	res, err := Extract(dir, filepath.Join(t.TempDir(), "dest"), ScopeCG)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(res.Warnings) == 0 {
		t.Fatal("已是解包目录时应提示「直接登记即可」")
	}
	if res.Files != 0 {
		t.Fatalf("已是解包目录时不该再解一遍：%+v", res)
	}
}

func TestUnpackUnknownDirExplainsWhy(t *testing.T) {
	dir := t.TempDir()
	p, err := Preview(dir)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if !p.Unsupported || p.Reason == "" {
		t.Fatalf("空目录应明确说不支持并给原因：%+v", p)
	}
}
