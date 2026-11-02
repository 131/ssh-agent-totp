package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/131/ssh-agent-totp/internal/clock"
	"github.com/131/ssh-agent-totp/internal/desktop"
	"golang.org/x/sys/windows"
)

// Opt-in: requires a real WebView2 runtime and briefly creates a test GUI.
func TestNativeSessionBridge(t *testing.T) {
	if os.Getenv("SSH_AGENT_TOTP_NATIVE_TESTS") != "1" {
		t.Skip("native GUI test not enabled")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	ole32 := windows.NewLazySystemDLL("ole32.dll")
	result, _, _ := ole32.NewProc("CoInitializeEx").Call(0, 2)
	if int32(result) < 0 {
		t.Fatalf("COM initialization: %#x", uint32(result))
	}
	defer ole32.NewProc("CoUninitialize").Call()
	data := t.TempDir()
	t.Setenv("APPDATA", data)
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Setenv("SSH_AUTH_SOCK", `\\.\pipe\ssh-agent-totp-native-test-no-agent`)
	path := filepath.Join(data, "ssh-agent-totp")
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "settings.yml"), []byte("update:\n  enable: false\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var ticks clock.Clock
	host, err := desktop.New(&ticks, "ssh-agent-totp-native-session-test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if !host.Closed() {
			host.Quit()
			host.Run()
		}
	}()
	var session *WindowSession
	session, err = NewWindowSession(NewApp(func(string) error { return nil }), host, func() {
		if session != nil {
			session.Close()
		}
	}, func() {
		if session != nil {
			session.Drain()
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if err := session.Show(); err != nil {
		t.Fatal(err)
	}
	var visible bool
	user32 := windows.NewLazySystemDLL("user32.dll")
	callback := windows.NewCallback(func(hwnd, data uintptr) uintptr {
		var name [256]uint16
		user32.NewProc("GetClassNameW").Call(hwnd, uintptr(unsafe.Pointer(&name[0])), 256)
		if strings.HasPrefix(windows.UTF16ToString(name[:]), "Chrome_WidgetWin_") {
			shown, _, _ := user32.NewProc("IsWindowVisible").Call(hwnd)
			if shown != 0 {
				visible = true
			}
		}
		return 1
	})
	user32.NewProc("EnumChildWindows").Call(uintptr(session.view.Window()), callback, 0)

	host.ProcessThread = session.Drain
	replies := map[string]string{}
	if err := session.view.Bind("test_probe", func(kind, value string) {
		replies[kind] = value
		if len(replies) >= 2 {
			host.Dispatch(host.Quit)
		}
	}); err != nil {
		t.Fatal(err)
	}
	session.view.Init(`window.addEventListener('DOMContentLoaded', () => {
  const id = 100000000, done = window.totpDone;
  window.totpDone = payload => {
   if (payload.id === id) test_probe('backend', payload.error || payload.result);
   else done(payload);
  };
  totp_call(id, 'version', []).then(() => test_probe('binding', 'ok'));
 });`)
	session.Reload()
	timeout := time.AfterFunc(30*time.Second, func() { host.Dispatch(host.Quit) })
	defer timeout.Stop()
	host.Run()
	visible = false
	user32.NewProc("EnumChildWindows").Call(uintptr(session.view.Window()), callback, 0)
	if !visible {
		t.Fatal("browser remains hidden after event loop and bridge completions")
	}
	if replies["backend"] != version || replies["binding"] != "ok" {
		t.Fatalf("native bridge replies = %v; need backend version and binding completion", replies)
	}
}
