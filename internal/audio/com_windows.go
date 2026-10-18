//go:build windows

package audio

import (
	"fmt"
	"sync"
	"syscall"
	"unsafe"
)

// guid 是 Windows COM GUID。
type guid struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

func (g guid) String() string {
	return fmt.Sprintf("{%08X-%04X-%04X-%02X%02X-%02X%02X%02X%02X%02X%02X}",
		g.Data1, g.Data2, g.Data3,
		g.Data4[0], g.Data4[1], g.Data4[2], g.Data4[3],
		g.Data4[4], g.Data4[5], g.Data4[6], g.Data4[7])
}

// hresult 包装 COM/HRESULT 错误码。
type hresult int32

func (h hresult) Error() string {
	return fmt.Sprintf("HRESULT 0x%08X", uint32(h))
}

func failed(hr uintptr) bool { return int32(uint32(hr)) < 0 }

func hrErr(hr uintptr) error {
	if failed(hr) {
		return hresult(int32(uint32(hr)))
	}
	return nil
}

// comCall 通过 vtable 调用 COM 方法。this 是接口指针，idx 为方法在 vtable 中的槽位。
func comCall(this unsafe.Pointer, idx uintptr, args ...uintptr) uintptr {
	vtbl := *(*unsafe.Pointer)(this)
	fn := *(*uintptr)(unsafe.Add(vtbl, idx*unsafe.Sizeof(uintptr(0))))
	full := make([]uintptr, 0, len(args)+1)
	full = append(full, uintptr(this))
	full = append(full, args...)
	r, _, _ := syscall.SyscallN(fn, full...)
	return r
}

// comRelease 调用 IUnknown::Release（槽位 2）。
func comRelease(this unsafe.Pointer) {
	if this != nil {
		comCall(this, 2)
	}
}

var (
	mfplat      = syscall.NewLazyDLL("mfplat.dll")
	mfreadwrite = syscall.NewLazyDLL("mfreadwrite.dll")
	ole32       = syscall.NewLazyDLL("ole32.dll")

	procMFStartup                   = mfplat.NewProc("MFStartup")
	procMFShutdown                  = mfplat.NewProc("MFShutdown")
	procMFCreateMediaType           = mfplat.NewProc("MFCreateMediaType")
	procMFCreateSourceReaderFromURL = mfreadwrite.NewProc("MFCreateSourceReaderFromURL")
	procCoInitializeEx              = ole32.NewProc("CoInitializeEx")
)

const (
	mfVersion           = 0x00020070 // MF_VERSION(2, 0x70)
	mfStartupNoSocket   = 0x1
	coInitMultithreaded = 0x0
)

var (
	mfOnce sync.Once
	mfErr  error
)

// ensureMF 一次性初始化 COM（MTA）与 Media Foundation。
func ensureMF() error {
	mfOnce.Do(func() {
		// 已初始化过会返回 RPC_E_CHANGED_MODE / S_FALSE，忽略即可。
		procCoInitializeEx.Call(0, coInitMultithreaded)
		hr, _, _ := procMFStartup.Call(uintptr(mfVersion), uintptr(mfStartupNoSocket))
		if err := hrErr(hr); err != nil {
			mfErr = fmt.Errorf("MFStartup: %w", err)
		}
	})
	return mfErr
}
