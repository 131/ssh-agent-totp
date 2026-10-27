package main

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"errors"
	"github.com/131/ssh-agent-totp/internal/i18n"
	"github.com/131/ssh-agent-totp/internal/startup"
	"github.com/131/ssh-agent-totp/settings"
	"os"
)

func TestSetLanguage(t *testing.T) {
	defer i18n.Set(i18n.Default)
	reloaded := 0
	app := &App{Settings: &settings.Store{Path: filepath.Join(t.TempDir(), "settings.yml")}, Reload: func() { reloaded++ }}
	if _, err := app.Handle("set_language", []json.RawMessage{json.RawMessage(`"de-de"`)}); err == nil {
		t.Fatal("unknown language accepted")
	}
	if _, err := app.Handle("set_language", []json.RawMessage{json.RawMessage(`"en-us"`)}); err != nil {
		t.Fatal(err)
	}
	saved, _ := app.Settings.Load()
	if saved.Language != "en-us" || i18n.Current() != "en-us" || reloaded != 1 {
		t.Fatalf("%+v %s %d", saved, i18n.Current(), reloaded)
	}
	if _, err := app.Handle("nope", nil); err == nil || err.Error() != "unknown method: nope" {
		t.Fatal(err)
	}
}

func TestStartupToggle(t *testing.T) {
	folder := t.TempDir()
	app := &App{Startup: &startup.Store{
		Folder:         func() (string, error) { return folder, nil },
		Executable:     func() (string, error) { return "fake.exe", nil },
		CreateShortcut: func(path, target string) error { return os.WriteFile(path, []byte(target), 0o600) },
	}}
	if result, err := app.Handle("startup_status", nil); err != nil || result != false {
		t.Fatal(result, err)
	}
	for _, enable := range []bool{true, false} {
		value, _ := json.Marshal(enable)
		if result, err := app.Handle("startup_enable", []json.RawMessage{value}); err != nil || result != enable {
			t.Fatal(result, err)
		}
	}
	if _, err := app.Handle("startup_enable", nil); err == nil {
		t.Fatal("missing toggle argument accepted")
	}
	app.Startup.CreateShortcut = func(string, string) error { return errors.New("write denied") }
	if _, err := app.Handle("startup_enable", []json.RawMessage{json.RawMessage("true")}); err == nil {
		t.Fatal("creation error ignored")
	}
}
