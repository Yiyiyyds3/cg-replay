package kirikiri

// build_ir.go —— KiriKiri 脚本 → IR 构建（P1 第一步，S3 剪枝用）。
//
// 当前范围（2026-10-17，最小剪枝 §7.5.5 方式 A）：
//   - 一个脚本 = 一个事件（event_id 取脚本名）；自动事件切分留待后续
//   - 图层/时间轴/台词/语音/转场按命令流保序构建
//   - 资源引用经 ctx.Resolver 解析为带哈希的 AssetRef（未注入则哈希为空，
//     ir.Validate 的 V1 会报缺，属预期）
//   - 未识别命令 → RecordUnrecognized 登记（裁剪安全原则：宁多勿缺）

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

// BuildIR 从 Ast 构建 IR（单事件版）。
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

	ev := &ir.Event{
		EventID: "script",
		Kind:    ir.KindScene,
		Ord:     1,
		Title:   "",
	}
	layers := map[string]*ir.Layer{} // layerID → 图层（可变状态）
	var layerOrder []string          // 保持首次出现顺序
	var assets []ir.AssetRef
	assetSeen := map[string]bool{}
	var clock int64 // ms
	textN := 0
	lastTextIdx := -1        // 最近一条 TextLine 下标
	var pendingVoice *ir.AssetRef // 待挂靠语音（KAG 的 [nm s=] 出现在其台词之前）
	reg := a.Transitions()

	addAsset := func(ref string) *ir.AssetRef {
		ar, ok := resolve(ref)
		if !ok {
			return nil
		}
		if !assetSeen[ar.OrigPath] {
			assetSeen[ar.OrigPath] = true
			assets = append(assets, *ar)
		}
		return ar
	}

	slotOf := func(c *Command, tag string) string {
		// 优先 layer/id 属性，其次 slot（第二参数），最后用 tag 名
		for _, k := range []string{"layer", "id", "page"} {
			if v := c.Attrs[k]; v != "" {
				return tag + v
			}
		}
		return tag
	}

	for _, it := range ast.Seq {
		if it.Kind == "text" {
			textN++
			id := fmt.Sprintf("L%03d", textN)
			spk, body := splitSpeaker(it.Text)
			tl := ir.TextLine{LineID: id, TMs: clock, Text: body, Speaker: spk}
			// 挂靠此前出现的语音（[nm s=…] 在其台词之前）
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
			lastTextIdx = len(ev.Texts) - 1
			continue
		}
		c := it.Cmd
		tag := strings.ToLower(c.Tag)

		// ① 图片层命令
		if kind, isLayer := layerCmds[tag]; isLayer {
			ref := firstResourceRef(c)
			if ref == "" {
				continue // stop/clear 等无引用的层操作
			}
			ar := addAsset(ref)
			lid := slotOf(c, tag)
			l, exists := layers[lid]
			if !exists {
				l = &ir.Layer{LayerID: lid, Kind: kind, Z: len(layerOrder) + 1, Visible: true}
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
				clock += 800 // 默认点击等待
			}
			continue
		}

		// ③ 转场：ex/extrans/trans/fade 家族 → 查注册表，未实现回退 L0 fade
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
			if impl, ok := reg.Impls[nativeID]; ok {
				tr.Impl = impl.ImplKey
				if _, hasTime := attrMs(c, "time"); !hasTime && impl.DurationMs > 0 {
					tr.DurationMs = impl.DurationMs
				}
				tr.Fallback = false // 引擎原生转场（L0 表内）
			}
			ev.Transitions = append(ev.Transitions, tr)
			clock += tr.DurationMs / 2 // 转场占用半时长（近似）
			continue
		}

		// ④ 语音：vo / [nm s=] / [playvo] —— KAG 方言里 nm 在其台词之前，
		// 故记为 pendingVoice，由下一条台词挂靠（见上方 text 分支）。
		if vr := voiceRef(c); vr != "" {
			ar := addAsset(vr)
			if ar == nil {
				continue
			}
			pendingVoice = ar
			if lastTextIdx < 0 {
				// 语音前没有台词（如旁白先声）：仍登记，LineID 留空
				ev.Voices = append(ev.Voices, ir.Voice{LineID: "", Asset: *ar})
			}
			continue
		}

		// ⑤ 其它资源引用（se/bgm/movie 等）
		if ref := firstResourceRef(c); ref != "" && isAudioVideoTag(tag) {
			addAsset(ref)
			continue
		}

		// ⑥ 未识别命令：登记后继续（不中断）
		u := ir.Unrecognized{
			NodePath: fmt.Sprintf("cmd[%s]", tag),
			Raw:      c.Raw,
			Reason:   "未实现的命令语义",
		}
		ev.Unrecognized = append(ev.Unrecognized, u)
		if ctx.RecordUnrecognized != nil {
			ctx.RecordUnrecognized(u)
		}
	}

	// 汇总图层（保序）
	for _, lid := range layerOrder {
		ev.Layers = append(ev.Layers, *layers[lid])
	}
	ev.Assets = assets

	// 分支：遇到 jump/call 目标登记 default（重演非重玩：沿默认走）
	for _, it := range ast.Seq {
		if it.Kind != "cmd" {
			continue
		}
		if t := strings.ToLower(it.Cmd.Tag); t == "jump" || t == "call" || t == "next" {
			if tgt := it.Cmd.Attrs["storage"]; tgt != "" {
				ev.Branches = append(ev.Branches, ir.Branch{
					BranchID: fmt.Sprintf("B%d", len(ev.Branches)+1),
					TMs:      clock, Type: "jump",
					DefaultTarget: tgt,
				})
			}
		}
	}

	out := &ir.IR{
		IRVersion: ir.IRVersion,
		GameID:    ctx.GameID,
		EngineID:  "kirikiri",
		Fidelity:  ir.FidelityGeneric,
		Meta: ir.Meta{
			ScriptEncoding: ast.Encoding,
			GeneratedBy:    "ucgr/kirikiri.BuildIR v1",
			GeneratedAt:    time.Now().Format(time.RFC3339),
		},
		Events: []ir.Event{*ev},
	}
	// 事件标题/ID 用脚本名（ctx.Config["script_name"] 传入）
	if n := ctx.Config["script_name"]; n != "" {
		out.Events[0].EventID = strings.TrimSuffix(filepath.Base(n), filepath.Ext(n))
		out.Events[0].Title = out.Events[0].EventID
	}
	return out, nil
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

// firstResourceRef 取命令中的第一个资源引用值。
// 先查资源类属性，再查按标签扩展属性，最后兜底位置参数
// （[EV EV0220A ...] 这类方言没有 storage=）。
func firstResourceRef(c *Command) string {
	extra := map[string]bool{}
	for _, k := range tagExtraAttrs[c.Tag] {
		extra[k] = true
	}
	// 稳定顺序：先 storage/file/src/sound，再扩展属性
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
