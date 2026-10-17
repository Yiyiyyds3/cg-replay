//go:build !windows

package catsystem2

import "errors"

// mbToWide 非 Windows 平台无 CP932/GB18030 内置支持，退化为报错
// （调用方会退化为原样返回）。目标平台为 Windows，此文件仅为可移植。
func mbToWide(b []byte, cp uint32) (string, error) {
	return "", errors.New("MultiByteToWideChar only on windows")
}
