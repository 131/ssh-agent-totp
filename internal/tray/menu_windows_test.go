package tray

import (
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

var procGetBitmapObject = windows.NewLazySystemDLL("gdi32.dll").NewProc("GetObjectW")

func TestNativeMenuBitmaps(t *testing.T) {
	menu, _, _ := procCreatePopupMenu.Call()
	tray := &Tray{menuHandle: menu, menuPinCount: 2}
	defer func() {
		procDestroyMenu.Call(menu)
		procDeleteObject.Call(tray.menuBitmap)
	}()
	appendMenu(menu, mfString, 3, "Alpha")
	appendMenu(menu, mfString, 4, "Bravo")
	appendMenu(menu, mfString, menuOpen, "Open")
	appendMenu(menu, mfString, menuQuit, "Quit")
	tray.menuIcon, _, _ = procLoadImage.Call(0, 32512, imageIcon, 16, 16, 0x8000) // shared application icon
	tray.updateMenuIcon()
	if tray.menuBitmap == 0 {
		t.Fatal("bitmap conversion failed")
	}
	getItem := user32.NewProc("GetMenuItemInfoW")
	for id := uintptr(1); id <= 4; id++ {
		item := menuItemInfo{Mask: 0x80 | 0x100} // MIIM_BITMAP | MIIM_FTYPE
		item.Size = uint32(unsafe.Sizeof(item))
		if ok, _, _ := getItem.Call(menu, id, 0, uintptr(unsafe.Pointer(&item))); ok == 0 {
			t.Fatal("cannot read native item")
		}
		if item.Type&0x100 != 0 {
			t.Fatal("owner-drawn row")
		}
		if id >= 3 && item.Bitmap != tray.menuBitmap {
			t.Fatal("pins do not share the frame")
		}
		if id < 3 && item.Bitmap != 0 {
			t.Fatal("Open/Quit gained an icon")
		}
	}
	old := tray.menuBitmap
	tray.menuIcon, _, _ = procLoadImage.Call(0, 32515, imageIcon, 16, 16, 0x8000) // shared warning icon
	tray.updateMenuIcon()
	if tray.menuBitmap == 0 || tray.menuBitmap == old {
		t.Fatal("frame was not replaced")
	}
	var bitmap [32]byte
	if size, _, _ := procGetBitmapObject.Call(old, uintptr(len(bitmap)), uintptr(unsafe.Pointer(&bitmap[0]))); size != 0 {
		t.Fatal("previous bitmap leaked")
	}
}
