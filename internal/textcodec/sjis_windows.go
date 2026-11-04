//go:build windows

package textcodec

import (
	"syscall"
	"unicode/utf16"
	"unsafe"
)

// decodeSJIS 用 Windows MultiByteToWideChar（代码页 932）解码。
func decodeSJIS(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	k32 := syscall.NewLazyDLL("kernel32.dll")
	proc := k32.NewProc("MultiByteToWideChar")
	const cpSJIS = 932
	n, _, _ := proc.Call(uintptr(cpSJIS), 0,
		uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)), 0, 0)
	if n == 0 {
		return string(b)
	}
	w := make([]uint16, n)
	proc.Call(uintptr(cpSJIS), 0,
		uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)),
		uintptr(unsafe.Pointer(&w[0])), uintptr(n))
	return string(utf16.Decode(w))
}
