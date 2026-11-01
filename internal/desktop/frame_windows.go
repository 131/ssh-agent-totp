package desktop

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Borderless window: the client area covers the whole window (WM_NCCALCSIZE)
// and the UI draws its own title bar. The frame style stays, so Windows keeps
// the shadow, the rounded corners, the minimize animation and the snap.

var (
	dwmapi             = windows.NewLazySystemDLL("dwmapi.dll")
	procGetWindowLong  = user32.NewProc("GetWindowLongPtrW")
	procSetWindowPos   = user32.NewProc("SetWindowPos")
	procReleaseCapture = user32.NewProc("ReleaseCapture")
	procExtendFrame    = dwmapi.NewProc("DwmExtendFrameIntoClientArea")
	procSetDwmAttr     = dwmapi.NewProc("DwmSetWindowAttribute")
)

const (
	gwlStyle            = ^uintptr(15) // -16
	wsMaximizeBox       = 0x00010000
	wmNcCalcSize        = 0x0083
	wmNcLButtonDown     = 0x00a1
	swpFrameChanged     = 0x0020
	swpNoMove           = 0x0002
	swpNoSize           = 0x0001
	swpNoZOrder         = 0x0004
	swMinimize          = 6
	dwmCornerPreference = 33
	dwmCornerRound      = 2
	htCaption           = 2
)

// Hit codes of the resize edges, as sent by the UI.
var edges = map[string]uintptr{
	"left":         10,
	"right":        11,
	"top":          12,
	"top-left":     13,
	"top-right":    14,
	"bottom":       15,
	"bottom-left":  16,
	"bottom-right": 17,
}

// Frame owns native styling and actions for one optional GUI session.
type Frame struct {
	Close            func()
	AfterModal       func()
	window, original uintptr
	attached         bool
}

// A single callback plus HWND lookup avoids permanently rooting a Go closure
// for every disposable session. Detach removes the session from the lookup.
var frames = map[uintptr]*Frame{}
var frameCallback = syscall.NewCallback(func(hwnd, msg, wparam, lparam uintptr) uintptr {
	if f := frames[hwnd]; f != nil {
		return f.frameProc(hwnd, msg, wparam, lparam)
	}
	result, _, _ := procDefWindowProc.Call(hwnd, msg, wparam, lparam)
	return result
})

func NewFrame(hwnd uintptr, close, afterModal func()) *Frame {
	f := &Frame{window: hwnd, Close: close, AfterModal: afterModal}
	f.Attach()
	return f
}

func (t *Frame) Attach() {
	if t.attached {
		return
	}
	frames[t.window] = t
	t.original, _, _ = procSetWindowLong.Call(t.window, gwlpWndProc, frameCallback)
	t.attached = true
	t.Borderless()
}

// Detach restores the GUI's original procedure before the browser destroys it.
func (t *Frame) Detach() {
	if t.attached {
		procSetWindowLong.Call(t.window, gwlpWndProc, t.original)
		delete(frames, t.window)
		t.attached = false
	}
}

func (t *Frame) frameProc(hwnd, msg, wparam, lparam uintptr) uintptr {
	switch msg {
	case wmNcCalcSize:
		if wparam != 0 {
			return 0
		}
	case wmClose:
		if t.Close != nil {
			t.Close()
		}
		return 0
	case wmExitSizeMove:
		if t.AfterModal != nil {
			t.AfterModal()
		}
	}
	result, _, _ := procCallWindowProc.Call(t.original, hwnd, msg, wparam, lparam)
	if msg == wmSize && wparam == sizeMinimized {
		procShowWindow.Call(hwnd, swHide)
	}
	return result
}

// Borderless removes the title bar; the window cannot be maximized.
func (t *Frame) Borderless() {
	style, _, _ := procGetWindowLong.Call(t.window, gwlStyle)
	procSetWindowLong.Call(t.window, gwlStyle, style&^wsMaximizeBox)
	// One pixel of DWM frame is enough for the shadow.
	margins := [4]int32{0, 0, 0, 1}
	procExtendFrame.Call(t.window, uintptr(unsafe.Pointer(&margins)))
	if procSetDwmAttr.Find() == nil {
		corner := uint32(dwmCornerRound)
		procSetDwmAttr.Call(t.window, dwmCornerPreference, uintptr(unsafe.Pointer(&corner)), 4)
	}
	procSetWindowPos.Call(t.window, 0, 0, 0, 0, 0, swpFrameChanged|swpNoMove|swpNoSize|swpNoZOrder)
}

// Frame runs a title bar action of the UI, on the window thread: "drag",
// "minimize", "close" or a resize edge ("left", "bottom-right"…).
func (t *Frame) Action(action string) bool {
	if !t.attached {
		return false
	}
	switch action {
	case "drag":
		procReleaseCapture.Call()
		procPostMessage.Call(t.window, wmNcLButtonDown, htCaption, 0)
	case "minimize":
		procShowWindow.Call(t.window, swMinimize)
	case "close":
		procPostMessage.Call(t.window, wmClose, 0, 0)

	default:
		edge, ok := edges[action]
		if !ok {
			return false
		}

		procReleaseCapture.Call()
		procPostMessage.Call(t.window, wmNcLButtonDown, edge, 0)
	}
	return true
}

func (t *Frame) Show() {
	if t.attached {
		procShowWindow.Call(t.window, swRestore)
		procSetForeground.Call(t.window)
	}
}
