package update

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestCleanupLegacyWindowsSharingLock(t *testing.T) {
	dir := t.TempDir()
	old := writeCleanupFile(t, dir, "app.exe.old.123")
	path, err := windows.UTF16PtrFromString(old)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateFile(path, windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	closed := false
	defer func() {
		if !closed {
			windows.CloseHandle(handle)
		}
	}()
	calls, waits := 0, 0
	err = cleanupLegacy(filepath.Join(dir, "app.exe"), func(path string) error { calls++; return os.Remove(path) }, func() {
		waits++
		if err := windows.CloseHandle(handle); err != nil {
			t.Fatal(err)
		}
		closed = true
	})
	if err != nil || calls != 2 || waits != 1 {
		t.Fatalf("sharing-lock retry: %v calls=%d waits=%d", err, calls, waits)
	}
}

func TestCleanupLegacyWindowsStartupEntryPoint(t *testing.T) {
	// The running test binary lives in a build/test directory, not the app install.
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	old, err := os.CreateTemp(filepath.Dir(executable), filepath.Base(executable)+".old.")
	if err != nil {
		t.Fatal(err)
	}
	path := old.Name()
	if err := old.Close(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(path) })
	// CreateTemp's suffix is decimal, matching a Wails rename-aside artifact.
	if err := CleanupLegacy(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("startup leftover retained: %v", err)
	}
}
