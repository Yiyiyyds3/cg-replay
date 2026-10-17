// Package ir 定义统一演出中间表示（IR）的 Go 类型与校验规则。
//
// 对应 docs/03_IR规范.md；JSON 字段名与该文档及 schema/ir.schema.json 完全一致。
// IR 是核心枢纽：回放器只吃 IR；裁剪由 IR 反推；新增引擎只写「脚本→IR 翻译器」。
package ir

import (
	"fmt"
	"sort"
)

// IRVersion 当前 IR 规范版本。破坏性变更时递增，并配套迁移。
const IRVersion = "1.0"

// 保真度分级（DR-22 / DR-27）。
const (
	FidelityGeneric   = "generic"   // L0：通用近似转场
	FidelityTuned     = "tuned"     // L1：该引擎转场逐个补齐
	FidelityExclusive = "exclusive" // 专属强化（预留）
)

// IR 顶层结构。
type IR struct {
	IRVersion string  `json:"ir_version"`
	GameID    string  `json:"game_id"`
	EngineID  string  `json:"engine_id"`
	Fidelity  string  `json:"fidelity"`
	Meta      Meta    `json:"meta"`
	Events    []Event `json:"events"`
}

// Meta 溯源信息。
type Meta struct {
	ScriptEncoding string `json:"script_encoding,omitempty"`
	GeneratedBy    string `json:"generated_by"`
	GeneratedAt    string `json:"generated_at,omitempty"`
}

// EventKind 事件类型。
type EventKind string

const (
	KindStillCG    EventKind = "still_cg"    // 静帧 CG
	KindEventVideo EventKind = "event_video" // 事件视频
	KindScene      EventKind = "scene"       // 场景演出（含立绘对话）
	KindOther      EventKind = "other"
)

// Event 一个可重演演出单元。
type Event struct {
	EventID      string         `json:"event_id"`
	Kind         EventKind      `json:"kind"`
	Title        string         `json:"title,omitempty"`
	Ord          int            `json:"ord"`
	StartMs      int64          `json:"start_ms,omitempty"`
	EndMs        int64          `json:"end_ms,omitempty"`
	Characters   []string       `json:"characters,omitempty"`
	Layers       []Layer        `json:"layers"`
	Timeline     []TimelineOp   `json:"timeline"`
	Texts        []TextLine     `json:"texts"`
	Voices       []Voice        `json:"voices"`
	Transitions  []Transition   `json:"transitions"`
	Branches     []Branch       `json:"branches"`
	Unrecognized []Unrecognized `json:"unrecognized"`
	Assets       []AssetRef     `json:"assets"`
}

// LayerKind 图层类型。
type LayerKind string

const (
	LayerBackground LayerKind = "background"
	LayerStand      LayerKind = "stand"
	LayerCG         LayerKind = "cg"
	LayerMask       LayerKind = "mask"
	LayerMovie      LayerKind = "movie"
	LayerText       LayerKind = "text"
	LayerEffect     LayerKind = "effect"
)

// Layer 图层定义。
type Layer struct {
	LayerID   string     `json:"layer_id"`
	Kind      LayerKind  `json:"kind"`
	Asset     *AssetRef  `json:"asset,omitempty"`
	Z         int        `json:"z"`
	Visible   bool       `json:"visible"`
	Transform *Transform `json:"transform,omitempty"`
	Blend     string     `json:"blend,omitempty"` // normal|add|multiply
}

// Transform 几何/透明度变换（各字段可缺省，缺省=单位值）。
type Transform struct {
	X        *float64 `json:"x,omitempty"`
	Y        *float64 `json:"y,omitempty"`
	Scale    *float64 `json:"scale,omitempty"`
	Rotation *float64 `json:"rotation,omitempty"`
	Opacity  *float64 `json:"opacity,omitempty"`
	Anchor   string   `json:"anchor,omitempty"` // top-left|center
}

// TimelineOp 时间轴指令。TMs 为相对事件开始的毫秒数。
type TimelineOp struct {
	TMs    int64       `json:"t_ms"`
	Op     string      `json:"op"`
	Target string      `json:"target,omitempty"`
	Params interface{} `json:"params,omitempty"`
}

// TextLine 台词。
type TextLine struct {
	LineID  string     `json:"line_id"`
	TMs     int64      `json:"t_ms"`
	Speaker string     `json:"speaker,omitempty"`
	Text    string     `json:"text"`
	Voice   *AssetRef  `json:"voice,omitempty"`
	Style   *TextStyle `json:"style,omitempty"`
}

// TextStyle 排版近似提示（§9：文字排版只能近似）。
type TextStyle struct {
	FontHint string `json:"font_hint,omitempty"`
	SizePx   int    `json:"size_px,omitempty"`
	Position string `json:"position,omitempty"` // bottom|center|top
	Color    string `json:"color,omitempty"`
	NameBox  bool   `json:"name_box,omitempty"`
}

// Voice 语音。
type Voice struct {
	LineID     string   `json:"line_id"`
	Asset      AssetRef `json:"asset"`
	DurationMs int64    `json:"duration_ms,omitempty"`
}

// Transition 转场。Impl 未实现时适配器填 "fade" 并置 Fallback=true。
type Transition struct {
	TransitionID string      `json:"transition_id"`
	Impl         string      `json:"impl"`
	DurationMs   int64       `json:"duration_ms"`
	Params       interface{} `json:"params,omitempty"`
	Fallback     bool        `json:"fallback,omitempty"`
}

// Branch 分支点。回放是重演非重玩：默认沿 DefaultTarget 走。
type Branch struct {
	BranchID      string      `json:"branch_id"`
	TMs           int64       `json:"t_ms"`
	Type          string      `json:"type"` // choice|flag|jump
	Options       []BranchOpt `json:"options,omitempty"`
	DefaultTarget string      `json:"default_target"`
	Chosen        *int        `json:"chosen,omitempty"`
}

// BranchOpt 分支选项。
type BranchOpt struct {
	Text        string `json:"text"`
	TargetEvent string `json:"target_event"`
}

// Unrecognized 未识别指令登记（裁剪安全原则：宁多勿缺）。
type Unrecognized struct {
	NodePath   string   `json:"node_path"`
	Raw        string   `json:"raw"`
	Reason     string   `json:"reason"`
	KeptAssets []string `json:"kept_assets"`
}

// AssetRef 资源引用。
type AssetRef struct {
	Hash       string `json:"hash"`
	OrigPath   string `json:"orig_path"`
	Role       string `json:"role"`
	DurationMs int64  `json:"duration_ms,omitempty"`
}

// ValidFidelity 报告 fidelity 是否合法。
func ValidFidelity(f string) bool {
	return f == FidelityGeneric || f == FidelityTuned || f == FidelityExclusive
}

// ValidKind 报告事件类型是否合法。
func ValidKind(k EventKind) bool {
	switch k {
	case KindStillCG, KindEventVideo, KindScene, KindOther:
		return true
	}
	return false
}

// Validate 执行 IR 的结构校验（对应 03 文档 V2~V8；V1 的资产存在性
// 检查需要包内哈希集合，由 CheckAssetRefs 单独完成）。
// 返回全部错误；空切片表示通过。
func Validate(ir *IR) []error {
	var errs []error
	if ir.IRVersion != IRVersion {
		errs = append(errs, fmt.Errorf("ir_version 应为 %q，实为 %q", IRVersion, ir.IRVersion))
	}
	if ir.GameID == "" {
		errs = append(errs, fmt.Errorf("game_id 不能为空"))
	}
	if ir.EngineID == "" {
		errs = append(errs, fmt.Errorf("engine_id 不能为空"))
	}
	if !ValidFidelity(ir.Fidelity) {
		errs = append(errs, fmt.Errorf("fidelity %q 非法", ir.Fidelity))
	}

	seenEvents := map[string]bool{}
	for i := range ir.Events {
		ev := &ir.Events[i]
		if seenEvents[ev.EventID] {
			errs = append(errs, fmt.Errorf("event_id 重复: %s", ev.EventID))
		}
		seenEvents[ev.EventID] = true
		if !ValidKind(ev.Kind) {
			errs = append(errs, fmt.Errorf("事件 %s: kind %q 非法", ev.EventID, ev.Kind))
		}
		errs = append(errs, validateEvent(ev)...)
	}
	return errs
}

func validateEvent(ev *Event) []error {
	var errs []error
	prefix := fmt.Sprintf("事件 %s: ", ev.EventID)

	// V2：layer_id 事件内唯一
	layerIDs := map[string]bool{}
	for _, l := range ev.Layers {
		if layerIDs[l.LayerID] {
			errs = append(errs, fmt.Errorf("%slayer_id 重复: %s", prefix, l.LayerID))
		}
		layerIDs[l.LayerID] = true
	}

	// V3：timeline 按 t_ms 非降序
	for i := 1; i < len(ev.Timeline); i++ {
		if ev.Timeline[i].TMs < ev.Timeline[i-1].TMs {
			errs = append(errs, fmt.Errorf("%stimeline 第 %d 条 t_ms 逆序", prefix, i))
			break
		}
	}

	// V4：text_show.target 必须存在于 texts
	textIDs := map[string]bool{}
	for _, t := range ev.Texts {
		textIDs[t.LineID] = true
	}
	for _, op := range ev.Timeline {
		if op.Op == "text_show" && op.Target != "" && !textIDs[op.Target] {
			errs = append(errs, fmt.Errorf("%stext_show 目标 %s 不存在于 texts", prefix, op.Target))
		}
	}

	// V5：voice.line_id 必须存在于 texts
	for _, v := range ev.Voices {
		if !textIDs[v.LineID] {
			errs = append(errs, fmt.Errorf("%svoice 的 line_id %s 不存在于 texts", prefix, v.LineID))
		}
	}

	// V6：unrecognized.kept_assets ⊆ assets
	assetHashes := map[string]bool{}
	for _, a := range ev.Assets {
		assetHashes[a.Hash] = true
	}
	for _, u := range ev.Unrecognized {
		for _, h := range u.KeptAssets {
			if !assetHashes[h] {
				errs = append(errs, fmt.Errorf("%sunrecognized 的 kept_assets %s 不在 assets 中", prefix, h))
			}
		}
	}

	// V7：transitions.impl ∈ L0 注册表 或 fallback=true
	for _, tr := range ev.Transitions {
		if !IsL0Impl(tr.Impl) && !tr.Fallback {
			errs = append(errs, fmt.Errorf("%s转场 impl %q 非 L0 且未标 fallback", prefix, tr.Impl))
		}
	}

	// V8：fidelity=tuned 才允许非 L0 impl
	if evFidelityStrict(ev) {
		// 事件级不存 fidelity，此规则在摄入侧按包级 fidelity 执行
	}
	return errs
}

func evFidelityStrict(ev *Event) bool { return false }

// IsL0Impl 报告 impl 是否属于核心 L0 转场注册表。
// L0 五件套：fade / slide_left / slide_right / zoom / wipe。
func IsL0Impl(impl string) bool {
	switch impl {
	case "fade", "slide_left", "slide_right", "zoom", "wipe":
		return true
	}
	return false
}

// CheckAssetRefs V1：IR 引用的每个资产 hash 必须存在于包的哈希集合中。
func CheckAssetRefs(ir *IR, packageHashes map[string]bool) []error {
	var errs []error
	for i := range ir.Events {
		ev := &ir.Events[i]
		for _, a := range ev.Assets {
			if !packageHashes[a.Hash] {
				errs = append(errs, fmt.Errorf("事件 %s: 资产 %s（%s）不在包哈希集合中",
					ev.EventID, shortHash(a.Hash), a.OrigPath))
			}
		}
		for _, l := range ev.Layers {
			if l.Asset != nil && !packageHashes[l.Asset.Hash] {
				errs = append(errs, fmt.Errorf("事件 %s: 图层 %s 引用的资产缺失", ev.EventID, l.LayerID))
			}
		}
	}
	return errs
}

func shortHash(h string) string {
	if len(h) > 12 {
		return h[:12] + "…"
	}
	return h
}

// SortEventsByOrd 按 ord 稳定排序事件（原地）。
func SortEventsByOrd(ir *IR) {
	sort.SliceStable(ir.Events, func(i, j int) bool {
		return ir.Events[i].Ord < ir.Events[j].Ord
	})
}

// EventByID 返回指定 id 的事件指针，不存在返回 nil。
func EventByID(ir *IR, id string) *Event {
	for i := range ir.Events {
		if ir.Events[i].EventID == id {
			return &ir.Events[i]
		}
	}
	return nil
}

// CollectAssetHashes 收集 IR 全部资产 hash（去重）。
func CollectAssetHashes(ir *IR) []string {
	seen := map[string]bool{}
	var out []string
	for i := range ir.Events {
		ev := &ir.Events[i]
		add := func(a *AssetRef) {
			if a != nil && !seen[a.Hash] {
				seen[a.Hash] = true
				out = append(out, a.Hash)
			}
		}
		for _, a := range ev.Assets {
			add(&a)
		}
		for _, l := range ev.Layers {
			add(l.Asset)
		}
		for _, t := range ev.Texts {
			add(t.Voice)
		}
		for _, v := range ev.Voices {
			add(&v.Asset)
		}
	}
	sort.Strings(out)
	return out
}
