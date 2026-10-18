package audio

import (
	"strings"
	"testing"
)

func TestPCMBytesPerFrameFramesDuration(t *testing.T) {
	cases := []struct {
		name   string
		pcm    PCM
		bpf    int
		frames int
		durMs  int64
	}{
		{"mono16_1s", PCM{SampleRate: 44100, Channels: 1, BitsPerSample: 16, Data: make([]byte, 88200)}, 2, 44100, 1000},
		{"stereo16_1s", PCM{SampleRate: 48000, Channels: 2, BitsPerSample: 16, Data: make([]byte, 48000*4)}, 4, 48000, 1000},
		{"stereo8_partial", PCM{SampleRate: 8000, Channels: 2, BitsPerSample: 8, Data: make([]byte, 3000)}, 2, 1500, 187},
		{"empty", PCM{SampleRate: 44100, Channels: 2, BitsPerSample: 16}, 4, 0, 0},
		{"zero_sample_rate", PCM{SampleRate: 0, Channels: 1, BitsPerSample: 16, Data: make([]byte, 100)}, 2, 50, 0},
		{"zero_bits", PCM{SampleRate: 44100, Channels: 2, BitsPerSample: 0, Data: make([]byte, 100)}, 0, 0, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.pcm.BytesPerFrame(); got != c.bpf {
				t.Errorf("BytesPerFrame()=%d, want %d", got, c.bpf)
			}
			if got := c.pcm.Frames(); got != c.frames {
				t.Errorf("Frames()=%d, want %d", got, c.frames)
			}
			if got := c.pcm.DurationMs(); got != c.durMs {
				t.Errorf("DurationMs()=%d, want %d", got, c.durMs)
			}
		})
	}
}

func TestPCMString(t *testing.T) {
	p := &PCM{SampleRate: 44100, Channels: 1, BitsPerSample: 16, Data: make([]byte, 88200)}
	s := p.String()
	for _, want := range []string{"44100 Hz", "1 声道", "16 bit", "44100 帧", "1000 ms", "88200 字节"} {
		if !strings.Contains(s, want) {
			t.Errorf("String()=%q, 缺少 %q", s, want)
		}
	}
}
