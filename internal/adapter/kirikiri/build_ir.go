package kirikiri

// build_ir.go —— KiriKiri 脚本 → IR 构建（P1：事件自动切分）。
//
// 当前范围（2026-10-17，最小剪枝 §7.5.5 方式 A 之后的自然延伸）：
//   - 事件切分：以 CG 显示/隐藏为边界（show → hide 配对），
//     无 CG 的脚本退化为单个 scene 事件（保底可回放）
//   - 每个事件内按命令流保序构建 图层/时间轴/台词/语音/转场/分支/资产
//   - 资源引用经 ctx.Resolver 解析为带哈希的 AssetRef（未注入则哈希为空，
//     ir.Validate 的 V1 会报缺，属预期）
//   - 未识别命令 → ev.Unrecognized 登记（裁剪安全原则：宁多勿缺）
//   - 同一 CG 多次显示 → 事件 ID 加 #N 后缀（回想模式可定位每次演出）

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"ucgr/internal/adapter"
	"ucgr/internal/ir"
)

// 图片层命令 → 层类型（含各游戏自定义宏方言的位置参数形式）。
var layerCmds = map[string]ir.LayerKind{
	"bg":      ir.LayerBackground,
	"cg":      ir.LayerCG,
	"fg":      ir.LayerStand,
	"eg":      ir.LayerStand,
	"fw":      ir.LayerStand,
	"stand":   ir.LayerStand,
	"image":   ir.LayerCG,
	"ev":      ir.LayerCG,
	"evset":   ir.LayerCG,
	"chr":     ir.LayerStand,
	"mface":   ir.LayerEffect,
	"newlay":  ir.LayerEffect,
	"newchar": ir.LayerStand,
	"stage":   ir.LayerBackground,
}

// roleOf 资源角色分类（IR AssetRef.role，回放器按角色决定叠放与字号）。
func roleOf(ref string) string {
	r := strings.ToLower(ref)
	switch {
	case strings.HasPrefix(r, "ev") || strings.Contains(r, "_ev") || strings.Contains(r, "event"):
		return "CG事件图"
	case strings.HasPrefix(r, "bg") || strings.Contains(r, "background"):
		return "背景"
	case strings.HasPrefix(r, "fg") || strings.Contains(r, "chara") || strings.Contains(r, "stand"):
		return "立绘差分"
	case strings.HasPrefix(r, "vo") || strings.Contains(r, "voice"):
		return "语音"
	case strings.HasPrefix(r, "bgm") || strings.Contains(r, "music"):
		return "BGM"
	case strings.HasPrefix(r, "se") || strings.Contains(r, "sound"):
		return "音效"
	case strings.Contains(r, "movie") || strings.Contains(r, "video"):
		return "事件视频"
	}
	return "其它"
}

// ---------------------------------------------------------------
// 事件切分
// ---------------------------------------------------------------

// eventRange 一个事件在 ast.Seq 上的区间 [Start, End)。
type eventRange struct {
	Start, End int
	CGRef      string       // 触发事件的 CG 引用（空 = scene 保底事件）
	Kind       ir.EventKind
}

// isCGHide 判断命令是否是「CG 隐藏」（[EV hide resetcolor] / [cgdel ...]）。
func isCGHide(c *Command) bool {
	if _, ok := layerCmds[strings.ToLower(c.Tag)]; !ok {
		return false
	}
	if v := c.Attrs["hide"]; v != "" {
		return true
	}
	for _, p := range positionalRefs(c) {
		if strings.EqualFold(p, "hide") {
			return true
		}
	}
	return false
}

// segmentRanges 以 CG show→hide 为边界切分事件。
// resolve 用于判断引用是否为 CG 类资源（角色分类走 roleOf）。
func segmentRanges(ast *Ast, resolve func(string) (*ir.AssetRef, bool)) []eventRange {
	var out []eventRange
	open := -1 // 当前事件的 Seq 起点（-1 = 无）
	openCG := ""
	openKind := ir.KindStillCG
	closeEvent := func(end int) {
		if open >= 0 && end > open {
			out = append(out, eventRange{Start: open, End: end, CGRef: openCG, Kind: openKind})
		}
		open = -1
		openCG = ""
	}
	for i, it := range ast.Seq {
		if it.Kind != "cmd" {
			continue
		}
		c := it.Cmd
		tag := strings.ToLower(c.Tag)
		kind, isLayer := layerCmds[tag]
		if !isLayer {
			continue
		}
		if isCGHide(c) {
			// 隐藏当前层对应的 CG：收尾已打开的事件
			closeEvent(i + 1)
			continue
		}
		ref := firstResourceRef(c)
		if ref == "" {
			continue
		}
		role := roleOf(ref)
		if resolve != nil {
			if ar, ok := resolve(ref); ok && ar.Role != "" && ar.Role != "其它" {
				role = ar.Role
			}
		}
		if role != "CG事件图" && role != "事件视频" {
			continue // 背景/立绘不算 CG 事件边界
		}
		// 新的 CG 显示：同一 CG 重复 show 忽略；否则先收尾上一个再开新事件
		if open >= 0 && ref == openCG {
			continue
		}
		closeEvent(i)
		open = i
		openCG = ref
		openKind = ir.KindStillCG
		if role == "事件视频" || kind == ir.LayerMovie {
			openKind = ir.KindEventVideo
		}
	}
	closeEvent(len(ast.Seq))
	return out
}

// ---------------------------------------------------------------
// 事件构建
// ---------------------------------------------------------------

// irBuilder 事件构建器（跨事件共享资产去重与解析器）。
type irBuilder struct {
	ctx     *adapter.BuildCtx
	resolve func(string) (*ir.AssetRef, bool)
	assets  []ir.AssetRef
	seen    map[string]bool
	reg     *adapter.TransitionRegistry
}

func (b *irBuilder) addAsset(ref string) *ir.AssetRef {
	ar, ok := b.resolve(ref)
	if !ok {
		return nil
	}
	if ar.Role == "" || ar.Role == "其它" {
		ar.Role = roleOf(ref)
	}
	if !b.seen[ar.OrigPath] {
		b.seen[ar.OrigPath] = true
		b.assets = append(b.assets, *ar)
	}
	return ar
}

// speakerRe 台词前缀的角色名：【壮太】「…」/【男１】「…」。
var speakerRe = regexp.MustCompile(`^【([^】]{1,12})】\s*(.*)$`)

// splitSpeaker 从台词行提取说话人（无前缀则返回空）。
func splitSpeaker(s string) (string, string) {
	if m := speakerRe.FindStringSubmatch(s); m != nil {
		return m[1], m[2]
	}
	return "", s
}

// buildEvent 构建单个事件（seq 为 ast.Seq 的子区间）。
func (b *irBuilder) buildEvent(seq []SeqItem, evID string, kind ir.EventKind) *ir.Event {
	ev := &ir.Event{EventID: evID, Kind: kind, Ord: 1}
	layers := map[string]*ir.Layer{}
	var layerOrder []string
	var clock int64
	textN := 0
	var pendingVoice *ir.AssetRef

	slotOf := func(c *Command, tag string) string {
		for _, k := range []string{"layer", "id", "page"} {
			if v := c.Attrs[k]; v != "" {
				return tag + v
			}
		}
		return tag
	}

	for _, it := range seq {
		if it.Kind == "text" {
			textN++
			id := fmt.Sprintf("L%03d", textN)
			spk, body := splitSpeaker(it.Text)
			tl := ir.TextLine{LineID: id, TMs: clock, Text: body, Speaker: spk}
			// 挂靠此前出现的语音（KAG 的 [nm s=] 在其台词之前）
			if pendingVoice != nil {
				tl.Voice = pendingVoice
				ev.Voices = append(ev.Voices, ir.Voice{LineID: id, Asset: *pendingVoice})
				ev.Timeline = append(ev.Timeline, ir.TimelineOp{
					TMs: clock, Op: "voice_play", Target: id,
				})
				pendingVoice = nil
			}
			ev.Texts = append(ev.Texts, tl)
			ev.Timeline = append(ev.Timeline, ir.TimelineOp{
				TMs: clock, Op: "text_show", Target: id,
			})
			continue
		}
		c := it.Cmd
		tag := strings.ToLower(c.Tag)

		// ① 图片层命令
		if lkind, isLayer := layerCmds[tag]; isLayer {
			ref := firstResourceRef(c)
			if ref == "" {
				continue
			}
			ar := b.addAsset(ref)
			lid := slotOf(c, tag)
			l, exists := layers[lid]
			if !exists {
				l = &ir.Layer{LayerID: lid, Kind: lkind, Z: len(layerOrder) + 1, Visible: true}
				layers[lid] = l
				layerOrder = append(layerOrder, lid)
			}
			if ar != nil {
				l.Asset = ar
			}
			ev.Timeline = append(ev.Timeline, ir.TimelineOp{
				TMs: clock, Op: "image_show", Target: lid,
				Params: map[string]interface{}{"ref": ref},
			})
			continue
		}

		// ② 等待：推进时钟
		if tag == "wait" || tag == "ws" {
			if ms, ok := attrMs(c, "time"); ok {
				clock += ms
			} else {
				clock += 800
			}
			continue
		}

		// ③ 转场：查注册表，未实现回退 L0 fade
		if isTransitionTag(tag) {
			nativeID := transitionNativeID(tag)
			d := int64(800)
			if ms, ok := attrMs(c, "time"); ok && ms > 0 {
				d = ms
			}
			tr := ir.Transition{
				TransitionID: fmt.Sprintf("T%d", len(ev.Transitions)+1),
				Impl:         "fade", DurationMs: d, Fallback: true,
			}
			if impl, ok := b.reg.Impls[nativeID]; ok {
				tr.Impl = impl.ImplKey
				if _, hasTime := attrMs(c, "time"); !hasTime && impl.DurationMs > 0 {
					tr.DurationMs = impl.DurationMs
				}
				tr.Fallback = false
			}
			ev.Transitions = append(ev.Transitions, tr)
			clock += tr.DurationMs / 2
			continue
		}

		// ④ 语音：vo / [nm s=] / [playvo]（KAG 方言里 nm 在其台词之前）
		if vr := voiceRef(c); vr != "" {
			ar := b.addAsset(vr)
			if ar == nil {
				continue
			}
			pendingVoice = ar
			continue
		}

		// ⑤ 其它音视频引用（se/bgm/movie 等）
		if ref := firstResourceRef(c); ref != "" && isAudioVideoTag(tag) {
			b.addAsset(ref)
			continue
		}

		// ⑥ 未识别命令：登记后继续（不中断）
		u := ir.Unrecognized{
			NodePath: fmt.Sprintf("cmd[%s]", tag),
			Raw:      c.Raw,
			Reason:   "未实现的命令语义",
		}
		ev.Unrecognized = append(ev.Unrecognized, u)
		if b.ctx.RecordUnrecognized != nil {
			b.ctx.RecordUnrecognized(u)
		}
	}

	for _, lid := range layerOrder {
		ev.Layers = append(ev.Layers, *layers[lid])
	}
	// 分支：jump/call/next 目标登记 default（重演非重玩：沿默认走）
	for _, it := range seq {
		if it.Kind != "cmd" {
			continue
		}
		if t := strings.ToLower(it.Cmd.Tag); t == "jump" || t == "call" || t == "next" {
			if tgt := it.Cmd.Attrs["storage"]; tgt != "" {
				ev.Branches = append(ev.Branches, ir.Branch{
					BranchID:      fmt.Sprintf("B%d", len(ev.Branches)+1),
					Type:          "jump",
					DefaultTarget: tgt,
				})
			}
		}
	}
	return ev
}

// BuildIR 从 Ast 构建 IR（CG 事件自动切分版）。
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
			return &ir.AssetRef{Hash: "", OrigPath: ref, Role: roleOf(ref)}, true
		}
	}
	b := &irBuilder{
		ctx: ctx, resolve: resolve,
		seen: map[string]bool{}, reg: a.Transitions(),
	}

	ranges := segmentRanges(ast, resolve)
	if len(ranges) == 0 {
		// 无 CG 事件：整脚本作为一个 scene 事件（保底，回放仍可看台词）
		ranges = []eventRange{{Start: 0, End: len(ast.Seq), Kind: ir.KindScene}}
	}

	out := &ir.IR{
		IRVersion: ir.IRVersion,
		GameID:    ctx.GameID,
		EngineID:  "kirikiri",
		Fidelity:  ir.FidelityGeneric,
		Meta: ir.Meta{
			ScriptEncoding: ast.Encoding,
			GeneratedBy:    "ucgr/kirikiri.BuildIR v2 (event-segmented)",
			GeneratedAt:    time.Now().Format(time.RFC3339),
		},
	}
	scriptName := ""
	if n := ctx.Config["script_name"]; n != "" {
		scriptName = strings.TrimSuffix(filepath.Base(n), filepath.Ext(n))
	}
	cgCount := map[string]int{}
	for i, r := range ranges {
		evID := scriptName
		if evID == "" {
			evID = fmt.Sprintf("ev%03d", i+1)
		}
		title := evID
		if r.CGRef != "" {
			cgCount[r.CGRef]++
			if cgCount[r.CGRef] > 1 {
				evID = fmt.Sprintf("%s#%d", r.CGRef, cgCount[r.CGRef])
			} else {
				evID = r.CGRef
			}
			title = r.CGRef
		} else if len(ranges) > 1 {
			evID = fmt.Sprintf("%s#%d", scriptName, i+1)
		}
		ev := b.buildEvent(ast.Seq[r.Start:r.End], evID, r.Kind)
		ev.Ord = i + 1
		ev.Title = title
		ev.Assets = b.assets // 包级共享汇总（V1 校验按包哈希集合）
		out.Events = append(out.Events, *ev)
	}
	return out, nil
}

// firstResourceRef 取命令中的第一个资源引用值。
// 先查资源类属性，再查按标签扩展属性，最后兜底位置参数
// （[EV EV0220A ...] 这类方言没有 storage=）。
func firstResourceRef(c *Command) string {
	extra := map[string]bool{}
	for _, k := range tagExtraAttrs[c.Tag] {
		extra[k] = true
	}
	for _, k := range []string{"storage", "file", "src", "sound"} {
		if v := c.Attrs[k]; v != "" && !isDynamic(v) {
			return v
		}
	}
	for k := range extra {
		if v := c.Attrs[k]; v != "" && !isDynamic(v) {
			return v
		}
	}
	for _, p := range positionalRefs(c) {
		if !numericRe.MatchString(p) {
			return p
		}
	}
	return ""
}

// voiceRef 取命令中的语音引用（vo 的 file / nm 的 s）。
func voiceRef(c *Command) string {
	switch strings.ToLower(c.Tag) {
	case "vo", "playvo", "playse":
		if v := c.Attrs["file"]; v != "" && !isDynamic(v) {
			return v
		}
		if v := c.Attrs["storage"]; v != "" && !isDynamic(v) {
			return v
		}
	case "nm", "name":
		if v := c.Attrs["s"]; v != "" && !isDynamic(v) {
			return v
		}
	}
	return ""
}

// isDynamic 判断引用是否为动态值（%参数|默认值 已由调用方处理，这里挡 &/$）。
func isDynamic(v string) bool {
	return strings.HasPrefix(v, "&") || strings.HasPrefix(v, "$")
}

// attrMs 解析毫秒属性（time=1000 / time=1s）。
func attrMs(c *Command, key string) (int64, bool) {
	v := c.Attrs[key]
	if v == "" {
		return 0, false
	}
	if strings.HasSuffix(v, "s") && !strings.HasSuffix(v, "ms") {
		if f, err := strconv.ParseFloat(strings.TrimSuffix(v, "s"), 64); err == nil {
			return int64(f * 1000), true
		}
	}
	if n, err := strconv.ParseInt(v, 10, 64); err == nil {
		return n, true
	}
	return 0, false
}

func isTransitionTag(tag string) bool {
	switch tag {
	case "ex", "extrans", "trans", "fade", "fadein", "fadeout", "crossfade",
		"blind", "wipe", "endtrans", "begintrans", "暗転", "黑屏", "白幕":
		return true
	}
	return false
}

// transitionNativeID 命令 → 引擎原生转场 id（查 Transitions 注册表）。
func transitionNativeID(tag string) string {
	switch tag {
	case "extrans":
		return "crossfade"
	case "blind":
		return "blind"
	case "fade", "fadein", "fadeout":
		return "fade"
	}
	return tag
}

func isAudioVideoTag(tag string) bool {
	switch tag {
	case "se", "bgm", "movie", "video", "playbgm", "stopbgm", "playse", "sysmovie":
		return true
	}
	return false
}
