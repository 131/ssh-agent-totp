// Package tray keeps the window in the notification area: minimizing or
// closing hides it, the icon restores it, and its menu is the only way to quit.
package tray

import (
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

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
	procCreateMutex     = kernel32.NewProc("CreateMutexW")
)

const (
	gwlpWndProc     = ^uintptr(3) // -4
	wmDestroy       = 0x0002
	wmSize          = 0x0005
	wmClose         = 0x0010
	wmCommand       = 0x0111
	wmLButtonUp     = 0x0202
	wmLButtonDblClk = 0x0203
	wmRButtonUp     = 0x0205
	wmTray          = 0x8000 + 1 // WM_APP + 1
	sizeMinimized   = 1
	swHide          = 0
	swRestore       = 9
	nimAdd          = 0
	nimDelete       = 2
	nifMessage      = 0x1
	nifIcon         = 0x2
	nifTip          = 0x4
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

func Install(hwnd uintptr) *Tray {
	t := &Tray{hwnd: hwnd}
	var instance windows.Handle
	windows.GetModuleHandleEx(0, nil, &instance)
	cx, _, _ := procGetSystemMetric.Call(smCxSmIcon)
	cy, _, _ := procGetSystemMetric.Call(smCySmIcon)
	icon, _, _ := procLoadImage.Call(uintptr(instance), IconID, imageIcon, cx, cy, 0)
	t.data = notifyIconData{Wnd: hwnd, ID: 1, Flags: nifMessage | nifIcon | nifTip, CallbackMessage: wmTray, Icon: icon}
	t.data.CbSize = uint32(unsafe.Sizeof(t.data))
	copy(t.data.Tip[:len(t.data.Tip)-1], windows.StringToUTF16(WindowTitle))
	t.taskbarCreated = registeredMessage("TaskbarCreated")
	t.show = registeredMessage("ssh-agent-totp-show")
	t.add()
	t.original, _, _ = procSetWindowLong.Call(hwnd, gwlpWndProc, syscall.NewCallback(t.wndproc))
	return t
}

func (t *Tray) add() { procNotifyIcon.Call(nimAdd, uintptr(unsafe.Pointer(&t.data))) }

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
	defer procDestroyMenu.Call(menu)
	pins := []tokens.PinnedItem{}
	if t.Pinned != nil {
		pins = t.Pinned()
	}
	for index, pin := range pins {
		appendMenu(menu, mfString, uintptr(index+3), strings.ReplaceAll(pin.Label, "&", "&&"))
	}
	if len(pins) > 0 {
		procAppendMenu.Call(menu, mfSeparator, 0, 0)
	}
	appendMenu(menu, mfString, menuOpen, i18n.T("tray.open"))
	procAppendMenu.Call(menu, mfSeparator, 0, 0)
	appendMenu(menu, mfString, menuQuit, i18n.T("tray.quit"))
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
		if command >= 3 && command-3 < uintptr(len(pins)) && t.CopyPinned != nil {
			t.CopyPinned(pins[command-3].ID)
		}
	}
}

func (t *Tray) wndproc(hwnd, msg, wparam, lparam uintptr) uintptr {
	switch {
	case msg == wmNcCalcSize && wparam != 0 && t.borderless:
		return 0 // no title bar nor border: the UI draws them
	case msg == wmTray:
		switch lparam & 0xffff {
		case wmLButtonUp, wmLButtonDblClk:
			t.restore()
		case wmRButtonUp:
			t.menu()
		}
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
		procNotifyIcon.Call(nimDelete, uintptr(unsafe.Pointer(&t.data)))
	}
	result, _, _ := procCallWindowProc.Call(t.original, hwnd, msg, wparam, lparam)
	if msg == wmSize && wparam == sizeMinimized {
		procShowWindow.Call(hwnd, swHide)
	}
	return result
}
