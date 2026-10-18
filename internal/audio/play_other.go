//go:build !windows

package audio

import "fmt"

// PlayOnce 在非 Windows 平台不支持。
func PlayOnce(*PCM, int64) error { return fmt.Errorf("当前平台不支持播放") }
