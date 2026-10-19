//go:build !windows

package bgi

import "errors"

// mbToWide 非 Windows 平台没有内置 CP932/GB18030 支持，退化为报错
// （调用方 decodeMB 会退化为原样返回字节）。目标平台是 Windows，此文件只为可移植。
func mbToWide(b []byte, cp uint32) (string, error) {
	return "", errors.New("MultiByteToWideChar only on windows")
}
