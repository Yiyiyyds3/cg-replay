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
// 归档 .int（内部 KIF，Blowfish+MT 加密，口令在 exe PE 资源）的
// 解密未实现——本适配器面向【已解包目录】（与 Python 原型同口径）。
package catsystem2

import (
	"compress/zlib"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"ucgr/internal/adapter"
	"ucgr/internal/ir"
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

// ParseArchive 列出目录内的条目（已解包口径）。
// .int 归档解密未实现（需 exe 口令），直接报错。
func (a *Adapter) ParseArchive(path string) ([]adapter.ArchiveEntry, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("CatSystem2 .int(KIF) 归档解密未实现（口令在 exe PE 资源），"+
			"请先解包再摄入: %s", path)
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

// ExtractRefs 提取资源引用。
//
// 规则（对应 cst_refs.parse_refs_cs2）：
//   - pcm <名>            语音（同时登记去 _<数字> take 后缀变体）
//   - se <buf> <名>       音效（跳过一个缓冲参数）
//   - bg <槽位> <名>      背景（跳过槽位参数）
//   - chr c/l/r/ll/rr=   立绘（属性）
//   - mface name=         差分（属性）
//   - movie <名> / evset file= / fesds ... 按需
//   - call <子剧本>       非资产（DR-25 剧本全留）
func (ast *Ast) ExtractRefs() []string {
	var refs []string
	add := func(v string) {
		v = strings.Trim(v, `"'`)
		if v == "" || strings.HasPrefix(v, "%") || strings.HasPrefix(v, "&") || strings.HasPrefix(v, "$") {
			return
		}
		v = strings.ReplaceAll(v, "\\", "/")
		if !refNameRe.MatchString(v) {
			return // 表达式噪声（括号/井号运算等），非资源名
		}
		refs = append(refs, v)
	}
	for _, c := range ast.Commands {
		switch strings.ToLower(c.Tag) {
		case "pcm", "pcmch":
			if len(c.Args) >= 1 {
				v := c.Args[0]
				add(v)
				// take 后缀变体：A_19_sayuki_ex01_022_1 → A_19_sayuki_ex01_022
				if i := strings.LastIndex(v, "_"); i > 0 && isAllDigits(v[i+1:]) {
					add(v[:i])
				}
			}
		case "se":
			// se <buf> <名>：第一个位置参数是缓冲号（数字），取其后
			for _, a := range c.Args {
				if isAllDigits(a) {
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
		case "bg", "bgm", "movie", "sysmovie":
			for _, a := range c.Args {
				if isAllDigits(a) {
					continue // 槽位/缓冲号
				}
				add(a)
				break
			}
		case "chr", "fg", "eg", "fw", "pl", "mpl":
			for _, k := range []string{"c", "l", "r", "ll", "rr"} {
				if v := c.Attrs[k]; v != "" {
					add(v)
				}
			}
			for _, a := range c.Args {
				if !isAllDigits(a) && !isCSCmdWord(a) {
					add(a)
				}
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
		case "evset", "ev", "cg", "image":
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
	case "blend", "pos", "tone", "stop", "mode", "attr_def", "disp", "scale",
		"fade", "hide", "show", "sync", "wait", "loop", "normal", "grayscale",
		"msgoff", "resetcolor", "time", "left", "right", "center", "true", "false":
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
		case "bg", "chr", "fg", "eg", "fw", "mface", "ev", "cg", "image", "evset":
			ref := firstRef(c)
			if ref == "" {
				continue
			}
			ar := add(ref)
			kind := ir.LayerCG
			if tag == "bg" {
				kind = ir.LayerBackground
			} else if tag == "chr" || tag == "fg" || tag == "eg" || tag == "fw" {
				kind = ir.LayerStand
			}
			lid := tag
			if v := c.Attrs["id"]; v != "" {
				lid = tag + v
			}
			l, ex := layers[lid]
			if !ex {
				l = &ir.Layer{LayerID: lid, Kind: kind, Z: len(order) + 1, Visible: true}
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
				if !isAllDigits(a) {
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
	out := &ir.IR{
		IRVersion: ir.IRVersion, GameID: ctx.GameID, EngineID: "catsystem2",
		Fidelity: ir.FidelityGeneric,
		Meta:     ir.Meta{ScriptEncoding: ast.Encoding, GeneratedBy: "ucgr/catsystem2.BuildIR v1"},
		Events:   []ir.Event{*ev},
	}
	if n := ctx.Config["script_name"]; n != "" {
		out.Events[0].EventID = strings.TrimSuffix(filepath.Base(n), filepath.Ext(n))
	}
	return out, nil
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
