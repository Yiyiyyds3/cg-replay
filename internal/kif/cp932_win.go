//go:build windows

package kif

import (
	"syscall"
	"unicode/utf16"
	"unsafe"
)

// decodeName 把名字字节解成字符串：纯 ASCII 直接转换，其余按 CP932（代码页 932）走
// Windows MultiByteToWideChar（与 catsystem2 适配器的做法一致，避免引入第三方依赖）。
func decodeName(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	ascii := true
	for _, c := range b {
		if c >= 0x80 {
			ascii = false
			break
		}
	}
	if ascii {
		return string(b)
	}
	if len(b) > 0x1000 {
		b = b[:0x1000]
	}
	proc := syscall.NewLazyDLL("kernel32.dll").NewProc("MultiByteToWideChar")
	n, _, _ := proc.Call(932, 0, uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)), 0, 0)
	if n == 0 {
		return string(b)
	}
	w := make([]uint16, n)
	proc.Call(932, 0, uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)), uintptr(unsafe.Pointer(&w[0])), n)
	return string(utf16.Decode(w))
}
