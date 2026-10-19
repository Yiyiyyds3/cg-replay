// Package audio 提供 KAG 演出所需的音频能力：解码（WAV/MP3 在 Windows 走
// Media Foundation，Ogg Vorbis 走纯 Go）与播放（Windows 走 winmm waveOut）。
// 本文件不含构建标签，PCM 结构在任意平台可用，便于纯逻辑单测。
package audio

import "fmt"

// PCM 是一次解码得到的线性 PCM。Data 为交错采样，小端。
type PCM struct {
	SampleRate    int
	Channels      int
	BitsPerSample int
	Data          []byte
}

// BytesPerFrame 返回每个采样帧的字节数（所有声道合计）。
func (p *PCM) BytesPerFrame() int {
	return (p.BitsPerSample / 8) * p.Channels
}

// Frames 返回采样帧数（每帧含 Channels 个采样）。
func (p *PCM) Frames() int {
	bpf := p.BytesPerFrame()
	if bpf <= 0 {
		return 0
	}
	return len(p.Data) / bpf
}

// DurationMs 返回时长（毫秒）。
func (p *PCM) DurationMs() int64 {
	if p.SampleRate <= 0 {
		return 0
	}
	return int64(p.Frames()) * 1000 / int64(p.SampleRate)
}

// String 返回一行人类可读的描述。
func (p *PCM) String() string {
	return fmt.Sprintf("%d Hz · %d 声道 · %d bit · %d 帧 · %d ms · %d 字节",
		p.SampleRate, p.Channels, p.BitsPerSample, p.Frames(), p.DurationMs(), len(p.Data))
}
