// build_ir.go —— 把 NSS 段落流转成 IR。
//
// 事件粒度：一个 .nss 一个场景事件（正文 scene 块就是全部 <PRE> 文本块的
// 载体；_MAIN 之类的路由块没有 <PRE>，自然不会产出内容）。
//
// 事件内：
//   - SegImage（CreateTextureEX / OnBG）→ 首个落背景层，其余登记资产；
//   - SegText → TextLine + text_clear/text/text_wait 三连（说话人随行）；
//   - SegVoice → 绑最近台词 + voice_play；
//   - SegBGM → bgm_play。
package nitro

import (
	"fmt"
	"strings"
	"time"

	"ucgr/internal/adapter"
	"ucgr/internal/ir"
)

const (
	clockPerCmd = 40
	clockPerMsg = 600
	textLayer   = "text"
)

// BuildIR 把解析好的脚本转成 IR。
func (a *Adapter) BuildIR(astObj adapter.ScriptAst, ctx *adapter.BuildCtx) (*ir.IR, error) {
	s, ok := astObj.(*Script)
	if !ok {
		return nil, fmt.Errorf("nitro: BuildIR 需要 *Script，实际 %T", astObj)
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
		name = "script.nss"
	}
	enc := "CP932"
	if s.TextCP == 936 {
		enc = "GBK"
	}
	b := &builder{
		out: &ir.IR{
			IRVersion: "1.0",
			GameID:    ctx.GameID,
			EngineID:  engineID,
			Fidelity:  "generic",
			Meta: ir.Meta{
				ScriptEncoding: enc,
				GeneratedBy:    "ucgr/nitro.BuildIR v1",
				GeneratedAt:    time.Now().UTC().Format(time.RFC3339),
			},
		},
		stem:    strings.TrimSuffix(name, filepathExt(name)),
		script:  s,
		resolve: resolve,
		ctx:     ctx,
	}
	b.run()
	b.flush()
	if len(b.out.Events) == 0 {
		return nil, fmt.Errorf("nitro: 脚本 %s 没有可重演的内容（%d 段）", name, len(s.Segs))
	}
	return b.out, nil
}

// filepathExt 返回含点扩展名（.nss）。
func filepathExt(name string) string {
	if i := strings.LastIndexByte(name, '.'); i >= 0 {
		return name[i:]
	}
	return ""
}

type builder struct {
	out     *ir.IR
	stem    string
	script  *Script
	resolve func(string) (*ir.AssetRef, bool)
	ctx     *adapter.BuildCtx
	clock   int64

	cur      *ir.Event
	evStart  int64
	bgDone   bool
	assets   map[string]bool
	chars    []string
	seen     map[string]bool
	speaker  string // 待绑定的说话人
	pendingVoice string // 紧邻台词的 <voice src>
	textOrd  int
	evOrd    int
}

func (b *builder) run() {
	for i := range b.script.Segs {
		seg := &b.script.Segs[i]
		switch seg.Kind {
		case SegImage:
			b.image(seg)
			b.clock += clockPerCmd
		case SegText:
			b.text(seg)
			b.clock += clockPerMsg
		case SegVoice:
			b.pendingVoice = seg.Ref
			b.clock += clockPerCmd
		case SegBGM:
			b.bgm(seg)
			b.clock += clockPerCmd
		}
	}
}

func (b *builder) ensureEvent() {
	if b.cur == nil {
		b.newEvent()
	}
}

func (b *builder) newEvent() {
	b.evOrd++
	b.cur = &ir.Event{
		EventID: fmt.Sprintf("%s#%03d", b.stem, b.evOrd),
		Kind:    ir.KindScene,
		Ord:     b.evOrd,
		StartMs: b.clock,
	}
	b.evStart = b.clock
	b.assets = map[string]bool{}
	b.seen = map[string]bool{}
	b.chars = nil
	b.pendingVoice = ""
	b.bgDone = false
}

func (b *builder) flush() {
	ev := b.cur
	if ev == nil {
		return
	}
	if len(ev.Texts) == 0 && len(ev.Layers) == 0 && len(ev.Assets) == 0 {
		b.cur = nil
		return
	}
	if ev.Title == "" && len(ev.Texts) > 0 {
		ev.Title = truncateRunes(ev.Texts[0].Text, 40)
	}
	ev.EndMs = b.clock
	ev.Characters = b.chars
	b.out.Events = append(b.out.Events, *ev)
	b.cur = nil
}

// image 图像引用：首个落背景层（cg/bg/ 引用），立绘/CG 引用只登记资产。
func (b *builder) image(seg *Seg) {
	b.ensureEvent()
	ref := seg.Ref
	lower := strings.ToLower(ref)
	isBG := strings.Contains(lower, "/bg/") || strings.Contains(lower, "\\bg\\")
	if isBG && !b.bgDone {
		b.bgDone = true
		l := b.layerOf(ir.LayerBackground, ref)
		b.cur.Timeline = append(b.cur.Timeline, ir.TimelineOp{
			TMs: b.clock - b.evStart, Op: "image_show", Target: l.LayerID,
			Params: map[string]any{"ref": ref},
		})
		b.cur.Show = append(b.cur.Show, ir.ShowOp{TMs: b.clock, Kind: ir.ShowLayerShow, Layer: l.LayerID, Ref: ref})
		return
	}
	b.addAsset(ref, assetRole(ref))
}

// assetRole 按路径猜资产角色。
func assetRole(ref string) string {
	l := strings.ToLower(ref)
	switch {
	case strings.Contains(l, "/bg/"), strings.Contains(l, "\\bg\\"):
		return "背景"
	case strings.Contains(l, "/st/"), strings.Contains(l, "\\st\\"):
		return "立绘"
	case strings.Contains(l, "/ev/"), strings.Contains(l, "\\ev\\"):
		return "CG"
	case strings.Contains(l, "voice"):
		return "语音"
	case strings.HasSuffix(l, ".ogg"):
		return "音频"
	}
	return "其它"
}

// layerOf 取/建图层。
func (b *builder) layerOf(kind ir.LayerKind, name string) *ir.Layer {
	l := ir.Layer{
		LayerID: "bg1",
		Kind:    kind,
		Asset:   b.addAsset(name, "背景"),
		Z:       0,
		Visible: true,
	}
	b.cur.Layers = append(b.cur.Layers, l)
	return &l
}

// text 一段 <PRE> 文本 → 一条台词。
func (b *builder) text(seg *Seg) {
	b.ensureEvent()
	b.textOrd++
	line := ir.TextLine{
		LineID:  fmt.Sprintf("L%03d", b.textOrd),
		TMs:     b.clock - b.evStart,
		Speaker: seg.Speaker,
		Text:    seg.Text,
	}
	// 紧邻的 <voice> 标签绑定本条台词
	if b.pendingVoice != "" {
		name := b.pendingVoice + ".ogg"
		if a := b.addAsset(name, "语音"); a != nil {
			line.Voice = a
			b.cur.Voices = append(b.cur.Voices, ir.Voice{LineID: line.LineID, Asset: *a})
		}
		b.cur.Show = append(b.cur.Show, ir.ShowOp{TMs: b.clock, Kind: ir.ShowVoicePlay, Ref: name})
		b.pendingVoice = ""
	}
	b.cur.Texts = append(b.cur.Texts, line)
	if seg.Speaker != "" && !b.seen[seg.Speaker] {
		b.seen[seg.Speaker] = true
		b.chars = append(b.chars, seg.Speaker)
	}
	b.cur.Timeline = append(b.cur.Timeline, ir.TimelineOp{TMs: b.clock - b.evStart, Op: "text_show", Target: line.LineID})
	b.cur.Show = append(b.cur.Show,
		ir.ShowOp{TMs: b.clock, Kind: ir.ShowTextClear, Layer: textLayer},
		ir.ShowOp{TMs: b.clock, Kind: ir.ShowTextRun, Layer: textLayer,
			Text: &ir.ShowText{Speaker: line.Speaker, Text: line.Text}},
		ir.ShowOp{TMs: b.clock, Kind: ir.ShowTextWait},
	)
}

// bgm BGM 引用（SoundPlay("@m07") → bgm/m07.ogg）。
func (b *builder) bgm(seg *Seg) {
	b.ensureEvent()
	name := seg.Ref
	if a := b.addAsset(name, "音频"); a != nil {
		b.cur.Show = append(b.cur.Show, ir.ShowOp{TMs: b.clock, Kind: ir.ShowBGMPlay, Ref: name})
	}
}

// addAsset 登记资产引用。
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
