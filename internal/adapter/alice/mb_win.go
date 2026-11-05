//go:build windows

package alice

import (
	"syscall"
	"unicode/utf16"
	"unsafe"
)

// decodeGBK 用 Windows MultiByteToWideChar（代码页 936）解码。
func decodeGBK(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	k32 := syscall.NewLazyDLL("kernel32.dll")
	proc := k32.NewProc("MultiByteToWideChar")
	const cpGBK = 936
	n, _, _ := proc.Call(uintptr(cpGBK), 0,
		uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)), 0, 0)
	if n == 0 {
		return string(b)
	}
	w := make([]uint16, n)
	proc.Call(uintptr(cpGBK), 0,
		uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)),
		uintptr(unsafe.Pointer(&w[0])), uintptr(n))
	return string(utf16.Decode(w))
}

// cp932Encode 反向编码（WideCharToMultiByte 932），测试用。
func cp932Encode(s string) ([]byte, error) {
	return wideToMulti(s, 932)
}

// gbkEncode 反向编码（WideCharToMultiByte 936），测试用。
func gbkEncode(s string) ([]byte, error) {
	return wideToMulti(s, 936)
}

// wideToMulti WideCharToMultiByte 通用封装。
func wideToMulti(s string, cp uint32) ([]byte, error) {
	if s == "" {
		return nil, nil
	}
	w := utf16.Encode([]rune(s))
	k32 := syscall.NewLazyDLL("kernel32.dll")
	proc := k32.NewProc("WideCharToMultiByte")
	n, _, _ := proc.Call(uintptr(cp), 0,
		uintptr(unsafe.Pointer(&w[0])), uintptr(len(w)), 0, 0, 0, 0)
	if n == 0 {
		return nil, errWinMulti
	}
	out := make([]byte, n)
	proc.Call(uintptr(cp), 0,
		uintptr(unsafe.Pointer(&w[0])), uintptr(len(w)),
		uintptr(unsafe.Pointer(&out[0])), uintptr(n), 0, 0)
	return out, nil
}

var errWinMulti = &multiError{msg: "WideCharToMultiByte failed"}

type multiError struct{ msg string }

func (e *multiError) Error() string { return e.msg }
