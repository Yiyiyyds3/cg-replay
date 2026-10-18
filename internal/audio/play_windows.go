//go:build windows

package audio

import (
	"fmt"
	"time"
)

// PlayOnce 把一段 PCM 经 waveOut 循环播放 ms 毫秒后返回（阻塞）。用于可听自检。
func PlayOnce(p *PCM, ms int64) error {
	if ms <= 0 {
		return nil
	}
	mix := &Mixer{}
	v := newVoice(ConvertTo(p, devSampleRate, devChannels), true)
	if !v.playing {
		return fmt.Errorf("无可播放采样")
	}
	mix.Add(v)
	d, err := OpenDevice(mix)
	if err != nil {
		return err
	}
	defer d.Close()
	time.Sleep(time.Duration(ms) * time.Millisecond)
	return nil
}
