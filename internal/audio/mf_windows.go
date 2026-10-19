//go:build windows

package audio

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

// —— Media Foundation 常量 ——

const (
	mfSourceReaderFirstAudioStream = 0xFFFFFFFD

	mfSourceReaderfError                   = 0x1
	mfSourceReaderfEndOfStream             = 0x2
	mfSourceReaderfCurrentMediaTypeChanged = 0x20
)

// MF 属性键 / 媒体类型 GUID。
var (
	guidMtmajorType      = guid{0x48eba18e, 0xf8c9, 0x4687, [8]byte{0xbf, 0x11, 0x0a, 0x74, 0xc9, 0xf9, 0x6a, 0x8f}} // MF_MT_MAJOR_TYPE
	guidMtSubtype        = guid{0xf7e34c9a, 0x42e8, 0x4714, [8]byte{0xb7, 0x4b, 0xcb, 0x29, 0xd7, 0x2c, 0x35, 0xe5}} // MF_MT_SUBTYPE
	guidMediaTypeAudio   = guid{0x73647561, 0x0000, 0x0010, [8]byte{0x80, 0x00, 0x00, 0xaa, 0x00, 0x38, 0x9b, 0x71}} // MFMediaType_Audio
	guidAudioFormatPCM   = guid{0x00000001, 0x0000, 0x0010, [8]byte{0x80, 0x00, 0x00, 0xaa, 0x00, 0x38, 0x9b, 0x71}} // MFAudioFormat_PCM
	guidAudioFormatFloat = guid{0x00000003, 0x0000, 0x0010, [8]byte{0x80, 0x00, 0x00, 0xaa, 0x00, 0x38, 0x9b, 0x71}} // MFAudioFormat_Float

	guidMtAudioNumChannels       = guid{0x37e48bf5, 0x645e, 0x4c5b, [8]byte{0x89, 0xde, 0xad, 0xa9, 0xe2, 0x9b, 0x69, 0x6a}} // MF_MT_AUDIO_NUM_CHANNELS
	guidMtAudioSamplesPerSecond  = guid{0x5faeeae7, 0x0290, 0x4c31, [8]byte{0x9e, 0x8a, 0xc5, 0x34, 0xf6, 0x8d, 0x9d, 0xba}} // MF_MT_AUDIO_SAMPLES_PER_SECOND
	guidMtAudioBitsPerSample     = guid{0xf2deb57f, 0x40fa, 0x4764, [8]byte{0xaa, 0x33, 0xed, 0x4f, 0x2d, 0x1f, 0xf6, 0x69}} // MF_MT_AUDIO_BITS_PER_SAMPLE
	guidMtAudioAvgBytesPerSecond = guid{0x1aab75c8, 0xcfef, 0x451c, [8]byte{0xab, 0x95, 0xac, 0x03, 0x4b, 0x8e, 0x17, 0x31}} // MF_MT_AUDIO_AVG_BYTES_PER_SECOND
	guidMtAudioBlockAlignment    = guid{0x322de230, 0x9eeb, 0x43bd, [8]byte{0xab, 0x7a, 0xff, 0x41, 0x22, 0x51, 0x54, 0x1d}} // MF_MT_AUDIO_BLOCK_ALIGNMENT
)

var debugAudio = os.Getenv("UCGR_AUDIO_DEBUG") != ""

func dprintf(format string, a ...any) {
	if debugAudio {
		fmt.Fprintf(os.Stderr, "[audio] "+format+"\n", a...)
	}
}

// IMFAttributes 槽位（IUnknown 0-2 之后共 30 个方法，占 3..32）：
// GetUINT32=7, GetGUID=10, SetGUID=24。
func setGUID(this unsafe.Pointer, key, val guid) uintptr {
	return comCall(this, 24, uintptr(unsafe.Pointer(&key)), uintptr(unsafe.Pointer(&val)))
}

func getGUID(this unsafe.Pointer, key guid, out *guid) uintptr {
	return comCall(this, 10, uintptr(unsafe.Pointer(&key)), uintptr(unsafe.Pointer(out)))
}

func getUINT32(this unsafe.Pointer, key guid, out *uint32) uintptr {
	return comCall(this, 7, uintptr(unsafe.Pointer(&key)), uintptr(unsafe.Pointer(out)))
}

func toFileURL(path string) string {
	// MF 的 URL 形式：反斜杠转正斜杠
	u := path
	if len(u) >= 2 && u[1] == ':' {
		u = "file:///" + u
	}
	out := make([]byte, 0, len(u))
	for i := 0; i < len(u); i++ {
		if u[i] == '\\' {
			out = append(out, '/')
		} else {
			out = append(out, u[i])
		}
	}
	return string(out)
}

// DecodeFile 把音频文件解码为线性 PCM。
// WAV/MP3 走 Media Foundation Source Reader；Ogg Vorbis 走纯 Go 解码器
// （普通 win32 进程里 MF 没有 Vorbis MFT）。
func DecodeFile(path string) (*PCM, error) {
	// Ogg Vorbis 走纯 Go 解码器（MF 在普通 win32 进程里没有 Vorbis MFT）。
	if isOgg(path) {
		return decodeOgg(path)
	}
	if err := ensureMF(); err != nil {
		return nil, err
	}
	p, err := syscall.UTF16PtrFromString(toFileURL(path))
	if err != nil {
		return nil, err
	}

	reader := new(unsafe.Pointer)
	hr, _, _ := procMFCreateSourceReaderFromURL.Call(
		uintptr(unsafe.Pointer(p)), 0, uintptr(unsafe.Pointer(reader)))
	dprintf("MFCreateSourceReaderFromURL hr=0x%08X reader=%p", uint32(hr), ptrOf(reader))
	if err := hrErr(hr); err != nil {
		return nil, fmt.Errorf("MFCreateSourceReaderFromURL: %w", err)
	}
	if *reader == nil {
		return nil, fmt.Errorf("MFCreateSourceReaderFromURL: 返回空 reader")
	}
	rd := *reader
	defer comRelease(rd)

	// 尽量请求 16bit PCM 输出；不支持时退回 MF 的原生（解压后）类型。
	mt := new(unsafe.Pointer)
	if hr, _, _ := procMFCreateMediaType.Call(uintptr(unsafe.Pointer(mt))); hrErr(hr) == nil && *mt != nil {
		setGUID(*mt, guidMtmajorType, guidMediaTypeAudio)
		setGUID(*mt, guidMtSubtype, guidAudioFormatPCM)
		// IMFSourceReader::SetCurrentMediaType(streamIndex, reserved, pMediaType)
		hr := comCall(rd, 7, uintptr(mfSourceReaderFirstAudioStream), 0, uintptr(*mt))
		dprintf("SetCurrentMediaType(PCM) hr=0x%08X", uint32(hr))
		comRelease(*mt)
	}

	cur := new(unsafe.Pointer)
	// IMFSourceReader::GetCurrentMediaType(streamIndex, ppMediaType)
	if hr := comCall(rd, 6, uintptr(mfSourceReaderFirstAudioStream), uintptr(unsafe.Pointer(cur))); hrErr(hr) != nil {
		return nil, fmt.Errorf("GetCurrentMediaType: %w", hrErr(hr))
	}
	if *cur == nil {
		return nil, fmt.Errorf("GetCurrentMediaType: 返回空 mediaType")
	}
	var sub guid
	getGUID(*cur, guidMtSubtype, &sub)
	var ch, sr, bits uint32
	getUINT32(*cur, guidMtAudioNumChannels, &ch)
	getUINT32(*cur, guidMtAudioSamplesPerSecond, &sr)
	getUINT32(*cur, guidMtAudioBitsPerSample, &bits)
	comRelease(*cur)
	dprintf("媒体类型 subtype=%s ch=%d sr=%d bits=%d", sub, ch, sr, bits)

	if ch == 0 || sr == 0 {
		return nil, fmt.Errorf("音频格式异常：%d Hz / %d 声道（子类型 %s）", sr, ch, sub)
	}

	out := &PCM{SampleRate: int(sr), Channels: int(ch), BitsPerSample: int(bits)}

	var (
		actualIdx uint32
		flags     uint32
		ts        int64
	)
	for i := 0; i < 1<<28; i++ {
		sample := new(unsafe.Pointer)
		flags = 0
		// IMFSourceReader::ReadSample(streamIndex, controlFlags, &actualIdx, &flags, &ts, &sample)
		hr := comCall(rd, 9,
			uintptr(mfSourceReaderFirstAudioStream), 0,
			uintptr(unsafe.Pointer(&actualIdx)),
			uintptr(unsafe.Pointer(&flags)),
			uintptr(unsafe.Pointer(&ts)),
			uintptr(unsafe.Pointer(sample)))
		if err := hrErr(hr); err != nil {
			return nil, fmt.Errorf("ReadSample: %w", err)
		}
		if *sample == nil {
			if flags&mfSourceReaderfError != 0 {
				return nil, fmt.Errorf("ReadSample: 流错误 flags=0x%X", flags)
			}
			if flags&mfSourceReaderfEndOfStream != 0 {
				dprintf("ReadSample 结束 flags=0x%X 累计 %d 字节", flags, len(out.Data))
				break
			}
			dprintf("ReadSample#%d 无 sample，flags=0x%X", i, flags)
			continue
		}
		sm := *sample

		// IMFSample : IMFAttributes（30 个方法占槽位 3..32）；ConvertToContiguousBuffer = 41。
		mb := new(unsafe.Pointer)
		if hr := comCall(sm, 41, uintptr(unsafe.Pointer(mb))); hrErr(hr) != nil {
			comRelease(sm)
			return nil, fmt.Errorf("ConvertToContiguousBuffer: %w", hrErr(hr))
		}
		if *mb == nil {
			comRelease(sm)
			return nil, fmt.Errorf("ConvertToContiguousBuffer: 返回空 buffer")
		}
		buf := *mb

		var (
			pdata          unsafe.Pointer
			maxLen, curLen uint32
		)
		// IMFMediaBuffer::Lock(&ptr, &maxLen, &curLen)
		if hr := comCall(buf, 3,
			uintptr(unsafe.Pointer(&pdata)),
			uintptr(unsafe.Pointer(&maxLen)),
			uintptr(unsafe.Pointer(&curLen))); hrErr(hr) != nil {
			comRelease(buf)
			comRelease(sm)
			return nil, fmt.Errorf("Lock: %w", hrErr(hr))
		}
		if curLen > 0 && pdata != nil {
			out.Data = append(out.Data, unsafe.Slice((*byte)(pdata), int(curLen))...)
		}
		comCall(buf, 4) // Unlock
		comRelease(buf)
		comRelease(sm)

		// 最后一个样本可能同时带 EndOfStream 标志，处理完再退出。
		if flags&mfSourceReaderfEndOfStream != 0 {
			dprintf("ReadSample 末尾 flags=0x%X 累计 %d 字节", flags, len(out.Data))
			break
		}
	}

	return out, nil
}

func ptrOf(p *unsafe.Pointer) unsafe.Pointer {
	if p == nil {
		return nil
	}
	return *p
}
