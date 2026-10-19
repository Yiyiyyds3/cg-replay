package audio

import (
	"math"
	"path/filepath"
	"strings"

	"ucgr/internal/audio/vorbis"
)

// isOgg 判断路径是否为 Ogg Vorbis（按扩展名）。
func isOgg(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".ogg")
}

// decodeOgg 用纯 Go 解码器解 Ogg Vorbis，并转成 16bit 交错 PCM。
// 本机 Media Foundation 在普通 win32 进程里没有 Vorbis 解码器，故 Ogg 走这里。
func decodeOgg(path string) (*PCM, error) {
	s, err := vorbis.DecodeFile(path)
	if err != nil {
		return nil, err
	}
	n := len(s.Samples)
	data := make([]byte, n*2)
	for i, v := range s.Samples {
		// 浮点 [-1,1] → int16；越界削顶，避免回绕。
		if v > 1 {
			v = 1
		} else if v < -1 {
			v = -1
		}
		x := int32(math.Round(float64(v) * 32767))
		data[i*2] = byte(uint16(x))
		data[i*2+1] = byte(uint16(x) >> 8)
	}
	return &PCM{
		SampleRate:    s.SampleRate,
		Channels:      s.Channels,
		BitsPerSample: 16,
		Data:          data,
	}, nil
}
