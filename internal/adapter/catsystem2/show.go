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
			ops = append(ops, ir.ShowOp{TMs: op.TMs, Kind: ir.ShowLayerHide, Layer: op.Target})
		}
	}
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
