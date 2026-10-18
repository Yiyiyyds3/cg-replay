//go:build windows

package audio

import (
	"testing"
	"time"
)

// TestDevicePlaysSilence 走一遍 waveOut 的设备开/Prepare/Write/Reset/Close 全流程。
// 无音频设备时跳过（服务器/无声卡环境）。
func TestDevicePlaysSilence(t *testing.T) {
	mix := &Mixer{}
	dev, err := OpenDevice(mix)
	if err != nil {
		t.Skipf("无音频设备，跳过：%v", err)
	}
	time.Sleep(80 * time.Millisecond)
	dev.Close()
}

// TestDevicePlaysVoice 让设备实际消费一段音源，确认喂流不停摆。
func TestDevicePlaysVoice(t *testing.T) {
	mix := &Mixer{}
	dev, err := OpenDevice(mix)
	if err != nil {
		t.Skipf("无音频设备，跳过：%v", err)
	}
	defer dev.Close()

	v := newVoice(ConvertTo(mkPCM(devSampleRate, devChannels, 0, 0), devSampleRate, devChannels), true)
	mix.Add(v)
	time.Sleep(120 * time.Millisecond)
	if mix.Voices() == 0 {
		t.Errorf("音源应仍在混音器中")
	}
}
