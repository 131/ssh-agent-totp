package main

import (
	"errors"
	"runtime"
	"testing"
	"unsafe"

	"github.com/131/ssh-agent-totp/internal/clock"
	"github.com/131/ssh-agent-totp/internal/desktop"
	webview "github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"
)

type fakeSessionView struct {
	webview.WebView
	hwnd                        uintptr
	err                         error
	closes, evals, html, drains int
}

func (v *fakeSessionView) Close() error {
	v.closes++
	if v.err != nil {
		return v.err
	}
	windows.NewLazySystemDLL("user32.dll").NewProc("DestroyWindow").Call(v.hwnd)
	return nil
}
func (v *fakeSessionView) Eval(string)      { v.evals++ }
func (v *fakeSessionView) SetHtml(string)   { v.html++ }
func (v *fakeSessionView) ProcessDispatch() { v.drains++ }

func sessionHost(t *testing.T) *desktop.Host {
	t.Helper()
	runtime.LockOSThread()
	t.Cleanup(runtime.UnlockOSThread)
	var ticks clock.Clock
	host, err := desktop.New(&ticks, "ssh-agent-totp-session-test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if !host.Closed() {
			host.Quit()
			host.Run()
		}
	})
	return host
}

func fakeSession(t *testing.T, host *desktop.Host) (*WindowSession, *fakeSessionView) {
	t.Helper()
	name, _ := windows.UTF16PtrFromString("STATIC")
	hwnd, _, err := windows.NewLazySystemDLL("user32.dll").NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(name)), 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)
	if hwnd == 0 {
		t.Fatal(err)
	}
	view := &fakeSessionView{hwnd: hwnd}
	session := &WindowSession{host: host, view: view}
	session.frame = desktop.NewFrame(hwnd, func() {
		if err := session.Close(); err != nil {
			t.Error(err)
		}
	}, nil)
	t.Cleanup(func() {
		view.err = nil
		if err := session.Close(); err != nil {
			t.Error(err)
		}
	})
	return session, view
}

func TestClosedSessionDropsWorkWithoutTouchingReplacement(t *testing.T) {
	host := sessionHost(t)
	old, view := fakeSession(t, host)
	oldCalls, newCalls := 0, 0
	old.dispatch(func() { oldCalls++ })
	if err := old.Close(); err != nil {
		t.Fatal(err)
	}
	fresh, _ := fakeSession(t, host)
	fresh.dispatch(func() { newCalls++ })
	old.dispatch(func() { oldCalls++ })
	old.Tick(clock.Tick{})
	old.Reload()
	old.Drain()
	old.Notice(errors.New("late error"))
	old.Show()
	if err := old.Close(); err != nil {
		t.Fatal(err)
	}
	host.Dispatch(host.Quit)
	host.Run()
	if oldCalls != 0 || newCalls != 1 || view.closes != 1 || view.evals != 0 || view.html != 0 || view.drains != 0 {
		t.Fatalf("stale session work: old=%d new=%d view=%+v", oldCalls, newCalls, view)
	}
}

func TestSessionCloseFailureCanRetry(t *testing.T) {
	host := sessionHost(t)
	session, view := fakeSession(t, host)
	view.err = errors.New("close failed")
	if session.Close() == nil || session.closed {
		t.Fatal("failed Close discarded live session")
	}
	session.Tick(clock.Tick{})
	if view.evals != 1 {
		t.Fatal("failed Close did not restore session")
	}
	view.err = nil
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	if !session.closed || view.closes != 2 {
		t.Fatal("Close retry failed")
	}
}

func TestCloseDuringSessionDispatch(t *testing.T) {
	host := sessionHost(t)
	session, _ := fakeSession(t, host)
	session.dispatch(func() {
		if err := session.Close(); err != nil {
			t.Error(err)
		}
	})
	session.dispatch(func() { t.Error("callback ran after session Close") })
	host.Dispatch(host.Quit)
	host.Run()
}
