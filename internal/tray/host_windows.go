package tray

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/131/ssh-agent-totp/internal/clock"
)

const hostClass = "ssh-agent-totp-tray"

var (
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

// New owns an invisible top-level window. Unlike a message-only window it
// receives Explorer's TaskbarCreated broadcast when the tray must be restored.
func New(ticks *clock.Clock) (*Tray, error) {
	var instance windows.Handle
	if err := windows.GetModuleHandleEx(0, nil, &instance); err != nil {
		return nil, err
	}
	name, _ := windows.UTF16PtrFromString(hostClass)
	title, _ := windows.UTF16PtrFromString(WindowTitle)
	class := windowClass{Proc: procDefWindowProc.Addr(), Instance: uintptr(instance), Name: name}
	class.Size = uint32(unsafe.Sizeof(class))
	if atom, _, err := procRegisterClass.Call(uintptr(unsafe.Pointer(&class))); atom == 0 && err != windows.ERROR_CLASS_ALREADY_EXISTS {
		return nil, fmt.Errorf("register tray window: %w", err)
	}
	hwnd, _, err := procCreateWindow.Call(0, uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(title)), 0, 0, 0, 0, 0, 0, 0, uintptr(instance), 0)
	if hwnd == 0 {
		return nil, fmt.Errorf("create tray window: %w", err)
	}
	return Install(hwnd, ticks), nil
}

// Window is the persistent clipboard/dispatch owner, not the optional GUI.
func (t *Tray) Window() uintptr { return t.hwnd }

// Run owns the message queue for the entire application's lifetime, including
// periods with no browser. WM_APP drains WebView2's binding completions.
func (t *Tray) Run() { t.runUntil(func() bool { return false }) }

func (t *Tray) runUntil(done func() bool) bool {
	var msg message
	for !done() {
		result, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(result) <= 0 {
			return false
		}
		if msg.ID == 0x8000 && t.ProcessThread != nil {
			t.ProcessThread()
		}
		root, _, _ := procGetAncestor.Call(msg.Window, 2) // GA_ROOT
		if handled, _, _ := procIsDialogMessage.Call(root, uintptr(unsafe.Pointer(&msg))); handled != 0 {
			continue
		}
		procTranslate.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
	}
	return true
}

// Error reports a tray failure without creating a browser just to show it.
func (t *Tray) Error(err error) {
	text, _ := windows.UTF16PtrFromString(err.Error())
	title, _ := windows.UTF16PtrFromString(WindowTitle)
	procMessageBox.Call(t.hwnd, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), 0x10)
	if t.AfterModal != nil {
		t.AfterModal()
	}
}
