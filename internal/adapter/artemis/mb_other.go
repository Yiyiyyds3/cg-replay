//go:build !windows

package artemis

import "errors"

// mbToWide 非 Windows 平台无 CP932 内置支持，退化为报错
// （decodePFSName 会退回原字节）。目标平台为 Windows，此文件仅为可移植。
func mbToWide(b []byte, cp uint32) (string, error) {
	return "", errors.New("MultiByteToWideChar only on windows")
}
