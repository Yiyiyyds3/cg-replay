//go:build !windows

package audio

import "errors"

// DecodeFile 在非 Windows 平台不可用。
func DecodeFile(string) (*PCM, error) {
	return nil, errors.New("audio: 仅支持 Windows（Media Foundation / waveOut）")
}
