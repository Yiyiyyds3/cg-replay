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
//	spine  —— 骨骼动画，需要 spine runtime 渲染（尚未接入，明确报错而不是黑屏）
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
	return &idx, nil
}

// Playable 这部 CG 现在能不能放（spine 还没接渲染）。
func Playable(cg CG) bool { return cg.Kind == "frames" || cg.Kind == "still" }

var numRe = regexp.MustCompile(`(\d+)`)

// sortFrames 按文件名里的数字排序（1,2,10 而不是 1,10,2）。
func sortFrames(files []string) []string {
	out := append([]string(nil), files...)
	sort.SliceStable(out, func(i, j int) bool {
		ni, nj := numRe.FindString(out[i]), numRe.FindString(out[j])
		if ni != "" && nj != "" {
			a, _ := strconv.Atoi(ni)
			b, _ := strconv.Atoi(nj)
			if a != b {
				return a < b
			}
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

	frames := cg.Frames
	if len(frames) == 0 && cg.Kind == "still" {
		frames = cg.Files.PNG
	}
	if len(frames) == 0 {
		if cg.Kind == "spine" {
			return nil, fmt.Errorf("%s 是 Spine 骨骼动画（%d 段动画），渲染器尚未接入", cg.ID, len(cg.SpineAnimations()))
		}
		return nil, fmt.Errorf("%s 没有可播放的帧（kind=%s）", cg.ID, cg.Kind)
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

// Ref 素材引用：相对游戏根的路径，桌面加载器会先按 root+rel 直接找。
func Ref(cgID, file string) string {
	return strings.Join([]string{"cg", cgID, file}, "/")
}
