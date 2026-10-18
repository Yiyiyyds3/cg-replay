// Package player 是演出台本（ir.Event.Show）的解释器 ——「窗口实时播放器」的引擎核心。
//
// 与 internal/replay 的分工：
//   - replay：按 t_ms 采样扁平时间轴出帧（离线出图/GIF 工具，已不再作为交付形态）。
//   - player：**顺序阻塞式**解释器。原作节奏靠「点击阻塞」推进，时间戳排不出来，
//     所以这里逐条执行台本，模拟 KAG 的双缓冲（fore/back 页）、转场、抖动、
//     台词点击点（自动模式按字数估算阅读时长，见 Options）。
//
// 产出 Frame 是纯数据（层状态 + 文字框 + 抖动偏移），渲染交给平台层（窗口/GDI），
// 因此本包可在无窗口环境下单测。
//
// 文字排版口径来自本作 system/Config.tjs（KAG 官方 Config，取值为本作实参）：
//   - 消息层文字：defaultFontSize = 48、defaultLineSpacing = 12、
//     白色（defaultChColor = 0xffffff）、粗体（defaultBold = true）、
//     黑阴影（defaultShadow = true）；忽略源码换行（global.ignoreCR = true），
//     换行只由 [r] 决定，[l] 是「换行 + 等点击」。
//   - 消息层色块：frameColor = 0x000000 + frameOpacity = 128（半透明黑），
//     但 [position] 的 opacity 属性就是 frameOpacity（**不是图层 alpha**）——
//     本作用 [position ... opacity=0] 把消息框底色关掉。
//   - 消息层默认几何 ml=32 mt=32 mw=1280-64 mh=960-64、margin 16；
//     本作 dialogue 由 namae.ks 的「台詞」宏改写为 message0 + win3.png 框，
//     见 adapter/kirikiri/build_show.go 的宏模拟。
package player

import (
	"image"
	"math"
	"sort"
	"strconv"
	"strings"

	"ucgr/internal/ir"
)

// Loader 把素材相对路径读成图像（与 replay.Loader 同形，便于直接复用解码链路）。
type Loader func(rel string) (image.Image, error)

// Options 播放策略。注意：自动推进时长是**播放器策略**，不是脚本保真 ——
// 原作是「等玩家点击」，重演时用字数估算阅读时长（本作无语音可依）。
type Options struct {
	Speed      float64 // 时间倍率（默认 1）
	CharMs     float64 // 每字阅读耗时（默认 75ms）
	BaseMs     int64   // 每个点击点基础停留（默认 800ms）
	MinWaitMs  int64   // 自动推进下限（默认 800ms）
	MaxWaitMs  int64   // 自动推进上限（默认 6000ms）
	MsgLines   int     // 每个消息层最多保留行数（默认 8）
	SkipWaits  bool    // 跳过 [wait]/[wt]/点击点（快进预览用）
	MsgDefault string  // 默认消息层 id（默认 message0）

	// 文字样式（Config.tjs 实参；渲染层可再覆盖）
	FontSize     float64  // 字号（默认 48）
	LineSpacing  float64  // 行间（默认 12；行距 = 字号 + 行间 = 60）
	FrameColor   [3]uint8 // 消息框底色（默认黑色）
	FrameOpacity uint8    // 消息框底色不透明度（默认 128；0=不画底色）
}

// DefaultOptions 默认播放策略。
func DefaultOptions() Options {
	return Options{
		Speed:        1,
		CharMs:       75,
		BaseMs:       800,
		MinWaitMs:    800,
		MaxWaitMs:    6000,
		MsgLines:     8,
		MsgDefault:   "message0",
		FontSize:     48,
		LineSpacing:  12,
		FrameColor:   [3]uint8{0, 0, 0},
		FrameOpacity: 128,
	}
}

func (o *Options) fill() {
	d := DefaultOptions()
	if o.Speed <= 0 {
		o.Speed = d.Speed
	}
	if o.CharMs <= 0 {
		o.CharMs = d.CharMs
	}
	if o.BaseMs <= 0 {
		o.BaseMs = d.BaseMs
	}
	if o.MinWaitMs <= 0 {
		o.MinWaitMs = d.MinWaitMs
	}
	if o.MaxWaitMs <= 0 {
		o.MaxWaitMs = d.MaxWaitMs
	}
	if o.MsgLines <= 0 {
		o.MsgLines = d.MsgLines
	}
	if o.MsgDefault == "" {
		o.MsgDefault = d.MsgDefault
	}
	if o.FontSize <= 0 {
		o.FontSize = d.FontSize
	}
	if o.LineSpacing <= 0 {
		o.LineSpacing = d.LineSpacing
	}
}

// Input 玩家输入。
type Input struct {
	Advance bool // 手动推进一次点击点（回车/点击）
}

// LayerState 某层在某一页上的状态（Frame 里给出的是「当前该画成什么样」）。
type LayerState struct {
	Layer    string
	Ref      string // 脚本原文引用（storage 名等）
	Resolved string // 解析后的素材相对路径
	Visible  bool
	X, Y     float64
	HasPos   bool
	Opacity  float64 // 0~1（脚本 opacity 为 0~255，已归一）
	ClipL    float64
	ClipT    float64
	ClipW    float64
	ClipH    float64
	HasClip  bool
	Z        int
	Attrs    map[string]string
}

// TextLine 消息层上的一行。
type TextLine struct {
	Speaker string
	Text    string
}

// TextBox 一个消息层（对话框/名字牌/选项框）：几何 + 底色 + 已写上的文字行。
//
// 本作用到的消息层：message0（台词框，frame=win3.png）、message1（名字牌）、
// message5/6/7（选项框，opacity=0 即无底色）。
type TextBox struct {
	Layer        string
	Frame        string // 框图素材名（如 win3.png）；非空则画框图，取代底色
	FrameColor   [3]uint8
	FrameOpacity uint8
	X, Y, W, H   float64
	MarginL      float64
	MarginT      float64
	MarginR      float64
	MarginB      float64
	HasGeom      bool
	Visible      bool
	Lines        []TextLine
	FontSize     float64
	LineSpacing  float64
}

// Frame 一帧要画的东西。
type Frame struct {
	Layers        []LayerState
	Texts         []TextBox
	QuakeX        int
	QuakeY        int
	Done          bool  // 台本走完
	Waiting       bool  // 停在原作点击点（等自动/手动推进）
	Now           int64 // 已流逝演出毫秒
	TransProgress float64
}

// surface 一个消息层的文字状态。
type surface struct {
	lines        []TextLine
	lineOpen     bool // 上一项是 [r]：下一段文本要另起一行
	pendingChars int  // 自上个点击点以来的字数（估算自动推进时长）
}

// Player 台本解释器。
type Player struct {
	ev  *ir.Event
	ops []ir.ShowOp
	pc  int
	opt Options
	now int64

	fore, back map[string]*LayerState
	order      []string

	blockKind  string // ""（可执行）| "text"（点击点）| "time"（时间等待）
	blockUntil int64
	done       bool

	transFrom  map[string]*LayerState
	transStart int64
	transDur   int64

	quakeStart, quakeDur int64
	quakeVmax, quakeHmax float64
	quakeCount           int

	texts  map[string]*surface
	curMsg string

	// Audio 非 nil 时按台本回调（BGM/SE/语音交给平台层）。
	Audio func(ir.ShowOp)
}

// New 建立播放器（ev 为 nil 或没有 Show 时立即 Done）。
func New(ev *ir.Event, opt Options) *Player {
	opt.fill()
	p := &Player{
		ev:     ev,
		opt:    opt,
		fore:   map[string]*LayerState{},
		back:   map[string]*LayerState{},
		texts:  map[string]*surface{},
		curMsg: opt.MsgDefault,
	}
	if ev != nil {
		p.ops = ev.Show
	}
	if len(p.ops) == 0 {
		p.done = true
	}
	return p
}

// Event 返回台本所属事件。
func (p *Player) Event() *ir.Event { return p.ev }

// SetSkip 打开/关闭「跳过等待」（跳过脚本里的 wait/转场时长，调试或速览用）。
func (p *Player) SetSkip(v bool) { p.opt.SkipWaits = v }

// Now 已流逝演出毫秒。
func (p *Player) Now() int64 { return p.now }

// Done 台本是否走完。
func (p *Player) Done() bool { return p.done }

// Advance 推进 dtMs 毫秒（内部乘 Speed）；in.Advance 表示玩家手动推进一次点击点。
//
// 注意：台本走完后时间仍继续推进——最后一个 [trans] 可能还在跑，
// 必须等它走完画面才定格（Done 表示台本执行完，不代表画面已定格）。
func (p *Player) Advance(dtMs int64, in Input) {
	if dtMs < 0 {
		dtMs = 0
	}
	dtMs = int64(float64(dtMs) * p.opt.Speed)
	p.now += dtMs
	if in.Advance && p.blockKind == "text" {
		p.blockUntil = p.now // 手动推进：点击点立刻结束
	}
	if !p.done {
		p.pump()
	}
	p.finishTrans()
}

// pump 连续执行台本，直到遇到还没走完的阻塞点（或台本结束）。
func (p *Player) pump() {
	for {
		p.finishTrans()
		if p.now < p.blockUntil {
			return
		}
		p.blockKind = ""
		if p.pc >= len(p.ops) {
			p.done = true
			return
		}
		op := p.ops[p.pc]
		p.pc++
		p.exec(op)
	}
}

// exec 执行一条台本指令（可能设置阻塞点）。
func (p *Player) exec(op ir.ShowOp) {
	switch op.Kind {
	case ir.ShowLayerShow, ir.ShowLayerHide, ir.ShowLayerOpt, ir.ShowLayerAttr:
		p.applyLayer(op)

	case ir.ShowScreenCopy:
		p.back = deepCopy(p.fore)

	case ir.ShowTransition:
		d := op.DurMs
		if d < 0 {
			d = 0
		}
		p.transFrom = deepCopy(p.fore)
		p.transStart = p.now
		p.transDur = d

	case ir.ShowQuake:
		p.quakeStart = p.now
		p.quakeDur = op.DurMs
		p.quakeVmax = fattr(op.Attr, "vmax")
		p.quakeHmax = fattr(op.Attr, "hmax")
		p.quakeCount = iattr(op.Attr, "count", 2)
		if !p.opt.SkipWaits && op.DurMs > 0 {
			p.blockKind, p.blockUntil = "time", p.now+op.DurMs
		}

	case ir.ShowWait:
		if p.opt.SkipWaits {
			return
		}
		// [wt]「等转场结束」：转场还在跑就等它；否则按 time（可能为 0，不等）
		if op.Attr["wt"] == "1" {
			if end := p.transStart + p.transDur; p.transDur > 0 && end > p.now {
				p.blockKind, p.blockUntil = "time", end
			}
			return
		}
		if op.DurMs > 0 {
			p.blockKind, p.blockUntil = "time", p.now+op.DurMs
		}

	case ir.ShowTextRun:
		p.addText(op)

	case ir.ShowTextBreak:
		p.surface(p.curMsg).lineOpen = true

	case ir.ShowTextWait:
		if p.opt.SkipWaits {
			return
		}
		p.blockKind, p.blockUntil = "text", p.now+p.autoWait(p.curMsg)

	case ir.ShowTextClear:
		if op.Layer != "" {
			p.clearSurface(op.Layer)
		} else {
			p.clearSurface(p.curMsg)
		}

	case ir.ShowTextStyle:
		// 文本样式（@style / [nowait] 等）：当前渲染层用默认样式，属性仍留在台本里

	case ir.ShowVoicePlay, ir.ShowBGMPlay, ir.ShowBGMFadeIn, ir.ShowBGMFadeOut,
		ir.ShowBGMStop, ir.ShowSEPlay, ir.ShowSEFadeIn, ir.ShowSEFadeOut, ir.ShowSEStop:
		if p.Audio != nil {
			p.Audio(op)
		}
	}
}

// surface 取（或建）某个消息层的文字状态。
func (p *Player) surface(id string) *surface {
	if id == "" {
		id = p.opt.MsgDefault
	}
	s, ok := p.texts[id]
	if !ok {
		s = &surface{}
		p.texts[id] = s
	}
	return s
}

// clearSurface 清一个消息层的文字（[cm]/[np]/[per]/[er]）。
func (p *Player) clearSurface(id string) {
	s := p.surface(id)
	s.lines = nil
	s.lineOpen = false
	s.pendingChars = 0
}

// addText 把一段文本追加到消息层（同一行未换行则拼接）。
func (p *Player) addText(op ir.ShowOp) {
	if op.Text == nil || strings.TrimSpace(op.Text.Text) == "" {
		return
	}
	if op.Layer != "" {
		p.curMsg = op.Layer
	}
	s := p.surface(p.curMsg)
	s.pendingChars += len([]rune(op.Text.Text))
	if n := len(s.lines); n > 0 && !s.lineOpen && s.lines[n-1].Speaker == op.Text.Speaker {
		s.lines[n-1].Text += op.Text.Text
	} else {
		s.lines = append(s.lines, TextLine{Speaker: op.Text.Speaker, Text: op.Text.Text})
	}
	s.lineOpen = false
	if len(s.lines) > p.opt.MsgLines {
		s.lines = s.lines[len(s.lines)-p.opt.MsgLines:]
	}
}

// autoWait 按「该消息层自上次点击点以来新增的字数」估算自动推进时长。
func (p *Player) autoWait(id string) int64 {
	s := p.surface(id)
	d := p.opt.BaseMs + int64(float64(s.pendingChars)*p.opt.CharMs)
	s.pendingChars = 0
	if d < p.opt.MinWaitMs {
		d = p.opt.MinWaitMs
	}
	if d > p.opt.MaxWaitMs {
		d = p.opt.MaxWaitMs
	}
	return d
}

// isMessageLayer 消息层（其 opacity 属性是 frameOpacity，不是图层 alpha）。
func isMessageLayer(id string) bool { return strings.HasPrefix(strings.ToLower(id), "message") }

// applyLayer 按 KAG 双缓冲语义改某一页上的层状态。
func (p *Player) applyLayer(op ir.ShowOp) {
	page := op.Page
	if page == "" {
		page = "fore" // KAG 默认：image/layopt/position 不写 page 就是表ページ（立即生效）
	}
	m := p.fore
	if page == "back" {
		m = p.back
	}
	st, ok := m[op.Layer]
	if !ok {
		st = &LayerState{Layer: op.Layer, Opacity: 1}
		m[op.Layer] = st
	}
	if st.Attrs == nil {
		st.Attrs = map[string]string{}
	}
	switch op.Kind {
	case ir.ShowLayerShow:
		st.Visible = true
		if op.Ref != "" {
			st.Ref = op.Ref
			st.Resolved = ResolveRef(p.ev, op.Ref)
		}
	case ir.ShowLayerHide:
		st.Visible = false
	case ir.ShowLayerOpt:
		// [layopt] 可带 visible；不带 visible 的 layopt（如只改 left）不动可见性
		if _, has := op.Attr["visible"]; has {
			st.Visible = truthy(op.Attr["visible"])
		}
	case ir.ShowLayerAttr:
		if _, has := op.Attr["visible"]; has { // [position ... visible=true]
			st.Visible = truthy(op.Attr["visible"])
		}
	}
	p.applyAttrs(st, op.Attr)
	p.ensureOrder(op.Layer)
}

// ensureOrder 记住层登记顺序（无 z 信息时稳定排序用）。
func (p *Player) ensureOrder(id string) {
	for _, v := range p.order {
		if v == id {
			return
		}
	}
	p.order = append(p.order, id)
}

// applyAttrs 把脚本属性写进层状态（「有才覆盖」，不编造 0）。
func (p *Player) applyAttrs(st *LayerState, attrs map[string]string) {
	if len(attrs) == 0 {
		return
	}
	for k, v := range attrs {
		st.Attrs[strings.ToLower(k)] = v
	}
	if v, ok := floatAttr(attrs, "left"); ok {
		st.X, st.HasPos = v, true
	}
	if v, ok := floatAttr(attrs, "top"); ok {
		st.Y, st.HasPos = v, true
	}
	// 消息层：opacity = frameOpacity（底色不透明度），不改图层 alpha。
	// 图像层：opacity = 图层 alpha（0~255，255=不透明）。
	if v, ok := floatAttr(attrs, "opacity"); ok && !isMessageLayer(st.Layer) {
		st.Opacity = v / 255
	}
	// 裁剪：clipleft/cliptop/clipwidth/clipheight（宽高齐全才算裁剪生效）
	cl, hasL := floatAttr(attrs, "clipleft")
	ct, hasT := floatAttr(attrs, "cliptop")
	cw, hasW := floatAttr(attrs, "clipwidth")
	ch, hasH := floatAttr(attrs, "clipheight")
	if hasL || hasT || hasW || hasH {
		st.HasClip = hasW && hasH
		if hasL {
			st.ClipL = cl
		}
		if hasT {
			st.ClipT = ct
		}
		if hasW {
			st.ClipW = cw
		}
		if hasH {
			st.ClipH = ch
		}
	}
}

// finishTrans 转场走完：back 页翻到前台。
func (p *Player) finishTrans() {
	if p.transDur <= 0 {
		return
	}
	if p.now >= p.transStart+p.transDur {
		p.fore = deepCopy(p.back)
		p.transFrom = nil
		p.transDur = 0
	}
}

// Frame 当前的画面状态（只读）。
func (p *Player) Frame() Frame {
	f := Frame{Now: p.now, Done: p.done}
	f.Texts = p.textBoxes()

	if p.transDur > 0 {
		prog := float64(p.now-p.transStart) / float64(p.transDur)
		if prog < 0 {
			prog = 0
		}
		if prog > 1 {
			prog = 1
		}
		f.TransProgress = prog
		f.Layers = p.blend(prog)
	} else {
		f.Layers = orderedCopy(p.fore, p.order)
	}
	// 抖动偏移（衰减振荡：count+1 次来回，振幅按 (1-p)² 收敛）
	if p.quakeDur > 0 && p.now < p.quakeStart+p.quakeDur {
		qp := float64(p.now-p.quakeStart) / float64(p.quakeDur)
		amp := 1 - qp
		phase := 2 * math.Pi * float64(p.quakeCount+1) * qp
		f.QuakeY = int(amp * amp * math.Sin(phase) * p.quakeVmax)
		f.QuakeX = int(amp * amp * math.Sin(phase) * p.quakeHmax)
	}
	f.Waiting = p.blockKind == "text" && p.now < p.blockUntil
	return f
}

// textBoxes 汇总所有有文字（或有几何）的消息层，按层名排序（message0 < message1 < …）。
func (p *Player) textBoxes() []TextBox {
	ids := make([]string, 0, len(p.texts))
	for id, s := range p.texts {
		if len(s.lines) > 0 {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	out := make([]TextBox, 0, len(ids))
	for _, id := range ids {
		s := p.texts[id]
		box := TextBox{Layer: id, FontSize: p.opt.FontSize, LineSpacing: p.opt.LineSpacing,
			Visible: true, Lines: append([]TextLine(nil), s.lines...)}
		st := p.fore[id]
		if st == nil {
			st = p.back[id]
		}
		if st == nil {
			// 消息层还没被 [position] 设置过：用 Config.tjs 的默认几何
			box.X, box.Y, box.W, box.H = 32, 32, 1280-64, 960-64
			box.MarginL, box.MarginT, box.MarginR, box.MarginB = 16, 16, 16, 16
			box.HasGeom = true
			box.FrameColor, box.FrameOpacity = p.opt.FrameColor, p.opt.FrameOpacity
			out = append(out, box)
			continue
		}
		box.Visible = st.Visible
		box.X, box.Y = st.X, st.Y
		box.W = fattr(st.Attrs, "width")
		box.H = fattr(st.Attrs, "height")
		box.HasGeom = box.W > 0 && box.H > 0
		box.MarginL = fattrDef(st.Attrs, "marginl", 16)
		box.MarginT = fattrDef(st.Attrs, "margint", 16)
		box.MarginR = fattrDef(st.Attrs, "marginr", 16)
		box.MarginB = fattrDef(st.Attrs, "marginb", 16)
		box.Frame = st.Attrs["frame"]
		box.FrameColor = p.opt.FrameColor
		if c, ok := colorAttr(st.Attrs, "color"); ok {
			box.FrameColor = c
		}
		box.FrameOpacity = p.opt.FrameOpacity
		if v, ok := floatAttr(st.Attrs, "opacity"); ok {
			box.FrameOpacity = clamp255(v)
		}
		out = append(out, box)
	}
	return out
}

// orderedCopy 按登记顺序复制层（未登记的按 id 排序补在后面）。
func orderedCopy(m map[string]*LayerState, order []string) []LayerState {
	out := make([]LayerState, 0, len(m))
	seen := map[string]bool{}
	for _, id := range order {
		if st, ok := m[id]; ok {
			out = append(out, *st)
			seen[id] = true
		}
	}
	rest := make([]string, 0, len(m))
	for id := range m {
		if !seen[id] {
			rest = append(rest, id)
		}
	}
	sort.Strings(rest)
	for _, id := range rest {
		out = append(out, *m[id])
	}
	return out
}

// blend 转场中的画面：from（转场开始时的前台）→ back（目标页）。
//
// 近似说明：KAG 的 [trans] 是逐像素融合；这里按层做插值。同一层换了图
// （Resolved 变化）时画两层互补不透明度（真交叉淡入淡出），没换图则一层内插。
func (p *Player) blend(prog float64) []LayerState {
	from, back := p.transFrom, p.back
	var out []LayerState
	seen := map[string]bool{}
	visit := func(id string) {
		if seen[id] {
			return
		}
		seen[id] = true
		lf, lt := from[id], back[id]
		sameRef := lf != nil && lt != nil && lf.Visible && lt.Visible && lf.Resolved == lt.Resolved
		if lf != nil && lf.Visible {
			if sameRef {
				st := *lt
				st.Opacity = lf.Opacity*(1-prog) + lt.Opacity*prog
				st.X = lf.X*(1-prog) + lt.X*prog
				st.Y = lf.Y*(1-prog) + lt.Y*prog
				st.HasPos = lf.HasPos || lt.HasPos
				out = append(out, st)
			} else {
				st := *lf
				st.Opacity = lf.Opacity * (1 - prog)
				out = append(out, st)
			}
		}
		if lt != nil && lt.Visible && !sameRef {
			st := *lt
			st.Opacity = lt.Opacity * prog
			out = append(out, st)
		}
	}
	for _, id := range p.order {
		visit(id)
	}
	rest := make([]string, 0)
	for id := range from {
		if !seen[id] {
			rest = append(rest, id)
		}
	}
	for id := range back {
		if !seen[id] {
			rest = append(rest, id)
		}
	}
	sort.Strings(rest)
	for _, id := range rest {
		visit(id)
	}
	return out
}

// deepCopy 复制一页的层状态（KAG 的 backlay/转场快照都要深拷贝）。
func deepCopy(m map[string]*LayerState) map[string]*LayerState {
	out := make(map[string]*LayerState, len(m))
	for k, v := range m {
		cp := *v
		out[k] = &cp
	}
	return out
}

// ResolveRef 把 KAG 的 storage 名对到事件资产表里的相对路径。
//
// 与 replay.resolveRef 同一口径（精确 → 去扩展名 → 文件名去扩展名），
// 匹配不上原样返回（交给 Loader 碰运气，不编造路径）。
func ResolveRef(ev *ir.Event, ref string) string {
	if ev == nil || ref == "" {
		return ref
	}
	slash := func(s string) string { return strings.ToLower(strings.ReplaceAll(s, "\\", "/")) }
	stripExt := func(s string) string {
		if i := strings.LastIndex(s, "."); i > 0 {
			return s[:i]
		}
		return s
	}
	base := func(s string) string {
		if i := strings.LastIndex(s, "/"); i >= 0 {
			return s[i+1:]
		}
		return s
	}
	norm := slash(ref)
	paths := make([]string, 0, len(ev.Assets))
	for _, a := range ev.Assets {
		if a.OrigPath != "" {
			paths = append(paths, a.OrigPath)
		}
	}
	for _, p := range paths {
		if slash(p) == norm {
			return p
		}
	}
	for _, p := range paths {
		if stripExt(slash(p)) == norm {
			return p
		}
	}
	for _, p := range paths {
		if stripExt(base(slash(p))) == stripExt(norm) {
			return p
		}
	}
	return ref
}

// ---------------------------------------------------------------
// 小工具
// ---------------------------------------------------------------

// truthy KAG 布尔属性（缺省=真；false/0/no/off=假）。
func truthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "false", "0", "no", "off":
		return false
	}
	return true
}

func fattr(m map[string]string, key string) float64 {
	f, _ := floatAttr(m, key)
	return f
}

// fattrDef 取数值属性，缺失时用默认值。
func fattrDef(m map[string]string, key string, def float64) float64 {
	if f, ok := floatAttr(m, key); ok {
		return f
	}
	return def
}

func floatAttr(m map[string]string, key string) (float64, bool) {
	if m == nil {
		return 0, false
	}
	v, ok := m[key]
	if !ok || strings.TrimSpace(v) == "" {
		return 0, false
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
	if err != nil {
		return 0, false
	}
	return f, true
}

func iattr(m map[string]string, key string, def int) int {
	f, ok := floatAttr(m, key)
	if !ok {
		return def
	}
	return int(f)
}

func clamp255(f float64) uint8 {
	if f < 0 {
		return 0
	}
	if f > 255 {
		return 255
	}
	return uint8(f)
}

// colorAttr 解析 0xRRGGBB / 0xRRGGBBAA 形式的颜色属性（KAG 的 color 属性）。
func colorAttr(m map[string]string, key string) ([3]uint8, bool) {
	v, ok := m[key]
	if !ok {
		return [3]uint8{}, false
	}
	v = strings.TrimSpace(strings.ToLower(v))
	v = strings.TrimPrefix(v, "0x")
	v = strings.TrimPrefix(v, "#")
	u, err := strconv.ParseUint(v, 16, 32)
	if err != nil {
		return [3]uint8{}, false
	}
	return [3]uint8{uint8(u >> 16), uint8(u >> 8), uint8(u)}, true
}
