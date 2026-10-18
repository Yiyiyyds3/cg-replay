// Package audio 提供 KAG 演出所需的音频能力：解码（Windows 走 Media Foundation）
// 与播放（Windows 走 winmm waveOut）。本文件不含构建标签：混音器与音源为纯逻辑，
// 任意平台可编译、可单测。
package audio

import (
	"math"
	"sync"
)

// 设备输出格式：waveOut 统一以 44100 Hz / 2 声道 / 16 bit 播放，所有音源在装载时
// 归一到该格式，混音器只需做整数加法。
const (
	devSampleRate = 44100
	devChannels   = 2
)

// ConvertTo 把 16bit 线性 PCM 重采样/声道转换到目标采样率与声道数。
// 采样率相同且声道数相同则原样返回。采样率转换用线性插值，足够回放使用。
func ConvertTo(p *PCM, sampleRate, channels int) *PCM {
	if p == nil {
		return nil
	}
	if p.BitsPerSample != 16 || p.Channels <= 0 || p.SampleRate <= 0 {
		// 无法解释的格式：返回空 PCM，避免误播。
		return &PCM{SampleRate: sampleRate, Channels: channels, BitsPerSample: 16}
	}
	if p.SampleRate == sampleRate && p.Channels == channels {
		return p
	}
	in := p.Frames()
	out := &PCM{SampleRate: sampleRate, Channels: channels, BitsPerSample: 16}
	if in == 0 {
		return out
	}
	ratio := float64(p.SampleRate) / float64(sampleRate)
	outFrames := int(math.Ceil(float64(in) / ratio))
	if outFrames < 1 {
		outFrames = 1
	}
	data := make([]byte, outFrames*channels*2)
	for i := 0; i < outFrames; i++ {
		src := float64(i) * ratio
		i0 := int(src)
		if i0 >= in {
			i0 = in - 1
		}
		i1 := i0 + 1
		if i1 >= in {
			i1 = in - 1
		}
		frac := src - float64(i0)
		for c := 0; c < channels; c++ {
			sc := c
			if sc >= p.Channels {
				sc = p.Channels - 1
			}
			a := float64(sampleAt(p, i0, sc))
			b := float64(sampleAt(p, i1, sc))
			v := a + (b-a)*frac
			if v > 32767 {
				v = 32767
			} else if v < -32768 {
				v = -32768
			}
			s := int16(v)
			off := (i*channels + c) * 2
			data[off] = byte(s)
			data[off+1] = byte(uint16(s) >> 8)
		}
	}
	out.Data = data
	return out
}

// sampleAt 读取第 frame 帧第 ch 声道的采样（16bit 小端）。
func sampleAt(p *PCM, frame, ch int) int16 {
	off := (frame*p.Channels + ch) * 2
	if off < 0 || off+2 > len(p.Data) {
		return 0
	}
	return int16(uint16(p.Data[off]) | uint16(p.Data[off+1])<<8)
}

// Voice 是一条正在播放的音源：已归一到设备格式的采样 + 播放位置 + 音量（含淡入淡出）。
type Voice struct {
	samples []int16 // 交错，devChannels 声道
	frames  int
	pos     int
	loop    bool

	cur    float64 // 当前增益
	target float64 // 目标增益
	step   float64 // 每帧增益增量；0 表示不在渐变

	playing       bool
	paused        bool
	stopAfterFade bool
}

// newVoice 用已归一化的 PCM 建立音源（cur=target=vol=1）。
func newVoice(p *PCM, loop bool) *Voice {
	v := &Voice{loop: loop, cur: 1, target: 1}
	if p != nil {
		n := p.Frames()
		v.samples = make([]int16, n*devChannels)
		for f := 0; f < n; f++ {
			for c := 0; c < devChannels; c++ {
				v.samples[f*devChannels+c] = sampleAt(p, f, c)
			}
		}
		v.frames = n
	}
	v.playing = v.frames > 0
	return v
}

// fadeTo 在 ms 毫秒内把增益平滑到 target；ms<=0 立即到位。
func (v *Voice) fadeTo(target float64, ms int64) {
	if target < 0 {
		target = 0
	} else if target > 1 {
		target = 1
	}
	v.target = target
	if ms <= 0 {
		v.cur = target
		v.step = 0
		return
	}
	frames := float64(ms) * float64(devSampleRate) / 1000.0
	if frames <= 0 {
		v.cur = target
		v.step = 0
		return
	}
	v.step = (target - v.cur) / frames
}

// fadeOutAndStop 用 ms 毫秒淡出后停止；ms<=0 立即停止。
func (v *Voice) fadeOutAndStop(ms int64) {
	if ms <= 0 {
		v.playing = false
		return
	}
	v.stopAfterFade = true
	v.fadeTo(0, ms)
}

// stop 立即停止。
func (v *Voice) stop() { v.playing = false }

// mixInto 把本音源叠加到 acc（长度 n*devChannels）并推进播放位置。
func (v *Voice) mixInto(acc []float32, n int) {
	if !v.playing || v.paused || v.frames == 0 {
		return
	}
	for i := 0; i < n; i++ {
		if v.pos >= v.frames {
			if v.loop {
				v.pos = 0
			} else {
				v.playing = false
				return
			}
		}
		if v.step != 0 {
			v.cur += v.step
			if (v.step > 0 && v.cur >= v.target) || (v.step < 0 && v.cur <= v.target) {
				v.cur = v.target
				v.step = 0
				if v.stopAfterFade {
					v.playing = false
					return
				}
			}
		}
		if v.cur > 0 {
			base := v.pos * devChannels
			g := float32(v.cur)
			acc[i*devChannels] += float32(v.samples[base]) * g
			acc[i*devChannels+1] += float32(v.samples[base+1]) * g
		}
		v.pos++
	}
}

// Mixer 把若干音源混成一路 16bit 立体声，供 waveOut 拉取。并发安全。
type Mixer struct {
	mu      sync.Mutex
	voices  []*Voice
	scratch []float32
}

// Add 加入一条音源。
func (m *Mixer) Add(v *Voice) {
	if v == nil {
		return
	}
	m.mu.Lock()
	m.voices = append(m.voices, v)
	m.mu.Unlock()
}

// Remove 移除一条音源（已在播且未结束的）。
func (m *Mixer) Remove(v *Voice) {
	if v == nil {
		return
	}
	m.mu.Lock()
	for i, x := range m.voices {
		if x == v {
			m.voices = append(m.voices[:i], m.voices[i+1:]...)
			break
		}
	}
	m.mu.Unlock()
}

// Voices 返回当前音源条数（调试用）。
func (m *Mixer) Voices() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.voices)
}

// Read 填充 p（16bit 立体声交错，长度须为 4 的倍数）；返回写入字节数。
func (m *Mixer) Read(p []byte) int {
	frames := len(p) / (devChannels * 2)
	if frames <= 0 {
		return 0
	}
	m.mu.Lock()
	if cap(m.scratch) < frames*devChannels {
		m.scratch = make([]float32, frames*devChannels)
	}
	acc := m.scratch[:frames*devChannels]
	for i := range acc {
		acc[i] = 0
	}
	alive := m.voices[:0]
	for _, v := range m.voices {
		v.mixInto(acc, frames)
		if v.playing {
			alive = append(alive, v)
		}
	}
	m.voices = alive
	m.mu.Unlock()

	for i, f := range acc {
		s := int32(f)
		if s > 32767 {
			s = 32767
		} else if s < -32768 {
			s = -32768
		}
		p[i*2] = byte(uint16(s))
		p[i*2+1] = byte(uint16(s) >> 8)
	}
	return frames * devChannels * 2
}
