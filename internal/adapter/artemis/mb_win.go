//go:build windows

package artemis

import (
	"syscall"
	"unicode/utf16"
	"unsafe"
)

// mbToWide 用 Windows MultiByteToWideChar 解码多字节编码（CP932 = 932）。
// 包内自带一份（与 catsystem2/bgi 同款）：不引第三方依赖，目标平台为 Windows。
func mbToWide(b []byte, cp uint32) (string, error) {
	if len(b) == 0 {
		return "", nil
	}
	k32 := syscall.NewLazyDLL("kernel32.dll")
	proc := k32.NewProc("MultiByteToWideChar")
	n, _, _ := proc.Call(uintptr(cp), 0,
		uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)), 0, 0)
	if n == 0 {
		return "", errWin
	}
	w := make([]uint16, n)
	proc.Call(uintptr(cp), 0,
		uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)),
		uintptr(unsafe.Pointer(&w[0])), uintptr(n))
	return string(utf16.Decode(w)), nil
}

var errWin = &winError{msg: "MultiByteToWideChar failed"}

type winError struct{ msg string }

func (e *winError) Error() string { return e.msg }
