// show.go —— CatSystem2 的演出台本（播放器要的 ir.Event.Show）。
//
// 和 Artemis 一样，CS2 适配器产的是通用 Timeline（本适配器只发 image_show），
// 播放器解释的是顺序台本 Show。这里做一次有损但忠实的转换：
//
//   - image_show → layer_show（带 ref 与几何属性，time 作阻塞时长）
//   - 文本行      → text + text_wait（点一下推进，语义对齐原作）
//
// 文本与图像按脚本时钟（TMs）归并，保持原作先后。
package catsystem2

import (
	"fmt"
	"strconv"

	"ucgr/internal/adapter"
	"ucgr/internal/ir"
)

// msgLayer 消息层名（与其它引擎一致）。
const msgLayer = "message0"

// BuildShowScript 解析一份 .cst 并构建成可播放的演出事件（带 Show 台本）。
func BuildShowScript(data []byte, name string) (*ir.Event, error) {
	a := &Adapter{}
	obj, err := a.ParseScript(data, &adapter.ScriptCtx{EntryPath: name})
	if err != nil {
		return nil, err
	}
	out, err := a.BuildIR(obj, &adapter.BuildCtx{})
	if err != nil {
		return nil, err
	}
	if len(out.Events) == 0 {
		return nil, fmt.Errorf("catsystem2: %s 没有可回放的演出", name)
	}
	ev := out.Events[0]
	ev.Show = BuildShowFromEvent(&ev)
	return &ev, nil
}

// BuildShowFromEvent 把 CS2 的 IR（Timeline + Texts）转成顺序台本。
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
			ops = append(ops, ir.ShowOp{TMs: t.TMs, Kind: ir.ShowVoicePlay, Ref: t.Voice.OrigPath})
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
			pm, _ := op.Params.(map[string]interface{})
			ref, _ := pm["ref"].(string)
			ops = append(ops, ir.ShowOp{
				TMs:   op.TMs,
				Kind:  ir.ShowLayerShow,
				Layer: op.Target,
				Ref:   ref,
				Attr:  cs2Attrs(pm),
				DurMs: durFromParams(pm),
			})
		case "image_hide":
			ops = append(ops, ir.ShowOp{TMs: op.TMs, Kind: ir.ShowLayerHide, Layer: op.Target})
		case "layer_set", "layer_fade":
			pm, _ := op.Params.(map[string]interface{})
			ops = append(ops, ir.ShowOp{
				TMs:   op.TMs,
				Kind:  ir.ShowLayerAttr,
				Layer: op.Target,
				Attr:  cs2Attrs(pm),
				DurMs: durFromParams(pm),
			})
		}
	}
	for ; ti < len(texts); ti++ {
		emitText(texts[ti])
	}
	return ops
}

// cs2Attrs 把 CS2 的参数字典转成播放器属性。
//
// CS2 的 opacity/from/to 是 0~1（FelineSystem2 的 alpha），播放器统一按
// 0~255 解释；转换侧在这里乘 255，避免改掉其它引擎（KAG）的口径。
func cs2Attrs(params map[string]interface{}) map[string]string {
	attr := map[string]string{}
	for k, v := range params {
		switch k {
		case "ref", "time", "dur", "ms", "duration", "duration_ms":
			continue
		case "opacity", "from", "to":
			if f, ok := numFromAny(v); ok && f >= 0 && f <= 1 {
				attr[k] = strconv.FormatFloat(f*255, 'f', -1, 64)
				continue
			}
		}
		attr[k] = fmt.Sprint(v)
	}
	return attr
}

func numFromAny(v interface{}) (float64, bool) {
	switch t := v.(type) {
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case float64:
		return t, true
	case float32:
		return float64(t), true
	}
	return 0, false
}

// durFromParams 从属性里取时长（time/dur/ms/duration_ms，毫秒）。
func durFromParams(params map[string]interface{}) int64 {
	if params == nil {
		return 0
	}
	for _, k := range []string{"duration_ms", "time", "dur", "ms", "duration"} {
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
