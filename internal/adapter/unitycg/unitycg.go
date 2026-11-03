// unitycg —— 把导出器（tools/unity_cg_export.py）产出的 CG 目录变成可回放的演出。
//
// 为什么不复用 VN 的适配器：Unity 的 CG 没有「剧本」，只有素材本身。
// 导出器已经把素材整理成 cg/<CG名>/ 与 cg-index.json，这里只负责
// 「按索引把一部 CG 拼成 ir.Event」——播放器、图层合成、导出全都照旧复用。
//
// 三类 CG 的处理：
//
//	frames —— 一串 PNG，按编号顺序切帧（Unity AnimationClip 逐帧）
//	still  —— 单张静态图
//	spine  —— 骨骼动画：台本只负责时长，画面由前端 Spine WebGL 组件渲染
package unitycg

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"ucgr/internal/ir"
)

// IndexFile 导出器写的索引文件名。
const IndexFile = "cg-index.json"

// Index 导出器产出的索引（只取这里用得上的字段）。
type Index struct {
	Schema   int      `json:"schema"`
	Engine   string   `json:"engine"`
	Game     string   `json:"game"`
	Counts   Counts   `json:"counts"`
	CGs      []CG     `json:"cgs"`
	Problems []string `json:"problems,omitempty"`
}

// Counts 数量汇总（日志/诊断用）。
type Counts struct {
	CG     int `json:"cg"`
	PNG    int `json:"png"`
	Text   int `json:"text"`
	Spine  int `json:"spine"`
	Frames int `json:"frames"`
	Still  int `json:"still"`
}

// CG 一部 CG 的记录。
type CG struct {
	ID         string   `json:"id"`
	Kind       string   `json:"kind"` // spine | frames | still
	Category   string   `json:"category"`
	EventGroup string   `json:"event_group"`
	Characters []string `json:"characters"`
	Files      struct {
		PNG  []string `json:"png"`
		Text []string `json:"text"`
	} `json:"files"`
	Spine  *SpineInfo `json:"spine"`
	Frames []string   `json:"frames"`
	Bytes  int64      `json:"bytes"`
}

// SpineInfo 骨架统计（渲染器接入后要用）。
type SpineInfo struct {
	Version    string    `json:"version"`
	Bones      int       `json:"bones"`
	Slots      int       `json:"slots"`
	Skins      []string  `json:"skins"`
	Animations []AnimRef `json:"animations"`
	// Error 骨架统计失败的原因（导出器返回 {"error": ...} 时）。
	// 不能只留零值，否则卡片会显示「0 骨骼 0 动画」看不出是失败。
	Error string `json:"error,omitempty"`
}

// AnimRef 一段动画。
type AnimRef struct {
	Name string  `json:"name"`
	Loop bool    `json:"loop"`
	Dur  float64 `json:"dur"`
}

// LoadIndex 读 <root>/cg-index.json。
func LoadIndex(root string) (*Index, error) {
	p := filepath.Join(root, IndexFile)
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, fmt.Errorf("读 CG 索引失败：%w", err)
	}
	var idx Index
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("解析 CG 索引 %s 失败：%w", p, err)
	}
	// 索引来自游戏/导出器，不能直接拼进文件路径：拒绝任何带分隔符或 ..
	// 的 id/文件名，避免 filepath.Join(root, ...) 被带出 root。
	for _, cg := range idx.CGs {
		if !validSegment(cg.ID) {
			return nil, fmt.Errorf("CG 索引里的 id 非法：%q", cg.ID)
		}
		for _, f := range cg.Frames {
			if !validSegment(f) {
				return nil, fmt.Errorf("CG %s 的帧文件名非法：%q", cg.ID, f)
			}
		}
		for _, f := range cg.Files.PNG {
			if !validSegment(f) {
				return nil, fmt.Errorf("CG %s 的 PNG 文件名非法：%q", cg.ID, f)
			}
		}
		for _, f := range cg.Files.Text {
			if !validSegment(f) {
				return nil, fmt.Errorf("CG %s 的文本文件名非法：%q", cg.ID, f)
			}
		}
	}
	return &idx, nil
}

// validSegment 只接受单个路径分量：不含分隔符、盘符或 ..。
func validSegment(s string) bool {
	if s == "" || s == "." || s == ".." {
		return false
	}
	if strings.ContainsAny(s, "/\\:\x00") {
		return false
	}
	return true
}

// Playable 这部 CG 现在能不能放（spine 由前端 WebGL 组件渲染）。
// spine 必须同时有 skeleton/atlas 文本和贴图，否则列出来点开只会报错。
func Playable(cg CG) bool {
	switch cg.Kind {
	case "frames", "still":
		return true
	case "spine":
		return len(cg.Files.Text) > 0 && len(cg.Files.PNG) > 0
	}
	return false
}

var numRe = regexp.MustCompile(`(\d+)`)

// lastNum 取文件名里最后一个数字串。
// CG 名常自带编号（Witch01_2.png），取第一个数字串会让所有帧比较恒等，
// 退化成字典序；末尾数字才是帧序号。
func lastNum(s string) (int, bool) {
	all := numRe.FindAllString(s, -1)
	if len(all) == 0 {
		return 0, false
	}
	n, err := strconv.Atoi(all[len(all)-1])
	return n, err == nil
}

// sortFrames 按文件名里的帧序号排序（1,2,10 而不是 1,10,2）。
func sortFrames(files []string) []string {
	out := append([]string(nil), files...)
	sort.SliceStable(out, func(i, j int) bool {
		a, aok := lastNum(out[i])
		b, bok := lastNum(out[j])
		if aok && bok && a != b {
			return a < b
		}
		return out[i] < out[j]
	})
	return out
}

// FPS 逐帧 CG 的默认帧率。Unity 那边的 AnimationClip 常见 12fps（手绘逐帧），
// 这里先按 12 走，后续可从 AnimationClip 里读真实帧率再覆盖。
const FPS = 12

// BuildShow 把一部 CG 拼成演出台本。
//
// 帧序列的写法：每帧一条 layer_show（Layer 固定 layer0）+ 一条 wait，
// 这样播放器按时间推进就是逐帧动画，不需要新增任何播放逻辑。
func BuildShow(cg CG) (*ir.Event, error) {
	if cg.ID == "" {
		return nil, fmt.Errorf("CG 记录没有 id")
	}
	ev := &ir.Event{EventID: cg.ID, Kind: ir.KindScene, Title: cg.Title()}

	if !validSegment(cg.ID) {
		return nil, fmt.Errorf("CG id 非法：%q", cg.ID)
	}
	if cg.Kind == "spine" {
		// Spine 的画面由前端 canvas 渲染；player 只需要一条等待撑住时间轴，
		// 进度条/播放/暂停/seek 仍复用既有会话。
		dur := cg.SpineDurationMs()
		ev.Show = append(ev.Show, ir.ShowOp{TMs: 0, Kind: ir.ShowWait, DurMs: dur})
		ev.EndMs = dur
		return ev, nil
	}
	frames := cg.Frames
	if len(frames) == 0 && cg.Kind == "still" {
		frames = cg.Files.PNG
	}
	if len(frames) == 0 {
		return nil, fmt.Errorf("%s 没有可播放的帧（kind=%s）", cg.ID, cg.Kind)
	}
	for _, f := range frames {
		if !validSegment(f) {
			return nil, fmt.Errorf("CG %s 的帧文件名非法：%q", cg.ID, f)
		}
	}

	step := int64(1000 / FPS)
	if cg.Kind == "still" {
		step = 2000 // 单图停留 2 秒
	}
	var clock int64
	for _, f := range sortFrames(frames) {
		ev.Show = append(ev.Show,
			ir.ShowOp{
				TMs:   clock,
				Kind:  ir.ShowLayerShow,
				Layer: "layer0",
				Ref:   Ref(cg.ID, f),
			},
			ir.ShowOp{TMs: clock, Kind: ir.ShowWait, DurMs: step},
		)
		clock += step
	}
	ev.EndMs = clock
	return ev, nil
}

// Title 卡片/标题用的名字：分类 + 编号。
func (cg CG) Title() string {
	if cg.Category != "" && cg.Category != "未分类" {
		return cg.Category + " · " + cg.ID
	}
	return cg.ID
}

// SpineAnimations 骨架的动画段（spine 类型才有）。
func (cg CG) SpineAnimations() []AnimRef {
	if cg.Spine == nil {
		return nil
	}
	return cg.Spine.Animations
}

// PickSpineAnimation 选默认播放的动画：优先名字里带 idle 的，否则第一段。
func (cg CG) PickSpineAnimation() (AnimRef, bool) {
	anims := cg.SpineAnimations()
	if len(anims) == 0 {
		return AnimRef{}, false
	}
	for _, a := range anims {
		if strings.Contains(strings.ToLower(a.Name), "idle") {
			return a, true
		}
	}
	return anims[0], true
}

// SpineDurationMs 回放时长：非循环动画按自身时长；循环动画至少给 5 秒，
// 让观众看完整段而不是刚起势就结束。索引缺动画统计时退回 3 秒。
func (cg CG) SpineDurationMs() int64 {
	const (
		minMs  = int64(1000)
		loopMs = int64(5000)
		fallMs = int64(3000)
	)
	a, ok := cg.PickSpineAnimation()
	if !ok || a.Dur <= 0 {
		return fallMs
	}
	ms := int64(a.Dur * 1000)
	if ms < minMs {
		ms = minMs
	}
	if a.Loop && ms < loopMs {
		ms = loopMs
	}
	return ms
}

// Ref 素材引用：相对游戏根的路径，桌面加载器会先按 root+rel 直接找。
// 调用前应已通过 LoadIndex/BuildShow 校验；这里再做一次兜底，非法名
// 不会被拼成可穿越的路径。
func Ref(cgID, file string) string {
	if !validSegment(cgID) {
		cgID = "_"
	}
	if !validSegment(file) {
		file = "_"
	}
	return strings.Join([]string{"cg", cgID, file}, "/")
}
