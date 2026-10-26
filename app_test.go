package main

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/131/ssh-agent-totp/internal/i18n"
	"github.com/131/ssh-agent-totp/settings"
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
