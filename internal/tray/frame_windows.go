package tray

import (
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

// Borderless removes the title bar; the window cannot be maximized.
func (t *Tray) Borderless() {
	t.borderless = true
	style, _, _ := procGetWindowLong.Call(t.hwnd, gwlStyle)
	procSetWindowLong.Call(t.hwnd, gwlStyle, style&^wsMaximizeBox)
	// One pixel of DWM frame is enough for the shadow.
	margins := [4]int32{0, 0, 0, 1}
	procExtendFrame.Call(t.hwnd, uintptr(unsafe.Pointer(&margins)))
	if procSetDwmAttr.Find() == nil {
		corner := uint32(dwmCornerRound)
		procSetDwmAttr.Call(t.hwnd, dwmCornerPreference, uintptr(unsafe.Pointer(&corner)), 4)
	}
	procSetWindowPos.Call(t.hwnd, 0, 0, 0, 0, 0, swpFrameChanged|swpNoMove|swpNoSize|swpNoZOrder)
}

// Frame runs a title bar action of the UI, on the window thread: "drag",
// "minimize", "close" or a resize edge ("left", "bottom-right"…).
func (t *Tray) Frame(action string) bool {
	switch action {
	case "drag":
		procReleaseCapture.Call()
		procPostMessage.Call(t.hwnd, wmNcLButtonDown, htCaption, 0)
	case "minimize":
		procShowWindow.Call(t.hwnd, swMinimize)
	case "close":
		procPostMessage.Call(t.hwnd, wmClose, 0, 0)

	default:
		edge, ok := edges[action]
		if !ok {
			return false
		}

		procReleaseCapture.Call()
		procPostMessage.Call(t.hwnd, wmNcLButtonDown, edge, 0)
	}
	return true
}
