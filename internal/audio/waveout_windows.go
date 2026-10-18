//go:build windows

package audio

import (
	"fmt"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var winmm = syscall.NewLazyDLL("winmm.dll")

var (
	procWaveOutOpen            = winmm.NewProc("waveOutOpen")
	procWaveOutClose           = winmm.NewProc("waveOutClose")
	procWaveOutPrepareHeader   = winmm.NewProc("waveOutPrepareHeader")
	procWaveOutUnprepareHeader = winmm.NewProc("waveOutUnprepareHeader")
	procWaveOutWrite           = winmm.NewProc("waveOutWrite")
	procWaveOutReset           = winmm.NewProc("waveOutReset")
	procWaveOutSetVolume       = winmm.NewProc("waveOutSetVolume")
)

type waveFormatEx struct {
	wFormatTag      uint16
	nChannels       uint16
	nSamplesPerSec  uint32
	nAvgBytesPerSec uint32
	nBlockAlign     uint16
	wBitsPerSample  uint16
	cbSize          uint16
}

type waveHdr struct {
	lpData          *byte
	dwBufferLength  uint32
	dwBytesRecorded uint32
	dwUser          uintptr
	dwFlags         uint32
	dwLoops         uint32
	lpNext          *waveHdr
	reserved        uintptr
}

const (
	waveMapper    = 0xFFFFFFFF
	waveFormatPCM = 1
	callbackNull  = 0
	whdrDone      = 0x00000001
)

const (
	devBufferFrames = 4096
	devBufferCount  = 3
)

type devBuf struct {
	hdr  waveHdr
	data []byte
}

// Device 是 winmm waveOut 输出设备：后台协程持续从 Mixer 取混音结果，灌入环形缓冲并
// waveOutWrite，实现流式连续播放（无音源时输出静音）。
type Device struct {
	h       uintptr
	mix     *Mixer
	bufs    []*devBuf
	stopCh  chan struct{}
	doneCh  chan struct{}
	closeMu sync.Once
}

func mmResultErr(r uintptr) error {
	if uint32(r) == 0 {
		return nil
	}
	return fmt.Errorf("waveOut 错误码 %d", uint32(r))
}

// OpenDevice 以 44100/2/16 打开默认波形输出设备，并从 mix 拉流播放。
func OpenDevice(mix *Mixer) (*Device, error) {
	wf := waveFormatEx{
		wFormatTag:      waveFormatPCM,
		nChannels:       devChannels,
		nSamplesPerSec:  devSampleRate,
		nAvgBytesPerSec: devSampleRate * devChannels * 2,
		nBlockAlign:     devChannels * 2,
		wBitsPerSample:  16,
	}
	var h uintptr
	r, _, _ := procWaveOutOpen.Call(
		uintptr(unsafe.Pointer(&h)), waveMapper,
		uintptr(unsafe.Pointer(&wf)), callbackNull, 0, 0)
	if err := mmResultErr(r); err != nil {
		return nil, fmt.Errorf("waveOutOpen: %w", err)
	}
	d := &Device{
		h:      h,
		mix:    mix,
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}
	size := devBufferFrames * devChannels * 2
	for i := 0; i < devBufferCount; i++ {
		b := &devBuf{data: make([]byte, size)}
		b.hdr.lpData = &b.data[0]
		b.hdr.dwBufferLength = uint32(size)
		if r, _, _ := procWaveOutPrepareHeader.Call(h, uintptr(unsafe.Pointer(&b.hdr)), unsafe.Sizeof(b.hdr)); uint32(r) != 0 {
			d.Close()
			return nil, fmt.Errorf("waveOutPrepareHeader: %w", mmResultErr(r))
		}
		d.bufs = append(d.bufs, b)
	}
	go d.feed()
	return d, nil
}

func (d *Device) feed() {
	defer close(d.doneCh)
	for {
		for _, b := range d.bufs {
			if !d.waitDone(b) {
				return
			}
			select {
			case <-d.stopCh:
				return
			default:
			}
			n := d.mix.Read(b.data)
			if n <= 0 {
				n = len(b.data)
			}
			b.hdr.dwBufferLength = uint32(n)
			b.hdr.dwFlags &^= whdrDone // 交还前清除“已完成”
			procWaveOutWrite.Call(d.h, uintptr(unsafe.Pointer(&b.hdr)), unsafe.Sizeof(b.hdr))
		}
	}
}

func (d *Device) waitDone(b *devBuf) bool {
	for b.hdr.dwFlags&whdrDone == 0 {
		select {
		case <-d.stopCh:
			return false
		default:
		}
		time.Sleep(2 * time.Millisecond)
	}
	return true
}

// Close 停止播放并释放设备（幂等）。
func (d *Device) Close() {
	d.closeMu.Do(func() {
		close(d.stopCh)
		if d.h != 0 {
			procWaveOutReset.Call(d.h)
		}
		<-d.doneCh
		for _, b := range d.bufs {
			procWaveOutUnprepareHeader.Call(d.h, uintptr(unsafe.Pointer(&b.hdr)), unsafe.Sizeof(b.hdr))
		}
		if d.h != 0 {
			procWaveOutClose.Call(d.h)
			d.h = 0
		}
	})
}

// SetVolume 设置设备音量（left 低 16 位、right 高 16 位；0xFFFF 为满）。
func (d *Device) SetVolume(leftRight uint32) {
	if d.h != 0 {
		procWaveOutSetVolume.Call(d.h, uintptr(leftRight))
	}
}
