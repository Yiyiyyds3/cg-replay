// events.go —— 场景函数 → IR 事件。
//
// System 4 脚本的「原作自己的事件分割」就是**函数**：兰斯10 的剧情/立绘/CG
// 函数用人类可读的中文/日文命名（「萨娜琪亚／参战」「t０５／洞窟中的兰斯
// 等人２」「◎ＭＡＰ制御「２０１／进入鱿鱼山」」），系统函数是 ASCII 名
// （parts::NumofConstructProcess 之类）。按「函数名含 CJK」筛选场景函数，
// 函数内反汇编出 S_PUSH（立绘/CG 名）与 MSG（台词）序列。
//
// 一个场景函数 = 一个场景事件（EventID 用函数名，与库清单一一对应）。
package alice

import (
	"strings"

	"ucgr/internal/ir"
)

const (
	clockPerCmd = 40
	clockPerMsg = 600
)

// SceneEventOf 把一个场景函数构造成 IR 事件（1024×768 舞台）。
func (a *AIN) SceneEventOf(fnIdx int) (*ir.Event, error) {
	if fnIdx < 0 || fnIdx >= len(a.Funcs) {
		return nil, errFuncRange
	}
	fn := &a.Funcs[fnIdx]
	instrs := a.DisasmFunc(fn.Address)
	if len(instrs) == 0 {
		return nil, errEmptyFunc
	}

	ev := &ir.Event{
		EventID: fn.Name,
		Kind:    ir.KindScene,
		Ord:     fnIdx + 1,
		Title:   fn.Name,
		Assets:  []ir.AssetRef{},
	}
	clock := int64(0)
	textOrd := 0
	assets := map[string]bool{}
	layers := map[string]string{} // 资产 stem → 层 id（同名 S_PUSH 覆盖同层）
	var chars []string

	first := true
	for _, in := range instrs {
		switch in.Op {
		case opSPUSH:
			name := strings.TrimSpace(in.Str)
			if name == "" {
				continue
			}
			if !assets[name] {
				assets[name] = true
				ev.Assets = append(ev.Assets, ir.AssetRef{
					OrigPath: name,
					Role:     assetRole(name),
				})
			}
			// 落图层：兰斯10 的立绘是「基础+差分」多部件叠加、CG 直接
			// 全屏展示，S_PUSH 顺序即叠放次序（后 push 盖上面）。
			layer, ok := layers[name]
			if !ok {
				layer = "img" + uitoa(len(layers)+1)
				layers[name] = layer
			}
			ev.Show = append(ev.Show, ir.ShowOp{
				TMs:   clock,
				Kind:  ir.ShowLayerShow,
				Layer: layer,
				Ref:   name,
			})
			clock += clockPerCmd
		case opMSG:
			text := strings.TrimSpace(in.Str)
			if text == "" {
				continue
			}
			textOrd++
			lineID := textLineID(fnIdx, textOrd)
			if first {
				// 对话框几何必须挂在文本层（"text"）上：player 的 textBox
				// 按「文本所在层」取几何，层名不一致就等于没设。
				// 舞台用兰斯10 原生 1920×1080（素材全该尺寸，见 docs/04 §11.4）。
				ev.Show = append(ev.Show, ir.MessageBoxOps("text", 1920, 1080)...)
				first = false
			}
			ev.Texts = append(ev.Texts, ir.TextLine{
				LineID: lineID,
				TMs:    clock,
				Text:   text,
			})
			ev.Show = append(ev.Show,
				ir.ShowOp{TMs: clock, Kind: ir.ShowTextClear, Layer: "text"},
				ir.ShowOp{TMs: clock, Kind: ir.ShowTextRun, Layer: "text",
					Text: &ir.ShowText{Text: text}},
				ir.ShowOp{TMs: clock, Kind: ir.ShowTextWait},
			)
			clock += clockPerMsg
		}
	}
	if len(ev.Texts) == 0 && len(ev.Assets) == 0 {
		return nil, errEmptyFunc
	}
	if len(ev.Texts) > 0 && ev.Title == "" {
		ev.Title = truncateRunes(ev.Texts[0].Text, 40)
	}
	ev.StartMs = 0
	ev.EndMs = clock
	ev.Characters = chars
	return ev, nil
}

func textLineID(fnIdx, ord int) string {
	return "L" + uitoa(ord)
}

func uitoa(v int) string {
	if v == 0 {
		return "0"
	}
	var b [8]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	return string(b[i:])
}

// assetRole 按兰斯10 的资产命名猜角色（「立／」立绘、「事／」事件 CG、
// 「シス／」系统图、卡牌/地图部件归其它）。
func assetRole(name string) string {
	switch {
	case strings.Contains(name, "立／"), strings.HasPrefix(name, "立"):
		return "立绘"
	case strings.Contains(name, "事／"), strings.HasPrefix(name, "事"):
		return "CG"
	case strings.Contains(name, "シス"), strings.Contains(name, "系统"):
		return "系统"
	}
	return "其它"
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// hasCJK 报告字符串是否含中日韩文字（场景函数名的判据）。
func hasCJK(s string) bool {
	for _, r := range s {
		if (r >= 0x2E80 && r <= 0x9FFF) || (r >= 0x3400 && r <= 0x4DBF) ||
			(r >= 0xF900 && r <= 0xFAFF) || (r >= 0xFF00 && r <= 0xFFEF) {
			return true
		}
	}
	return false
}

// SceneFuncs 列出全部场景函数（函数名含 CJK）的下标。
func (a *AIN) SceneFuncs() []int {
	var out []int
	for i := range a.Funcs {
		if hasCJK(a.Funcs[i].Name) {
			out = append(out, i)
		}
	}
	return out
}

// ---- 错误 ----

type ainError string

func (e ainError) Error() string { return string(e) }

const (
	errFuncRange = ainError("alice: 函数下标越界")
	errEmptyFunc = ainError("alice: 函数没有可回放的演出内容")
)

// BuildSceneEvents 解析全部场景函数 → 事件列表（坏函数跳过）。
func (a *AIN) BuildSceneEvents() ([]ir.Event, int) {
	var out []ir.Event
	skipped := 0
	for _, i := range a.SceneFuncs() {
		ev, err := a.SceneEventOf(i)
		if err != nil {
			skipped++
			continue
		}
		out = append(out, *ev)
	}
	return out, skipped
}
