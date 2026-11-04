// build_ir.go —— 把 NScripter 脚本转成 IR（事件切分 + Show 台本）。
//
// 切分口径与 BGI 一致：当前事件已有台词、又出现新的背景图时开新事件。
// 事件内：
//   - bg / ld / cd / lsp / mld 等图像命令 → 图层 + Show 的 layer_show；
//   - 台词行 → TextLine + Show 的 text 三连（清屏、写入、点击等待）；
//   - `;＜说话人` 注释（07th 约定）→ 说话人；voice 命令的语音绑最近台词；
//   - select / selgosub → Branch（回放是重演，默认走第一条）；
//   - 其余命令 → 按命令名聚合 Unrecognized（照实登记，不解释语义）。
//
// 时间只能近似：按「每条命令 +40ms、每条台词 +600ms」给单调近似值，
// 播放器按 Show 顺序执行，不依赖这些数字。
package nscript

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"ucgr/internal/adapter"
	"ucgr/internal/ir"
)

const (
	clockPerCmd = 40  // 每条命令推进
	clockPerMsg = 600 // 每条台词推进
)

// textLayer 本适配器的台词层名（与 BGI 同款，回放前置 MessageBoxOps）。
const textLayer = "text"

// BuildIR 把解析好的脚本转成 IR。
func (a *Adapter) BuildIR(astObj adapter.ScriptAst, ctx *adapter.BuildCtx) (*ir.IR, error) {
	s, ok := astObj.(*Script)
	if !ok {
		return nil, fmt.Errorf("nscript: BuildIR 需要 *Script，实际 %T", astObj)
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
	stem := strings.TrimSuffix(name, filepath.Ext(name))
	if stem == "" || stem == "nscript" {
		stem = "script"
	}

	b := &builder{
		out: &ir.IR{
			IRVersion: "1.0",
			GameID:    ctx.GameID,
			EngineID:  engineID,
			Fidelity:  "generic",
			Meta: ir.Meta{
				ScriptEncoding: "CP932",
				GeneratedBy:    "ucgr/nscript.BuildIR v1",
				GeneratedAt:    time.Now().UTC().Format(time.RFC3339),
			},
		},
		stem:    stem,
		script:  s,
		resolve: resolve,
		ctx:     ctx,
	}
	b.run()
	b.flush()
	if len(b.out.Events) == 0 {
		return nil, fmt.Errorf("nscript: 脚本 %s 没有可重演的内容（%d 行）", name, len(s.Lines))
	}
	return b.out, nil
}

type unrecAgg struct {
	n      int
	sample string
}

type builder struct {
	out     *ir.IR
	stem    string
	script  *Script
	resolve func(string) (*ir.AssetRef, bool)
	ctx     *adapter.BuildCtx
	clock   int64

	cur     *ir.Event
	evStart int64
	layers  map[string]*ir.Layer
	layerLs []*ir.Layer
	assets  map[string]bool
	unrec   map[string]*unrecAgg
	unrecK  []string
	chars   []string
	seen    map[string]bool

	pendingVoice *ir.AssetRef
	evOrd        int
	textOrd      int
	bgName       string
}

func (b *builder) run() {
	for i := range b.script.Lines {
		ln := &b.script.Lines[i]
		switch ln.Kind {
		case LineText:
			b.text(ln)
			b.clock += clockPerMsg
		case LineCmd:
			b.command(ln)
			b.clock += clockPerCmd
		}
	}
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
	b.unrec = map[string]*unrecAgg{}
	b.unrecK = nil
	b.chars = nil
	b.seen = map[string]bool{}
	b.pendingVoice = nil
	b.bgName = ""
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
	// 标题 = 首句台词（截 40 字），事件库里直接可读。
	if len(ev.Texts) > 0 {
		ev.Title = truncateRunes(ev.Texts[0].Text, 40)
	}
	ev.EndMs = b.clock
	for _, l := range b.layerLs {
		ev.Layers = append(ev.Layers, *l)
	}
	ev.Characters = b.chars
	sort.Strings(b.unrecK)
	for _, k := range b.unrecK {
		agg := b.unrec[k]
		u := ir.Unrecognized{
			NodePath: fmt.Sprintf("cmd[%s]", k),
			Raw:      agg.sample,
			Reason:   fmt.Sprintf("未实现的命令语义（本事件 %d 次）", agg.n),
		}
		ev.Unrecognized = append(ev.Unrecognized, u)
		if b.ctx.RecordUnrecognized != nil {
			b.ctx.RecordUnrecognized(u)
		}
	}
	b.out.Events = append(b.out.Events, *ev)
	b.cur = nil
}

// layerOf 取/建图层（事件内按「种类:资源名」复用）。
func (b *builder) layerOf(kind ir.LayerKind, name string) *ir.Layer {
	key := string(kind) + ":" + name
	if l, ok := b.layers[key]; ok {
		return l
	}
	l := &ir.Layer{
		LayerID: fmt.Sprintf("%s%d", layerPrefix(kind), len(b.layerLs)+1),
		Kind:    kind,
		Asset:   b.addAsset(name, roleOf(kind)),
		Z:       zOf(kind, len(b.layerLs)),
		Visible: true,
	}
	b.layers[key] = l
	b.layerLs = append(b.layerLs, l)
	return l
}

func layerPrefix(k ir.LayerKind) string {
	switch k {
	case ir.LayerBackground:
		return "bg"
	case ir.LayerStand:
		return "st"
	case ir.LayerCG:
		return "cg"
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

// ---- 命令落库 ----

func (b *builder) command(ln *Line) {
	switch ln.Cmd {
	case "bg":
		b.bgCmd(ln)
	case "ld", "cd":
		b.standCmd(ln)
	case "cl":
		b.clearCmd(ln)
	case "lsp":
		b.spriteCmd(ln)
	case "csp":
		b.clearSpriteCmd(ln)
	case "msp", "mld", "mlspd":
		b.moveCmd(ln)
	case "select", "selgosub", "selsel", "sel":
		b.selectCmd(ln)
	case "voice", "vo":
		b.voiceCmd(ln)
	case "mp3", "bgm", "bgmplay", "bgmonce", "play":
		b.bgmCmd(ln)
	case "wave", "dwave", "seplay", "dwavestop", "bgmstop", "mp3stop", "wavestop", "voicestop":
		b.seCmd(ln)
	default:
		b.noteUnrec(ln)
	}
}

// arg0Str 第一个参数按字符串引用解（$别名 → stralias）。
func (b *builder) arg0Str(ln *Line) string {
	if len(ln.Args) == 0 {
		return ""
	}
	return b.script.ResolveImage(ln.Args[0])
}

func (b *builder) bgCmd(ln *Line) {
	name := b.arg0Str(ln)
	if name == "" {
		b.noteUnrec(ln)
		return
	}
	// 已有台词又换背景 → 场景切换（与 BGI 同口径）。
	if b.bgName != "" && b.bgName != name && b.cur != nil && len(b.cur.Texts) > 0 {
		b.newEvent()
	}
	b.ensureEvent()
	l := b.layerOf(ir.LayerBackground, name)
	b.cur.Timeline = append(b.cur.Timeline, ir.TimelineOp{
		TMs: b.clock - b.evStart, Op: "image_show", Target: l.LayerID,
		Params: map[string]any{"ref": name},
	})
	b.cur.Show = append(b.cur.Show, ir.ShowOp{TMs: b.clock, Kind: ir.ShowLayerShow, Layer: l.LayerID, Ref: name})
	b.bgName = name
}

func (b *builder) standCmd(ln *Line) {
	// ld <l|c|r>,<名字>,<效果>；cd <up|down|l|c|r>,<名字>,<效果>
	args := ln.Args
	name := ""
	if len(args) >= 2 {
		name = b.script.ResolveStr(args[1])
	} else if len(args) == 1 {
		name = b.script.ResolveImage(args[0]) // 有的写法省略位置
	}
	if name == "" {
		b.noteUnrec(ln)
		return
	}
	b.ensureEvent()
	l := b.layerOf(ir.LayerStand, name)
	b.cur.Timeline = append(b.cur.Timeline, ir.TimelineOp{
		TMs: b.clock - b.evStart, Op: "image_show", Target: l.LayerID,
		Params: map[string]any{"ref": name},
	})
	b.cur.Show = append(b.cur.Show, ir.ShowOp{TMs: b.clock, Kind: ir.ShowLayerShow, Layer: l.LayerID, Ref: name})
}

func (b *builder) clearCmd(ln *Line) {
	// cl <a|l|c|r>[,<效果>]：清立绘。层 id 只有在已登记时才知道，简单起见
	// 隐藏本事件全部立绘层。
	b.ensureEvent()
	for _, l := range b.layerLs {
		if l.Kind == ir.LayerStand {
			b.cur.Show = append(b.cur.Show, ir.ShowOp{TMs: b.clock, Kind: ir.ShowLayerHide, Layer: l.LayerID})
		}
	}
}

func (b *builder) spriteCmd(ln *Line) {
	// lsp <n>,"<名字>",x,y[,alpha]
	name := ""
	for i, a := range ln.Args {
		if i > 0 && a != "" && !isNumber(a) {
			name = b.script.ResolveImage(a)
			break
		}
	}
	if name == "" {
		b.noteUnrec(ln)
		return
	}
	b.ensureEvent()
	l := b.layerOf(ir.LayerCG, name)
	b.cur.Timeline = append(b.cur.Timeline, ir.TimelineOp{
		TMs: b.clock - b.evStart, Op: "image_show", Target: l.LayerID,
		Params: map[string]any{"ref": name},
	})
	b.cur.Show = append(b.cur.Show, ir.ShowOp{TMs: b.clock, Kind: ir.ShowLayerShow, Layer: l.LayerID, Ref: name})
}

func (b *builder) clearSpriteCmd(ln *Line) {
	// csp <n>：隐藏 sprite 层没有名字对应，保守隐藏全部 CG 层。
	b.ensureEvent()
	for _, l := range b.layerLs {
		if l.Kind == ir.LayerCG {
			b.cur.Show = append(b.cur.Show, ir.ShowOp{TMs: b.clock, Kind: ir.ShowLayerHide, Layer: l.LayerID})
		}
	}
}

func (b *builder) moveCmd(ln *Line) {
	// msp/mld 带引用名（$别名或字符串），登记资产 + 显示。
	name := ""
	for _, a := range ln.Args {
		if a != "" && !isNumber(a) {
			name = b.script.ResolveImage(a)
			break
		}
	}
	if name == "" {
		b.noteUnrec(ln)
		return
	}
	b.ensureEvent()
	l := b.layerOf(ir.LayerCG, name)
	b.cur.Show = append(b.cur.Show, ir.ShowOp{TMs: b.clock, Kind: ir.ShowLayerShow, Layer: l.LayerID, Ref: name})
}

func (b *builder) selectCmd(ln *Line) {
	b.ensureEvent()
	// select "文本",*label,"文本",*label…
	var opts []ir.BranchOpt
	for _, a := range ln.Args {
		if strings.HasPrefix(a, "*") {
			continue
		}
		if a != "" {
			opts = append(opts, ir.BranchOpt{Text: a})
		}
	}
	if len(opts) == 0 {
		b.noteUnrec(ln)
		return
	}
	b.cur.Branches = append(b.cur.Branches, ir.Branch{
		BranchID: fmt.Sprintf("B%02d", len(b.cur.Branches)+1),
		TMs:      b.clock - b.evStart,
		Type:     "choice",
		Options:  opts,
	})
}

func (b *builder) voiceCmd(ln *Line) {
	// voice <通道>,"<文件>"
	name := b.arg0Str(ln)
	if name == "" || isNumber(name) {
		if len(ln.Args) >= 2 {
			name = b.script.ResolveStr(ln.Args[1])
		}
	}
	if name == "" {
		b.noteUnrec(ln)
		return
	}
	b.ensureEvent()
	a := b.addAsset(name, "语音")
	if a != nil {
		b.pendingVoice = a
	}
	b.cur.Show = append(b.cur.Show, ir.ShowOp{TMs: b.clock, Kind: ir.ShowVoicePlay, Ref: name})
}

func (b *builder) bgmCmd(ln *Line) {
	// mp3 "<文件>" / bgmplay <编号> / play "*track"（CD 音轨，跳过）
	if len(ln.Args) == 0 {
		b.noteUnrec(ln)
		return
	}
	a0 := ln.Args[0]
	if strings.HasPrefix(a0, "*") || isNumber(a0) {
		b.noteUnrec(ln)
		return
	}
	name := b.script.ResolveStr(a0)
	b.ensureEvent()
	b.addAsset(name, "BGM")
	b.cur.Show = append(b.cur.Show, ir.ShowOp{TMs: b.clock, Kind: ir.ShowBGMPlay, Ref: name})
}

func (b *builder) seCmd(ln *Line) {
	// wave "<文件>" / dwave <通道>,"<文件>" / seplay <n>,<ms> / 各类 stop
	name := ""
	isStop := strings.HasSuffix(ln.Cmd, "stop")
	if isStop {
		return // stop 类命令在回放里是空操作
	}
	for _, a := range ln.Args {
		if a != "" && !isNumber(a) {
			name = b.script.ResolveStr(a)
			break
		}
	}
	if name == "" {
		b.noteUnrec(ln)
		return
	}
	b.ensureEvent()
	b.addAsset(name, "音效")
	b.cur.Show = append(b.cur.Show, ir.ShowOp{TMs: b.clock, Kind: ir.ShowSEPlay, Ref: name})
}

// text 台词行落库（说话人来自 `;＜名前` 注释，07th 约定）。
func (b *builder) text(ln *Line) {
	b.ensureEvent()
	speaker := strings.TrimPrefix(strings.TrimSpace(ln.Comment), "＜")
	b.textOrd++
	line := ir.TextLine{
		LineID:  fmt.Sprintf("L%03d", b.textOrd),
		TMs:     b.clock - b.evStart,
		Speaker: speaker,
		Text:    ln.Text,
	}
	if b.pendingVoice != nil {
		line.Voice = b.pendingVoice
		b.cur.Voices = append(b.cur.Voices, ir.Voice{LineID: line.LineID, Asset: *b.pendingVoice})
		b.pendingVoice = nil
	}
	b.cur.Texts = append(b.cur.Texts, line)
	if speaker != "" && !b.seen[speaker] {
		b.seen[speaker] = true
		b.chars = append(b.chars, speaker)
	}
	b.cur.Timeline = append(b.cur.Timeline, ir.TimelineOp{TMs: b.clock - b.evStart, Op: "text_show", Target: line.LineID})
	// 一句一屏：清上一句、写当前句、等点击。
	b.cur.Show = append(b.cur.Show,
		ir.ShowOp{TMs: b.clock, Kind: ir.ShowTextClear, Layer: textLayer},
		ir.ShowOp{TMs: b.clock, Kind: ir.ShowTextRun, Layer: textLayer,
			Text: &ir.ShowText{Speaker: speaker, Text: ln.Text}},
		ir.ShowOp{TMs: b.clock, Kind: ir.ShowTextWait},
	)
}

// noteUnrec 登记未实现的命令（按命令名聚合），引用形参数保守收进资产。
func (b *builder) noteUnrec(ln *Line) {
	b.ensureEvent()
	agg := b.unrec[ln.Cmd]
	if agg == nil {
		agg = &unrecAgg{sample: ln.Cmd + " " + strings.Join(ln.Args, ",")}
		b.unrec[ln.Cmd] = agg
		b.unrecK = append(b.unrecK, ln.Cmd)
	}
	agg.n++
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

func isNumber(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if (r < '0' || r > '9') && r != '-' && r != '+' && r != '.' && r != '%' {
			return false
		}
	}
	return true
}

// truncateRunes 按字符截断（多字节安全）。
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
