package kirikiri

// build_show.go —— KiriKiri 脚本 → 演出台本（ir.Event.Show，见 ir/show.go）。
//
// 为什么单独走一遍 Seq：buildEvent 产出的是**扁平时间轴**（给裁剪/分析用），
// 而播放器要的是**顺序执行台本**——KAG 的阅读节奏由「点击阻塞」决定，
// 脚本时钟在台词之间不推进（71kaigan01.ks 的 clock 只被 [wait]/[wt]/[trans] 推动），
// 用时间戳排序播放会丢掉「每行一次点击」的原作节奏。
//
// 阻塞语义在播放器侧实现（internal/player）：
//   - [trans] 只**启动**（非阻塞）；[wt] 才「等转场结束」——若把 trans 当阻塞，
//     `[trans][playse][wt]` 这类写法会把音效推迟一个转场时长。
//   - [wait]/[ws]、[quake]、[l] 为阻塞点。
//   - [r] 换行、[cm]/[np]/[per]/[p] 清页、[l] 等点击，四者共同复原原作阅读节奏。

import (
	"strings"

	"ucgr/internal/ir"
)

// buildShow 顺序遍历 seq 生成演出台本。
func (b *irBuilder) buildShow(seq []SeqItem) []ir.ShowOp {
	var ops []ir.ShowOp
	var clock int64
	curSpeaker := ""
	curMsg := "message0" // [current layer=message5] 可改当前消息层

	add := func(op ir.ShowOp) {
		op.TMs = clock
		ops = append(ops, op)
	}
	addWait := func(ms int64, attr map[string]string) {
		add(ir.ShowOp{Kind: ir.ShowWait, DurMs: ms, Attr: attr})
	}

	for _, it := range seq {
		if it.Kind == "text" {
			spk, body := splitSpeaker(it.Text)
			body = strings.TrimSpace(body)
			if body == "" {
				continue
			}
			if spk == "" {
				spk = curSpeaker
			}
			add(ir.ShowOp{Kind: ir.ShowTextRun, Layer: curMsg,
				Text: &ir.ShowText{Speaker: spk, Text: body}})
			continue
		}
		c := it.Cmd
		tag := strings.ToLower(c.Tag)
		attrs := c.Attrs
		_, isImg := layerCmds[tag]

		switch {
		// ①「[台詞 name=X]」= namae.ks 里的说话人宏。这里**模拟宏展开的效果**
		// （不实现宏引擎）：名字写进 message1（ネームプレート 宏的几何），
		// 台词框换到 message0（frame=win3.png 的窗口框）。
		case c.Tag == "台詞" || tag == "台詞" || tag == "talk":
			name := strings.TrimSpace(attrs["name"])
			if name == "" {
				continue
			}
			curSpeaker = name
			// [macro name=ネームプレート]：名字牌位置 + 半透明黑底关掉（opacity=0）
			add(ir.ShowOp{Kind: ir.ShowLayerAttr, Layer: "message1", Page: "fore",
				Attr: map[string]string{
					"visible": "true", "left": "90", "top": "646",
					"width": "400", "height": "500", "opacity": "0", "margint": "4",
				}})
			// 宏里靠 [r] 换行累加，这里等价地每句重写一次（避免名字牌堆叠）
			add(ir.ShowOp{Kind: ir.ShowTextClear, Layer: "message1"})
			add(ir.ShowOp{Kind: ir.ShowTextRun, Layer: "message1",
				Text: &ir.ShowText{Speaker: name, Text: name}})
			// [macro name=台詞] 尾部：[layopt layer=message0 page=fore visible=true]
			// + [position layer=message0 frame="win3.png" …] + [current layer=message0]
			add(ir.ShowOp{Kind: ir.ShowLayerOpt, Layer: "message0", Page: "fore",
				Attr: map[string]string{"visible": "true"}})
			add(ir.ShowOp{Kind: ir.ShowLayerAttr, Layer: "message0", Page: "fore",
				Attr: map[string]string{
					"frame": "win3.png", "left": "56", "top": "700",
					"width": "1456", "height": "476",
					"margint": "110", "marginl": "50", "marginr": "50", "marginb": "60",
				}})
			curMsg = "message0"

		// ② 图片层命令（image/img/cg/bg/fg…）
		case isImg:
			ref := firstResourceRef(c)
			if ref == "" {
				continue
			}
			kind := ir.ShowLayerShow
			attr := copyAttrs(attrs)
			if !visibleTrue(attrs["visible"]) {
				// base/背景层的 visible 在 KAG 里无意义（始终绘制）：本作 CG 鉴赏正是
				// `[image layer=base storage=… page=back visible=false]` + [trans][wt]，
				// 若照字面隐藏，画廊与 CG 回放会整屏全黑。故对背景层忽略 visible=false。
				if isBgLayer(c) {
					attr["visible"] = "true"
				} else {
					kind = ir.ShowLayerHide
				}
			}
			page := normPage(attrs["page"])
			if page == "" && tag == "img" {
				// [img] 是本作把裁剪图案直接画到屏幕上的宏（出现在 [trans][wt] 之后，
				// 后面没有转场），KAG 默认 page=fore ⇒ 立即显示。
				page = "fore"
			}
			add(ir.ShowOp{Kind: kind, Layer: showLayer(c, tag),
				Page: page, Ref: ref, Attr: attr})

		// ③ 层几何/透明度（[position layer=message5 …]）
		case tag == "position":
			add(ir.ShowOp{Kind: ir.ShowLayerAttr, Layer: showLayer(c, tag),
				Page: normPage(attrs["page"]), Attr: copyAttrs(attrs)})

		// ④ 层可见性（[layopt]/[layon]/[layoff]）
		case tag == "layopt" || tag == "layon" || tag == "layoff":
			add(ir.ShowOp{Kind: ir.ShowLayerOpt, Layer: showLayer(c, tag),
				Page: normPage(attrs["page"]), Attr: copyAttrs(attrs)})

		// ⑤ 双缓冲：backlay（fore → back）
		case tag == "backlay":
			add(ir.ShowOp{Kind: ir.ShowScreenCopy})

		// ⑥ 转场（只启动，不阻塞；[wt] 才等）
		case isTransitionTag(tag):
			d := int64(800)
			if ms, ok := attrMs(c, "time"); ok && ms > 0 {
				d = ms
			}
			add(ir.ShowOp{Kind: ir.ShowTransition, DurMs: d, Attr: copyAttrs(attrs)})

		// ⑦ 画面抖动（阻塞）
		case tag == "quake":
			d := int64(400)
			if ms, ok := attrMs(c, "time"); ok && ms > 0 {
				d = ms
			}
			add(ir.ShowOp{Kind: ir.ShowQuake, DurMs: d, Attr: copyAttrs(attrs)})

		// ⑧ 等待（阻塞）
		case tag == "wait" || tag == "ws":
			ms := int64(800)
			if v, ok := attrMs(c, "time"); ok {
				ms = v
			}
			addWait(ms, nil)

		// ⑧' [wt]「等转场结束」：转场仍在跑就等它，否则按 time（或 0）
		case tag == "wt":
			ms, _ := attrMs(c, "time")
			addWait(ms, map[string]string{"wt": "1"})

		// ⑨ 当前消息层（[current layer=message5]）
		case tag == "current":
			if lid := showLayer(c, tag); lid != "" {
				curMsg = lid
			}

		// ⑩ 文本推进标记：点击等待 / 换行 / 清页
		case tag == "l":
			add(ir.ShowOp{Kind: ir.ShowTextWait})
		case tag == "r":
			add(ir.ShowOp{Kind: ir.ShowTextBreak})
		case tag == "er": // [er] 清当前消息层
			add(ir.ShowOp{Kind: ir.ShowTextClear, Layer: curMsg})
		case tag == "per": // namae.ks 的 per 宏：换回 message0 再 [er]
			curMsg = "message0"
			add(ir.ShowOp{Kind: ir.ShowTextClear, Layer: "message0"})
		case tag == "cm" || tag == "np" || tag == "p":
			add(ir.ShowOp{Kind: ir.ShowTextClear})

		// ⑪ 音频：BGM / 音效
		case tag == "playbgm":
			add(ir.ShowOp{Kind: ir.ShowBGMPlay, Ref: firstResourceRef(c), Attr: copyAttrs(attrs)})
		case tag == "fadeinbgm":
			ms, _ := attrMs(c, "time")
			add(ir.ShowOp{Kind: ir.ShowBGMFadeIn, Ref: firstResourceRef(c), DurMs: ms, Attr: copyAttrs(attrs)})
		case tag == "fadeoutbgm":
			ms, _ := attrMs(c, "time")
			add(ir.ShowOp{Kind: ir.ShowBGMFadeOut, DurMs: ms, Attr: copyAttrs(attrs)})
		case tag == "stopbgm":
			add(ir.ShowOp{Kind: ir.ShowBGMStop, Attr: copyAttrs(attrs)})
		case tag == "playse":
			add(ir.ShowOp{Kind: ir.ShowSEPlay, Ref: firstResourceRef(c), Attr: copyAttrs(attrs)})
		case tag == "fadeinse":
			ms, _ := attrMs(c, "time")
			add(ir.ShowOp{Kind: ir.ShowSEFadeIn, Ref: firstResourceRef(c), DurMs: ms, Attr: copyAttrs(attrs)})
		case tag == "fadeoutse":
			ms, _ := attrMs(c, "time")
			add(ir.ShowOp{Kind: ir.ShowSEFadeOut, DurMs: ms, Attr: copyAttrs(attrs)})
		case tag == "stopse":
			add(ir.ShowOp{Kind: ir.ShowSEStop, Attr: copyAttrs(attrs)})
		case tag == "vo" || tag == "playvo":
			add(ir.ShowOp{Kind: ir.ShowVoicePlay, Ref: voiceRef(c), Attr: copyAttrs(attrs)})

			// 其余（clickskip/sysbutton/s/syb/eval/if/jump/call/@宏/未识别）不进台本：
			// 台本只记「怎么演」，控制流与宏由适配器（本文件）消化。
		}

		// 时钟：仅 [wait]/[ws]/[wt] 推进（与 buildEvent 口径一致，供进度显示用）
		switch tag {
		case "wait", "ws":
			if v, ok := attrMs(c, "time"); ok {
				clock += v
			} else {
				clock += 800
			}
		case "wt":
			if v, ok := attrMs(c, "time"); ok {
				clock += v
			}
		case "quake":
			if v, ok := attrMs(c, "time"); ok {
				clock += v
			}
		}
	}
	return ops
}

// normPage 归一化 page 属性（未写/非法 → 空，由播放器按 KAG 默认处理）。
// showLayer 与 layerSlot 同口径，但**消息层保留原名**（message0/message5…）。
// layerSlot 把「layer=message5」规范成「layermessage5」（IR 图层表沿用它），
// 播放器里消息层要看前缀判断「opacity 属性其实是 frameOpacity」，故此处不改名。
func showLayer(c *Command, tag string) string {
	if v := strings.TrimSpace(c.Attrs["layer"]); strings.HasPrefix(strings.ToLower(v), "message") {
		return v
	}
	return layerSlot(c, tag)
}

// normPage 规范页名：fore/back，其余（含空）返回空串，由播放器按 KAG 默认（fore）处理。
func normPage(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "fore":
		return "fore"
	case "back":
		return "back"
	}
	return ""
}

// visibleTrue KAG 的 visible 属性（缺省=真；false/0/no/off=假；数值非零为真）。
func visibleTrue(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "false", "0", "no", "off":
		return false
	}
	return true
}

// isBgLayer 目标层是否为「背景层」（base/stage/back/background，或 layer=0）。
// KAG 里背景层始终绘制，其 visible 属性无意义——本作 CG 鉴赏正是靠
// `[image layer=base storage=… page=back visible=false]` 把 CG 直接铺在背景层上。
func isBgLayer(c *Command) bool {
	if c == nil {
		return false
	}
	v := strings.ToLower(strings.TrimSpace(c.Attrs["layer"]))
	switch v {
	case "base", "stage", "back", "background":
		return true
	}
	if v == "" {
		return false
	}
	for i := 0; i < len(v); i++ {
		if v[i] != '0' {
			return false
		}
	}
	return true
}

// copyAttrs 复制命令属性（台本原样保留脚本写法，便于回放时取几何/时长）。
func copyAttrs(m map[string]string) map[string]string {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
