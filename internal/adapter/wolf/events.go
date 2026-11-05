// events.go —— Wolf 公共事件 → IR 场景事件。
//
// 切分策略（与 alice 的「场景函数」同构）：Wolf 的剧情主体在 CommonEvent
// 里按章节组织（塔纳托斯实测：善第1-5章 / 悪第2-4章 / 世間話 等含大段
// Message 的剧情 CE），地图事件多为触发器（3003 个 page 仅 177 条
// Message，且大头是 Debug 图的测试文本）。所以 v1 以「含 Message 的
// CommonEvent」为场景事件单元，地图事件不入库（记录在文档）。
//
// 命令映射：
//
//	Message(101)  → TextLine + text_clear/text/text_wait 三连
//	Picture(150)  → ShowLayerShow（类型 0=file 且非 SystemFile/ 消息底图）
//	Sound(140)    → 路径含 BGM/ → bgm_play；SE/ → se_play
//	其余指令      → 忽略（条件/变量/数据库等运行时语义）
package wolf

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"ucgr/internal/ir"
)

// errNoMessage 无 Message 的系统 CE（调用方跳过）。
var errNoMessage = errors.New("wolf: 无 Message")

// 文本清理：Wolf 的行内宏在回放端无法求值（\cdb 是数据库引用、\cself 是
// 变量、@N 是文本速度标记），原样显示会变成乱码占位。这里做保守清理：
// 只删「无法在回放端呈现的引用」，保留 \n 换行与未知宏（宁滥勿缺）。
var (
	wolfReSpeed = regexp.MustCompile(`(?m)^@\d+`)
	wolfReDB    = regexp.MustCompile(`\\cdb\[\d+:\d+:\d+\]`)
	wolfReSelf  = regexp.MustCompile(`\\cself\[\d+\]`)
	wolfReIcon  = regexp.MustCompile(`\\i\[\d+\]`)
)

// sanitizeWolfText 清理回放端无法呈现的 Wolf 行内宏。
func sanitizeWolfText(s string) string {
	s = wolfReSpeed.ReplaceAllString(s, "")
	s = wolfReDB.ReplaceAllString(s, "")
	s = wolfReSelf.ReplaceAllString(s, "")
	s = wolfReIcon.ReplaceAllString(s, "")
	return s
}

const (
	clockPerCmd = 40
	clockPerMsg = 600
	// stageW/stageH Wolf RPG Editor 的默认舞台（816×624）。
	stageW = 816
	stageH = 624
)

// SceneEventOf 把一个 CommonEvent 构造成 IR 事件。
// 无 Message 时返回 errNoMessage（调用方过滤）。
func SceneEventOf(ce CommonEvent) (*ir.Event, error) {
	ev := &ir.Event{
		EventID: fmt.Sprintf("CE%03d", ce.ID),
		Kind:    ir.KindScene,
		Ord:     ce.ID + 1,
		Title:   firstNonEmpty(ce.Name, ce.Description, evIDOf(ce.ID)),
		Assets:  []ir.AssetRef{},
	}
	clock := int64(0)
	assets := map[string]bool{}
	layers := map[int32]string{}
	first := true

	for _, in := range ce.Commands {
		switch in.Op {
		case opMessage:
			if len(in.Strs) == 0 {
				clock += clockPerCmd
				continue
			}
			text := strings.TrimSpace(sanitizeWolfText(in.Strs[0]))
			if text == "" {
				clock += clockPerCmd
				continue
			}
			if first {
				ev.Show = append(ev.Show, ir.MessageBoxOps("text", stageW, stageH)...)
				first = false
			}
			ev.Texts = append(ev.Texts, ir.TextLine{LineID: lineID(len(ev.Texts) + 1), TMs: clock, Text: text})
			ev.Show = append(ev.Show,
				ir.ShowOp{TMs: clock, Kind: ir.ShowTextClear, Layer: "text"},
				ir.ShowOp{TMs: clock, Kind: ir.ShowTextRun, Layer: "text", Text: &ir.ShowText{Text: text}},
				ir.ShowOp{TMs: clock, Kind: ir.ShowTextWait},
			)
			clock += clockPerMsg
		case opPicture:
			if len(in.Args) == 0 || len(in.Strs) == 0 {
				clock += clockPerCmd
				continue
			}
			name := strings.TrimSpace(in.Strs[0])
			// args[0] 低 4 位起 3 位 = 图片来源类型：0=file、2=text 等；
			// text 型（\i[...] 图标宏）与消息窗口底图对回放无意义，跳过。
			ptype := (in.Args[0] >> 4) & 7
			if ptype != 0 || name == "" || strings.HasPrefix(name, "SystemFile/") {
				clock += clockPerCmd
				continue
			}
			slot := in.Args[1]
			layer, ok := layers[slot]
			if !ok {
				layer = "pic" + strconv.Itoa(int(slot)%1000)
				layers[slot] = layer
			}
			if !assets[name] {
				assets[name] = true
				ev.Assets = append(ev.Assets, ir.AssetRef{OrigPath: name, Role: assetRole(name)})
			}
			ev.Show = append(ev.Show, ir.ShowOp{
				TMs: clock, Kind: ir.ShowLayerShow, Layer: layer, Ref: name,
			})
			clock += clockPerCmd
		case opSound:
			if len(in.Strs) == 0 {
				clock += clockPerCmd
				continue
			}
			name := strings.TrimSpace(in.Strs[0])
			if name == "" || strings.HasPrefix(name, `\`) {
				clock += clockPerCmd
				continue
			}
			kind := "音频"
			var soundKind string
			switch {
			case strings.Contains(name, "BGM/"), strings.Contains(name, "bgm/"):
				soundKind = ir.ShowBGMPlay
				kind = "BGM"
			case strings.Contains(name, "SE/"), strings.Contains(name, "se/"):
				soundKind = ir.ShowSEPlay
				kind = "SE"
			default:
				clock += clockPerCmd
				continue
			}
			if !assets[name] {
				assets[name] = true
				ev.Assets = append(ev.Assets, ir.AssetRef{OrigPath: name, Role: kind})
			}
			ev.Show = append(ev.Show, ir.ShowOp{
				TMs: clock, Kind: soundKind, Layer: "sound", Ref: name,
			})
			clock += clockPerCmd
		default:
			clock += clockPerCmd
		}
	}
	if len(ev.Texts) == 0 {
		return nil, errNoMessage
	}
	if ev.Title == "" {
		ev.Title = truncateRunes(ev.Texts[0].Text, 40)
	}
	ev.StartMs = 0
	ev.EndMs = clock
	return ev, nil
}

// SceneEvents 过滤出「含 Message 的剧情 CommonEvent」并逐个构建 IR 事件。
// 返回 (事件列表, 跳过的系统 CE 数)。
func (c *commonEvents) SceneEvents() ([]ir.Event, int, error) {
	var out []ir.Event
	skipped := 0
	for _, ce := range c.Events {
		ev, err := SceneEventOf(ce)
		if err != nil {
			if err == errNoMessage {
				skipped++
				continue
			}
			return nil, skipped, fmt.Errorf("CE%d %s: %w", ce.ID, ce.Name, err)
		}
		out = append(out, *ev)
	}
	return out, skipped, nil
}

func evIDOf(id int) string { return fmt.Sprintf("CE%03d", id) }

func lineID(ord int) string { return "L" + strconv.Itoa(ord) }

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

// assetRole 按路径猜资产角色。
func assetRole(name string) string {
	switch {
	case strings.Contains(name, "CG/"):
		return "CG"
	case strings.Contains(name, "CharaGraphic/"), strings.Contains(name, "CharaChip/"):
		return "立绘"
	case strings.Contains(name, "Picture/"):
		return "图片"
	case strings.Contains(name, "BGM/"):
		return "BGM"
	case strings.Contains(name, "SE/"):
		return "SE"
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
