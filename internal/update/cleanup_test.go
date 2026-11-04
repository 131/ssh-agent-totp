package update

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func writeCleanupFile(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("test executable"), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCleanupLegacyScope(t *testing.T) {
	dir := t.TempDir()
	name := "ssh-agent-totp.exe"
	keep := []string{name, name + ".bak", name + ".old", name + ".old.", name + ".old.notes",
		name + ".old.-1", name + ".old.+1", name + ".old.1.tmp", name + ".old.1.2", "other.exe.old.1793811820719100200"}
	for _, file := range keep {
		writeCleanupFile(t, dir, file)
	}
	old := writeCleanupFile(t, dir, name+".old.1793811820719100200")
	folder := filepath.Join(dir, name+".old.123")
	if err := os.Mkdir(folder, 0700); err != nil {
		t.Fatal(err)
	}
	inside := writeCleanupFile(t, folder, "untouched")
	if err := cleanupLegacy(filepath.Join(dir, name), os.Remove, func() { t.Fatal("unexpected retry") }); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(old); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old file retained: %v", err)
	}
	for _, file := range append(keep, filepath.Join(filepath.Base(folder), filepath.Base(inside))) {
		if _, err := os.Stat(filepath.Join(dir, file)); err != nil {
			t.Fatalf("unrelated path removed: %s (%v)", file, err)
		}
	}
}

func TestCleanupLegacyPreservesSymlinks(t *testing.T) {
	dir := t.TempDir()
	target := writeCleanupFile(t, dir, "not-an-update")
	link := filepath.Join(dir, "app.exe.old.123")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}
	if err := cleanupLegacy(filepath.Join(dir, "app.exe"), os.Remove, func() { t.Fatal("unexpected retry") }); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(link); err != nil {
		t.Fatalf("symlink removed: %v", err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("symlink target removed: %v", err)
	}
}

func TestCleanupLegacyRetriesOnlyFailures(t *testing.T) {
	dir := t.TempDir()
	locked := writeCleanupFile(t, dir, "app.exe.old.1")
	ready := writeCleanupFile(t, dir, "app.exe.old.2")
	calls := map[string]int{}
	waits := 0
	remove := func(path string) error {
		calls[path]++
		if path == locked && calls[path] == 1 {
			return os.ErrPermission
		}
		return os.Remove(path)
	}
	if err := cleanupLegacy(filepath.Join(dir, "app.exe"), remove, func() { waits++ }); err != nil {
		t.Fatal(err)
	}
	if waits != 1 || calls[locked] != 2 || calls[ready] != 1 {
		t.Fatalf("waits=%d calls=%v", waits, calls)
	}
}

func TestCleanupLegacyStopsAfterBoundedRetries(t *testing.T) {
	dir := t.TempDir()
	old := writeCleanupFile(t, dir, "app.exe.old.1")
	calls, waits := 0, 0
	err := cleanupLegacy(filepath.Join(dir, "app.exe"), func(string) error { calls++; return os.ErrPermission }, func() { waits++ })
	if !errors.Is(err, os.ErrPermission) || calls != legacyCleanupAttempts || waits != legacyCleanupAttempts-1 {
		t.Fatalf("err=%v calls=%d waits=%d", err, calls, waits)
	}
	if _, err := os.Stat(old); err != nil {
		t.Fatalf("failed removal lost file: %v", err)
	}
}

func TestCleanupLegacyAlreadyRemoved(t *testing.T) {
	dir := t.TempDir()
	writeCleanupFile(t, dir, "app.exe.old.1")
	if err := cleanupLegacy(filepath.Join(dir, "app.exe"), func(path string) error {
		if err := os.Remove(path); err != nil {
			return err
		}
		return os.ErrNotExist // Another process completed cleanup before our unlink.
	}, func() { t.Fatal("vanished file retried") }); err != nil {
		t.Fatal(err)
	}
}

func TestCleanupLegacyMissingDirectory(t *testing.T) {
	err := cleanupLegacy(filepath.Join(t.TempDir(), "missing", "app.exe"), os.Remove, func() { t.Fatal("directory read retried") })
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("directory read error lost: %v", err)
	}
}
