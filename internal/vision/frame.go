// Package vision 提供「有效画面 / 纯黑」的统一判定。
//
// 单帧纯黑在 galgame 演出里非常常见：入场淡入、转场黑幕、场景切换都可能
// 只黑一瞬。因此这里不把单帧当结论，而是要求多帧采样：只有整段采样都
// 没有有效画面（低非黑像素比例 + 低颜色方差）才判 AllBlack；采样不足时
// 一律 Inconclusive，绝不触发会话拒绝或交互判定。
package vision

import (
	"errors"
	"fmt"
	"image"
	"math"
)

const (
	// MinTemporalSamples 判定「整段全黑」所需的最少采样帧数。
	// 少于这个数只给 Inconclusive，避免把单帧黑幕当成坏事件。
	MinTemporalSamples = 3
	// 纯黑阈值：任一通道 ≤ 8 视为黑；纯黑帧压缩噪声通常也落在这个区间。
	blackChannel = 8
	// 非黑像素占比低于 0.5% 且颜色方差低于 9（标准差约 3/255）才算黑帧。
	minNonBlackRatio = 0.005
	minColorVariance = 9.0
	maxSamples       = 20000
)

// ErrNoFrames 没有可判定的帧。
var ErrNoFrames = errors.New("没有可判定的画面帧")

// ErrAllBlack 多帧采样后整段仍无有效画面。
var ErrAllBlack = errors.New("事件所有采样帧均为纯黑")

// FrameStats 单帧的可见度统计。
type FrameStats struct {
	Sampled       int     `json:"sampled"`
	NonBlack      int     `json:"non_black"`
	NonBlackRatio float64 `json:"non_black_ratio"`
	MeanLuma      float64 `json:"mean_luma"`
	ColorVariance float64 `json:"color_variance"`
	HasPicture    bool    `json:"has_picture"`
}

// Report 一段多帧采样的汇总。
type Report struct {
	Frames           int          `json:"frames"`
	SampledPixels    int          `json:"sampled_pixels"`
	PictureFrames    int          `json:"picture_frames"`
	MaxNonBlackRatio float64      `json:"max_non_black_ratio"`
	MaxColorVariance float64      `json:"max_color_variance"`
	HasPicture       bool         `json:"has_picture"`
	AllBlack         bool         `json:"all_black"`
	Inconclusive     bool         `json:"inconclusive"`
	Stats            []FrameStats `json:"stats"`
}

// Analyze 统计一帧的非黑像素比例、平均亮度与颜色方差。
//
// 按采样步长最多取约 2 万像素；对大舞台图足够快，也不会被 4K 图拖慢。
func Analyze(img image.Image) FrameStats {
	if img == nil {
		return FrameStats{}
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return FrameStats{}
	}
	stride := 1
	for (w*h)/(stride*stride) > maxSamples {
		stride++
	}
	var st FrameStats
	var sum, sumSq float64
	for y := b.Min.Y; y < b.Max.Y; y += stride {
		for x := b.Min.X; x < b.Max.X; x += stride {
			r16, g16, b16, a16 := img.At(x, y).RGBA()
			r8, g8, b8 := int(r16>>8), int(g16>>8), int(b16>>8)
			if a16 == 0 {
				r8, g8, b8 = 0, 0, 0
			}
			luma := 0.2126*float64(r8) + 0.7152*float64(g8) + 0.0722*float64(b8)
			sum += luma
			sumSq += luma * luma
			st.Sampled++
			if r8 > blackChannel || g8 > blackChannel || b8 > blackChannel {
				st.NonBlack++
			}
		}
	}
	if st.Sampled == 0 {
		return st
	}
	st.NonBlackRatio = float64(st.NonBlack) / float64(st.Sampled)
	st.MeanLuma = sum / float64(st.Sampled)
	variance := sumSq/float64(st.Sampled) - st.MeanLuma*st.MeanLuma
	if variance < 0 {
		variance = 0 // 浮点误差
	}
	st.ColorVariance = variance
	st.HasPicture = st.NonBlackRatio >= minNonBlackRatio || st.ColorVariance >= minColorVariance
	return st
}

// Evaluate 汇总多帧采样。只有帧数达到 MinTemporalSamples 且每一帧都没有
// 有效画面时才返回 AllBlack；单帧/少帧纯黑返回 Inconclusive，调用方不得
// 据此拒绝会话。
func Evaluate(frames []image.Image) Report {
	r := Report{Frames: len(frames)}
	for _, img := range frames {
		st := Analyze(img)
		r.Stats = append(r.Stats, st)
		r.SampledPixels += st.Sampled
		if st.NonBlackRatio > r.MaxNonBlackRatio {
			r.MaxNonBlackRatio = st.NonBlackRatio
		}
		if st.ColorVariance > r.MaxColorVariance {
			r.MaxColorVariance = st.ColorVariance
		}
		if st.HasPicture {
			r.PictureFrames++
		}
	}
	r.HasPicture = r.PictureFrames > 0
	switch {
	case r.HasPicture:
		// 有任何一帧能看见画面，就不是全黑；哪怕首帧是纯黑淡入。
	case len(frames) < MinTemporalSamples:
		r.Inconclusive = true
	default:
		r.AllBlack = true
	}
	return r
}

// CheckFrames 是多帧判定的错误返回形式：采样不足不报错；整段全黑报
// ErrAllBlack。调用方可以把错误展示给用户，但不要拿单帧结果拒绝会话。
func CheckFrames(frames []image.Image) (Report, error) {
	r := Evaluate(frames)
	if len(frames) == 0 {
		return r, ErrNoFrames
	}
	if r.AllBlack {
		return r, fmt.Errorf("%w（采样 %d 帧，最大非黑像素 %.2f%%，最大颜色方差 %.1f）",
			ErrAllBlack, r.Frames, r.MaxNonBlackRatio*100, r.MaxColorVariance)
	}
	return r, nil
}

// IsBlackFrame 只回答「这一帧本身是不是纯黑」，不用于整段事件判定。
// 需要判断事件是否有效时用 Evaluate/CheckFrames。
func IsBlackFrame(img image.Image) bool {
	return !Analyze(img).HasPicture
}

// StdDev 返回颜色方差对应的标准差，便于日志阅读（颜色方差与亮度同量纲）。
func (s FrameStats) StdDev() float64 {
	return math.Sqrt(s.ColorVariance)
}
