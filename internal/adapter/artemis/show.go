// show.go —— Artemis 的演出台本（播放器要的是 ir.Event.Show）。
//
// Artemis 适配器产出的 IR 用的是**通用 Timeline**（image_show / image_hide +
// 文本行），而播放器解释的是**顺序台本** ir.Event.Show（KiriKiri 那一套）。
// 这里做一次有损但忠实的转换：
//
//   - image_show → layer_show（带 ref 与几何属性，time 属性作为阻塞时长）
//   - image_hide → layer_hide
//   - 文本行      → text + text_wait（点一下推进，语义对齐 VN 原作）
//
// 文本与图像按脚本时钟（TMs）归并，保持原作的先后顺序。
package artemis

import (
	"fmt"
	"strconv"

	"ucgr/internal/ir"
)

// msgLayer 消息层名（与 kirikiri 一致：默认消息层 message0）。
const msgLayer = "message0"

// BuildShowFromEvent 把 Artemis 的 IR（Timeline + Texts + Voices）转成顺序台本。
func BuildShowFromEvent(ev *ir.Event) []ir.ShowOp {
	if ev == nil {
		return nil
	}
	var ops []ir.ShowOp
	texts := ev.Texts
	ti := 0

	emitText := func(t ir.TextLine) {
		if t.Text == "" {
			return
		}
		ops = append(ops, ir.ShowOp{
			TMs:   t.TMs,
			Kind:  ir.ShowTextRun,
			Layer: msgLayer,
			Text:  &ir.ShowText{Speaker: t.Speaker, Text: t.Text},
		})
		if t.Voice != nil && t.Voice.OrigPath != "" {
			ops = append(ops, ir.ShowOp{
				TMs:  t.TMs,
				Kind: ir.ShowVoicePlay,
				Ref:  t.Voice.OrigPath,
			})
		}
		// 点一下推进（原作台词是阻塞的）。
		ops = append(ops, ir.ShowOp{TMs: t.TMs, Kind: ir.ShowTextWait, Layer: msgLayer})
	}
	flushTexts := func(before int64) {
		for ti < len(texts) && texts[ti].TMs <= before {
			emitText(texts[ti])
			ti++
		}
	}

	for _, op := range ev.Timeline {
		flushTexts(op.TMs)
		switch op.Op {
		case "image_show":
			// Params 是 interface{}：artemis 那边放的是 map[string]interface{}。
			pm, _ := op.Params.(map[string]interface{})
			ref, _ := pm["ref"].(string)
			attr := map[string]string{}
			for k, v := range pm {
				if k == "ref" {
					continue
				}
				attr[k] = fmt.Sprint(v)
			}
			ops = append(ops, ir.ShowOp{
				TMs:   op.TMs,
				Kind:  ir.ShowLayerShow,
				Layer: op.Target,
				Ref:   ref,
				Attr:  attr,
				DurMs: durFromParams(pm),
			})
		case "image_hide":
			ops = append(ops, ir.ShowOp{
				TMs:   op.TMs,
				Kind:  ir.ShowLayerHide,
				Layer: op.Target,
			})
		}
	}
	// 末尾剩余的文本。
	for ; ti < len(texts); ti++ {
		emitText(texts[ti])
	}
	return ops
}

// durFromParams 从属性里取时长（time/dur/ms，毫秒）。
func durFromParams(params map[string]interface{}) int64 {
	if params == nil {
		return 0
	}
	for _, k := range []string{"time", "dur", "ms", "duration"} {
		v, ok := params[k]
		if !ok {
			continue
		}
		switch t := v.(type) {
		case int:
			return int64(t)
		case int64:
			return t
		case float64:
			return int64(t)
		case string:
			if n, err := strconv.ParseInt(t, 10, 64); err == nil {
				return n
			}
		}
	}
	return 0
}
