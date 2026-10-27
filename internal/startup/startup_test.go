package startup

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestToggleStartupShortcut(t *testing.T) {
	folder := filepath.Join(t.TempDir(), "startup")
	target := filepath.Join(t.TempDir(), "app.exe")
	created := 0
	store := &Store{
		Folder:     func() (string, error) { return folder, nil },
		Executable: func() (string, error) { return target, nil },
		CreateShortcut: func(path, executable string) error {
			created++
			if path != filepath.Join(folder, ShortcutName) || executable != target {
				t.Fatal("unexpected shortcut destination")
			}
			return os.WriteFile(path, []byte(executable), 0o600)
		},
	}

	if enabled, err := store.Enabled(); enabled || err != nil {
		t.Fatalf("initial state: %v %v", enabled, err)
	}
	if _, err := os.Stat(folder); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("reading status created the startup directory")
	}

	if enabled, err := store.SetEnabled(true); !enabled || err != nil {
		t.Fatalf("enable: %v %v", enabled, err)
	}
	if enabled, err := store.Enabled(); !enabled || err != nil {
		t.Fatalf("persisted state: %v %v", enabled, err)
	}
	if created != 1 {
		t.Fatal("shortcut was not created once")
	}

	other := filepath.Join(folder, "other-app.lnk")
	if err := os.WriteFile(other, []byte("untouched"), 0o600); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if enabled, err := store.SetEnabled(false); enabled || err != nil {
			t.Fatalf("disable: %v %v", enabled, err)
		}
	}
	if _, err := os.Stat(other); err != nil {
		t.Fatal("another startup entry was touched")
	}
	if enabled, err := store.Enabled(); enabled || err != nil {
		t.Fatalf("disabled state: %v %v", enabled, err)
	}

	store.CreateShortcut = func(string, string) error { return errors.New("cannot create shortcut") }
	if _, err := store.SetEnabled(true); err == nil {
		t.Fatal("shortcut creation error ignored")
	}
	if enabled, _ := store.Enabled(); enabled {
		t.Fatal("failed creation enabled startup")
	}
	store.Executable = func() (string, error) { return "", errors.New("executable unavailable") }
	if _, err := store.SetEnabled(true); err == nil {
		t.Fatal("executable error ignored")
	}
	store.Folder = func() (string, error) { return "", errors.New("startup directory unavailable") }
	if _, err := store.Enabled(); err == nil {
		t.Fatal("folder error ignored")
	}
}
