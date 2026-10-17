// Package kirikiri 实现 KiriKiri / XP3 引擎适配器。
//
// 对应 docs/04 引擎适配器接口规范 与 P0 任务（KiriKiri 脚本解析 + 引用裁剪）。
// 格式要点（2026-10 实测，见 gar_tool/probe_xp3.py 与各 README_解包说明.md）：
//
//	XP3 归档：
//	  头部 11 字节魔数 "XP3\r\n \n\x1a\x8bg\x01"，0x0B 处 int64 LE 为索引偏移；
//	  索引偏移处 u32==0x80 表示重定向，真实索引偏移在 +9 处的 int64 LE；
//	  索引处 htype=1 时，+1 处 int64 为压缩长度、+9 处 int64 为原始长度、
//	  +17 处起为 zlib 压缩的索引表。
//	索引表由块组成：[u32 签名 LE][i64 长度 LE][载荷]
//	  "hnfn"：u32 hash + u16 名长 + UTF-16LE 名（hash→名 映射）
//	  "File"：子块 "info"(u32 加密标志 + i64 原始长 + i64 压缩长 + u16 名长 + UTF-16LE 名)
//	          "segm"(重复 28 字节：i32 压缩标志 + i64 偏移 + i64 原始长 + i64 压缩长)
//	          "adlr"(u32 hash)
//
//	脚本 .ks 编码：UTF-16LE BOM / 单字节 XOR 出 BOM / UTF-8 / Shift_JIS 明文，
//	按 KAG 结构分探测（详见 DecodeScript）。
package kirikiri

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"unicode/utf16"
	"unicode/utf8"
	"unsafe"

	"ucgr/internal/adapter"
)

// 块签名（little-endian u32）。
const (
	sigFile uint32 = 0x656C6946 // "File"
	sigHnfn uint32 = 0x6E666E68 // "hnfn"
	sigInfo uint32 = 0x6F666E69 // "info"
	sigSegm uint32 = 0x6D676573 // "segm"
	sigAdlr uint32 = 0x726C6461 // "adlr"
)

// xp3MagicPrefix XP3 魔数前缀（前 8 字节即可判定）。
var xp3MagicPrefix = []byte("XP3\r\n \n\x1a")

// Adapter KiriKiri 适配器。
type Adapter struct{}

func init() {
	adapter.Register("kirikiri", &Adapter{})
}

// Identify 识别 XP3 归档或含 XP3 的目录。
func (a *Adapter) Identify(archiveOrDir string) (string, error) {
	info, err := os.Stat(archiveOrDir)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		f, err := os.Open(archiveOrDir)
		if err != nil {
			return "", err
		}
		defer f.Close()
		head := make([]byte, 8)
		if _, err := io.ReadFull(f, head); err != nil {
			return "", nil // 文件太短，不是 XP3
		}
		if bytes.HasPrefix(head, xp3MagicPrefix) {
			return "kirikiri", nil
		}
		return "", nil
	}
	// 目录：根层存在 *.xp3，或树内含 *.ks（已解包脚本树）即判 KiriKiri。
	// （配合 exe/dll 线索更稳，此处从简。）
	ents, err := os.ReadDir(archiveOrDir)
	if err != nil {
		return "", err
	}
	for _, e := range ents {
		if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".xp3") {
			return "kirikiri", nil
		}
	}
	// 已解包目录（无 .xp3）：靠 .ks 脚本判定，避免“纯解包树”识别失败。
	if hasExtWithin(archiveOrDir, ".ks", dirProbeMaxDepth) {
		return "kirikiri", nil
	}
	return "", nil
}

// dirProbeMaxDepth 目录探测最大深度（避免在大树上做全量遍历）。
const dirProbeMaxDepth = 3

// hasExtWithin 在 root 树下（深度 < maxDepth，跳过副本树）寻找指定扩展名的文件。
func hasExtWithin(root, ext string, maxDepth int) bool {
	found := false
	base := strings.Count(filepath.Clean(root), string(os.PathSeparator))
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if d.Name() == "解包输出" || d.Name() == "_分类" {
				return filepath.SkipDir
			}
			if strings.Count(filepath.Clean(p), string(os.PathSeparator))-base >= maxDepth {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.EqualFold(filepath.Ext(p), ext) {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

// ParseArchive 解析 XP3 索引，返回全部条目。
func (a *Adapter) ParseArchive(path string) ([]adapter.ArchiveEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	head := make([]byte, 32)
	if _, err := io.ReadFull(f, head); err != nil {
		return nil, fmt.Errorf("读取头部失败: %w", err)
	}
	if !bytes.HasPrefix(head, xp3MagicPrefix) {
		return nil, fmt.Errorf("不是 XP3 归档: %s", path)
	}
	dirOff := int64(binary.LittleEndian.Uint64(head[0x0B:0x13]))
	fileSize := int64(0)
	if fi, err := f.Stat(); err == nil {
		fileSize = fi.Size()
	}
	if dirOff < 0x13 || (fileSize > 0 && dirOff >= fileSize) {
		return nil, fmt.Errorf("dir_offset 越界: %d", dirOff)
	}

	// 重定向：索引偏移处 u32==0x80 时，真实偏移在 +9
	if _, err := f.Seek(dirOff, io.SeekStart); err != nil {
		return nil, err
	}
	var mark [4]byte
	if _, err := io.ReadFull(f, mark[:]); err != nil {
		return nil, err
	}
	if binary.LittleEndian.Uint32(mark[:]) == 0x80 {
		var buf [8]byte
		if _, err := f.ReadAt(buf[:], dirOff+9); err != nil {
			return nil, err
		}
		dirOff = int64(binary.LittleEndian.Uint64(buf[:]))
		if dirOff < 0x13 || (fileSize > 0 && dirOff >= fileSize) {
			return nil, fmt.Errorf("重定向后 dir_offset 越界: %d", dirOff)
		}
	}

	// 读索引表
	if _, err := f.Seek(dirOff, io.SeekStart); err != nil {
		return nil, err
	}
	ih := make([]byte, 17)
	if _, err := io.ReadFull(f, ih); err != nil {
		return nil, fmt.Errorf("读取索引头失败: %w", err)
	}
	var table []byte
	switch ih[0] {
	case 0: // 明文索引
		hsize := int64(binary.LittleEndian.Uint64(ih[1:9]))
		if _, err := f.Seek(dirOff+9, io.SeekStart); err != nil {
			return nil, err
		}
		table = make([]byte, hsize)
		if _, err := io.ReadFull(f, table); err != nil {
			return nil, fmt.Errorf("读取明文索引失败: %w", err)
		}
	case 1: // zlib 压缩索引
		packed := int64(binary.LittleEndian.Uint64(ih[1:9]))
		if _, err := f.Seek(dirOff+17, io.SeekStart); err != nil {
			return nil, err
		}
		zr, err := zlib.NewReader(io.NewSectionReader(f, dirOff+17, packed))
		if err != nil {
			return nil, fmt.Errorf("索引不是合法 zlib: %w", err)
		}
		table, err = io.ReadAll(zr)
		if err != nil {
			return nil, fmt.Errorf("解压索引失败: %w", err)
		}
	default:
		return nil, fmt.Errorf("未知索引类型: %d", ih[0])
	}

	return parseIndexTable(table)
}

// parseIndexTable 遍历索引表块，组装条目。
func parseIndexTable(table []byte) ([]adapter.ArchiveEntry, error) {
	hnfn := map[uint32]string{}
	type rawEntry struct {
		name    string
		enc     bool
		usize   int64
		hash    uint32
		hasHash bool
		segs    []adapter.ArchiveEntry
	}
	var raws []rawEntry

	pos := 0
	n := len(table)
	for pos+12 <= n {
		magic := binary.LittleEndian.Uint32(table[pos : pos+4])
		size := int64(binary.LittleEndian.Uint64(table[pos+4 : pos+12]))
		pos += 12
		if size < 0 || pos+int(size) > n {
			break
		}
		pay := table[pos : pos+int(size)]
		pos += int(size)

		switch magic {
		case sigHnfn:
			if len(pay) >= 6 {
				h := binary.LittleEndian.Uint32(pay[0:4])
				nl := int(binary.LittleEndian.Uint16(pay[4:6]))
				if 6+nl*2 <= len(pay) {
					hnfn[h] = decodeUTF16LE(pay[6 : 6+nl*2])
				}
			}
		case sigFile:
			re, err := parseFileChunk(pay)
			if err != nil {
				continue // 单条目损坏不拖垮整体
			}
			raws = append(raws, re)
		}
	}

	var out []adapter.ArchiveEntry
	for _, r := range raws {
		name := r.name
		if r.hasHash {
			if mapped, ok := hnfn[r.hash]; ok {
				name = mapped
			}
		}
		if name == "" || len(r.segs) == 0 {
			continue
		}
		for _, s := range r.segs {
			s.Path = name
			s.Encrypted = r.enc
			if r.enc {
				s.KeyHint = "adlr"
			}
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// parseFileChunk 解析 File 块的 info/segm/adlr 子块。
func parseFileChunk(pay []byte) (struct {
	name    string
	enc     bool
	usize   int64
	hash    uint32
	hasHash bool
	segs    []adapter.ArchiveEntry
}, error) {
	var re struct {
		name    string
		enc     bool
		usize   int64
		hash    uint32
		hasHash bool
		segs    []adapter.ArchiveEntry
	}
	q, size := 0, len(pay)
	for q+12 <= size {
		sec := binary.LittleEndian.Uint32(pay[q : q+4])
		ssize := int64(binary.LittleEndian.Uint64(pay[q+4 : q+12]))
		q += 12
		if ssize < 0 || q+int(ssize) > size {
			break
		}
		sp := pay[q : q+int(ssize)]
		switch sec {
		case sigInfo:
			if len(sp) >= 22 {
				re.enc = binary.LittleEndian.Uint32(sp[0:4]) != 0
				re.usize = int64(binary.LittleEndian.Uint64(sp[4:12]))
				nl := int(binary.LittleEndian.Uint16(sp[20:22]))
				if 22+nl*2 <= len(sp) {
					re.name = decodeUTF16LE(sp[22 : 22+nl*2])
				}
			}
		case sigSegm:
			o := 0
			for o+28 <= len(sp) {
				comp := int32(binary.LittleEndian.Uint32(sp[o : o+4]))
				off := int64(binary.LittleEndian.Uint64(sp[o+4 : o+12]))
				sz := int64(binary.LittleEndian.Uint64(sp[o+12 : o+20]))
				pk := int64(binary.LittleEndian.Uint64(sp[o+20 : o+28]))
				e := adapter.ArchiveEntry{
					Offset:           off,
					Size:             sz,
					UncompressedSize: sz,
				}
				e.Compression = adapter.CompNone
				if comp != 0 {
					e.Compression = adapter.CompZlib
					e.Size = pk
				}
				re.segs = append(re.segs, e)
				o += 28
			}
		case sigAdlr:
			if len(sp) >= 4 {
				re.hash = binary.LittleEndian.Uint32(sp[0:4])
				re.hasHash = true
			}
		}
		q += int(ssize)
	}
	return re, nil
}

func decodeUTF16LE(b []byte) string {
	if len(b)%2 != 0 {
		b = b[:len(b)-1]
	}
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = binary.LittleEndian.Uint16(b[i*2 : i*2+2])
	}
	return string(utf16.Decode(u))
}

// ReadArchive 读取并解压条目完整内容（段拼接 + zlib）。
func ReadArchive(f *os.File, e adapter.ArchiveEntry) ([]byte, error) {
	if _, err := f.Seek(e.Offset, io.SeekStart); err != nil {
		return nil, err
	}
	raw := make([]byte, e.Size)
	if _, err := io.ReadFull(f, raw); err != nil {
		return nil, fmt.Errorf("读取段失败 %s: %w", e.Path, err)
	}
	if e.Compression == adapter.CompZlib {
		zr, err := zlib.NewReader(bytes.NewReader(raw))
		if err != nil {
			return nil, fmt.Errorf("段不是合法 zlib %s: %w", e.Path, err)
		}
		return io.ReadAll(zr)
	}
	return raw, nil
}

// ---------------------------------------------------------------
// 脚本解析
// ---------------------------------------------------------------

// Command KAG 脚本的一条指令。
type Command struct {
	Tag   string            `json:"tag"`
	Attrs map[string]string `json:"attrs,omitempty"`
	Raw   string            `json:"raw"`
}

// SeqItem 脚本顺序项：命令或文本行（BuildIR 按序遍历建时间轴）。
type SeqItem struct {
	Kind string   `json:"kind"` // "cmd" | "text"
	Text string   `json:"text,omitempty"`
	Cmd  *Command `json:"-"` // Kind=="cmd" 时指向 ast.Commands 中的元素
}

// Ast KiriKiri 引擎专有 AST（对核心不透明）。
type Ast struct {
	Encoding string    `json:"encoding"`
	Commands []Command `json:"commands"`
	Texts    []string  `json:"texts,omitempty"`
	// Seq 保序的 命令/文本 交替流（IR 时间轴的依据）
	Seq []SeqItem `json:"-"`
	// DynamicVoice 检测到动态语音引用（voice= 属性 / voiceBase 变量拼名）。
	// 此时静态解析不到语音文件 → 调用方应按"宁多勿缺"保留全部语音。
	DynamicVoice bool `json:"-"`
}

// detectDynamicVoice 检查命令是否使用动态语音引用。
// 夏空实测：`[nm t=… voice=1]` + `[eval exp='f.voiceBase="214"']`，
// 语音文件名由 voiceBase + 行号在运行时拼接，静态不可解。
func detectDynamicVoice(c *Command) bool {
	if v := c.Attrs["voice"]; v != "" {
		return true
	}
	if exp := c.Attrs["exp"]; exp != "" {
		if strings.Contains(strings.ToLower(exp), "voicebase") ||
			strings.Contains(strings.ToLower(exp), "voicename") {
			return true
		}
	}
	return false
}

// 非台词的裸行首词（标签/变量/控制流）， collected as text 时过滤掉。
var bareSkipPrefix = []string{"*", "#", "if(", "if ", "else", "endif", "jumpstop", "undo", "return"}

func isBareNoise(s string) bool {
	for _, p := range bareSkipPrefix {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

// emitLine 把一行拆成保序的命令/文本项。
func (ast *Ast) emitLine(line string) {
	s := strings.TrimRight(line, "\r")
	if strings.TrimSpace(s) == "" {
		return
	}
	rest := s
	// 行首 @宏 调用（忽略前导空白）
	if m := atCallRe.FindStringSubmatch(strings.TrimLeft(s, " \t")); m != nil {
		lead := len(s) - len(strings.TrimLeft(s, " \t"))
		c := Command{Tag: m[1], Attrs: parseAttrs(m[2]), Raw: m[0]}
		ast.Commands = append(ast.Commands, c)
		ast.Seq = append(ast.Seq, SeqItem{Kind: "cmd", Cmd: &ast.Commands[len(ast.Commands)-1]})
		rest = s[lead+len(m[0]):]
	}
	// [tag] 与文本段交错
	pos := 0
	for _, tm := range tagRe.FindAllStringSubmatchIndex(rest, -1) {
		if seg := strings.TrimSpace(rest[pos:tm[0]]); seg != "" && !isBareNoise(seg) {
			ast.Texts = append(ast.Texts, seg)
			ast.Seq = append(ast.Seq, SeqItem{Kind: "text", Text: seg})
		}
		c := Command{Tag: rest[tm[2]:tm[3]], Attrs: parseAttrs(rest[tm[4]:tm[5]]), Raw: rest[tm[0]:tm[1]]}
		ast.Commands = append(ast.Commands, c)
		ast.Seq = append(ast.Seq, SeqItem{Kind: "cmd", Cmd: &ast.Commands[len(ast.Commands)-1]})
		if detectDynamicVoice(&c) {
			ast.DynamicVoice = true
		}
		pos = tm[1]
	}
	if seg := strings.TrimSpace(rest[pos:]); seg != "" && !isBareNoise(seg) {
		ast.Texts = append(ast.Texts, seg)
		ast.Seq = append(ast.Seq, SeqItem{Kind: "text", Text: seg})
	}
}

// 标签正则：允许 CJK 标签名（夏空 [暗転]/[主人公の部屋] 等日文宏名，
// 只认 ASCII 会把它们整行误判为台词）。
var tagRe = regexp.MustCompile(`[@\[]([\p{L}_][\p{L}\p{N}_]*)((?:\s+[^\]]*?)?)[\]]`)
var attrRe = regexp.MustCompile(`([A-Za-z_][A-Za-z0-9_]*)\s*=\s*("[^"]*"|'[^']*'|[^\s\]@]+)`)
var atCallRe = regexp.MustCompile(`^@([A-Za-z_][A-Za-z0-9_]*)\s*(.*)$`)
var tagStartRe = regexp.MustCompile(`[@\[]([A-Za-z])`)
var kagHintRe = regexp.MustCompile(`\[(macro|endmacro|name|text|storage|eval|end\b|if|else|elsif|endif|jump|return|call|wait|ws|bg|cg|chara|playbgm|stopbgm|vo|movie|trans|sel|graph|position|layer|clearlayers|history|sysmovie|linemode|init)`)
var numericRe = regexp.MustCompile(`^[0-9]+$`)

// DecodeScript 编码探测（对应 ks_refs.decode_ks）：
//  1. UTF-16LE BOM
//  2. 单字节 XOR k 后成 UTF-16LE BOM（9nine 系统脚本等）
//  3. UTF-8 / Shift_JIS 明文，按 KAG 结构分择优；坏字符超 5% 的候选出局
//     （防 SJIS 字节被 UTF-8 解成乱码后凭 ASCII 结构分持平胜出）
func DecodeScript(data []byte) (string, string) {
	if len(data) >= 2 && data[0] == 0xFF && data[1] == 0xFE {
		return decodeUTF16LE(data[2:]), "utf16le-bom"
	}
	for k := 1; k < 256; k++ {
		if data[0]^byte(k) == 0xFF && data[1]^byte(k) == 0xFE {
			d := make([]byte, len(data))
			for i, b := range data {
				d[i] = b ^ byte(k)
			}
			return decodeUTF16LE(d[2:]), fmt.Sprintf("xor-0x%02x+utf16le", k)
		}
	}
	best, bestText, bestEnc := 0, "", ""
	// UTF-8
	if s, ok := tryDecode(data, "utf-8"); ok {
		if sc := kagScore(s); sc > best && badRatio(s) <= 0.05 {
			best, bestText, bestEnc = sc, s, "plain-utf-8"
		}
	}
	// Shift_JIS（Windows CP932，坏字节替换为 U+FFFD）
	if s, ok := tryDecodeSJIS(data); ok {
		if sc := kagScore(s); sc > best && badRatio(s) <= 0.05 {
			best, bestText, bestEnc = sc, s, "plain-shift_jis"
		}
	}
	if bestEnc != "" && best >= 10 {
		return bestText, bestEnc
	}
	// 弱接受（对应 ks_refs.weak_ok）：无 [tag] 的零引用存根
	// （如 dummy_scene.ks = "*dummy| シーン回想ダミー @msgoff @endrecollection"），
	// 结构分打不够但内容确实干净。要求：坏字符 ≤1%、含 KAG 标记、含换行。
	if s, ok := weakAccept(data); ok {
		return s, "weak"
	}
	return "", "undecodable"
}

// weakAccept 弱接受判定（仅用于干净内容，防乱码混入）。
func weakAccept(data []byte) (string, bool) {
	var s string
	if len(data) >= 2 && data[0] == 0xFF && data[1] == 0xFE {
		s = decodeUTF16LE(data[2:])
	} else {
		s = decodeSJIS(data)
	}
	if s == "" || badRatio(s) > 0.01 {
		return "", false
	}
	if !strings.ContainsAny(s, "[@*;") {
		return "", false
	}
	if !strings.Contains(s, "\n") && !strings.Contains(s, "\r") {
		return "", false
	}
	return s, true
}

// badRatio 坏字符（U+FFFD）占比。超过 5% 的候选一律出局——
// 防 SJIS 字节被 UTF-8 解成乱码后凭 ASCII 结构分持平胜出，
// 也防前人解密不完整的脚本被当明文收下。
func badRatio(s string) float64 {
	if s == "" {
		return 0
	}
	bad := strings.Count(s, "\ufffd")
	return float64(bad) / float64(len([]rune(s)))
}

func tryDecode(data []byte, enc string) (string, bool) {
	if enc == "utf-8" {
		if !utf8.Valid(data) {
			return "", false
		}
		return string(data), true
	}
	return "", false
}

// decodeSJIS 解码 Shift_JIS（码页 932），坏字节替换为 U+FFFD。
// 走 Windows MultiByteToWideChar（零第三方依赖）；非 Windows 平台退化为原样返回。
func decodeSJIS(data []byte) string {
	if runtime.GOOS != "windows" || len(data) == 0 {
		return string(data)
	}
	k32 := syscall.NewLazyDLL("kernel32.dll")
	proc := k32.NewProc("MultiByteToWideChar")
	const cpSJIS = 932
	n, _, _ := proc.Call(cpSJIS, 0,
		uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)), 0, 0)
	if n == 0 {
		return string(data)
	}
	w := make([]uint16, n)
	proc.Call(cpSJIS, 0,
		uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)),
		uintptr(unsafe.Pointer(&w[0])), uintptr(n))
	return string(utf16.Decode(w))
}

func tryDecodeSJIS(data []byte) (string, bool) {
	return decodeSJIS(data), true
}

// kagScore KAG 结构分：标签起始 / storage= / text= / 命令词密度。
// 与 ks_refs._kag_score 保持一致（少任一项都会改变解码通过边界）。
func kagScore(t string) int {
	sc := 3*len(tagStartRe.FindAllString(t, -1)) + 2*strings.Count(t, "storage=")
	sc += strings.Count(t, "text=")
	sc += len(kagHintRe.FindAllString(t, -1))
	return sc
}

// ParseScript 解析 .ks 脚本，产出 Ast。
func (a *Adapter) ParseScript(data []byte, ctx *adapter.ScriptCtx) (adapter.ScriptAst, error) {
	text, enc := DecodeScript(data)
	if text == "" {
		return nil, fmt.Errorf("脚本解码失败: %s", enc)
	}
	ast := &Ast{Encoding: enc}
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimLeft(strings.TrimRight(line, "\r"), " \t")
		if trimmed == "" || strings.HasPrefix(trimmed, ";") {
			continue // 空行 / 注释行：作者注掉的旧代码，引用不生效
		}
		ast.emitLine(line)
	}
	return ast, nil
}

func parseAttrs(body string) map[string]string {
	attrs := map[string]string{}
	for _, m := range attrRe.FindAllStringSubmatch(body, -1) {
		v := strings.Trim(m[2], "\"'")
		attrs[m[1]] = v
	}
	return attrs
}

// ResourceAttrs 资源类属性（全局）+ 按标签扩展属性。
var (
	resourceAttrs = map[string]bool{"storage": true, "file": true, "src": true, "sound": true}
	scriptTags    = map[string]bool{"jump": true, "call": true, "addsysscript": true, "addsyshook": true}
	tagExtraAttrs = map[string][]string{
		"chr":     {"c", "l", "r", "ll", "rr"},
		"mface":   {"name"},
		"nm":      {"s"},
		"newchar": {"name"},
		"newlay":  {"file"},
	}
)

// 位置参数（非 attr=value）里的常见关键字——不是资源名。
// 各游戏自定义宏方言不同，此处收集镇锦；漏掉的会变成未命中引用（无害）。
var posKeywords = map[string]bool{
	"hide": true, "show": true, "normal": true, "grayscale": true,
	"resetcolor": true, "msgoff": true, "msgon": true, "stop": true,
	"clear": true, "sync": true, "wait": true, "time": true, "loop": true,
	"mode": true, "pos": true, "blend": true, "fade": true, "left": true,
	"right": true, "center": true, "true": true, "false": true,
	"beginskip": true, "endskip": true, "begintrans": true, "endtrans": true,
	"initscene": true, "initbase": true, "history": true, "save": true,
	"load": true, "quake": true, "se": true, "bgm": true, "vo": true,
	"trans": true, "rule": true, "layer": true, "page": true, "fore": true,
	"back": true, "color": true, "alpha": true, "delay": true, "cond": true,
	"canskip": true, "notrans": true, "nontrans": true, "keep": true,
	"nowait": true, "click": true, "auto": true, "fast": true, "slow": true,
}

// 裸 token：字母/数字/下划线/连字符开头（含 CJK，如「山道1_A」）。
var bareTokRe = regexp.MustCompile(`[\p{L}\p{N}_][\p{L}\p{N}_\-]*`)

// positionalRefs 提取命令体里的位置参数资源候选。
// 先剥掉 attr=value 再扫裸 token——各游戏自定义宏（如夏空 [EV EV0220A ...]）
// 把图片名放在位置参数而不是 storage=，只认属性会整类漏掉（误裁风险）。
func positionalRefs(c *Command) []string {
	body := c.Raw
	body = strings.TrimPrefix(body, "[")
	body = strings.TrimPrefix(body, "@")
	body = strings.TrimPrefix(body, c.Tag)
	body = strings.TrimSuffix(strings.TrimSpace(body), "]")
	body = attrRe.ReplaceAllString(body, " ")
	var out []string
	for _, m := range bareTokRe.FindAllString(body, -1) {
		if len(m) < 3 || posKeywords[strings.ToLower(m)] {
			continue
		}
		out = append(out, m)
	}
	return out
}

// ExtractRefs 从 Ast 提取资源引用（噪声过滤同 ks_refs.parse_refs）。
func (ast *Ast) ExtractRefs() []string {
	var refs []string
	for _, c := range ast.Commands {
		extra := map[string]bool{}
		for _, k := range tagExtraAttrs[c.Tag] {
			extra[k] = true
		}
		for k, v := range c.Attrs {
			if !resourceAttrs[k] && !extra[k] {
				continue
			}
			if v == "" || strings.HasPrefix(v, "$") {
				continue
			}
			// %参数|默认值：取 | 后的默认值（KAG 宏默认参数，常是真资源）
			if strings.HasPrefix(v, "%") || strings.HasPrefix(v, "&") {
				idx := strings.Index(v, "|")
				if idx < 0 {
					continue
				}
				v = v[idx+1:]
				if v == "" {
					continue
				}
			}
			if numericRe.MatchString(v) {
				continue
			}
			if k == "storage" && scriptTags[c.Tag] {
				continue
			}
			// 剧本/二进制脚本引用：按 DR-25 剧本全留，不算资产引用
			low := strings.ToLower(v)
			if strings.HasSuffix(low, ".ks") || strings.HasSuffix(low, ".tjs") {
				continue
			}
			refs = append(refs, strings.ReplaceAll(v, "\\", "/"))
		}
		// 位置参数候选（[EV EV0220A ...] 这类方言；由匹配层过滤真假）
		for _, p := range positionalRefs(&c) {
			if numericRe.MatchString(p) {
				continue
			}
			refs = append(refs, strings.ReplaceAll(p, "\\", "/"))
		}
	}
	return refs
}

// Transitions KiriKiri 转场注册表（P0 初版，详见 docs/04 §9.5）。
func (a *Adapter) Transitions() *adapter.TransitionRegistry {
	r := adapter.NewTransitionRegistry()
	r.Register("crossfade", adapter.TransitionImpl{ImplKey: "fade", DurationMs: 800})
	r.Register("fade", adapter.TransitionImpl{ImplKey: "fade", DurationMs: 800})
	r.Register("fadeout", adapter.TransitionImpl{ImplKey: "fade", DurationMs: 800})
	r.Register("fadein", adapter.TransitionImpl{ImplKey: "fade", DurationMs: 800})
	r.Register("blind", adapter.TransitionImpl{ImplKey: "wipe", DurationMs: 800})
	return r
}
