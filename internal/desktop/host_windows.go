// Package desktop owns the resident Win32 host, dispatch queue and GUI frames.
package desktop

import (
	"fmt"
	"syscall"
	"time"
	"unsafe"

	"github.com/131/ssh-agent-totp/internal/clock"
	"golang.org/x/sys/windows"
)

const (
	hostClass      = "ssh-agent-totp-tray"
	gwlpWndProc    = ^uintptr(3)
	wmDestroy      = 0x0002
	wmSize         = 0x0005
	wmClose        = 0x0010
	wmTimer        = 0x0113
	wmExitSizeMove = 0x0232
	wmDispatch     = 0x8000 + 2
	clockTimerID   = 1
	sizeMinimized  = 1
	swHide         = 0
	swRestore      = 9
)

var (
	user32              = windows.NewLazySystemDLL("user32.dll")
	kernel32            = windows.NewLazySystemDLL("kernel32.dll")
	procRegisterClass   = user32.NewProc("RegisterClassExW")
	procCreateWindow    = user32.NewProc("CreateWindowExW")
	procDefWindowProc   = user32.NewProc("DefWindowProcW")
	procDestroyWindow   = user32.NewProc("DestroyWindow")
	procGetMessage      = user32.NewProc("GetMessageW")
	procTranslate       = user32.NewProc("TranslateMessage")
	procDispatchMessage = user32.NewProc("DispatchMessageW")
	procPostQuitMessage = user32.NewProc("PostQuitMessage")
	procGetAncestor     = user32.NewProc("GetAncestor")
	procIsDialogMessage = user32.NewProc("IsDialogMessageW")
	procMessageBox      = user32.NewProc("MessageBoxW")
	procSetWindowLong   = user32.NewProc("SetWindowLongPtrW")
	procCallWindowProc  = user32.NewProc("CallWindowProcW")
	procPostMessage     = user32.NewProc("PostMessageW")
	procShowWindow      = user32.NewProc("ShowWindow")
	procSetForeground   = user32.NewProc("SetForegroundWindow")
	procSetTimer        = user32.NewProc("SetTimer")
	procKillTimer       = user32.NewProc("KillTimer")
	procRegisterMessage = user32.NewProc("RegisterWindowMessageW")
	procFindWindow      = user32.NewProc("FindWindowW")
	procCreateMutex     = kernel32.NewProc("CreateMutexW")
	procGetDpiForSystem = user32.NewProc("GetDpiForSystem")
)

type windowClass struct {
	Size, Style                        uint32
	Proc                               uintptr
	ClassExtra, WindowExtra            int32
	Instance, Icon, Cursor, Background uintptr
	Menu, Name                         *uint16
	SmallIcon                          uintptr
}

type message struct {
	Window         uintptr
	ID             uint32
	WParam, LParam uintptr
	Time           uint32
	Point          struct{ X, Y int32 }
	Private        uint32
}

type Host struct {
	Open                 func()
	ProcessThread        func()
	OnDestroy            func()
	Message              func(hwnd, msg, wparam, lparam uintptr) (uintptr, bool)
	hwnd, original, show uintptr
	title                string
	clock                *clock.Clock
	dispatch             uiDispatch
	closed               bool
}

func registeredMessage(name string) uintptr {
	p, _ := windows.UTF16PtrFromString(name)
	msg, _, _ := procRegisterMessage.Call(uintptr(unsafe.Pointer(p)))
	return msg
}

// ActivateExisting preserves the identity even while no GUI exists. After an
// update, wait for the previous executable to release its session mutex.
func ActivateExisting(waitPrevious bool, title string) bool {
	name, _ := windows.UTF16PtrFromString(`Local\ssh-agent-totp`)
	for attempt := 0; ; attempt++ {
		handle, _, err := procCreateMutex.Call(0, 0, uintptr(unsafe.Pointer(name)))
		if err != windows.ERROR_ALREADY_EXISTS {
			return false
		}
		if !waitPrevious || attempt == 75 {
			break
		}
		windows.CloseHandle(windows.Handle(handle))
		time.Sleep(200 * time.Millisecond)
	}
	class, _ := windows.UTF16PtrFromString(hostClass)
	caption, _ := windows.UTF16PtrFromString(title)
	if hwnd, _, _ := procFindWindow.Call(uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(caption))); hwnd != 0 {
		procPostMessage.Call(hwnd, registeredMessage("ssh-agent-totp-show"), 0, 0)
	}
	return true
}

// New creates a hidden top-level window so Explorer broadcasts still arrive.
func New(ticks *clock.Clock, title string) (*Host, error) {
	var instance windows.Handle
	if err := windows.GetModuleHandleEx(0, nil, &instance); err != nil {
		return nil, err
	}
	name, _ := windows.UTF16PtrFromString(hostClass)
	caption, _ := windows.UTF16PtrFromString(title)
	class := windowClass{Proc: procDefWindowProc.Addr(), Instance: uintptr(instance), Name: name}
	class.Size = uint32(unsafe.Sizeof(class))
	if atom, _, err := procRegisterClass.Call(uintptr(unsafe.Pointer(&class))); atom == 0 && err != windows.ERROR_CLASS_ALREADY_EXISTS {
		return nil, fmt.Errorf("register host: %w", err)
	}
	hwnd, _, err := procCreateWindow.Call(0, uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(caption)), 0, 0, 0, 0, 0, 0, 0, uintptr(instance), 0)
	if hwnd == 0 {
		return nil, fmt.Errorf("create host: %w", err)
	}
	h := &Host{hwnd: hwnd, title: title, clock: ticks, show: registeredMessage("ssh-agent-totp-show")}
	h.original, _, _ = procSetWindowLong.Call(hwnd, gwlpWndProc, syscall.NewCallback(h.wndproc))
	procSetTimer.Call(hwnd, clockTimerID, 1000, 0)
	return h, nil
}

func (h *Host) Window() uintptr { return h.hwnd }
func (h *Host) Closed() bool    { return h.closed }
func (h *Host) Quit()           { procPostMessage.Call(h.hwnd, wmClose, 0, 0) }

func (h *Host) wndproc(hwnd, msg, wparam, lparam uintptr) uintptr {
	switch msg {
	case wmDispatch:
		h.runDispatched()
		return 0
	case wmTimer:
		if wparam == clockTimerID {
			h.clock.Fire(time.Now())
			return 0
		}
	case h.show:
		if h.Open != nil {
			h.Open()
		}
		return 0
	case wmClose:
		h.closed = true
		procDestroyWindow.Call(hwnd)
		return 0
	case wmDestroy:
		h.closed = true
		procKillTimer.Call(hwnd, clockTimerID)
		if h.OnDestroy != nil {
			h.OnDestroy()
		}
		procPostQuitMessage.Call(0)
	}
	if h.Message != nil {
		if result, handled := h.Message(hwnd, msg, wparam, lparam); handled {
			return result
		}
	}
	result, _, _ := procCallWindowProc.Call(h.original, hwnd, msg, wparam, lparam)
	return result
}

func (h *Host) Run() { h.runUntil(func() bool { return false }) }
func (h *Host) runUntil(done func() bool) bool {
	var msg message
	for !done() {
		result, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(result) <= 0 {
			return false
		}
		if msg.ID == 0x8000 && h.ProcessThread != nil {
			h.ProcessThread()
		}
		root, _, _ := procGetAncestor.Call(msg.Window, 2)
		if handled, _, _ := procIsDialogMessage.Call(root, uintptr(unsafe.Pointer(&msg))); handled != 0 {
			continue
		}
		procTranslate.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
	}
	return true
}

func (h *Host) Error(err error) {
	text, _ := windows.UTF16PtrFromString(err.Error())
	title, _ := windows.UTF16PtrFromString(h.title)
	procMessageBox.Call(h.hwnd, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), 0x10)
	if h.ProcessThread != nil {
		h.ProcessThread()
	}
}

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
