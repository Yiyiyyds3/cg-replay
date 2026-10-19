// build_ir.go：Artemis `.ast` → IR。
//
// 事件粒度：一个 `block` = 一个事件 —— block 是引擎自己的分块单位，`linkback`/`linknext`
// 定义串接顺序，`label` 表给出命名标签（类似 KiriKiri 的 `*label`）。EventID =
// `<脚本名>#<block 名>`；标题取块内 `savetitle` 的 `text=`（ast.go 已填 `Block.Title`）。
// 块内含 `evset`（事件 CG）的标 `kind=still_cg`，其余 `scene`。
//
// 时间轴（启发式，写进注释以免后人误以为精确）：
//   - `ex func="wait" time=N` → clock += N
//   - 图像命令 `sync` 缺省或非 "0"（即同步）→ clock += time=
//   - `extrans time=N` → 发一条转场并 clock += N
//   - `msgoff time=N` → clock += N
//
// 图像命令发 `image_show`（`cgdel` 发 `image_hide`）；文本行绑 `text.vo` 的语音；
// 未知命令按 P1-7 保守保留：进 `unrecognized`，其操作数里能解析的引用一律入 `assets`。
package artemis

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"ucgr/internal/adapter"
	"ucgr/internal/ir"
)

// imageTags 会发 image_show 的命令。
var imageTags = map[string]bool{
	"bg": true, "fg": true, "fgf": true, "fgact": true,
	"evset": true, "cgact": true, "scene": true, "movie": true,
}

// noopTags 已知语义但 v1 不发时间轴 op 的命令（仍会收取其引用）。
var noopTags = map[string]bool{
	"savetitle": true, "msg": true, "msgon": true, "text": true,
	"vo": true, "vostop": true, "bgm": true, "se": true, "selback": true,
	"quake": true, "flash": true, "colortone": true, "eval": true,
	"ruby": true, "txruby": true, "txkey": true, "gaiji": true,
	"heroname": true, "autoplay": true, "skipstop": true, "keyskip": true,
	"cacheclear": true, "ntclear": true, "brandlogo": true, "staffroll": true,
	"user": true, "exfont": true, "line": true, "every": true, "sysvo": true,
}

// layerKindFor 把 Artemis 图像命令映射到 IR 图层种类。
// `path=` 别名是引擎自带的分类提示（`:bg/`、`:cg/`、`:fg/xx/z1/`、`:ani/`、`:sys/`），
// 优先按它判定（P2 报告口径）；别名缺失时退回标签本身。
func layerKindFor(tag string, c *Command) ir.LayerKind {
	if c != nil {
		p := strings.ToLower(strings.TrimSpace(c.Attrs["path"]))
		switch {
		case strings.HasPrefix(p, ":bg"):
			return ir.LayerBackground
		case strings.HasPrefix(p, ":fg"):
			return ir.LayerStand
		case strings.HasPrefix(p, ":cg"), strings.HasPrefix(p, ":ani"):
			return ir.LayerCG
		}
	}
	switch tag {
	case "bg":
		return ir.LayerBackground
	case "fg", "fgf", "fgact":
		return ir.LayerStand
	case "movie":
		return ir.LayerMovie
	default: // evset / cgact / scene 以及未知图像命令
		return ir.LayerCG
	}
}

// layerIDFor 生成稳定图层 id：优先 `tag+id`，其次 `tag+ch`，否则 `tag`。
func layerIDFor(tag string, c *Command) string {
	if c != nil {
		if v := c.Attrs["id"]; v != "" {
			return tag + v
		}
		if v := c.Attrs["ch"]; v != "" {
			return tag + v
		}
	}
	return tag
}

// artemisTime 读 `time=`（毫秒）；缺失或非数字时返回 def。
func artemisTime(c *Command, def int64) int64 {
	if c == nil {
		return def
	}
	v := strings.TrimSpace(c.Attrs["time"])
	if v == "" {
		return def
	}
	var n int64
	for i := 0; i < len(v); i++ {
		if v[i] < '0' || v[i] > '9' {
			return def
		}
		n = n*10 + int64(v[i]-'0')
	}
	return n
}

// syncBlocking 报告图像命令是否阻塞（sync 缺省视为阻塞）。
func syncBlocking(c *Command) bool {
	if c == nil {
		return false
	}
	v := strings.TrimSpace(c.Attrs["sync"])
	return v != "0"
}

// transitionImpl 把 Artemis 的 `rule=` 映射到 L0 通用键；第二返回值表示是否回退。
func transitionImpl(rule string) (string, bool) {
	r := strings.ToLower(strings.TrimSpace(rule))
	switch {
	case r == "":
		return "fade", true
	case strings.Contains(r, "fade"), strings.Contains(r, "cross"):
		return "fade", false
	case strings.Contains(r, "wipe"), strings.Contains(r, "blind"), strings.Contains(r, "mask"):
		return "wipe", false
	case strings.Contains(r, "slide"), strings.Contains(r, "scroll"), strings.Contains(r, "move"):
		return "slide_left", false
	case strings.Contains(r, "zoom"), strings.Contains(r, "scale"):
		return "zoom", false
	default:
		return "fade", true
	}
}

// commandRefs 返回命令携带的候选引用（按 refAttrs 口径取属性，已剥 `:别名/`）。
func commandRefs(c *Command) []string {
	if c == nil {
		return nil
	}
	keys, registered := refAttrs[strings.ToLower(c.Tag)]
	if !registered {
		keys = refFallbackAttrs // §6：未登记命令也要保守保留路径型操作数
	}
	var out []string
	for _, k := range keys {
		if v := c.Attrs[k]; v != "" {
			out = append(out, splitRefValue(v)...)
		}
	}
	if len(out) == 0 {
		for _, a := range c.Args {
			out = append(out, splitRefValue(a)...)
		}
	}
	for i := range out {
		out[i] = cleanRef(out[i])
	}
	return out
}

// firstCommandRef 返回命令的首个引用。
func firstCommandRef(c *Command) string {
	refs := commandRefs(c)
	if len(refs) == 0 {
		return ""
	}
	return refs[0]
}

// paramMap 收集命令里值得原样保留的属性（供 IR params 使用）。
func paramMap(c *Command) map[string]interface{} {
	if c == nil || len(c.Attrs) == 0 {
		return nil
	}
	out := map[string]interface{}{}
	for _, k := range []string{"rule", "vague", "time", "path", "set", "id", "lv", "ch", "mode", "stage", "act", "face"} {
		if v := c.Attrs[k]; v != "" {
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// eventBuilder 累积一个 block 的事件。
type eventBuilder struct {
	base    string
	ev      *ir.Event
	resolve func(string) (*ir.AssetRef, bool)

	assets []ir.AssetRef
	seen   map[string]bool
	layers map[string]*ir.Layer
	order  []string
	chars  map[string]bool
	clock  int64
	transN int
}

func (b *eventBuilder) add(ref string) *ir.AssetRef {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil
	}
	ar, ok := b.resolve(ref)
	if !ok || ar == nil {
		return nil
	}
	if ar.OrigPath == "" {
		ar.OrigPath = ref
	}
	if !b.seen[ar.OrigPath] {
		b.seen[ar.OrigPath] = true
		b.assets = append(b.assets, *ar)
	}
	return ar
}

func (b *eventBuilder) layer(lid string, kind ir.LayerKind) *ir.Layer {
	l, ok := b.layers[lid]
	if !ok {
		l = &ir.Layer{LayerID: lid, Kind: kind, Z: len(b.order) + 1, Visible: true}
		b.layers[lid] = l
		b.order = append(b.order, lid)
	}
	return l
}

func (b *eventBuilder) transition(impl string, ms int64, fallback bool, params map[string]interface{}) {
	if ms < 0 {
		ms = 0
	}
	b.transN++
	b.ev.Transitions = append(b.ev.Transitions, ir.Transition{
		TransitionID: fmt.Sprintf("T%02d", b.transN),
		Impl:         impl,
		DurationMs:   ms,
		Params:       params,
		Fallback:     fallback,
	})
}

func (b *eventBuilder) unrecognized(c *Command, reason string) {
	raw := ""
	if c != nil {
		raw = c.Raw
	}
	u := ir.Unrecognized{
		NodePath: "block[" + blockName(b.ev.EventID) + "]/cmd[" + strings.ToLower(tagOf(c)) + "]",
		Raw:      raw,
		Reason:   reason,
	}
	for _, ref := range commandRefs(c) {
		if ar := b.add(ref); ar != nil && ar.Hash != "" {
			u.KeptAssets = append(u.KeptAssets, ar.Hash)
		}
	}
	b.ev.Unrecognized = append(b.ev.Unrecognized, u)
}

func tagOf(c *Command) string {
	if c == nil {
		return ""
	}
	return c.Tag
}

func blockName(eventID string) string {
	if i := strings.IndexByte(eventID, '#'); i >= 0 {
		return eventID[i+1:]
	}
	return eventID
}

// buildBlockEvent 把一个 block 译成 IR 事件。
func buildBlockEvent(base string, blk *Block, ord int, resolve func(string) (*ir.AssetRef, bool)) *ir.Event {
	kind := ir.KindScene
	for _, it := range blk.Seq {
		if it.Kind == "cmd" && it.Cmd != nil && strings.EqualFold(it.Cmd.Tag, "evset") {
			kind = ir.KindStillCG
			break
		}
	}
	ev := &ir.Event{
		EventID: base + "#" + blk.Name,
		Kind:    kind,
		Title:   blk.Title,
		Ord:     ord,
	}
	b := &eventBuilder{
		base:    base,
		ev:      ev,
		resolve: resolve,
		seen:    map[string]bool{},
		layers:  map[string]*ir.Layer{},
		chars:   map[string]bool{},
	}

	for _, it := range blk.Seq {
		switch it.Kind {
		case "text":
			line := ir.TextLine{
				LineID:  fmt.Sprintf("L%03d", len(ev.Texts)+1),
				TMs:     b.clock,
				Speaker: it.Speaker,
				Text:    it.Text,
			}
			if it.Speaker != "" {
				b.chars[it.Speaker] = true
			}
			if ar := b.add(it.Voice); ar != nil {
				line.Voice = ar
				ev.Voices = append(ev.Voices, ir.Voice{LineID: line.LineID, Asset: *ar})
			}
			ev.Texts = append(ev.Texts, line)
		case "cmd":
			b.command(it.Cmd)
		}
	}

	for _, lid := range b.order {
		ev.Layers = append(ev.Layers, *b.layers[lid])
	}
	ev.Assets = b.assets
	ev.EndMs = b.clock
	if len(b.chars) > 0 {
		names := make([]string, 0, len(b.chars))
		for n := range b.chars {
			names = append(names, n)
		}
		sort.Strings(names)
		ev.Characters = names
	}
	return ev
}

func (b *eventBuilder) command(c *Command) {
	if c == nil {
		return
	}
	tag := strings.ToLower(c.Tag)
	switch {
	case imageTags[tag]:
		ref := firstCommandRef(c)
		ar := b.add(ref)
		lid := layerIDFor(tag, c)
		l := b.layer(lid, layerKindFor(tag, c))
		if ar != nil {
			l.Asset = ar
		}
		params := paramMap(c)
		if params == nil {
			params = map[string]interface{}{}
		}
		if ref != "" {
			params["ref"] = ref
		}
		b.ev.Timeline = append(b.ev.Timeline, ir.TimelineOp{
			TMs: b.clock, Op: "image_show", Target: lid, Params: params,
		})
		if ms := artemisTime(c, 0); ms > 0 {
			impl, fb := transitionImpl("")
			b.transition(impl, ms, fb, params)
			if syncBlocking(c) {
				b.clock += ms
			}
		}
	case tag == "cgdel":
		lid := layerIDFor("cg", c)
		b.ev.Timeline = append(b.ev.Timeline, ir.TimelineOp{TMs: b.clock, Op: "image_hide", Target: lid})
		ms := artemisTime(c, 0)
		if ms > 0 {
			impl, fb := transitionImpl(c.Attrs["rule"])
			b.transition(impl, ms, fb, paramMap(c))
			if syncBlocking(c) {
				b.clock += ms
			}
		}
		b.add(firstCommandRef(c))
	case tag == "ex":
		fn := strings.ToLower(strings.TrimSpace(c.Attrs["func"]))
		if fn == "wait" || fn == "" {
			if ms := artemisTime(c, 500); ms > 0 {
				b.clock += ms
			}
		}
	case tag == "extrans":
		ms := artemisTime(c, 800)
		impl, fb := transitionImpl(c.Attrs["rule"])
		b.transition(impl, ms, fb, paramMap(c))
		b.clock += ms
	case tag == "msgoff":
		if ms := artemisTime(c, 0); ms > 0 {
			b.clock += ms
		}
	case tag == "select":
		opts, def := selectOptions(c)
		if len(opts) == 0 {
			b.unrecognized(c, "未实现的命令语义")
			return
		}
		b.ev.Branches = append(b.ev.Branches, ir.Branch{
			BranchID:      fmt.Sprintf("B%02d", len(b.ev.Branches)+1),
			TMs:           b.clock,
			Type:          "choice",
			Options:       opts,
			DefaultTarget: def,
		})
	case noopTags[tag]:
		for _, ref := range commandRefs(c) {
			b.add(ref)
		}
	default:
		b.unrecognized(c, "未实现的命令语义")
	}
}

// selectOptions 从 `select` 的 label=/text= 生成分支选项（缺一不可，否则返回空）。
func selectOptions(c *Command) ([]ir.BranchOpt, string) {
	labels := splitList(c.Attrs["label"])
	texts := splitList(c.Attrs["text"])
	if len(labels) == 0 {
		return nil, ""
	}
	opts := make([]ir.BranchOpt, 0, len(labels))
	for i, l := range labels {
		t := ""
		if i < len(texts) {
			t = texts[i]
		}
		opts = append(opts, ir.BranchOpt{Text: t, TargetEvent: l})
	}
	return opts, labels[0]
}

// splitList 按逗号拆列表（Artemis 的 select 用逗号分隔 label/text）。
func splitList(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// BuildIR 实现 adapter.EngineAdapter。
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

	base := strings.TrimSuffix(filepath.Base(ast.Name), filepath.Ext(ast.Name))
	if n := ctx.Config["script_name"]; n != "" {
		base = strings.TrimSuffix(filepath.Base(n), filepath.Ext(n))
	}
	if base == "" || base == "." {
		base = "script"
	}

	out := &ir.IR{
		IRVersion: ir.IRVersion,
		GameID:    ctx.GameID,
		EngineID:  "artemis",
		Fidelity:  ir.FidelityGeneric,
		Meta: ir.Meta{
			ScriptEncoding: ast.Encoding,
			GeneratedBy:    "ucgr/artemis.BuildIR v1",
		},
	}
	for i, blk := range ast.Blocks {
		out.Events = append(out.Events, *buildBlockEvent(base, blk, i+1, resolve))
	}
	if len(out.Events) == 0 {
		return nil, fmt.Errorf("artemis: %s 里没有 block", base)
	}
	return out, nil
}
