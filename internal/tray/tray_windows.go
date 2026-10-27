// Package tray keeps the window in the notification area: minimizing or
// closing hides it, a single click opens shortcuts, and a double click restores it.
package tray

import (
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/131/ssh-agent-totp/internal/clock"
	"github.com/131/ssh-agent-totp/internal/i18n"
	"github.com/131/ssh-agent-totp/internal/tokens"
)

var (
	user32              = windows.NewLazySystemDLL("user32.dll")
	shell32             = windows.NewLazySystemDLL("shell32.dll")
	kernel32            = windows.NewLazySystemDLL("kernel32.dll")
	procSetWindowLong   = user32.NewProc("SetWindowLongPtrW")
	procCallWindowProc  = user32.NewProc("CallWindowProcW")
	procShowWindow      = user32.NewProc("ShowWindow")
	procSetForeground   = user32.NewProc("SetForegroundWindow")
	procLoadImage       = user32.NewProc("LoadImageW")
	procGetSystemMetric = user32.NewProc("GetSystemMetrics")
	procCreatePopupMenu = user32.NewProc("CreatePopupMenu")
	procAppendMenu      = user32.NewProc("AppendMenuW")
	procTrackPopupMenu  = user32.NewProc("TrackPopupMenu")
	procDestroyMenu     = user32.NewProc("DestroyMenu")
	procGetCursorPos    = user32.NewProc("GetCursorPos")
	procPostMessage     = user32.NewProc("PostMessageW")
	procRegisterMessage = user32.NewProc("RegisterWindowMessageW")
	procGetDpiForSystem = user32.NewProc("GetDpiForSystem")
	procNotifyIcon      = shell32.NewProc("Shell_NotifyIconW")
	procFindWindow      = user32.NewProc("FindWindowW")
	procSetTimer        = user32.NewProc("SetTimer")
	procKillTimer       = user32.NewProc("KillTimer")
	procDoubleClickTime = user32.NewProc("GetDoubleClickTime")
	procCreateMutex     = kernel32.NewProc("CreateMutexW")
)

const (
	gwlpWndProc     = ^uintptr(3) // -4
	wmDestroy       = 0x0002
	wmSize          = 0x0005
	wmClose         = 0x0010
	wmCommand       = 0x0111
	wmTimer         = 0x0113
	wmExitSizeMove  = 0x0232
	wmLButtonUp     = 0x0202
	wmLButtonDblClk = 0x0203
	wmRButtonUp     = 0x0205
	wmTray          = 0x8000 + 1 // WM_APP + 1
	wmDispatch      = 0x8000 + 2
	sizeMinimized   = 1
	swHide          = 0
	swRestore       = 9
	nimAdd          = 0
	nimModify       = 1
	nimDelete       = 2
	nifMessage      = 0x1
	nifIcon         = 0x2
	nifTip          = 0x4
	nifInfo         = 0x10
	imageIcon       = 1
	smCxSmIcon      = 49
	smCySmIcon      = 50
	mfString        = 0
	mfSeparator     = 0x800
	tpmReturnCmd    = 0x100
	tpmRightButton  = 0x2
	menuOpen        = 1
	menuQuit        = 2
	IconID          = 1
)

const (
	clockTimerID    = 1
	clickTimerID    = 2
	firstPinCommand = 3
	firstFrameID    = 2
	copiedFrameID   = 32
	menuFrameOffset = 32
	lrShared        = 0x8000
)

type notifyIconData struct {
	CbSize          uint32
	Wnd             uintptr
	ID              uint32
	Flags           uint32
	CallbackMessage uint32
	Icon            uintptr
	Tip             [128]uint16
	State           uint32
	StateMask       uint32
	Info            [256]uint16
	Version         uint32
	InfoTitle       [64]uint16
	InfoFlags       uint32
	GUIDItem        windows.GUID
	BalloonIcon     uintptr
}

type Tray struct {
	Pinned         func() []tokens.PinnedItem
	CopyPinned     func(int)
	AfterModal     func()
	dispatch       uiDispatch
	clock          *clock.Clock
	instance       windows.Handle
	flashUntil     time.Time
	doubleClick    bool
	menuIcon       uintptr
	menuHandle     uintptr
	menuBitmap     uintptr
	menuPinCount   int
	hwnd           uintptr
	original       uintptr
	data           notifyIconData
	taskbarCreated uintptr
	show           uintptr
	quitting       bool
	borderless     bool
}

const WindowTitle = "ssh-agent-totp"

func registeredMessage(name string) uintptr {
	pointer, _ := windows.UTF16PtrFromString(name)
	message, _, _ := procRegisterMessage.Call(uintptr(unsafe.Pointer(pointer)))
	return message
}

// ActivateExisting shows the window of an instance already running in this
// session and reports whether one exists. Instances cannot share the WebView2
// data folder, and one window is enough. After an update the new executable
// waits (up to 15 s) for the previous instance to exit instead.
func ActivateExisting(waitPrevious bool) bool {
	name, _ := windows.UTF16PtrFromString(`Local\ssh-agent-totp`)
	for attempt := 0; ; attempt++ {
		handle, _, err := procCreateMutex.Call(0, 0, uintptr(unsafe.Pointer(name)))
		if err != windows.ERROR_ALREADY_EXISTS {
			return false
		}

		if !waitPrevious || attempt == 75 {
			break
		}
		// Our handle keeps the mutex alive: release it before retrying.

		windows.CloseHandle(windows.Handle(handle))
		time.Sleep(200 * time.Millisecond)
	}

	class, _ := windows.UTF16PtrFromString("webview")
	title, _ := windows.UTF16PtrFromString(WindowTitle)
	if hwnd, _, _ := procFindWindow.Call(uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(title))); hwnd != 0 {
		procPostMessage.Call(hwnd, registeredMessage("ssh-agent-totp-show"), 0, 0)
	}
	return true
}

// Quit ends the application, as Quitter does.
func (t *Tray) Quit() {
	t.quitting = true
	procPostMessage.Call(t.hwnd, wmClose, 0, 0)
}

// Scale converts 96-dpi window dimensions for the per-monitor-aware manifest.
func Scale(value uint) uint {
	if procGetDpiForSystem.Find() != nil {
		return value
	}

	dpi, _, _ := procGetDpiForSystem.Call()
	if dpi == 0 {
		return value
	}

	return value * uint(dpi) / 96
}

func Install(hwnd uintptr, ticks *clock.Clock) *Tray {
	t := &Tray{hwnd: hwnd, clock: ticks}
	var instance windows.Handle
	windows.GetModuleHandleEx(0, nil, &instance)
	t.instance = instance
	cx, _, _ := procGetSystemMetric.Call(smCxSmIcon)
	cy, _, _ := procGetSystemMetric.Call(smCySmIcon)
	icon, _, _ := procLoadImage.Call(uintptr(instance), IconID, imageIcon, cx, cy, lrShared)
	t.menuIcon, _, _ = procLoadImage.Call(uintptr(instance), IconID+menuFrameOffset, imageIcon, cx, cy, lrShared)
	t.data = notifyIconData{Wnd: hwnd, ID: 1, Flags: nifMessage | nifIcon | nifTip, CallbackMessage: wmTray, Icon: icon}
	t.data.CbSize = uint32(unsafe.Sizeof(t.data))
	copy(t.data.Tip[:len(t.data.Tip)-1], windows.StringToUTF16(WindowTitle))
	t.taskbarCreated = registeredMessage("TaskbarCreated")
	t.show = registeredMessage("ssh-agent-totp-show")
	t.add()
	t.original, _, _ = procSetWindowLong.Call(hwnd, gwlpWndProc, syscall.NewCallback(t.wndproc))
	ticks.Subscribe(t.spin)
	ticks.Subscribe(func(clock.Tick) { t.updateMenuIcon() })
	procSetTimer.Call(hwnd, clockTimerID, 1000, 0)
	return t
}

func (t *Tray) add() {
	t.data.Flags = nifMessage | nifIcon | nifTip
	procNotifyIcon.Call(nimAdd, uintptr(unsafe.Pointer(&t.data)))
}

// Show restores the window to report a tray action error.
func (t *Tray) Show() { t.restore() }

func (t *Tray) restore() {
	procShowWindow.Call(t.hwnd, swRestore)
	procSetForeground.Call(t.hwnd)
}

func appendMenu(menu, flags, id uintptr, label string) {
	text, _ := windows.UTF16PtrFromString(label)
	procAppendMenu.Call(menu, flags, id, uintptr(unsafe.Pointer(text)))
}

func (t *Tray) menu() {
	menu, _, _ := procCreatePopupMenu.Call()
	t.menuHandle = menu
	defer func() {
		procDestroyMenu.Call(menu)
		procDeleteObject.Call(t.menuBitmap)
		t.menuHandle, t.menuBitmap, t.menuPinCount = 0, 0, 0
		if t.AfterModal != nil {
			t.AfterModal()
		}
	}()

	pins := []tokens.PinnedItem{}
	if t.Pinned != nil {
		pins = t.Pinned()
	}
	t.menuPinCount = len(pins)
	for index, pin := range pins {
		appendMenu(menu, mfString, uintptr(index+firstPinCommand), strings.ReplaceAll(pin.Label, "&", "&&"))
	}

	if len(pins) > 0 {
		procAppendMenu.Call(menu, mfSeparator, 0, 0)
	}
	appendMenu(menu, mfString, menuOpen, i18n.T("tray.open"))
	procAppendMenu.Call(menu, mfSeparator, 0, 0)
	appendMenu(menu, mfString, menuQuit, i18n.T("tray.quit"))
	t.updateMenuIcon()
	var point struct{ X, Y int32 }
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&point)))
	// Required so the menu closes when clicking elsewhere.
	procSetForeground.Call(t.hwnd)
	command, _, _ := procTrackPopupMenu.Call(menu, tpmReturnCmd|tpmRightButton, uintptr(point.X), uintptr(point.Y), 0, t.hwnd, 0)
	switch command {
	case menuOpen:
		t.restore()
	case menuQuit:
		t.Quit()
	default:
		if command >= firstPinCommand && command-firstPinCommand < uintptr(len(pins)) && t.CopyPinned != nil {
			t.CopyPinned(pins[command-firstPinCommand].ID)
		}
	}
}

func (t *Tray) wndproc(hwnd, msg, wparam, lparam uintptr) uintptr {
	switch {
	case msg == wmNcCalcSize && wparam != 0 && t.borderless:
		return 0 // no title bar nor border: the UI draws them
	case msg == wmDispatch:
		t.runDispatched()
		return 0

	case msg == wmTray:
		switch lparam & 0xffff {

		case wmLButtonUp:
			if t.doubleClick {
				t.doubleClick = false
				break
			}

			delay, _, _ := procDoubleClickTime.Call()
			procSetTimer.Call(hwnd, clickTimerID, delay, 0)
		case wmLButtonDblClk:
			procKillTimer.Call(hwnd, clickTimerID)
			t.doubleClick = true
			t.restore()
		case wmRButtonUp:
			procKillTimer.Call(hwnd, clickTimerID)
			t.menu()
		}
		return 0
	case msg == wmExitSizeMove:
		if t.AfterModal != nil {
			t.AfterModal()
		}
	case msg == wmTimer && wparam == clockTimerID:
		t.clock.Fire(time.Now())
		return 0
	case msg == wmTimer && wparam == clickTimerID:
		procKillTimer.Call(hwnd, clickTimerID)
		t.menu()
		return 0
	case msg == t.taskbarCreated && msg != 0:
		t.add()
	case msg == t.show && msg != 0:
		t.restore()
		return 0
	case msg == wmClose && !t.quitting:
		procShowWindow.Call(hwnd, swHide)
		return 0
	case msg == wmDestroy:
		procKillTimer.Call(hwnd, clockTimerID)
		procKillTimer.Call(hwnd, clickTimerID)
		procNotifyIcon.Call(nimDelete, uintptr(unsafe.Pointer(&t.data)))
	}
	result, _, _ := procCallWindowProc.Call(t.original, hwnd, msg, wparam, lparam)
	if msg == wmSize && wparam == sizeMinimized {
		procShowWindow.Call(hwnd, swHide)
	}
	return result
}

// spin reuses shared icon resources: one sector per second, no drawing loop.
func (t *Tray) spin(tick clock.Tick) {
	id := uintptr(firstFrameID + tick.Modulus)
	if tick.Now < t.flashUntil.UnixMilli() && tick.Modulus%2 == 0 {
		id = copiedFrameID
	}
	cx, _, _ := procGetSystemMetric.Call(smCxSmIcon)
	cy, _, _ := procGetSystemMetric.Call(smCySmIcon)
	t.data.Icon, _, _ = procLoadImage.Call(uintptr(t.instance), id, imageIcon, cx, cy, lrShared)
	t.menuIcon, _, _ = procLoadImage.Call(uintptr(t.instance), id+menuFrameOffset, imageIcon, cx, cy, lrShared)
	t.data.Flags = nifIcon
	procNotifyIcon.Call(nimModify, uintptr(unsafe.Pointer(&t.data)))
}

// Copied gives feedback without opening the window or exposing the code.
func (t *Tray) Copied() {
	t.flashUntil = time.Now().Add(3 * time.Second)
	t.data.Flags, t.data.InfoFlags = nifInfo, 1|0x10 // information, no sound
	copy(t.data.Info[:], windows.StringToUTF16(i18n.T("tray.copied")))
	copy(t.data.InfoTitle[:], windows.StringToUTF16(WindowTitle))
	procNotifyIcon.Call(nimModify, uintptr(unsafe.Pointer(&t.data)))
}
