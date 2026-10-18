package audio

import (
	"encoding/binary"
	"testing"
)

// mkPCM 用给定的交错采样构造 16bit PCM。
func mkPCM(sr, ch int, samples ...int16) *PCM {
	data := make([]byte, len(samples)*2)
	for i, s := range samples {
		binary.LittleEndian.PutUint16(data[i*2:], uint16(s))
	}
	return &PCM{SampleRate: sr, Channels: ch, BitsPerSample: 16, Data: data}
}

func TestConvertToIdentity(t *testing.T) {
	p := mkPCM(devSampleRate, devChannels, 1, 2, 3, 4)
	if got := ConvertTo(p, devSampleRate, devChannels); got != p {
		t.Fatalf("同格式应原样返回")
	}
}

func TestConvertToMonoToStereo(t *testing.T) {
	got := ConvertTo(mkPCM(devSampleRate, 1, 100, -200), devSampleRate, devChannels)
	if got.Channels != 2 || got.Frames() != 2 {
		t.Fatalf("channels=%d frames=%d", got.Channels, got.Frames())
	}
	if l := sampleAt(got, 0, 0); l != 100 {
		t.Errorf("L0=%d want 100", l)
	}
	if r := sampleAt(got, 0, 1); r != 100 {
		t.Errorf("R0=%d want 100", r)
	}
	if r := sampleAt(got, 1, 1); r != -200 {
		t.Errorf("R1=%d want -200", r)
	}
}

func TestConvertToResample(t *testing.T) {
	got := ConvertTo(mkPCM(22050, 1, 0, 1000, 2000, 3000), 44100, 1)
	if got.SampleRate != 44100 {
		t.Fatalf("sr=%d", got.SampleRate)
	}
	// 4 帧 @22050 → 约 8 帧 @44100
	if got.Frames() < 7 || got.Frames() > 9 {
		t.Fatalf("frames=%d want ~8", got.Frames())
	}
}

func TestConvertToUnsupportedBits(t *testing.T) {
	p := &PCM{SampleRate: 44100, Channels: 2, BitsPerSample: 8, Data: []byte{1, 2, 3, 4}}
	got := ConvertTo(p, devSampleRate, devChannels)
	if got.Frames() != 0 {
		t.Fatalf("非 16bit 应返回空 PCM, frames=%d", got.Frames())
	}
}

func TestVoiceMixAndEnd(t *testing.T) {
	v := newVoice(ConvertTo(mkPCM(devSampleRate, devChannels, 1000, 2000), devSampleRate, devChannels), false)
	if !v.playing {
		t.Fatal("应处于播放中")
	}
	mix := &Mixer{}
	mix.Add(v)
	out := make([]byte, devChannels*2) // 1 帧（L,R）
	if n := mix.Read(out); n != len(out) {
		t.Fatalf("n=%d", n)
	}
	if got := int16(binary.LittleEndian.Uint16(out[0:])); got != 1000 {
		t.Errorf("L=%d want 1000", got)
	}
	if got := int16(binary.LittleEndian.Uint16(out[2:])); got != 2000 {
		t.Errorf("R=%d want 2000", got)
	}
	mix.Read(make([]byte, devChannels*2))
	if v.playing {
		t.Errorf("非循环音源读完后应停止")
	}
}

func TestMixerSumsAndClips(t *testing.T) {
	a := newVoice(ConvertTo(mkPCM(devSampleRate, devChannels, 3000, 30000), devSampleRate, devChannels), true)
	b := newVoice(ConvertTo(mkPCM(devSampleRate, devChannels, 4000, 30000), devSampleRate, devChannels), true)
	mix := &Mixer{}
	mix.Add(a)
	mix.Add(b)
	out := make([]byte, devChannels*2) // 1 帧（L,R）
	mix.Read(out)
	if got := int16(binary.LittleEndian.Uint16(out[0:])); got != 7000 {
		t.Errorf("叠加=%d want 7000", got)
	}
	if got := int16(binary.LittleEndian.Uint16(out[2:])); got != 32767 {
		t.Errorf("削顶=%d want 32767", got)
	}
}

func TestVoiceFadeOutAndStop(t *testing.T) {
	v := newVoice(ConvertTo(mkPCM(devSampleRate, devChannels, 1000, 1000, 1000, 1000), devSampleRate, devChannels), true)
	v.fadeOutAndStop(10)
	if v.step >= 0 {
		t.Fatalf("应向下渐变, step=%v", v.step)
	}
	mix := &Mixer{}
	mix.Add(v)
	mix.Read(make([]byte, 20*devSampleRate/1000*devChannels*2))
	if v.playing {
		t.Errorf("淡出 10ms 后应在 20ms 内停止")
	}
}

func TestVoiceFadeInRamps(t *testing.T) {
	v := newVoice(ConvertTo(mkPCM(devSampleRate, devChannels, 10000, 10000), devSampleRate, devChannels), true)
	v.cur = 0
	v.fadeTo(1, 100)
	if v.step <= 0 {
		t.Fatalf("应向上渐变, step=%v", v.step)
	}
	if v.cur != 0 {
		t.Errorf("起始增益应为 0, got %v", v.cur)
	}
}
