// build_ir.go —— 把 .bin 脚本转成 IR。
//
// NeXAS 的段落流没有显式的背景切换命令（背景由 .spm 粒子/标题画面资产
// 与引擎状态机接管），所以事件粒度取「说话人/场景切换」的天然停顿：
// 一个 .bin 一个场景事件（Baldr Sky 一个章节 300-900 段，作为场景回放
// 粒度合适）。
//
// 事件内：
//   - SegText → TextLine + Show 三连（清屏、写入、点击等待）；
//   - SegSpeaker → 说话人（绑定下一条台词）；
//   - SegAsset（*.png）→ 背景层 + layer_show；
//   - SegVoice（@v 语音号）→ 绑最近台词 + voice_play（Voice.pac 内条目号）；
//   - SegBranch → Branch（回放是重演，默认走第一条）；
//   - SegCmd / 其余 → Unrecognized 聚合登记。
package nexas

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"ucgr/internal/adapter"
	"ucgr/internal/ir"
)

const (
	clockPerCmd = 40  // 每条指令推进
	clockPerMsg = 600 // 每条台词推进
	textLayer   = "text"
)

// BuildIR 把解析好的脚本转成 IR。
func (a *Adapter) BuildIR(astObj adapter.ScriptAst, ctx *adapter.BuildCtx) (*ir.IR, error) {
	s, ok := astObj.(*Script)
	if !ok {
		return nil, fmt.Errorf("nexas: BuildIR 需要 *Script，实际 %T", astObj)
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
	name := s.Name
	if name == "" {
		name = "script"
	}
	enc := "GBK"
	if s.TextCP == 932 {
		enc = "CP932"
	}

	b := &builder{
		out: &ir.IR{
			IRVersion: "1.0",
			GameID:    ctx.GameID,
			EngineID:  engineID,
			Fidelity:  "generic",
			Meta: ir.Meta{
				ScriptEncoding: enc,
				GeneratedBy:    "ucgr/nexas.BuildIR v1",
				GeneratedAt:    time.Now().UTC().Format(time.RFC3339),
			},
		},
		stem:    strings.TrimSuffix(name, ".bin"),
		script:  s,
		resolve: resolve,
		ctx:     ctx,
	}
	b.run()
	b.flush()
	if len(b.out.Events) == 0 {
		return nil, fmt.Errorf("nexas: 脚本 %s 没有可重演的内容（%d 段）", name, len(s.Segs))
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

	cur        *ir.Event
	evStart    int64
	layers     map[string]*ir.Layer
	layerLs    []*ir.Layer
	assets     map[string]bool
	unrec      map[string]*unrecAgg
	unrecK     []string
	chars      []string
	seen       map[string]bool
	speaker    string            // 待绑定的说话人
	pendingOpt []string          // 连续的分支引用（选项）
	textOrd    int
	evOrd      int
}

func (b *builder) run() {
	for i := range b.script.Segs {
		seg := &b.script.Segs[i]
		switch seg.Kind {
		case SegText:
			b.flushBranch()
			b.text(seg)
			b.clock += clockPerMsg
		case SegSpeaker:
			// 连续说话人段会互相覆盖；两段说话人间有分支引用也一起 flush。
			b.flushBranch()
			b.speaker = seg.Text
			b.clock += clockPerCmd
		case SegLabel:
			b.flushBranch()
			b.clock += clockPerCmd
		case SegAsset:
			b.flushBranch()
			b.asset(seg)
			b.clock += clockPerCmd
		case SegBranch:
			b.pendingOpt = append(b.pendingOpt, seg.Text)
			b.clock += clockPerCmd
		case SegCmd:
			b.flushBranch()
			b.noteUnrec(seg.Text)
			b.clock += clockPerCmd
		}
	}
	b.flushBranch()
}

// flushBranch 连续 .bin 引用聚成一个选项分支。
// 分支是「重演默认走第一条」的记录（BranchOpt.Text = 目标脚本名）。
func (b *builder) flushBranch() {
	if len(b.pendingOpt) == 0 {
		return
	}
	b.ensureEvent()
	var opts []ir.BranchOpt
	seen := map[string]bool{}
	for _, t := range b.pendingOpt {
		if !seen[t] {
			seen[t] = true
			opts = append(opts, ir.BranchOpt{Text: t})
		}
	}
	b.pendingOpt = nil
	if len(opts) == 0 {
		return
	}
	b.cur.Branches = append(b.cur.Branches, ir.Branch{
		BranchID: fmt.Sprintf("B%02d", len(b.cur.Branches)+1),
		TMs:      b.clock - b.evStart,
		Type:     "choice",
		Options:  opts,
	})
}

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
			NodePath: fmt.Sprintf("seg[%s]", k),
			Raw:      agg.sample,
			Reason:   fmt.Sprintf("未实现的指令段语义（本事件 %d 次）", agg.n),
		}
		ev.Unrecognized = append(ev.Unrecognized, u)
		if b.ctx.RecordUnrecognized != nil {
			b.ctx.RecordUnrecognized(u)
		}
	}
	b.out.Events = append(b.out.Events, *ev)
	b.cur = nil
}

func (b *builder) asset(seg *Seg) {
	// *.png 是唯一能落到层的资源（背景/CG）；spm/fnt 是引擎内部件，
	// 登记不解释。
	name := seg.Text
	if hasSuffixFold(name, ".fnt") || hasSuffixFold(name, ".spm") {
		b.addAsset(name, "其它")
		return
	}
	b.ensureEvent()
	l := b.layerOf(ir.LayerBackground, name)
	b.cur.Timeline = append(b.cur.Timeline, ir.TimelineOp{
		TMs: b.clock - b.evStart, Op: "image_show", Target: l.LayerID,
		Params: map[string]any{"ref": name},
	})
	b.cur.Show = append(b.cur.Show, ir.ShowOp{TMs: b.clock, Kind: ir.ShowLayerShow, Layer: l.LayerID, Ref: name})
}

func (b *builder) layerOf(kind ir.LayerKind, name string) *ir.Layer {
	key := string(kind) + ":" + name
	if l, ok := b.layers[key]; ok {
		return l
	}
	prefix := "bg"
	z := 0
	role := "背景"
	if kind == ir.LayerStand {
		prefix, z, role = "st", 100+len(b.layerLs), "立绘"
	} else if kind == ir.LayerCG {
		prefix, z, role = "cg", 300+len(b.layerLs), "CG"
	}
	l := &ir.Layer{
		LayerID: fmt.Sprintf("%s%d", prefix, len(b.layerLs)+1),
		Kind:    kind,
		Asset:   b.addAsset(name, role),
		Z:       z,
		Visible: true,
	}
	b.layers[key] = l
	b.layerLs = append(b.layerLs, l)
	return l
}

func (b *builder) text(seg *Seg) {
	b.ensureEvent()
	b.textOrd++
	line := ir.TextLine{
		LineID:  fmt.Sprintf("L%03d", b.textOrd),
		TMs:     b.clock - b.evStart,
		Speaker: b.speaker,
		Text:    seg.Text,
	}
	if seg.Voice != "" {
		name := seg.Voice + ".ogg"
		if a := b.addAsset(name, "语音"); a != nil {
			line.Voice = a
			b.cur.Voices = append(b.cur.Voices, ir.Voice{LineID: line.LineID, Asset: *a})
		}
		b.cur.Show = append(b.cur.Show, ir.ShowOp{TMs: b.clock, Kind: ir.ShowVoicePlay, Ref: name})
	}
	b.cur.Texts = append(b.cur.Texts, line)
	if b.speaker != "" && !b.seen[b.speaker] {
		b.seen[b.speaker] = true
		b.chars = append(b.chars, b.speaker)
	}
	// 台词后说话人清零（叙述段没有说话人）。
	b.speaker = ""
	b.cur.Timeline = append(b.cur.Timeline, ir.TimelineOp{TMs: b.clock - b.evStart, Op: "text_show", Target: line.LineID})
	b.cur.Show = append(b.cur.Show,
		ir.ShowOp{TMs: b.clock, Kind: ir.ShowTextClear, Layer: textLayer},
		ir.ShowOp{TMs: b.clock, Kind: ir.ShowTextRun, Layer: textLayer,
			Text: &ir.ShowText{Speaker: line.Speaker, Text: line.Text}},
		ir.ShowOp{TMs: b.clock, Kind: ir.ShowTextWait},
	)
}

func (b *builder) noteUnrec(name string) {
	b.ensureEvent()
	agg := b.unrec[name]
	if agg == nil {
		agg = &unrecAgg{sample: name}
		b.unrec[name] = agg
		b.unrecK = append(b.unrecK, name)
	}
	agg.n++
}

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

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
