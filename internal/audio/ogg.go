package audio

import (
	"math"
	"path/filepath"
	"strings"

	"ucgr/internal/audio/vorbis"
)

// isOgg 判断路径是否为 Ogg Vorbis（按扩展名）。
func isOgg(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	// .ogg 明文之外，RPG Maker 的加密语音（.rpgmvo/.ogg_/.rpgmvm）同样是
	// Vorbis 流（还原头后走纯 Go 解码），不能落到 Media Foundation——
	// MF 不认这些扩展名，直接报不支持的格式。
	switch ext {
	case ".ogg", ".rpgmvo", ".ogg_", ".rpgmvm":
		return true
	}
	return false
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
