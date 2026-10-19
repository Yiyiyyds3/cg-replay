// build_ir.go —— 把 .bs 脚本转成 IR。
//
// BGI 脚本没有显式事件边界，v1 的切分口径是「场景切换」：
// 当前事件已经有台词，又出现新的背景图（0x0280 一类）时开新事件。
// 事件内：
//   - 图像命令 → 图层（背景 / 立绘 / CG）+ Timeline 的 image_show + Show 的 layer_show；
//   - 消息命令（0x0140 / 0x0143）→ TextLine：最后一条压栈串是正文，前一条是说话人；
//   - 语音 / 音效 / BGM → 资产 + Show 的 voice_play / se_play / bgm_play；语音绑最近一条台词；
//   - 选项屏（0x0160 / _SelectEx 调用）→ Branch（回放是重演，默认走第一条）；
//   - 其余命令 → 每个 opcode 在本事件聚合成一条 Unrecognized（照实登记，不解释语义）。
//
// 时间只能近似（§9）：BGI 的推进靠玩家点击，脚本里没有时钟，这里按
// 「每条指令 +40ms、每条台词 +600ms」给一个单调的近似值；
// 播放器按 Show 的顺序执行，不依赖这些数字。
package bgi

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"ucgr/internal/adapter"
	"ucgr/internal/ir"
)

const (
	clockPerCmd = 40  // 每条指令推进
	clockPerMsg = 600 // 每条台词推进（阅读节奏只能近似）
)

var (
	evNameRe    = regexp.MustCompile(`(?i)^ev[a-z]?\d{3,}`)  // 事件 CG：ev1000a…
	standNameRe = regexp.MustCompile(`^[a-z]{2}\d{4}[a-z]$`) // 角色代号：km0000a、ay0000a…
	// cgPrefixRe 是「不是立绘」的已知前缀黑名单（背景/事件/特效/系统图）：
	// 立绘只在 CG 系命令上、且名字是角色代号形状时才认，认不准一律按 CG。
	cgPrefixRe = regexp.MustCompile(`(?i)^(bg|ev|sp|ic|map|logo|white|black|hs|th|cut|sys)`)
)

// BuildIR 把解析好的 .bs 脚本转成 IR。
func (a *Adapter) BuildIR(astObj adapter.ScriptAst, ctx *adapter.BuildCtx) (*ir.IR, error) {
	s, ok := astObj.(*Script)
	if !ok {
		return nil, fmt.Errorf("bgi: BuildIR 需要 *Script，实际 %T", astObj)
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
	name := filepath.Base(s.EntryPath)
	if ctx.Config != nil && ctx.Config["script_name"] != "" {
		name = filepath.Base(ctx.Config["script_name"])
	}
	stem := strings.TrimSuffix(name, filepath.Ext(name))
	if stem == "" {
		stem = "script"
	}

	b := &builder{
		out: &ir.IR{
			IRVersion: "1.0",
			GameID:    ctx.GameID,
			EngineID:  engineID,
			Fidelity:  "generic",
			Meta: ir.Meta{
				ScriptEncoding: s.Encoding,
				GeneratedBy:    "ucgr/bgi.BuildIR v1",
				GeneratedAt:    time.Now().UTC().Format(time.RFC3339),
			},
		},
		stem:    stem,
		resolve: resolve,
		ctx:     ctx,
		assets:  map[string]bool{},
	}
	for i := range s.Commands {
		b.step(&s.Commands[i])
	}
	b.flush()
	if len(b.out.Events) == 0 {
		return nil, fmt.Errorf("bgi: 脚本 %s 没有可重演的内容（代码段 %d 字节 / %d 条指令）",
			name, s.CodeLen, len(s.Commands))
	}
	return b.out, nil
}

type unrecAgg struct {
	op     uint32
	n      int
	sample string
	kept   map[string]bool
}

type builder struct {
	out     *ir.IR
	stem    string
	resolve func(string) (*ir.AssetRef, bool)
	ctx     *adapter.BuildCtx
	clock   int64

	cur     *ir.Event
	evStart int64
	layers  map[string]*ir.Layer
	layerLs []*ir.Layer
	assets  map[string]bool
	unrec   map[uint32]*unrecAgg
	unrecOp []uint32
	chars   []string
	seen    map[string]bool // 说话人去重

	lastLine     string
	lastIdx      int
	pendingVoice *ir.AssetRef
	evOrd        int
	textOrd      int
	bgName       string
	cgSeen       bool
	flushedUnrec int
}

func (b *builder) step(c *Command) {
	switch {
	case c.Op == opUserCall:
		b.userCall(c)
	case opKind(c.Op) == "text":
		b.text(c)
	case opKind(c.Op) == "image":
		b.image(c)
	case opKind(c.Op) == "voice", opKind(c.Op) == "se", opKind(c.Op) == "bgm":
		b.audio(c, opKind(c.Op))
	case opKind(c.Op) == "choice":
		b.choice(c)
	default:
		b.noteUnrec(c)
	}
	b.clock += clockPerCmd
}

// ---- 事件与图层 ----

func (b *builder) ensureEvent() {
	if b.cur == nil {
		b.newEvent()
	}
}

func (b *builder) newEvent() {
	b.flush()
	b.evOrd++
	b.cur = &ir.Event{
		EventID: fmt.Sprintf("%s#%03d", b.stem, b.evOrd),
		Kind:    ir.KindScene,
		Ord:     b.evOrd,
		StartMs: b.clock,
	}
	b.evStart = b.clock
	b.layers = map[string]*ir.Layer{}
	b.layerLs = nil
	b.assets = map[string]bool{}
	b.unrec = map[uint32]*unrecAgg{}
	b.unrecOp = nil
	b.chars = nil
	b.seen = map[string]bool{}
	b.lastLine = ""
	b.lastIdx = -1
	b.pendingVoice = nil
	b.bgName = ""
	b.cgSeen = false
}

func (b *builder) flush() {
	ev := b.cur
	if ev == nil {
		return
	}
	if len(ev.Texts) == 0 && len(ev.Layers) == 0 && len(ev.Assets) == 0 {
		b.cur = nil
		return // 空事件不留
	}
	if b.pendingVoice != nil && b.lastLine != "" {
		// 兜底：语音命令之后整段没有新台词，就绑到上一条台词。
		ev.Voices = append(ev.Voices, ir.Voice{LineID: b.lastLine, Asset: *b.pendingVoice})
	}
	b.pendingVoice = nil
	if b.cgSeen {
		ev.Kind = ir.KindStillCG
	}
	ev.EndMs = b.clock
	for _, l := range b.layerLs {
		ev.Layers = append(ev.Layers, *l)
	}
	ev.Characters = b.chars
	sort.Slice(b.unrecOp, func(i, j int) bool { return b.unrecOp[i] < b.unrecOp[j] })
	for _, op := range b.unrecOp {
		agg := b.unrec[op]
		u := ir.Unrecognized{
			NodePath: fmt.Sprintf("op[0x%04X]", op),
			Raw:      agg.sample,
			Reason:   fmt.Sprintf("未实现的命令语义（本事件 %d 次）", agg.n),
		}
		if len(agg.kept) > 0 {
			for h := range agg.kept {
				u.KeptAssets = append(u.KeptAssets, h)
			}
			sort.Strings(u.KeptAssets)
		}
		ev.Unrecognized = append(ev.Unrecognized, u)
		if b.ctx.RecordUnrecognized != nil {
			b.ctx.RecordUnrecognized(u)
		}
		b.flushedUnrec++
	}
	b.out.Events = append(b.out.Events, *ev)
	b.cur = nil
}

// layerOf 取/建图层（事件内按「种类:资源名」复用）。
func (b *builder) layerOf(kind ir.LayerKind, name string) (*ir.Layer, string) {
	key := string(kind) + ":" + name
	if l, ok := b.layers[key]; ok {
		return l, ""
	}
	id := fmt.Sprintf("%s%d", layerPrefix(kind), len(b.layerLs)+1)
	l := &ir.Layer{
		LayerID: id,
		Kind:    kind,
		Asset:   b.addAsset(name, roleOf(kind)),
		Z:       zOf(kind, len(b.layerLs)),
		Visible: true,
	}
	b.layers[key] = l
	b.layerLs = append(b.layerLs, l)
	return l, name
}

func layerPrefix(k ir.LayerKind) string {
	switch k {
	case ir.LayerBackground:
		return "bg"
	case ir.LayerStand:
		return "st"
	case ir.LayerCG:
		return "cg"
	case ir.LayerEffect:
		return "fx"
	}
	return "ly"
}

func zOf(k ir.LayerKind, n int) int {
	switch k {
	case ir.LayerBackground:
		return 0
	case ir.LayerStand:
		return 100 + n
	case ir.LayerCG:
		return 300 + n
	}
	return 50 + n
}

func roleOf(k ir.LayerKind) string {
	switch k {
	case ir.LayerBackground:
		return "背景"
	case ir.LayerStand:
		return "立绘"
	case ir.LayerCG:
		return "CG"
	}
	return "其它"
}

// layerKindOf 判定图层种类（启发式，保守：认不准就往 CG 靠，宁可标错种类也不丢资产）。
func layerKindOf(op uint32, name string) ir.LayerKind {
	switch {
	case evNameRe.MatchString(name):
		return ir.LayerCG
	case op < 0x02C0:
		return ir.LayerBackground
	case standNameRe.MatchString(name) && !cgPrefixRe.MatchString(name):
		return ir.LayerStand
	}
	return ir.LayerCG
}

// ---- 各命令的落库 ----

func (b *builder) text(c *Command) {
	b.ensureEvent()
	if len(c.Pushed) == 0 {
		b.noteUnrec(c)
		return
	}
	text := c.Pushed[len(c.Pushed)-1]
	speaker := ""
	if len(c.Pushed) >= 2 {
		speaker = c.Pushed[len(c.Pushed)-2]
	}
	// 三串消息组 = [语音名, 说话人, 正文]（《秽翼》实测）：首条语音名绑到本行。
	if len(c.Pushed) >= 3 {
		if v := c.Pushed[0]; plausibleRef(v) {
			if a := b.addAsset(v, "语音"); a != nil {
				b.pendingVoice = a
			}
		}
	}
	if strings.TrimSpace(text) == "" {
		b.noteUnrec(c)
		return
	}
	b.textOrd++
	line := ir.TextLine{
		LineID:  fmt.Sprintf("L%03d", b.textOrd),
		TMs:     b.clock - b.evStart,
		Speaker: speaker,
		Text:    text,
	}
	if b.pendingVoice != nil {
		line.Voice = b.pendingVoice
		b.cur.Voices = append(b.cur.Voices, ir.Voice{LineID: line.LineID, Asset: *b.pendingVoice})
		b.pendingVoice = nil
	}
	b.cur.Texts = append(b.cur.Texts, line)
	b.lastLine = line.LineID
	b.lastIdx = len(b.cur.Texts) - 1
	if speaker != "" && !b.seen[speaker] {
		b.seen[speaker] = true
		b.chars = append(b.chars, speaker)
	}
	b.cur.Timeline = append(b.cur.Timeline, ir.TimelineOp{TMs: b.clock - b.evStart, Op: "text_show", Target: line.LineID})
	b.cur.Show = append(b.cur.Show, ir.ShowOp{
		TMs: b.clock, Kind: ir.ShowTextRun, Layer: "text",
		Text: &ir.ShowText{Speaker: speaker, Text: text},
	})
	b.clock += clockPerMsg
}

func (b *builder) image(c *Command) {
	b.ensureEvent()
	name := firstPush(c)
	if name == "" {
		b.noteUnrec(c) // 有的图像命令靠栈传 int（如 0x02C6），v1 不解释
		return
	}
	if evNameRe.MatchString(name) {
		b.cgSeen = true
	}
	isBG := imageOps[c.Op] && c.Op < 0x02C0
	if isBG && b.bgName != "" && b.bgName != name && len(b.cur.Texts) > 0 {
		b.newEvent() // 已有台词又换背景 → 场景切换
		b.ensureEvent()
	}
	kind := layerKindOf(c.Op, name)
	l, _ := b.layerOf(kind, name)
	b.cur.Timeline = append(b.cur.Timeline, ir.TimelineOp{
		TMs: b.clock - b.evStart, Op: "image_show", Target: l.LayerID,
		Params: map[string]any{"ref": name},
	})
	b.cur.Show = append(b.cur.Show, ir.ShowOp{TMs: b.clock, Kind: ir.ShowLayerShow, Layer: l.LayerID, Ref: name})
	if isBG {
		b.bgName = name
	}
}

func (b *builder) audio(c *Command, kind string) {
	b.ensureEvent()
	name := firstPush(c)
	if name == "" {
		b.noteUnrec(c)
		return
	}
	role := map[string]string{"voice": "语音", "se": "音效", "bgm": "BGM"}[kind]
	a := b.addAsset(name, role)
	switch kind {
	case "voice":
		if a != nil {
			b.pendingVoice = a
		}
		b.cur.Show = append(b.cur.Show, ir.ShowOp{TMs: b.clock, Kind: ir.ShowVoicePlay, Ref: name})
	case "se":
		b.cur.Show = append(b.cur.Show, ir.ShowOp{TMs: b.clock, Kind: ir.ShowSEPlay, Ref: name})
	case "bgm":
		b.cur.Show = append(b.cur.Show, ir.ShowOp{TMs: b.clock, Kind: ir.ShowBGMPlay, Ref: name})
	}
}

func (b *builder) choice(c *Command) {
	b.ensureEvent()
	opts := c.Pushed
	if len(opts) < 2 {
		b.noteUnrec(c)
		return
	}
	b.addBranch(opts)
}

// userCall 处理 0x001C 用户函数调用：_SelectEx 是选项屏，其余照实登记。
func (b *builder) userCall(c *Command) {
	b.ensureEvent()
	if n := len(c.Pushed); n >= 3 && c.Pushed[n-1] == "_SelectEx" {
		b.addBranch(c.Pushed[:n-1])
		b.cur.Show = append(b.cur.Show, ir.ShowOp{TMs: b.clock, Kind: ir.ShowWait, DurMs: 0})
		return
	}
	b.noteUnrec(c)
}

func (b *builder) addBranch(options []string) {
	opts := make([]ir.BranchOpt, 0, len(options))
	for _, o := range options {
		opts = append(opts, ir.BranchOpt{Text: o})
	}
	b.cur.Branches = append(b.cur.Branches, ir.Branch{
		BranchID:      fmt.Sprintf("B%02d", len(b.cur.Branches)+1),
		TMs:           b.clock - b.evStart,
		Type:          "choice",
		Options:       opts,
		DefaultTarget: "",
	})
}

// noteUnrec 登记未实现的命令（按 opcode 聚合），并把「形似资源名」的压栈串
// 保守收进资产（§6：不认识命令也不能丢引用）。
func (b *builder) noteUnrec(c *Command) {
	b.ensureEvent()
	agg := b.unrec[c.Op]
	if agg == nil {
		agg = &unrecAgg{op: c.Op, sample: sampleOf(c), kept: map[string]bool{}}
		b.unrec[c.Op] = agg
		b.unrecOp = append(b.unrecOp, c.Op)
	}
	agg.n++
	for _, p := range c.Pushed {
		if refNameRe.MatchString(p) && looksLikeRef(p) {
			if a := b.addAsset(p, "其它"); a != nil && a.Hash != "" {
				agg.kept[a.Hash] = true
			}
		}
	}
}

// addAsset 解析并登记资产（事件内按哈希去重）。
func (b *builder) addAsset(ref, role string) *ir.AssetRef {
	if ref == "" || b.cur == nil {
		return nil
	}
	a, ok := b.resolve(ref)
	if !ok || a == nil {
		return nil
	}
	if a.OrigPath == "" {
		a.OrigPath = ref
	}
	if a.Role == "" || a.Role == "其它" {
		a.Role = role
	}
	key := a.Hash
	if key == "" {
		key = "path:" + a.OrigPath
	}
	if !b.assets[key] {
		b.assets[key] = true
		b.cur.Assets = append(b.cur.Assets, *a)
	}
	return a
}

// ---- 小工具 ----

func firstPush(c *Command) string {
	if len(c.Pushed) == 0 {
		return ""
	}
	return c.Pushed[0]
}

func sampleOf(c *Command) string {
	if len(c.Pushed) == 0 {
		return c.Raw
	}
	return c.Raw + " push=" + strings.Join(c.Pushed, "|")
}
