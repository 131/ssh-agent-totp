package tray

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	procGetIconInfo     = user32.NewProc("GetIconInfo")
	procCopyImage       = user32.NewProc("CopyImage")
	procSetMenuItemInfo = user32.NewProc("SetMenuItemInfoW")
	procGetMenuItemRect = user32.NewProc("GetMenuItemRect")
	procWindowFromPoint = user32.NewProc("WindowFromPoint")
	procInvalidateRect  = user32.NewProc("InvalidateRect")
	procDeleteObject    = windows.NewLazySystemDLL("gdi32.dll").NewProc("DeleteObject")
)

const (
	imageBitmap        = 0
	lrCreateDIBSection = 0x2000
	miimBitmap         = 0x80
)

type iconInfo struct {
	Icon               int32
	XHotspot, YHotspot uint32
	Mask, Color        uintptr
}

type menuItemInfo struct {
	Size, Mask, Type, State, ID uint32
	SubMenu, Checked, Unchecked uintptr
	Data                        uintptr
	Text                        *uint16
	TextLength                  uint32
	Bitmap                      uintptr
}

// updateMenuIcon fills the standard bitmap gutter, not an owner-drawn row.
// One shared bitmap serves every pin; replace and release it once per second.
func (t *Tray) updateMenuIcon() {
	if t.menuHandle == 0 || t.menuPinCount == 0 {
		return
	}

	var image iconInfo
	if ok, _, _ := procGetIconInfo.Call(t.menuIcon, uintptr(unsafe.Pointer(&image))); ok == 0 {
		return
	}

	bitmap, _, _ := procCopyImage.Call(image.Color, imageBitmap, 0, 0, lrCreateDIBSection)
	procDeleteObject.Call(image.Mask)
	procDeleteObject.Call(image.Color)
	if bitmap == 0 {
		return
	}

	item := menuItemInfo{Mask: miimBitmap, Bitmap: bitmap}
	item.Size = uint32(unsafe.Sizeof(item))
	for index := 0; index < t.menuPinCount; index++ {
		procSetMenuItemInfo.Call(t.menuHandle, uintptr(index+firstPinCommand), 0, uintptr(unsafe.Pointer(&item)))
	}

	procDeleteObject.Call(t.menuBitmap)
	t.menuBitmap = bitmap

	var bounds windows.Rect
	if ok, _, _ := procGetMenuItemRect.Call(0, t.menuHandle, 0, uintptr(unsafe.Pointer(&bounds))); ok != 0 {
		point := uintptr(uint32(bounds.Left+1)) | uintptr(uint32(bounds.Top+1))<<32
		if popup, _, _ := procWindowFromPoint.Call(point); popup != 0 {
			procInvalidateRect.Call(popup, 0, 0)
		}
	}
}
