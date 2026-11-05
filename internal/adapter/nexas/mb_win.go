//go:build windows

package nexas

import (
	"syscall"
	"unicode/utf16"
	"unsafe"
)

// mbToWide 用 Windows MultiByteToWideChar 解码多字节编码
// （932 = CP932，936 = GBK），坏字节替换为 U+FFFD。
// 包内自带一份（与 nscript/catsystem2 同款）：不改核心，不引第三方依赖。
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

// mbStrict 用 MB_ERR_INVALID_CHARS 严格解码：有非法字节序列即失败。
// （当前解码路径未依赖它——名字投票用纯 Go 的 cp932WellFormed，
// 保留供后续需要严格语义时使用。）
const mbErrInvalidChars = 0x1

func mbStrict(b []byte, cp uint32) bool {
	if len(b) == 0 {
		return true
	}
	k32 := syscall.NewLazyDLL("kernel32.dll")
	proc := k32.NewProc("MultiByteToWideChar")
	n, _, _ := proc.Call(uintptr(cp), uintptr(mbErrInvalidChars),
		uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)), 0, 0)
	return n != 0
}

var errWin = &winError{msg: "MultiByteToWideChar failed"}

type winError struct{ msg string }

func (e *winError) Error() string { return e.msg }
