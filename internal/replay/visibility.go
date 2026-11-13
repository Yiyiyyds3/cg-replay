package replay

import (
	"fmt"
	"image"

	"ucgr/internal/ir"
	"ucgr/internal/player"
	"ucgr/internal/vision"
)

// SampleEventFrames 在事件时间轴上等间隔采样若干帧。
//
// 采样点包含 t=0 与末尾；末尾常用于收黑转场，所以绝不能只看首帧或末帧。
// 返回的帧全部交给 vision.Evaluate：只有整段都没有有效画面才可能判全黑。
func SampleEventFrames(ev *ir.Event, load player.Loader, video player.VideoProvider, w, h, samples int) ([]image.Image, error) {
	if ev == nil {
		return nil, fmt.Errorf("事件为空")
	}
	if samples < 2 {
		samples = 2
	}
	// 事件时长不能只看 TMs：很多台本只有 [wait]（DurMs）推进时间，
	// 末尾的 TMs 仍是 0。把每条指令自己的持续时间也算上，否则所有采样点
	// 都会落在 t=0 的入场黑幕上，正常事件会被误判为全黑。
	dur := int64(0)
	for _, op := range ev.Show {
		end := op.TMs
		if op.DurMs > 0 {
			end += op.DurMs
		}
		if end > dur {
			dur = end
		}
	}
	if ev.EndMs > dur {
		dur = ev.EndMs
	}
	opt := player.DefaultOptions()
	// 采样要真实演到各时间点，不能用 SkipWaits 一口气推到结尾。
	opt.SkipWaits = false
	p := player.New(ev, opt)
	p.Advance(0, player.Input{})

	var frames []image.Image
	for i := 0; i < samples; i++ {
		target := int64(0)
		if dur > 0 {
			target = dur * int64(i) / int64(samples-1)
		}
		for p.Now() < target && !p.Done() {
			p.Advance(16, player.Input{})
		}
		img, _, err := player.RenderFrameWithVideo(p, load, video, w, h)
		if err != nil {
			return nil, fmt.Errorf("采样 t=%dms 渲染失败：%w", target, err)
		}
		frames = append(frames, img)
	}
	return frames, nil
}

// CheckEventVisibility 返回事件多帧采样的可见度报告。
//
// 单帧纯黑（入场淡入）或首帧黑、后续有画面时不会报错；只有采样数达到
// vision.MinTemporalSamples 且整段都黑时才返回 vision.ErrAllBlack。调用方
// 应把 ErrAllBlack 当作「事件确实没有可看画面」的真实错误，而不是会话
// 交互判定——不要在 Player.Advance/点击处理里使用单帧黑屏结果。
func CheckEventVisibility(ev *ir.Event, load player.Loader, video player.VideoProvider, w, h, samples int) (vision.Report, error) {
	frames, err := SampleEventFrames(ev, load, video, w, h, samples)
	if err != nil {
		return vision.Report{}, err
	}
	return vision.CheckFrames(frames)
}
