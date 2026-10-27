package startup

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestNativeShortcut(t *testing.T) {
	fail := func(format string, args ...any) {
		message := fmt.Sprintf(format, args...)
		// Public check annotations retain the diagnostic even when logs need auth.
		fmt.Printf("::error file=internal/startup/startup_windows_test.go::%s\n", message)
		t.Fatal(message)
	}
	// All COM writes are inside the test directory, never shell:startup.
	folder := filepath.Join(t.TempDir(), "startup ' with spaces")
	store := DefaultStore()
	store.Folder = func() (string, error) { return folder, nil }
	if enabled, err := store.SetEnabled(true); !enabled || err != nil {
		fail("native creation: %v %v", enabled, err)
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
		fail("read shortcut: %v %q", err, output)
	}
	parts := strings.Split(strings.TrimSpace(string(output)), "|")
	if len(parts) != 2 {
		fail("invalid shortcut response: %q", output)
	}

	// WScript expands DOS 8.3 aliases (RUNNER~1) to long names. Compare the
	// actual file/directory identities, not their textual spellings.
	for index, expected := range []string{target, filepath.Dir(target)} {
		actualInfo, err := os.Stat(parts[index])
		if err != nil {
			fail("shortcut path: %v", err)
		}
		expectedInfo, err := os.Stat(expected)
		if err != nil || !os.SameFile(actualInfo, expectedInfo) {
			fail("shortcut mismatch: got=%q; expected=%q; error=%v", parts[index], expected, err)
		}
	}
	if enabled, err := store.SetEnabled(false); enabled || err != nil {
		fail("native removal: %v %v", enabled, err)
	}
}
