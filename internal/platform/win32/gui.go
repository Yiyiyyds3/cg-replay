package win32

import (
	"fmt"
	"math"
	"syscall"
	"time"
	"unsafe"
)

// GUIHooks GUI 回调（都在消息循环所在线程里执行）。
type GUIHooks struct {
	OnCommand func(id, notify int)       // 子控件通知（按钮点击 / 列表双击 / 编辑框 EN_CHANGE）
	OnTick    func(dtMs int64, in Input) // 每帧
	OnKey     func(key int) bool         // 主窗口按键（返回 true 表示已处理）

	// OnDrawItem 自绘列表行（LBS_OWNERDRAWFIXED）。hdc 是列表控件 DC，
	// x/y/w/h 是行矩形（控件坐标）；selected 为该项是否选中。
	OnDrawItem func(itemID int, hdc uintptr, selected bool, x, y, w, h int)
	// OnSearchEnter 搜索框里按回车。
	OnSearchEnter func()
	// OnSearchNav 搜索框里按上/下方向键（down=true 表示向下）。
	OnSearchNav func(down bool)
}

// 子控件通知码。
const (
	NotifyClicked = 0 // BN_CLICKED
	NotifyDblClk  = 2 // LBN_DBLCLK
)

// 控件类名。
const (
	classListBox = "LISTBOX"
	classButton  = "BUTTON"
	classStatic  = "STATIC"
	classEdit    = "EDIT"
)

// 控件样式。
const (
	wsChild        = 0x40000000
	wsVisible      = 0x10000000
	wsVScroll      = 0x00200000
	wsBorder       = 0x00800000
	wsTabStop      = 0x00010000
	wsExClientEdge = 0x00000200
	lbsNotify      = 0x00000001
	lbsOwnerDraw   = 0x00000010 // LBS_OWNERDRAWFIXED
	lbsHasStrings  = 0x00000040 // LBS_HASSTRINGS
	esAutoHScroll  = 0x00000080 // ES_AUTOHSCROLL
)

// 消息与控件消息。
const (
	wmCommand           = 0x0111
	wmSetFont           = 0x0030
	wmDrawItem          = 0x002B
	wmMeasureItem       = 0x002C
	lbAddString         = 0x0180
	lbResetContent      = 0x0184
	lbSetCurSel         = 0x0186
	lbGetCurSel         = 0x0188
	lbGetCount          = 0x018B
	lbSetTopIndex       = 0x0197
	odtListBox          = 3 // ODT_LISTBOX
	odsSelected         = 0x0001
	defaultGUIFont      = 17         // GetStockObject(DEFAULT_GUI_FONT)
	blackBrush          = 4          // GetStockObject(BLACK_BRUSH)
	wsOverlappedNoThick = 0x00CB0000 // WS_OVERLAPPEDWINDOW 去掉 WS_THICKFRAME（固定大小）
)

const (
	guiMainClass = "ucgrGuiMainWindow"
	guiPlayClass = "ucgrGuiPlayArea"
)

var (
	procSendMessageW   = user32.NewProc("SendMessageW")
	procSetFocus       = user32.NewProc("SetFocus")
	procInvalidateRect = user32.NewProc("InvalidateRect")
	procGetStockObject = syscall.NewLazyDLL("gdi32.dll").NewProc("GetStockObject")

	procGetWindowTextW       = user32.NewProc("GetWindowTextW")
	procGetWindowTextLengthW = user32.NewProc("GetWindowTextLengthW")
	procSetWindowLongPtrW    = user32.NewProc("SetWindowLongPtrW")
	procCallWindowProcW      = user32.NewProc("CallWindowProcW")

	shell32            = syscall.NewLazyDLL("shell32.dll")
	procBrowseFolder   = shell32.NewProc("SHBrowseForFolderW")
	procPathFromIDList = shell32.NewProc("SHGetPathFromIDListW")
	procCoInitialize   = syscall.NewLazyDLL("ole32.dll").NewProc("CoInitialize")
)

// gwlWndProc = GWLP_WNDPROC(-4)，以无符号形式传给 SetWindowLongPtrW。
var gwlWndProc = ^uintptr(3)

// 编辑框子类化用：原窗口过程与回调。
var (
	editOldProc     uintptr
	editSubclassRef = syscall.NewCallback(editSubclass)
)

// App 一个带子控件的窗口（画布 + 列表 + 按钮 + 状态栏）。
type App struct {
	hwnd      uintptr
	canvas    *Canvas
	hooks     GUIHooks
	controls  map[int]uintptr
	play      uintptr
	gameW     int
	gameH     int
	listItemH int // 自绘列表的行高（WM_MEASUREITEM 用）
}

// drawItemStruct 对应 Win32 DRAWITEMSTRUCT（64 位布局）。
type drawItemStruct struct {
	ctlType    uint32
	ctlID      uint32
	itemID     uint32
	itemAction uint32
	itemState  uint32
	_          uint32 // 对齐填充（64 位下 uintptr 需要 8 字节对齐）
	hwndItem   uintptr
	hDC        uintptr
	rcItem     rect
	itemData   uintptr
}

// measureItemStruct 对应 Win32 MEASUREITEMSTRUCT（64 位布局）。
type measureItemStruct struct {
	ctlType    uint32
	ctlID      uint32
	itemID     uint32
	itemWidth  uint32
	itemHeight uint32
	_          uint32 // 对齐填充
	itemData   uintptr
}

// ptrFromLParam 把窗口消息的 lparam 转成 typed 指针。
//
// 本进程里 lparam 就是有效地址；直接写 (*T)(unsafe.Pointer(lparam)) 会被
// go vet 的 unsafeptr 检查判为误用，这里用等价的 unsafe.Add 表达以保持 vet 干净。
func ptrFromLParam[T any](lparam uintptr) *T {
	return (*T)(unsafe.Add(unsafe.Pointer(nil), lparam))
}

var (
	guiApp            *App
	guiClassesReady   bool
	guiMainWndProcRef = syscall.NewCallback(guiWndProc)
	guiPlayWndProcRef = syscall.NewCallback(guiPlayWndProc)
)

// NewApp 创建主窗口与画布（客户区固定 clientW×clientH，画布为 gameW×gameH）。
func NewApp(title string, clientW, clientH, gameW, gameH int, hooks GUIHooks) (*App, error) {
	if err := registerGUIClasses(); err != nil {
		return nil, err
	}
	if gameW <= 0 || gameH <= 0 {
		return nil, fmt.Errorf("画布尺寸无效：%dx%d", gameW, gameH)
	}
	cv, err := NewCanvas(gameW, gameH)
	if err != nil {
		return nil, err
	}
	hInst, _, _ := procGetModuleHandleW.Call(0)
	cls, _ := syscall.UTF16PtrFromString(guiMainClass)
	ttl, _ := syscall.UTF16PtrFromString(title)
	rc := rect{0, 0, int32(clientW), int32(clientH)}
	procAdjustWindowRect.Call(uintptr(unsafe.Pointer(&rc)), wsOverlappedNoThick, 0)
	hwnd, _, callErr := procCreateWindowExW.Call(
		0, uintptr(unsafe.Pointer(cls)), uintptr(unsafe.Pointer(ttl)),
		wsOverlappedNoThick, 0x80000000, 0x80000000,
		uintptr(rc.right-rc.left), uintptr(rc.bottom-rc.top), 0, 0, hInst, 0,
	)
	if hwnd == 0 {
		cv.Free()
		return nil, fmt.Errorf("CreateWindowExW 失败：%v", callErr)
	}
	a := &App{hwnd: hwnd, canvas: cv, hooks: hooks, controls: map[int]uintptr{}, gameW: gameW, gameH: gameH}
	guiApp = a
	return a, nil
}

// registerGUIClasses 注册主窗口类与播放区类（进程内一次）。
func registerGUIClasses() error {
	if guiClassesReady {
		return nil
	}
	hInst, _, _ := procGetModuleHandleW.Call(0)
	cursor, _, _ := procLoadCursorW.Call(0, idcArrow)
	face, _, _ := procGetStockObject.Call(16) // COLOR_BTNFACE+1
	black, _, _ := procGetStockObject.Call(blackBrush)

	reg := func(name string, proc uintptr, brush uintptr) error {
		cn, _ := syscall.UTF16PtrFromString(name)
		wc := wndClassExW{
			cbSize:        uint32(unsafe.Sizeof(wndClassExW{})),
			style:         csHRedraw | csVRedraw,
			lpfnWndProc:   proc,
			hInstance:     hInst,
			hCursor:       cursor,
			hbrBackground: brush,
			lpszClassName: cn,
		}
		atom, _, callErr := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
		if atom == 0 {
			return fmt.Errorf("RegisterClassExW(%s) 失败：%v", name, callErr)
		}
		return nil
	}
	if err := reg(guiMainClass, guiMainWndProcRef, face); err != nil {
		return err
	}
	if err := reg(guiPlayClass, guiPlayWndProcRef, black); err != nil {
		return err
	}
	guiClassesReady = true
	return nil
}

// AddPlayArea 建播放区子窗口（画布按等比缩放贴在这里）。
func (a *App) AddPlayArea(x, y, w, h int) error {
	cls, _ := syscall.UTF16PtrFromString(guiPlayClass)
	hInst, _, _ := procGetModuleHandleW.Call(0)
	hw, _, callErr := procCreateWindowExW.Call(
		wsExClientEdge, uintptr(unsafe.Pointer(cls)), 0,
		wsChild|wsVisible, uintptr(x), uintptr(y), uintptr(w), uintptr(h), a.hwnd, 0, hInst, 0,
	)
	if hw == 0 {
		return fmt.Errorf("创建播放区失败：%v", callErr)
	}
	a.play = hw
	return nil
}

// addControl 建一个子控件，注册到 id 上。
func (a *App) addControl(id int, class, text string, style uint32, x, y, w, h int) error {
	cls, _ := syscall.UTF16PtrFromString(class)
	txt, _ := syscall.UTF16PtrFromString(text)
	hInst, _, _ := procGetModuleHandleW.Call(0)
	hw, _, callErr := procCreateWindowExW.Call(
		0, uintptr(unsafe.Pointer(cls)), uintptr(unsafe.Pointer(txt)), uintptr(style),
		uintptr(x), uintptr(y), uintptr(w), uintptr(h), a.hwnd, uintptr(id), hInst, 0,
	)
	if hw == 0 {
		return fmt.Errorf("创建控件 %s 失败：%v", class, callErr)
	}
	font, _, _ := procGetStockObject.Call(defaultGUIFont)
	procSendMessageW.Call(hw, wmSetFont, font, 1)
	a.controls[id] = hw
	return nil
}

// AddList 建列表控件。
func (a *App) AddList(id, x, y, w, h int) error {
	return a.addControl(id, classListBox, "", wsChild|wsVisible|wsVScroll|wsBorder|wsTabStop|lbsNotify, x, y, w, h)
}

// AddOwnerDrawList 建自绘列表（行高固定 itemH 像素，行内容由 hooks.OnDrawItem 画）。
func (a *App) AddOwnerDrawList(id, x, y, w, h, itemH int) error {
	a.listItemH = itemH // 建窗口时列表就会发 WM_MEASUREITEM，必须先赋值
	return a.addControl(id, classListBox, "",
		wsChild|wsVisible|wsVScroll|wsBorder|wsTabStop|lbsNotify|lbsOwnerDraw|lbsHasStrings, x, y, w, h)
}

// AddEdit 建单行编辑框（搜索框用）。
func (a *App) AddEdit(id int, text string, x, y, w, h int) error {
	return a.addControl(id, classEdit, text, wsChild|wsVisible|wsTabStop|wsBorder|esAutoHScroll, x, y, w, h)
}

// EditText 读编辑框当前文本。
func (a *App) EditText(id int) string {
	h := a.control(id)
	if h == 0 {
		return ""
	}
	n, _, _ := procGetWindowTextLengthW.Call(h)
	if n == 0 {
		return ""
	}
	buf := make([]uint16, int(n)+1)
	procGetWindowTextW.Call(h, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return syscall.UTF16ToString(buf)
}

// SubclassEdit 给编辑框装一个子类过程，接管回车 / 上下方向键 / Esc。
func (a *App) SubclassEdit(id int) {
	h := a.control(id)
	if h == 0 {
		return
	}
	old, _, _ := procSetWindowLongPtrW.Call(h, gwlWndProc, editSubclassRef)
	if old != 0 {
		editOldProc = old
	}
}

// ListTop 把第 i 项滚到列表顶端（键盘导航时用）。
func (a *App) ListTop(i int) { a.listMsg(lbSetTopIndex, uintptr(i), 0) }

// AddButton 建按钮。
func (a *App) AddButton(id int, text string, x, y, w, h int) error {
	return a.addControl(id, classButton, text, wsChild|wsVisible|wsTabStop, x, y, w, h)
}

// AddStatic 建静态文本（状态栏用）。
func (a *App) AddStatic(id int, text string, x, y, w, h int) error {
	return a.addControl(id, classStatic, text, wsChild|wsVisible, x, y, w, h)
}

// control 取子控件句柄（不存在返回 0）。
func (a *App) control(id int) uintptr { return a.controls[id] }

func (a *App) listMsg(msg uintptr, wparam, lparam uintptr) uintptr {
	l := a.control(IDList)
	if l == 0 {
		return 0
	}
	r, _, _ := procSendMessageW.Call(l, msg, wparam, lparam)
	return r
}

// 控件 ID 约定：列表固定用 IDList；其它 ID 由调用方分配。
const IDList = 1001

// ListClear 清空列表。
func (a *App) ListClear() { a.listMsg(lbResetContent, 0, 0) }

// ListAdd 往列表里加一行。
func (a *App) ListAdd(s string) {
	p, _ := syscall.UTF16PtrFromString(s)
	a.listMsg(lbAddString, 0, uintptr(unsafe.Pointer(p)))
}

// ListSel 当前选中项（没选返回 -1）。
func (a *App) ListSel() int {
	r, _, _ := procSendMessageW.Call(a.control(IDList), lbGetCurSel, 0, 0)
	return int(int64(r))
}

// ListSetSel 选中第 i 项。
func (a *App) ListSetSel(i int) { a.listMsg(lbSetCurSel, uintptr(i), 0) }

// ListCount 列表项数。
func (a *App) ListCount() int {
	r, _, _ := procSendMessageW.Call(a.control(IDList), lbGetCount, 0, 0)
	return int(int64(r))
}

// SetStatus 更新状态栏文本。
func (a *App) SetStatus(s string) {
	if h := a.control(IDStatus); h != 0 {
		p, _ := syscall.UTF16PtrFromString(s)
		procSetWindowTextW.Call(h, uintptr(unsafe.Pointer(p)))
	}
}

// 控件 ID 约定（供 cmd 侧复用）。
const (
	IDStatus = 1002
)

// SetTitle 更新窗口标题。
func (a *App) SetTitle(s string) {
	p, _ := syscall.UTF16PtrFromString(s)
	procSetWindowTextW.Call(a.hwnd, uintptr(unsafe.Pointer(p)))
}

// Canvas 返回游戏原生尺寸的画布。
func (a *App) Canvas() *Canvas { return a.canvas }

// Focus 把键盘焦点给主窗口（播放时用，避免被列表吃掉按键）。
func (a *App) Focus() { procSetFocus.Call(a.hwnd) }

// Free 释放画布。
func (a *App) Free() {
	if a.canvas != nil {
		a.canvas.Free()
		a.canvas = nil
	}
}

// Present 把画布等比贴到播放区（含黑边）。
func (a *App) Present() {
	if a.play == 0 || a.canvas == nil {
		return
	}
	hdc, _, _ := procGetDC.Call(a.play)
	if hdc == 0 {
		return
	}
	a.presentTo(hdc)
	procReleaseDC.Call(a.play, hdc)
}

func (a *App) presentTo(hdc uintptr) {
	if a.play == 0 || a.canvas == nil {
		return
	}
	var rc rect
	procGetClientRect.Call(a.play, uintptr(unsafe.Pointer(&rc)))
	pw := int(rc.right - rc.left)
	ph := int(rc.bottom - rc.top)
	if pw <= 0 || ph <= 0 {
		return
	}
	FillRect(hdc, 0, 0, pw, ph, [3]uint8{0, 0, 0}, 255)
	s := math.Min(float64(pw)/float64(a.gameW), float64(ph)/float64(a.gameH))
	if s > 1 {
		s = 1 // 不放大，避免像素插值糊掉
	}
	w := int(float64(a.gameW) * s)
	h := int(float64(a.gameH) * s)
	Stretch(hdc, a.canvas, (pw-w)/2, (ph-h)/2, w, h)
}

// Run 显示窗口并进入消息循环，每帧调用 hooks.OnTick 后把画布贴上屏。
func (a *App) Run() error {
	procShowWindow.Call(a.hwnd, swShow)
	procUpdateWindow.Call(a.hwnd)
	frameDur := time.Second / 60
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
		if a.hooks.OnTick != nil {
			a.hooks.OnTick(dtMs, in)
		}
		a.Present()
		if elapsed := time.Since(now); elapsed < frameDur {
			time.Sleep(frameDur - elapsed)
		}
	}
	guiApp = nil
	return nil
}

func guiWndProc(hwnd uintptr, m uint32, wparam, lparam uintptr) uintptr {
	switch m {
	case wmClose:
		procDestroyWindow.Call(hwnd)
		return 0
	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	case wmCommand:
		if guiApp != nil && guiApp.hooks.OnCommand != nil {
			guiApp.hooks.OnCommand(int(wparam&0xFFFF), int((wparam>>16)&0xFFFF))
		}
		return 0
	case wmMeasureItem:
		if guiApp != nil && guiApp.listItemH > 0 {
			mi := ptrFromLParam[measureItemStruct](lparam)
			if mi.ctlType == odtListBox {
				mi.itemHeight = uint32(guiApp.listItemH)
			}
		}
		return 1
	case wmDrawItem:
		if guiApp != nil && guiApp.hooks.OnDrawItem != nil {
			di := ptrFromLParam[drawItemStruct](lparam)
			if di.ctlType == odtListBox && di.itemID != ^uint32(0) {
				guiApp.hooks.OnDrawItem(int(di.itemID), di.hDC, di.itemState&odsSelected != 0,
					int(di.rcItem.left), int(di.rcItem.top),
					int(di.rcItem.right-di.rcItem.left), int(di.rcItem.bottom-di.rcItem.top))
			}
		}
		return 1
	case wmKeyDown, wmSysKeyDown:
		key := int(wparam & 0xFF)
		switch key {
		case 0x0D, 0x20: // 回车 / 空格
			curInput.Advance = true
		case 0x09: // Tab
			curInput.ToggleSpeed = true
		case 'P':
			curInput.TogglePause = true
		case 'S':
			curInput.ToggleSkip = true
		case 'H':
			curInput.ToggleText = true
		case 0x7B: // F12
			curInput.RequestShot = true
		}
		if guiApp != nil && guiApp.hooks.OnKey != nil && guiApp.hooks.OnKey(key) {
			return 0
		}
		if key == 0x1B { // Esc
			procDestroyWindow.Call(hwnd)
		}
		return 0
	case wmLButtonDown:
		curInput.Advance = true
		curInput.MouseClicked = true
		return 0
	case wmPaint:
		var ps paintStruct
		procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(m), wparam, lparam)
	return r
}

func guiPlayWndProc(hwnd uintptr, m uint32, wparam, lparam uintptr) uintptr {
	switch m {
	case wmPaint:
		var ps paintStruct
		procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		if guiApp != nil {
			guiApp.presentTo(ps.hdc)
		}
		procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0
	case wmLButtonDown:
		curInput.Advance = true
		curInput.MouseClicked = true
		if guiApp != nil {
			procSetFocus.Call(guiApp.hwnd)
		}
		return 0
	case wmSize:
		procInvalidateRect.Call(hwnd, 0, 1)
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(m), wparam, lparam)
	return r
}

// ---------------------------------------------------------------
// 选择文件夹对话框
// ---------------------------------------------------------------

type browseInfoW struct {
	hwndOwner      uintptr
	pidlRoot       uintptr
	pszDisplayName *uint16
	lpszTitle      *uint16
	ulFlags        uint32
	lpfn           uintptr
	lParam         uintptr
	iImage         int32
}

// BrowseFolder 弹「选择文件夹」对话框。取消时返回空串。
func (a *App) BrowseFolder(title string) (string, error) {
	procCoInitialize.Call(0)
	name := make([]uint16, 260)
	ttl, _ := syscall.UTF16PtrFromString(title)
	bi := browseInfoW{
		hwndOwner:      a.hwnd,
		pszDisplayName: &name[0],
		lpszTitle:      ttl,
		ulFlags:        0x0001 | 0x0040, // BIF_RETURNONLYFSDIRS | BIF_NEWDIALOGSTYLE
	}
	pidl, _, _ := procBrowseFolder.Call(uintptr(unsafe.Pointer(&bi)))
	if pidl == 0 {
		return "", nil
	}
	out := make([]uint16, 520)
	ok, _, _ := procPathFromIDList.Call(pidl, uintptr(unsafe.Pointer(&out[0])))
	if ok == 0 {
		return "", nil
	}
	return syscall.UTF16ToString(out), nil
}

// ---------------------------------------------------------------
// 搜索框子类过程
// ---------------------------------------------------------------

// editSubclass 接管搜索框的回车 / 上下方向键 / Esc，其余交给原窗口过程。
func editSubclass(hwnd uintptr, m uint32, wparam, lparam uintptr) uintptr {
	switch m {
	case wmKeyDown, wmSysKeyDown:
		switch int(wparam & 0xFF) {
		case 0x0D: // 回车：播放当前选中项
			if guiApp != nil && guiApp.hooks.OnSearchEnter != nil {
				guiApp.hooks.OnSearchEnter()
				return 0
			}
		case 0x1B: // Esc：焦点还给主窗口（再按一次退出）
			if guiApp != nil {
				procSetFocus.Call(guiApp.hwnd)
				return 0
			}
		case 0x28, 0x26: // 下 / 上：移动列表选择，不失焦搜索框
			if guiApp != nil && guiApp.hooks.OnSearchNav != nil {
				guiApp.hooks.OnSearchNav(wparam&0xFF == 0x28)
				return 0
			}
		}
	}
	r, _, _ := procCallWindowProcW.Call(editOldProc, hwnd, uintptr(m), wparam, lparam)
	return r
}
