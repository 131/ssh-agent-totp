package startup

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestNativeShortcut(t *testing.T) {
	// All COM writes are inside the test directory, never shell:startup.
	folder := filepath.Join(t.TempDir(), "startup ' with spaces")
	store := DefaultStore()
	store.Folder = func() (string, error) { return folder, nil }
	if enabled, err := store.SetEnabled(true); !enabled || err != nil {
		t.Fatalf("native creation: %v %v", enabled, err)
	}

	target, _ := os.Executable()
	path := filepath.Join(folder, ShortcutName)
	script := "$ErrorActionPreference='Stop'; [Console]::OutputEncoding=[Text.Encoding]::UTF8; " +
		"$link=(New-Object -ComObject WScript.Shell).CreateShortcut(" + quote(path) + "); " +
		"[Console]::Write($link.TargetPath+'|'+$link.WorkingDirectory)"
	command := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("read shortcut: %v %s", err, output)
	}
	parts := strings.Split(strings.TrimSpace(string(output)), "|")
	if len(parts) != 2 || !strings.EqualFold(parts[0], target) || !strings.EqualFold(parts[1], filepath.Dir(target)) {
		t.Fatal("shortcut does not target the running executable and its directory")
	}
	if enabled, err := store.SetEnabled(false); enabled || err != nil {
		t.Fatalf("native removal: %v %v", enabled, err)
	}
}
