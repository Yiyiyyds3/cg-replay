// Package catsystem2 实现 CatSystem2 引擎适配器。
//
// 对应 docs/04 引擎适配器接口规范；格式要点（2026-10-16/17 实测，
// 见 gar_tool/cst_refs.py 与 P2_原型报告_CatSystem2引用提取.md）：
//
//	剧本 .cst：CatScene 魔数 + u32 压缩长 + zlib 数据；
//	  解压后条目为 01 <type> <bytes> 00（00 分隔），
//	  type 0x20=对白 / 0x21=角色名 / 0x30=脚本命令
//	命令语法（位置参数 + k=v 混合）：
//	  bg <槽位> <图片名>      chr c=/l=/r=/ll=/rr=<立绘>
//	  mface name=<差分>       pcm <语音>      se <buf> <音效>
//	  movie <影片>            call <子剧本>   fesds ...
//	两个 CS2 专属匹配规则：
//	  1) 多图层 HG3 被展开为 <name>_<序号>.hg3，裸名指【整组】
//	     （由 prune.Pool.ByGroup 组匹配处理）
//	  2) 语音引用带 take 后缀（A_19_sayuki_ex01_022_1 →
//	     A_19_sayuki_ex01_022.ogg），提取时同时登记去后缀变体
//	E-mote（.psb/.psz/.lpb）按硬边界不复刻，由 prune 统一剥出
//
// 归档 .int（内部 KIF，Blowfish+MT 加密）的索引与数据解密见 internal/kif：
// **解包数据不需要游戏主程序口令**，只有名字的**大小写**需要口令才能精确还原
// （口令在个别游戏的 exe PE 资源 DATA/V_CODE2 里，本机这批游戏没有该资源）。
// 剧本 .cst 的引用提取面向【已解包目录】（与 Python 原型同口径）。
package catsystem2

import (
	"compress/zlib"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"ucgr/internal/adapter"
	"ucgr/internal/ir"
	"ucgr/internal/kif"
)

// .cst 条目类型。
const (
	typeText = 0x20 // 对白
	typeName = 0x21 // 角色名
	typeCmd  = 0x30 // 脚本命令
)

// cstMagic .cst 文件魔数。
var cstMagic = []byte("CatScene")

// Adapter CatSystem2 适配器。
type Adapter struct{}

func init() {
	adapter.Register("catsystem2", &Adapter{})
}

// Identify 识别 CS2：目录含 .cst 剧本，或文件是 .int 归档。
func (a *Adapter) Identify(archiveOrDir string) (string, error) {
	info, err := os.Stat(archiveOrDir)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		ext := strings.ToLower(filepath.Ext(archiveOrDir))
		if ext == ".int" || ext == ".kif" {
			return "catsystem2", nil
		}
		return "", nil
	}
	// 目录：直接找 .cst（浅层即可，剧本量大时不必全遍历）
	for _, sub := range []string{"", "scene", "原始归档解包", "原始归档解包/scene"} {
		d := archiveOrDir
		if sub != "" {
			d = filepath.Join(archiveOrDir, sub)
		}
		ents, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		for _, e := range ents {
			if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".cst") {
				return "catsystem2", nil
			}
		}
	}
	// 再深一层：update01 等
	ents, err := os.ReadDir(archiveOrDir)
	if err != nil {
		return "", err
	}
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		sub := filepath.Join(archiveOrDir, e.Name())
		if es, err := os.ReadDir(sub); err == nil {
			for _, x := range es {
				if !x.IsDir() && strings.EqualFold(filepath.Ext(x.Name()), ".cst") {
					return "catsystem2", nil
				}
			}
		}
	}
	return "", nil
}

// ParseArchive 列出归档内的条目。
//   - 目录 → 已解包口径（跳过剧本，剧本单独走 ParseScript）；
//   - .int/.kif 文件 → 走 internal/kif 解出 KIF 索引（索引与数据都能解，
//     不需要游戏主程序口令；未持有口令时名字的**大小写不保证**）。
func (a *Adapter) ParseArchive(path string) ([]adapter.ArchiveEntry, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return parseKIFArchive(path)
	}
	var out []adapter.ArchiveEntry
	err = filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() {
			return nil
		}
		if strings.EqualFold(filepath.Ext(p), ".cst") {
			return nil // 剧本单独走 ParseScript
		}
		if fi, err := d.Info(); err == nil {
			rel, _ := filepath.Rel(path, p)
			out = append(out, adapter.ArchiveEntry{
				Path: filepath.ToSlash(rel), Size: fi.Size(),
				UncompressedSize: fi.Size(),
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// parseKIFArchive 读 CatSystem2 的 KIF 归档（.int/.kif）并列出条目。
// 名字可信度写进 KeyHint，让上层能区分"名字确定"与"大小写/歧义待定"。
func parseKIFArchive(path string) ([]adapter.ArchiveEntry, error) {
	arc, err := kif.Open(path)
	if err != nil {
		return nil, err
	}
	defer arc.Close()
	out := make([]adapter.ArchiveEntry, 0, len(arc.Entries))
	for _, e := range arc.Entries {
		hint := "kif/blowfish-mt"
		if e.NameKind != kif.NameExactKind {
			hint += ";name-" + e.NameKind
		}
		out = append(out, adapter.ArchiveEntry{
			Path: e.Name, Size: e.Size, Offset: e.Offset,
			Compression:      adapter.CompNone,
			UncompressedSize: e.Size,
			Encrypted:        true,
			KeyHint:          hint,
		})
	}
	return out, nil
}

// ---------------------------------------------------------------
// 剧本解析
// ---------------------------------------------------------------

// Command CS2 脚本命令（首词为命令名，其余为参数）。
type Command struct {
	Tag   string            `json:"tag"`
	Attrs map[string]string `json:"attrs,omitempty"` // k=v 参数
	Args  []string          `json:"args,omitempty"`  // 位置参数
	Raw   string            `json:"raw"`
}

// Ast CS2 引擎专有 AST。
type Ast struct {
	Encoding string    `json:"encoding"`
	Commands []Command `json:"commands"`
	Texts    []string  `json:"texts,omitempty"`
	Seq      []SeqItem `json:"-"`
}

// SeqItem 顺序项（命令/对白/角色名）。
type SeqItem struct {
	Kind string   `json:"kind"` // "cmd" | "text" | "name"
	Text string   `json:"text,omitempty"`
	Cmd  *Command `json:"-"`
}

// DecompressCST 解压 .cst（CatScene + u32 压缩长 + zlib）。
func DecompressCST(data []byte) ([]byte, error) {
	if len(data) < 16 || string(data[:8]) != string(cstMagic) {
		return nil, fmt.Errorf("不是 CatScene 文件")
	}
	comp := int(le32(data[8:12]))
	body := data[16:]
	if comp > 0 && comp <= len(body) {
		body = body[:comp]
	}
	zr, err := zlib.NewReader(bytesReader(body))
	if err != nil {
		return nil, fmt.Errorf("cst zlib 失败: %w", err)
	}
	defer zr.Close()
	return io.ReadAll(zr)
}

func le32(b []byte) uint32 {
	return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
}

func bytesReader(b []byte) io.Reader { return &sliceReader{b: b} }

type sliceReader struct {
	b []byte
	i int
}

func (r *sliceReader) Read(p []byte) (int, error) {
	if r.i >= len(r.b) {
		return 0, io.EOF
	}
	n := copy(p, r.b[r.i:])
	r.i += n
	return n, nil
}

// rawEntries 解压后按 00 分隔提取条目：01 <type> <bytes>。
func rawEntries(raw []byte) []([]byte) {
	var out []([]byte)
	start := 0
	for i := 0; i < len(raw); i++ {
		if raw[i] != 0 {
			continue
		}
		run := raw[start:i]
		start = i + 1
		if len(run) < 3 || run[0] != 0x01 {
			continue
		}
		switch run[1] {
		case typeText, typeName, typeCmd:
			out = append(out, run[2:])
		}
	}
	return out
}

// pickEncoding 编码判别（对应 cst_refs.pick_encoding）：
//  1. 整文件 UTF-8 成功率 ≥ 0.9 → utf-8
//  2. 否则 cp932 解码，半角片假名占比 > 平假名占比 → GB18030（被误读特征）
//  3. 其余 cp932
func pickEncoding(payloads [][]byte) string {
	good := 0
	for _, b := range payloads {
		if s, ok := tryUTF8(b); ok && !badChar(s) {
			good++
		}
	}
	if len(payloads) > 0 && float64(good)/float64(len(payloads)) >= 0.9 {
		return "utf-8"
	}
	half, hira, total := 0, 0, 0
	for _, b := range payloads {
		s := decodeCP932(b)
		for _, c := range s {
			o := int(c)
			total++
			switch {
			case o >= 0xFF61 && o <= 0xFF9F:
				half++
			case o >= 0x3040 && o <= 0x309F:
				hira++
			}
		}
	}
	if total > 0 {
		hr, jr := float64(half)/float64(total), float64(hira)/float64(total)
		if hr > 0.02 && hr >= jr {
			return "gb18030"
		}
	}
	return "cp932"
}

func tryUTF8(b []byte) (string, bool) {
	if !utf8.Valid(b) {
		return "", false
	}
	return string(b), true
}

func badChar(s string) bool {
	if strings.ContainsRune(s, '\ufffd') {
		return true
	}
	for _, c := range s {
		if c < 0x20 && c != '\t' && c != '\n' && c != '\r' {
			return true
		}
	}
	return false
}

// ParseScript 解析 .cst 剧本。
func (a *Adapter) ParseScript(data []byte, ctx *adapter.ScriptCtx) (adapter.ScriptAst, error) {
	raw, err := DecompressCST(data)
	if err != nil {
		return nil, err
	}
	ents := rawEntries(raw)
	enc := pickEncoding(ents)
	ast := &Ast{Encoding: enc}
	for _, b := range ents {
		s := decodeBy(enc, b)
		if s == "" {
			continue
		}
		s = unescape(s)
		if badChar(s) {
			continue
		}
		// 命令：首词 + 参数（可能带 k=v）
		if c, ok := parseCommand(s); ok {
			ast.Commands = append(ast.Commands, c)
			ast.Seq = append(ast.Seq, SeqItem{Kind: "cmd", Cmd: &ast.Commands[len(ast.Commands)-1]})
			continue
		}
		// 纯文本：作为对白（角色名由 name 类条目单独记）
		ast.Texts = append(ast.Texts, s)
		ast.Seq = append(ast.Seq, SeqItem{Kind: "text", Text: s})
	}
	return ast, nil
}

var escDec = regexp.MustCompile(`\\\$\s*(\d{1,7})\s*;`)
var escEntDec = regexp.MustCompile(`&#(\d{1,7});`)
var escEntHex = regexp.MustCompile(`&#[xX]([0-9A-Fa-f]{1,6});`)

// unescape 还原 \$<十进制码点>; / &#NNN; / &#xHHHH; 转义（机翻流水线产物）。
func unescape(s string) string {
	s = escDec.ReplaceAllStringFunc(s, func(m string) string {
		sub := escDec.FindStringSubmatch(m)
		if sub == nil {
			return m
		}
		v := atoi(sub[1])
		if v >= 0xD800 && v <= 0xDBFF {
			// 代理对：由调用方整体处理太复杂，此处直接丢（罕见）
			return ""
		}
		if v > 0x10FFFF {
			return m
		}
		return string(rune(v))
	})
	s = escEntDec.ReplaceAllStringFunc(s, func(m string) string {
		sub := escEntDec.FindStringSubmatch(m)
		v := atoi(sub[1])
		if v > 0x10FFFF {
			return m
		}
		return string(rune(v))
	})
	s = escEntHex.ReplaceAllStringFunc(s, func(m string) string {
		sub := escEntHex.FindStringSubmatch(m)
		v := hexToInt(sub[1])
		if v > 0x10FFFF {
			return m
		}
		return string(rune(v))
	})
	return s
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return -1
		}
		n = n*10 + int(c-'0')
	}
	return n
}

func hexToInt(s string) int {
	n := 0
	for _, c := range s {
		var d int
		switch {
		case c >= '0' && c <= '9':
			d = int(c - '0')
		case c >= 'a' && c <= 'f':
			d = int(c-'a') + 10
		case c >= 'A' && c <= 'F':
			d = int(c-'A') + 10
		default:
			return -1
		}
		n = n*16 + d
	}
	return n
}

// kvRe k=v 参数。
var kvRe = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)=(.*)$`)

// parseCommand 解析命令串：首词为命令名；参数先试 k=v，否则位置参数。
func parseCommand(s string) (Command, bool) {
	f := strings.Fields(s)
	if len(f) == 0 {
		return Command{}, false
	}
	// 首词必须是命令名（字母开头）；否则视为对白
	if !isCmdName(f[0]) {
		return Command{}, false
	}
	c := Command{Tag: f[0], Attrs: map[string]string{}, Raw: s}
	for _, tok := range f[1:] {
		if m := kvRe.FindStringSubmatch(tok); m != nil {
			c.Attrs[m[1]] = strings.Trim(m[2], `"'`)
			continue
		}
		c.Args = append(c.Args, tok)
	}
	return c, true
}

func isCmdName(s string) bool {
	if s == "" {
		return false
	}
	c := s[0]
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// ---------------------------------------------------------------
// 引用提取
// ---------------------------------------------------------------

// 资源类属性 + 按命令的位置参数语义（CS2 方言）。
var (
	resourceAttrs = map[string]bool{"storage": true, "file": true, "src": true}
	scriptCmds    = map[string]bool{"call": true, "jump": true, "goto": true}
)

// refNameRe 资源名形态（与 Python 原型 cst_refs.py 的 IMG/PCM/SE/MOVIE
// 正则字符类同口径）：字母/数字/下划线/点/连字符/斜杠，且不以标点开头。
// 用于滤掉 `(70+0)`、`#(950+#300)` 这类表达式噪声。
var refNameRe = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.\-/]*$`)

// layerNameRe 静态图层图片名（对应 cst_refs.IMG_RE 的第三个捕获组）：
// 只接受字母开头的裸名，排除 `63%`、`-80`、`#570+100` 等表达式/数字。
var layerNameRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]*$`)

// layerRef 取静态图层命令的图片名。
//
// CS2 的图层命令是「<命令> <槽位> <图片名> [其余参数]」；当第二个位置参数
// （Args[1]）是 scale/pos/fade/... 时，这是一条对已有图层的变换命令，不是
// 新图片。Python 原型用 IMG_RE 的第三个 token 做同样的事，这里保持同口径。
// 例：`bg 0 bg03_smr_b` → bg03_smr_b；`bg 0 scale 63% 63%` → ""。
func layerRef(c *Command) string {
	if c == nil || len(c.Args) < 2 {
		return ""
	}
	v := c.Args[1]
	if i := strings.IndexByte(v, ','); i >= 0 {
		v = v[:i]
	}
	v = strings.Trim(v, `"'`)
	if v == "" || isAllDigits(v) || isCSCmdWord(v) || strings.HasPrefix(strings.ToLower(v), "m2") {
		return ""
	}
	if !layerNameRe.MatchString(v) {
		return ""
	}
	return v
}

// ExtractRefs 提取资源引用。
//
// 规则（对应 cst_refs.parse_refs_cs2）：
//   - pcm <名>            语音（同时登记去 _<数字> take 后缀变体）
//   - se <buf> <名>       音效（跳过一个缓冲参数）
//   - bg/cg/fg/eg <槽位> <名>  静态图层图片（只取第三个 token；变换命令不取）
//   - mface name=         差分（属性）
//   - movie <名> / evset file= / fesds ... 按需
//   - call <子剧本>       非资产（DR-25 剧本全留）
//
// fw/mpl 的 E-mote 数据（.psz）按硬边界不复刻，不登记为资源；pl 是粒子控制。
func (ast *Ast) ExtractRefs() []string {
	var refs []string
	add := func(v string) {
		v = strings.Trim(v, `"'`)
		if v == "" || isAllDigits(v) || strings.HasPrefix(v, "%") || strings.HasPrefix(v, "&") || strings.HasPrefix(v, "$") {
			return
		}
		v = strings.ReplaceAll(v, "\\", "/")
		if !refNameRe.MatchString(v) {
			return // 表达式噪声（括号/井号运算等），非资源名
		}
		refs = append(refs, v)
	}
	for _, c := range ast.Commands {
		tag := strings.ToLower(c.Tag)
		switch tag {
		case "pcm", "pcmch":
			if len(c.Args) >= 1 {
				v := c.Args[0]
				// `pcm 0 end 90` 是控制命令（停止/淡出），不是语音名。
				if !isAllDigits(v) && !isCSCmdWord(v) {
					add(v)
					// take 后缀变体：A_19_sayuki_ex01_022_1 → A_19_sayuki_ex01_022
					if i := strings.LastIndex(v, "_"); i > 0 && isAllDigits(v[i+1:]) {
						add(v[:i])
					}
				}
			}
		case "se":
			// se <buf> <名>：第一个位置参数是缓冲号（数字），取其后
			for _, a := range c.Args {
				if isAllDigits(a) || isCSCmdWord(a) {
					continue
				}
				add(a)
				break
			}
			for k, v := range c.Attrs {
				if resourceAttrs[k] {
					add(v)
				}
			}
		case "bg", "cg", "fg", "eg":
			if v := layerRef(&c); v != "" {
				add(v)
			}
			for k, v := range c.Attrs {
				if resourceAttrs[k] {
					add(v)
				}
			}
		case "bgm", "movie", "sysmovie":
			for _, a := range c.Args {
				if isAllDigits(a) || isCSCmdWord(a) {
					continue // 槽位/缓冲号/end/vol 等控制词
				}
				add(a)
				break
			}
		case "mface", "face":
			if v := c.Attrs["name"]; v != "" {
				add(v)
			}
			for _, a := range c.Args {
				if !isCSCmdWord(a) {
					add(a)
				}
			}
		case "evset", "ev", "image":
			for k, v := range c.Attrs {
				if resourceAttrs[k] {
					add(v)
				}
			}
			for _, a := range c.Args {
				if !isAllDigits(a) && !isCSCmdWord(a) {
					add(a)
				}
			}
		default:
			// 其它命令：属性里的资源引用照收（storage/file/src）
			for k, v := range c.Attrs {
				if resourceAttrs[k] {
					add(v)
				}
			}
		}
	}
	return refs
}

// isCSCmdWord CS2 命令里的非资源关键字。
func isCSCmdWord(s string) bool {
	switch strings.ToLower(s) {
	case "blend", "pos", "blur", "tone", "stop", "mode", "attr_def", "disp",
		"scale", "fade", "arc", "wait", "quake", "anti", "clear", "copy",
		"effect", "hide", "show", "sync", "loop", "normal", "grayscale",
		"msgoff", "resetcolor", "time", "left", "right", "center", "true", "false",
		"end", "vol", "m2fade", "m2move", "m2amove", "m2amove1", "m2amove3", "pri":
		return true
	}
	return false
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------
// 编码辅助（Windows CP932/GB18030）
// ---------------------------------------------------------------

func decodeBy(enc string, b []byte) string {
	switch enc {
	case "gb18030":
		return decodeMB(b, 54936) // GB18030
	default:
		return decodeMB(b, 932) // CP932
	}
}

func decodeCP932(b []byte) string { return decodeMB(b, 932) }

// decodeMB 用 Windows MultiByteToWideChar 解码（码页 932/54936），
// 坏字节替换为 U+FFFD。非 Windows 退化为原样返回。
func decodeMB(b []byte, cp int) string {
	if len(b) == 0 {
		return ""
	}
	s, err := multiByteToWide(b, uint32(cp))
	if err != nil {
		return string(b)
	}
	return s
}

func multiByteToWide(b []byte, cp uint32) (string, error) {
	return mbToWide(b, cp)
}

// ---------------------------------------------------------------
// BuildIR（P1）：单事件骨架，事件切分沿用 KiriKiri 的思路后续补
// ---------------------------------------------------------------

// BuildIR CS2 → IR（v1：整脚本一个事件；图层/时间轴/台词按命令流构建）。
func (a *Adapter) BuildIR(astObj adapter.ScriptAst, ctx *adapter.BuildCtx) (*ir.IR, error) {
	ast, ok := astObj.(*Ast)
	if !ok {
		return nil, fmt.Errorf("AST 类型不匹配: %T", astObj)
	}
	if ctx == nil {
		ctx = &adapter.BuildCtx{}
	}
	resolve := ctx.Resolver
	if resolve == nil {
		resolve = func(ref string) (*ir.AssetRef, bool) {
			return &ir.AssetRef{OrigPath: ref, Role: "其它"}, true
		}
	}
	ev := &ir.Event{EventID: "script", Kind: ir.KindScene, Ord: 1}
	layers := map[string]*ir.Layer{}
	states := map[string]*layerState{}
	var order []string
	var clock int64
	var assets []ir.AssetRef
	seen := map[string]bool{}
	add := func(ref string) *ir.AssetRef {
		ar, ok := resolve(ref)
		if !ok {
			return nil
		}
		if !seen[ar.OrigPath] {
			seen[ar.OrigPath] = true
			assets = append(assets, *ar)
		}
		return ar
	}
	for _, it := range ast.Seq {
		if it.Kind != "cmd" {
			if it.Kind == "text" {
				ev.Texts = append(ev.Texts, ir.TextLine{
					LineID: fmt.Sprintf("L%03d", len(ev.Texts)+1), TMs: clock, Text: it.Text,
				})
			}
			continue
		}
		c := it.Cmd
		tag := strings.ToLower(c.Tag)
		switch tag {
		case "bg", "cg", "fg", "eg":
			// 图层命令有两种形态：
			//   bg 0 bg03_smr_b             换图（第三 token 是图片名）
			//   bg 0 scale 63% 63% / pos …  对已有槽位做变换（不是新资源）
			// 变换必须按槽位累计，并在 image_show / layer_set 里带给回放器；
			// 否则 2560×1440 的原图会按 100% 画到 1600×900 舞台上。
			ref := layerRef(c)
			lid := layerID(tag, c)
			kind := ir.LayerCG
			if tag == "bg" {
				kind = ir.LayerBackground
			} else if tag == "fg" || tag == "eg" {
				kind = ir.LayerStand
			}
			l, ex := layers[lid]
			if !ex {
				// 事件初态为空：真正的画面由 t=0 之后的 image_show 给出。
				// 若沿用「最后一张图」做初态，脚本中途才出现的槽位会在开场
				// 就露出来（CS2 一个脚本只切一个事件，这种错误尤其明显）。
				l = &ir.Layer{LayerID: lid, Kind: kind, Z: len(order) + 1, Visible: false}
				layers[lid] = l
				order = append(order, lid)
			}
			st := states[lid]
			if st == nil {
				st = &layerState{scale: 1, opacity: 1}
				states[lid] = st
			}
			if ref != "" {
				ar := add(ref)
				if ar != nil {
					l.Asset = ar
				}
				params := st.params()
				params["ref"] = ref
				ev.Timeline = append(ev.Timeline, ir.TimelineOp{
					TMs: clock, Op: "image_show", Target: lid, Params: params,
				})
				continue
			}
			delta, reason := applyLayerTransform(c, st)
			if len(delta) > 0 {
				op := "layer_set"
				if _, ok := delta["duration_ms"]; ok {
					op = "layer_fade"
				}
				ev.Timeline = append(ev.Timeline, ir.TimelineOp{
					TMs: clock, Op: op, Target: lid, Params: delta,
				})
				continue
			}
			if reason != "" {
				ev.Unrecognized = append(ev.Unrecognized, ir.Unrecognized{
					NodePath: "cmd[" + tag + "]", Raw: c.Raw, Reason: reason,
				})
			}
		case "fw", "mpl", "chr":
			// E-mote/动态立绘：需要 .psz + .pfl/.dif/.inc 才能还原。
			// 按 docs 的硬边界不复刻；不要把 HG3 部件当静态立绘。
			ev.HasEmote = true
			ev.Unrecognized = append(ev.Unrecognized, ir.Unrecognized{
				NodePath: "cmd[" + tag + "]", Raw: c.Raw,
				Reason: "E-mote/动态立绘（不复刻）",
			})
		case "mface", "ev", "image", "evset":
			ref := firstRef(c)
			if ref == "" {
				continue
			}
			ar := add(ref)
			kind := ir.LayerCG
			lid := tag
			if v := c.Attrs["id"]; v != "" {
				lid = tag + v
			}
			l, ex := layers[lid]
			if !ex {
				l = &ir.Layer{LayerID: lid, Kind: kind, Z: len(order) + 1, Visible: false}
				layers[lid] = l
				order = append(order, lid)
			}
			if ar != nil {
				l.Asset = ar
			}
			ev.Timeline = append(ev.Timeline, ir.TimelineOp{
				TMs: clock, Op: "image_show", Target: lid,
				Params: map[string]interface{}{"ref": ref},
			})
		case "wait":
			clock += 800
		case "pcm", "pcmch":
			if len(c.Args) >= 1 {
				if ar := add(c.Args[0]); ar != nil && len(ev.Texts) > 0 {
					i := len(ev.Texts) - 1
					ev.Texts[i].Voice = ar
					ev.Voices = append(ev.Voices, ir.Voice{LineID: ev.Texts[i].LineID, Asset: *ar})
				}
			}
		case "se", "bgm", "movie", "sysmovie":
			for _, a := range c.Args {
				if !isAllDigits(a) && !isCSCmdWord(a) {
					add(a)
					break
				}
			}
		default:
			ev.Unrecognized = append(ev.Unrecognized, ir.Unrecognized{
				NodePath: "cmd[" + tag + "]", Raw: c.Raw, Reason: "未实现的命令语义",
			})
		}
	}
	for _, lid := range order {
		ev.Layers = append(ev.Layers, *layers[lid])
	}
	ev.Assets = assets
	canvasW, canvasH := 1600, 900 // 甜蜜女友2+ m2config/m2view.cnf #wndsize
	if v, err := strconv.Atoi(ctx.Config["canvas_w"]); err == nil && v > 0 {
		canvasW = v
	}
	if v, err := strconv.Atoi(ctx.Config["canvas_h"]); err == nil && v > 0 {
		canvasH = v
	}
	out := &ir.IR{
		IRVersion: ir.IRVersion, GameID: ctx.GameID, EngineID: "catsystem2",
		Fidelity: ir.FidelityGeneric,
		CanvasW:  canvasW, CanvasH: canvasH,
		HasEmote: ev.HasEmote,
		Meta:     ir.Meta{ScriptEncoding: ast.Encoding, GeneratedBy: "ucgr/catsystem2.BuildIR v2"},
		Events:   []ir.Event{*ev},
	}
	if n := ctx.Config["script_name"]; n != "" {
		out.Events[0].EventID = strings.TrimSuffix(filepath.Base(n), filepath.Ext(n))
	}
	return out, nil
}

// layerState 是 CS2 单个槽位的累计变换。CS2 的 scale/pos/blend 可以出现在
// 换图之前，也可以出现在换图之后；只在命令流里按槽位累计，才能把
// `bg 0 scale 63%` -> `bg 0 bg11_smr_b` -> `bg 0 pos -80 -410`
// 还原成「图片以 63% 缩放并偏移」而不是三条互不相关的命令。
type layerState struct {
	x, y           float64
	hasX, hasY     bool
	scale, opacity float64
}

func (st *layerState) params() map[string]interface{} {
	p := map[string]interface{}{}
	if st.hasX {
		p["left"] = st.x
	}
	if st.hasY {
		p["top"] = st.y
	}
	if st.scale != 1 {
		p["scale"] = st.scale
	}
	if st.opacity != 1 {
		p["opacity"] = st.opacity
	}
	return p
}

func layerID(tag string, c *Command) string {
	lid := tag
	if len(c.Args) > 0 && isAllDigits(c.Args[0]) {
		lid = tag + c.Args[0]
	}
	if v := c.Attrs["id"]; v != "" {
		lid = tag + v
	}
	return lid
}

// applyLayerTransform 解析 CS2 的槽位属性命令，返回需要写进 layer_set 的
// 增量。解析不了（变量表达式/颜色命令等）时返回 reason，交给 unrecognized。
func applyLayerTransform(c *Command, st *layerState) (map[string]interface{}, string) {
	if c == nil || len(c.Args) < 2 {
		return nil, ""
	}
	key := strings.ToLower(c.Args[1])
	args := c.Args[2:]
	switch key {
	case "scale":
		if len(args) == 0 {
			return nil, "scale 缺参数"
		}
		v, ok := parseScale(args[0])
		if !ok || v <= 0 {
			return nil, "scale 表达式不可解析"
		}
		st.scale = v
		return map[string]interface{}{"scale": v}, ""
	case "pos":
		if len(args) == 0 {
			return nil, "pos 缺参数"
		}
		x, ok := parseCoord(args[0])
		if !ok {
			return nil, "pos 表达式不可解析"
		}
		st.x, st.hasX = x, true
		delta := map[string]interface{}{"left": x}
		if len(args) >= 2 {
			y, ok := parseCoord(args[1])
			if !ok {
				return nil, "pos 表达式不可解析"
			}
			st.y, st.hasY = y, true
			delta["top"] = y
		}
		return delta, ""
	case "blend":
		if len(args) == 0 {
			return nil, "blend 缺参数"
		}
		v, ok := parseAlpha(args[0])
		if !ok {
			return nil, "blend 表达式不可解析"
		}
		st.opacity = v
		return map[string]interface{}{"opacity": v}, ""
	case "fade":
		// fade <帧数> <起始 alpha> <目标 alpha>；引擎按 1 帧 = 16ms 推进，
		// 并用 easeInOutQuad 插值（FelineSystem2 Image::fade）。
		if len(args) < 3 {
			return nil, ""
		}
		frames, err := strconv.Atoi(strings.TrimSpace(args[0]))
		if err != nil || frames < 0 {
			return nil, "fade 帧数不可解析"
		}
		from, ok := parseAlpha(args[1])
		if !ok {
			return nil, "fade 起始透明度不可解析"
		}
		to, ok := parseAlpha(args[2])
		if !ok {
			return nil, "fade 目标透明度不可解析"
		}
		st.opacity = to
		return map[string]interface{}{
			"from": from, "to": to, "opacity": to,
			"duration_ms": frames * 16,
		}, ""
	case "tone", "mode", "blur":
		return nil, "CS2 视觉属性（静态回放未映射）"
	}
	return nil, ""
}

func parseScale(s string) (float64, bool) {
	s = strings.Trim(strings.TrimSpace(s), `"'`)
	if strings.HasSuffix(s, "%") {
		f, err := strconv.ParseFloat(strings.TrimSuffix(s, "%"), 64)
		if err != nil || f <= 0 {
			return 0, false
		}
		return f / 100, true
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f <= 0 {
		return 0, false
	}
	if f > 10 { // 兼容不带 % 的 63 这种写法
		f /= 100
	}
	return f, true
}

func parseAlpha(s string) (float64, bool) {
	s = strings.Trim(strings.TrimSpace(s), `"'`)
	percent := strings.HasSuffix(s, "%")
	if percent {
		s = strings.TrimSuffix(s, "%")
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f < 0 {
		return 0, false
	}
	if percent {
		f /= 100
	} else if f > 1 {
		f /= 255
	}
	if f > 1 {
		f = 1
	}
	return f, true
}

func parseCoord(s string) (float64, bool) {
	s = strings.Trim(strings.TrimSpace(s), `"'`)
	if s == "" || strings.ContainsAny(s, "#@()[]$%") {
		return 0, false
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return f, true
}

// firstRef 命令的第一个资源引用（属性优先，其次位置参数）。
func firstRef(c *Command) string {
	for _, k := range []string{"storage", "file", "src", "name"} {
		if v := c.Attrs[k]; v != "" {
			return v
		}
	}
	for _, k := range []string{"c", "l", "r", "ll", "rr"} {
		if v := c.Attrs[k]; v != "" {
			return v
		}
	}
	for _, a := range c.Args {
		if !isAllDigits(a) && !isCSCmdWord(a) {
			return a
		}
	}
	return ""
}

// Transitions CS2 转场注册表（P0 初版）。
func (a *Adapter) Transitions() *adapter.TransitionRegistry {
	r := adapter.NewTransitionRegistry()
	r.Register("crossfade", adapter.TransitionImpl{ImplKey: "fade", DurationMs: 800})
	r.Register("fade", adapter.TransitionImpl{ImplKey: "fade", DurationMs: 800})
	r.Register("blind", adapter.TransitionImpl{ImplKey: "wipe", DurationMs: 800})
	return r
}
