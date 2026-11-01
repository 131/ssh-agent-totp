package tray

import (
	"runtime"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/131/ssh-agent-totp/internal/clock"
)

func TestNativeHostBeforeGUI(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var ticks clock.Clock
	icon, err := New(&ticks)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		icon.Quit()
		icon.runUntil(func() bool { return false })
	}()
	if icon.Window() == 0 || icon.window != 0 {
		t.Fatal("tray requires a GUI")
	}
	if visible, _, _ := user32.NewProc("IsWindowVisible").Call(icon.Window()); visible != 0 {
		t.Fatal("tray host is visible")
	}
	if icon.Frame("close") {
		t.Fatal("frame action accepted before a GUI exists")
	}
	ready := false
	icon.Dispatch(func() { ready = true })
	if !icon.runUntil(func() bool { return ready }) {
		t.Fatal("native dispatch failed before GUI creation")
	}
	opened := 0
	icon.Open = func() { opened++ }
	procPostMessage.Call(icon.Window(), icon.show, 0, 0)
	if !icon.runUntil(func() bool { return opened == 1 }) {
		t.Fatal("single-instance activation did not request the GUI")
	}
}

func TestQuitWithoutGUI(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var ticks clock.Clock
	icon, err := New(&ticks)
	if err != nil {
		t.Fatal(err)
	}
	icon.Quit()
	if icon.runUntil(func() bool { return false }) {
		t.Fatal("quit handed off to a nonexistent GUI")
	}
	if exists, _, _ := user32.NewProc("IsWindow").Call(icon.Window()); exists != 0 {
		t.Fatal("quit leaked the tray host")
	}
}

func TestAttachedFrameClosesWithoutQuittingTray(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var ticks clock.Clock
	icon, err := New(&ticks)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		icon.Quit()
		icon.runUntil(func() bool { return false })
	}()
	name, _ := windows.UTF16PtrFromString(hostClass)
	frame, _, err := procCreateWindow.Call(0, uintptr(unsafe.Pointer(name)), 0, 0, 0, 0, 0, 0, 0, 0, uintptr(icon.instance), 0)
	if frame == 0 {
		t.Fatal(err)
	}
	icon.Attach(frame)
	icon.Close = func() {
		icon.Detach()
		procDestroyWindow.Call(frame)
	}
	if !icon.Frame("close") {
		t.Fatal("frame close rejected")
	}
	done := false
	icon.Dispatch(func() { done = true })
	if !icon.runUntil(func() bool { return done }) {
		t.Fatal("frame close quit the tray")
	}
	if exists, _, _ := user32.NewProc("IsWindow").Call(frame); exists != 0 {
		t.Fatal("frame close did not destroy the GUI")
	}
	if visible, _, _ := user32.NewProc("IsWindowVisible").Call(frame); visible != 0 {
		t.Fatal("frame close did not hide the GUI")
	}
}
