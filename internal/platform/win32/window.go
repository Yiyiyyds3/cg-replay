// Package win32 提供 Windows 图形界面渲染（GDI）与窗口消息循环。
//
// 本包只依赖标准库与 Windows 系统 DLL（gdi32.dll / user32.dll），
// 不使用任何第三方库（本机无网络，不能拉取模块）。
//
// 分工：画面合成分解到 Go 侧（internal/player），本包负责
//  1. 建一块 32bpp 顶朝下的 DIB（Canvas）供 Go 侧直接写像素；
//  2. 开一个窗口，按客户区大小等比缩放把画布贴上去；
//  3. 收集键盘/鼠标输入，驱动调用方的逐帧回调。
package win32

import (
	"fmt"
	"syscall"
	"time"
	"unsafe"
)

var (
	user32 = syscall.NewLazyDLL("user32.dll")

	procRegisterClassExW = user32.NewProc("RegisterClassExW")
	procCreateWindowExW  = user32.NewProc("CreateWindowExW")
	procDefWindowProcW   = user32.NewProc("DefWindowProcW")
	procDestroyWindow    = user32.NewProc("DestroyWindow")
	procPostQuitMessage  = user32.NewProc("PostQuitMessage")
	procShowWindow       = user32.NewProc("ShowWindow")
	procUpdateWindow     = user32.NewProc("UpdateWindow")
	procPeekMessageW     = user32.NewProc("PeekMessageW")
	procTranslateMessage = user32.NewProc("TranslateMessage")
	procDispatchMessageW = user32.NewProc("DispatchMessageW")
	procGetClientRect    = user32.NewProc("GetClientRect")
	procGetDC            = user32.NewProc("GetDC")
	procReleaseDC        = user32.NewProc("ReleaseDC")
	procBeginPaint       = user32.NewProc("BeginPaint")
	procEndPaint         = user32.NewProc("EndPaint")
	procLoadCursorW      = user32.NewProc("LoadCursorW")
	procSetWindowTextW   = user32.NewProc("SetWindowTextW")
	procGetSystemMetrics = user32.NewProc("GetSystemMetrics")
	procAdjustWindowRect = user32.NewProc("AdjustWindowRect")

	procGetModuleHandleW = syscall.NewLazyDLL("kernel32.dll").NewProc("GetModuleHandleW")
)

// 窗口消息
const (
	wmDestroy       = 0x0002
	wmClose         = 0x0010
	wmQuit          = 0x0012
	wmPaint         = 0x000F
	wmEraseBkgnd    = 0x0014
	wmKeyDown       = 0x0100
	wmSysKeyDown    = 0x0104
	wmLButtonDown   = 0x0201
	wmSize          = 0x0005
	wmGetMinMaxInfo = 0x0024
)

// 窗口样式
const (
	wsOverlappedWindow = 0x00CF0000
	csHRedraw          = 0x0002
	csVRedraw          = 0x0001
	swShow             = 5
	pmRemove           = 0x0001
	idcArrow           = 32512
	smCXScreen         = 0
	smCYScreen         = 1
)

type wndClassExW struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     uintptr
	hIcon         uintptr
	hCursor       uintptr
	hbrBackground uintptr
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       uintptr
}

type point struct {
	x, y int32
}

type msgT struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      point
}

type paintStruct struct {
	hdc         uintptr
	fErase      int32
	rcPaint     rect
	fRestore    int32
	fIncUpdate  int32
	rgbReserved [32]byte
}

// Input 是一帧内收集到的输入事件。
type Input struct {
	Advance      bool // 回车 / 空格 / 鼠标左键：推进或结束当前点击点
	TogglePause  bool // P 键 / 空格：暂停/继续
	ToggleSpeed  bool // Tab：加速
	ToggleSkip   bool // S：跳过等待（快速走完）
	ToggleText   bool // H：显示/隐藏文本（调试）
	RequestShot  bool // F12：截图请求
	MouseX       int  // 最近一次鼠标左键位置（画布坐标）
	MouseY       int
	MouseClicked bool
}

// Options 是窗口参数。
type Options struct {
	Title  string
	Width  int // 画布宽（像素）
	Height int // 画布高（像素）
	FPS    int // 目标帧率，默认 60
}

// wndProc 回调必须长期持有，避免被 GC 回收。
var wndProcRef = syscall.NewCallback(wndProc)

var (
	curWindow *Window
	curOpt    Options
	curFrame  func(c *Canvas, dtMs int64, in Input)
	curQuit   bool
	curInput  Input
	canvasX   int32
	canvasY   int32
	canvasW   int32
	canvasH   int32
)

// Window 持有一个窗口与配套画布。
type Window struct {
	hwnd   uintptr
	canvas *Canvas
	opt    Options
}

// Canvas 返回窗口画布（固定为 Options 的尺寸）。
func (w *Window) Canvas() *Canvas { return w.canvas }

func wndProc(hwnd uintptr, m uint32, wparam, lparam uintptr) uintptr {
	switch m {
	case wmClose:
		procDestroyWindow.Call(hwnd)
		return 0
	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	case wmPaint:
		var ps paintStruct
		procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0
	case wmEraseBkgnd:
		return 1
	case wmKeyDown, wmSysKeyDown:
		switch wparam {
		case 0x0D, 0x20: // 回车 / 空格
			curInput.Advance = true
		case 0x09: // Tab
			curInput.ToggleSpeed = true
		case 0x1B: // Esc
			procDestroyWindow.Call(hwnd)
		case 'P':
			curInput.TogglePause = true
		case 'S':
			curInput.ToggleSkip = true
		case 'H':
			curInput.ToggleText = true
		case 0x7B: // F12
			curInput.RequestShot = true
		}
		return 0
	case wmLButtonDown:
		curInput.Advance = true
		curInput.MouseClicked = true
		curInput.MouseX = int(int16(lparam & 0xFFFF))
		curInput.MouseY = int(int16((lparam >> 16) & 0xFFFF))
		return 0
	case wmSize:
		layoutClient(hwnd)
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(m), wparam, lparam)
	return r
}

// layoutClient 按客户区大小计算画布等比缩放后的位置与尺寸。
func layoutClient(hwnd uintptr) {
	var rc rect
	procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
	cw := int32(rc.right - rc.left)
	ch := int32(rc.bottom - rc.top)
	if cw <= 0 || ch <= 0 || curOpt.Width <= 0 || curOpt.Height <= 0 {
		return
	}
	sx := float64(cw) / float64(curOpt.Width)
	sy := float64(ch) / float64(curOpt.Height)
	s := sx
	if sy < s {
		s = sy
	}
	if s > 1 {
		s = 1 // 不放大，避免像素插值糊掉
	}
	w := int32(float64(curOpt.Width) * s)
	h := int32(float64(curOpt.Height) * s)
	canvasW, canvasH = w, h
	canvasX = (cw - w) / 2
	canvasY = (ch - h) / 2
}

// Run 创建窗口并进入消息循环，逐帧调用 frame。
//
// frame 收到窗口画布与距上一帧的毫秒数，负责把这一帧画进画布
// （可以写像素，也可以用 TextDrawer 在画布 DC 上画字）。
// frame 为 nil 时画布内容保持不变。
func Run(opt Options, frame func(c *Canvas, dtMs int64, in Input)) error {
	if opt.FPS <= 0 {
		opt.FPS = 60
	}
	if opt.Width <= 0 || opt.Height <= 0 {
		return fmt.Errorf("画布尺寸无效：%dx%d", opt.Width, opt.Height)
	}
	cv, err := NewCanvas(opt.Width, opt.Height)
	if err != nil {
		return err
	}
	defer cv.Free()

	hInst, _, _ := procGetModuleHandleW.Call(0)
	className := "ucgrPlayWindowClass"
	cn, _ := syscall.UTF16PtrFromString(className)
	title, _ := syscall.UTF16PtrFromString(opt.Title)
	cursor, _, _ := procLoadCursorW.Call(0, idcArrow)

	wc := wndClassExW{
		cbSize:        uint32(unsafe.Sizeof(wndClassExW{})),
		style:         csHRedraw | csVRedraw,
		lpfnWndProc:   wndProcRef,
		hInstance:     hInst,
		hCursor:       cursor,
		hbrBackground: 0,
		lpszClassName: cn,
	}
	atom, _, callErr := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	if atom == 0 {
		return fmt.Errorf("RegisterClassExW 失败：%v", callErr)
	}

	// 窗口尺寸：客户区不小于画布，超出屏幕时收缩
	var rc = rect{0, 0, int32(opt.Width), int32(opt.Height)}
	procAdjustWindowRect.Call(uintptr(unsafe.Pointer(&rc)), wsOverlappedWindow, 0)
	winW := int(rc.right - rc.left)
	winH := int(rc.bottom - rc.top)
	scrW, _, _ := procGetSystemMetrics.Call(smCXScreen)
	scrH, _, _ := procGetSystemMetrics.Call(smCYScreen)
	if winW > int(scrW)-80 {
		winW = int(scrW) - 80
	}
	if winH > int(scrH)-120 {
		winH = int(scrH) - 120
	}

	hwnd, _, callErr := procCreateWindowExW.Call(
		0, uintptr(unsafe.Pointer(cn)), uintptr(unsafe.Pointer(title)),
		wsOverlappedWindow, 0x80000000 /*CW_USEDEFAULT*/, 0x80000000,
		uintptr(winW), uintptr(winH), 0, 0, hInst, 0,
	)
	if hwnd == 0 {
		return fmt.Errorf("CreateWindowExW 失败：%v", callErr)
	}
	curWindow = &Window{hwnd: hwnd, canvas: cv, opt: opt}
	curOpt = opt
	curFrame = frame
	curQuit = false
	curInput = Input{}

	layoutClient(hwnd)
	procShowWindow.Call(hwnd, swShow)
	procUpdateWindow.Call(hwnd)

	frameDur := time.Second / time.Duration(opt.FPS)
	last := time.Now()

	for !curQuit {
		var m msgT
		for {
			r, _, _ := procPeekMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0, pmRemove)
			if r == 0 {
				break
			}
			if m.message == wmQuit {
				curQuit = true
				break
			}
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
			procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
		}
		if curQuit {
			break
		}

		now := time.Now()
		dt := now.Sub(last)
		last = now
		dtMs := dt.Milliseconds()
		if dtMs < 0 {
			dtMs = 0
		}

		in := curInput
		curInput = Input{}

		if curFrame != nil {
			curFrame(cv, dtMs, in)
		}

		hdc, _, _ := procGetDC.Call(hwnd)
		if hdc != 0 {
			if canvasW > 0 && canvasH > 0 {
				Stretch(hdc, cv, int(canvasX), int(canvasY), int(canvasW), int(canvasH))
			}
			procReleaseDC.Call(hwnd, hdc)
		}

		if elapsed := time.Since(now); elapsed < frameDur {
			time.Sleep(frameDur - elapsed)
		}
	}
	curWindow = nil
	curFrame = nil
	return nil
}
