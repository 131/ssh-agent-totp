// Package clipboard copies the codes outside the WebView: Windows keeps them
// out of the clipboard history (Win+V) and of the cloud clipboard.
package clipboard

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32             = windows.NewLazySystemDLL("user32.dll")
	kernel32           = windows.NewLazySystemDLL("kernel32.dll")
	procOpenClipboard  = user32.NewProc("OpenClipboard")
	procCloseClipboard = user32.NewProc("CloseClipboard")
	procEmptyClipboard = user32.NewProc("EmptyClipboard")
	procSetClipboard   = user32.NewProc("SetClipboardData")
	procRegisterFormat = user32.NewProc("RegisterClipboardFormatW")
	procGlobalAlloc    = kernel32.NewProc("GlobalAlloc")
	procGlobalLock     = kernel32.NewProc("GlobalLock")
	procGlobalUnlock   = kernel32.NewProc("GlobalUnlock")
	procGlobalFree     = kernel32.NewProc("GlobalFree")
	procMoveMemory     = kernel32.NewProc("RtlMoveMemory")
)

const (
	cfUnicodeText = 13
	gmemMoveable  = 0x2
)

func globalText(data []uint16) uintptr {
	size := uintptr(len(data) * 2)
	handle, _, _ := procGlobalAlloc.Call(gmemMoveable, size)
	if handle == 0 {
		return 0
	}

	pointer, _, _ := procGlobalLock.Call(handle)
	if pointer == 0 {
		procGlobalFree.Call(handle)
		return 0
	}

	procMoveMemory.Call(pointer, uintptr(unsafe.Pointer(&data[0])), size)
	procGlobalUnlock.Call(handle)
	return handle
}

// Copy puts text on the clipboard, kept out of the Windows clipboard
// history and cloud sync: codes are short lived.
func Copy(hwnd uintptr, text string) error {
	data, err := windows.UTF16FromString(text)
	if err != nil {
		return err
	}

	if ok, _, err := procOpenClipboard.Call(hwnd); ok == 0 {
		return err
	}

	defer procCloseClipboard.Call()

	procEmptyClipboard.Call()
	handle := globalText(data)
	if handle == 0 {
		return windows.ERROR_NOT_ENOUGH_MEMORY
	}

	if ok, _, err := procSetClipboard.Call(cfUnicodeText, handle); ok == 0 {
		procGlobalFree.Call(handle)
		return err
	}

	for _, name := range []string{"CanIncludeInClipboardHistory", "CanUploadToCloudClipboard"} {
		format, _ := windows.UTF16PtrFromString(name)
		if id, _, _ := procRegisterFormat.Call(uintptr(unsafe.Pointer(format))); id != 0 {
			// A DWORD 0: excluded.
			if zero := globalText([]uint16{0, 0}); zero != 0 {
				if ok, _, _ := procSetClipboard.Call(id, zero); ok == 0 {
					procGlobalFree.Call(zero)
				}
			}
		}
	}

	return nil
}
