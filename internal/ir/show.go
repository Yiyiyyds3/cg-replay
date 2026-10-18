// show.go —— 演出台本（回放用的**顺序**指令流）。
//
// 与 Timeline 的分工（两者同存于一个 ir.json，统一管理）：
//   - Timeline：扁平事件流，按 t_ms 升序，给裁剪/分析/校验用。
//   - Show    ：顺序执行台本，给播放器用。**顺序不可省**：KAG 的台词推进靠
//     「玩家点击」阻塞，脚本时钟在台词之间根本不推进（本作 71kaigan01.ks
//     5.4k 行的 clock 只被 [wait]/[wt]/[trans] 推动），时间戳无法还原阅读节奏。
//
// 台本只记录「怎么演」，不记录「素材在哪」——素材由 Refs + 事件 Assets 解析，
// 因此同一份台本对原始解包目录与 S2 裁剪产物都成立。
package ir

import "fmt"

// 台本指令种类。
const (
	ShowLayerShow  = "layer_show"  // 层显示/更新（image/img/cg…，带 Ref 与几何属性）
	ShowLayerHide  = "layer_hide"  // 层隐藏（visible=false）
	ShowLayerOpt   = "layer_opt"   // 层可见性切换（layopt/layon/layoff）
	ShowLayerAttr  = "layer_attr"  // 层几何/透明度修改（position）
	ShowScreenCopy = "screen_copy" // backlay：把 fore 页复制到 back 页（KAG 双缓冲）
	ShowTransition = "transition"  // 转场：DurMs 内把 back 页融进 fore（阻塞 DurMs）
	ShowQuake      = "quake"       // 画面抖动（阻塞 DurMs）
	ShowWait       = "wait"        // 等待（阻塞 DurMs）
	ShowTextRun    = "text"        // 台词文本（追加到当前消息层）
	ShowTextBreak  = "text_break"  // [r] 换行
	ShowTextWait   = "text_wait"   // [l]/[er] 原作点击点（自动模式按字数估算）
	ShowTextClear  = "text_clear"  // [cm]/[np]/[per]/[p] 清空消息层
	ShowTextStyle  = "text_style"  // 文本样式/对齐（@style 等宏）
	ShowBGMPlay    = "bgm_play"
	ShowBGMFadeIn  = "bgm_fadein"
	ShowBGMFadeOut = "bgm_fadeout"
	ShowBGMStop    = "bgm_stop"
	ShowSEPlay     = "se_play"
	ShowSEFadeIn   = "se_fadein"
	ShowSEFadeOut  = "se_fadeout"
	ShowSEStop     = "se_stop"
	ShowVoicePlay  = "voice_play" // 语音（本作无，保留以对齐其它引擎）
)

// ShowOp 一条台本指令。TMs 为脚本时钟（仅供参考/进度条，播放器不据此排序）。
type ShowOp struct {
	TMs   int64             `json:"t_ms"`
	Kind  string            `json:"kind"`
	Layer string            `json:"layer,omitempty"` // layer_* 的目标层
	Page  string            `json:"page,omitempty"`  // fore|back（空 = 脚本未写）
	Ref   string            `json:"ref,omitempty"`   // 原始资源引用（storage 名等）
	Attr  map[string]string `json:"attr,omitempty"`  // 原样保留的脚本属性
	Text  *ShowText         `json:"text,omitempty"`  // text 指令的文本
	DurMs int64             `json:"dur_ms,omitempty"`
}

// ShowText 一条台词文本段（同一行的多个片段会在播放时拼起来）。
type ShowText struct {
	Speaker string `json:"speaker,omitempty"` // 说话人（空 = 沿用/旁白）
	Text    string `json:"text"`
}

// ValidShowKind 报告台本指令种类是否已知。
func ValidShowKind(k string) bool {
	switch k {
	case ShowLayerShow, ShowLayerHide, ShowLayerOpt, ShowLayerAttr,
		ShowScreenCopy, ShowTransition, ShowQuake, ShowWait,
		ShowTextRun, ShowTextBreak, ShowTextWait, ShowTextClear, ShowTextStyle,
		ShowBGMPlay, ShowBGMFadeIn, ShowBGMFadeOut, ShowBGMStop,
		ShowSEPlay, ShowSEFadeIn, ShowSEFadeOut, ShowSEStop, ShowVoicePlay:
		return true
	}
	return false
}

// ShowKindCounts 统计台本各类指令条数（报告/摘要用）。
func ShowKindCounts(ev *Event) map[string]int {
	m := map[string]int{}
	for _, op := range ev.Show {
		m[op.Kind]++
	}
	return m
}

// ValidateShow V9：台本结构校验（种类合法、t_ms 非降序、text 必须带文本）。
func ValidateShow(ev *Event) []error {
	var errs []error
	prefix := ""
	if ev != nil {
		prefix = "事件 " + ev.EventID + ": "
	}
	for i, op := range ev.Show {
		if !ValidShowKind(op.Kind) {
			errs = append(errs, fmt.Errorf("%sshow 第 %d 条 kind %q 非法", prefix, i+1, op.Kind))
			break
		}
		if op.Kind == ShowTextRun && (op.Text == nil || op.Text.Text == "") {
			errs = append(errs, fmt.Errorf("%sshow 第 %d 条 text 缺文本", prefix, i+1))
			break
		}
		if i > 0 && op.TMs < ev.Show[i-1].TMs {
			errs = append(errs, fmt.Errorf("%sshow 第 %d 条 t_ms 逆序", prefix, i+1))
			break
		}
	}
	return errs
}
