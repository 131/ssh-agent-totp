package settings

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestDefaultsAndPartialFile(t *testing.T) {
	store := &Store{Path: filepath.Join(t.TempDir(), "settings.yml")}
	if settings, err := store.Load(); err != nil || !reflect.DeepEqual(settings, Defaults()) {
		t.Fatalf("missing file: %+v %v", settings, err)
	}
	os.WriteFile(store.Path, []byte("update:\n  enable: false\n  upstream: https://github.com/me/fork/\n"), 0o600)
	settings, err := store.Load()
	want := Update{Enable: false, Upstream: "https://github.com/me/fork", Channel: DefaultChannel}
	if err != nil || settings.Update != want {
		t.Fatalf("partial file: %+v %v", settings, err)
	}
	for language, want := range map[string]string{"en-us": "en-us", "EN-US": "en-us", "de-de": "fr-fr", "": "fr-fr"} {
		os.WriteFile(store.Path, []byte("language: "+language+"\n"), 0o600)
		if settings, err := store.Load(); err != nil || settings.Language != want {
			t.Fatalf("language %q: %+v %v", language, settings, err)
		}
	}
}

func TestUpdateWritesYAML(t *testing.T) {
	store := &Store{Path: filepath.Join(t.TempDir(), "ssh-agent-totp", "settings.yml")}
	fetched := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	_, err := store.Update(func(s *Settings) {
		s.Update.Channel = "beta"
		s.RemoteChannels = &RemoteChannels{Upstream: DefaultUpstream, TTL: DefaultTTL, Fetched: fetched, List: map[string]string{"beta": "v0.2.0-beta.1"}}
	})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(store.Path)
	for _, line := range []string{"enable: true", "upstream: " + DefaultUpstream, "channel: beta", "ttl: 86400", "beta: v0.2.0-beta.1"} {
		if !strings.Contains(string(body), line) {
			t.Fatalf("%q missing from\n%s", line, body)
		}
	}
	// The default token file is not written.
	if strings.Contains(string(body), "path:") {
		t.Fatalf("default tokens.path written:\n%s", body)
	}
	settings, _ := store.Load()
	if !settings.RemoteChannels.Fresh(DefaultUpstream, fetched.Add(time.Minute)) ||
		settings.RemoteChannels.Fresh(DefaultUpstream, fetched.Add(25*time.Hour)) ||
		settings.RemoteChannels.Fresh("https://github.com/me/fork", fetched) {
		t.Fatal("cache freshness")
	}
}

func TestUnreadableFileKept(t *testing.T) {
	store := &Store{Path: filepath.Join(t.TempDir(), "settings.yml")}
	os.WriteFile(store.Path, []byte("update: [\n"), 0o600)
	if _, err := store.Load(); err == nil {
		t.Fatal("invalid YAML accepted")
	}
	if _, err := store.Update(func(s *Settings) { s.Update.Channel = "beta" }); err == nil {
		t.Fatal("invalid file overwritten")
	}
	if body, _ := os.ReadFile(store.Path); string(body) != "update: [\n" {
		t.Fatal("invalid file modified")
	}
}

func TestTokensPath(t *testing.T) {
	store := &Store{Path: filepath.Join("conf", "ssh-agent-totp", "settings.yml")}
	if path := store.TokensPath(Settings{}); path != filepath.Join("conf", "ssh-agent-totp", "tokens.crypt") {
		t.Fatal(path)
	}
	t.Setenv("TOTP_HOME", "home")
	for value, want := range map[string]string{"%TOTP_HOME%/t.yml": "home/t.yml", "$TOTP_HOME/t.yml": "home/t.yml", "/abs/t.yml": "/abs/t.yml"} {
		if path := store.TokensPath(Settings{Tokens: Tokens{Path: value}}); path != want {
			t.Fatalf("%s: %s", value, path)
		}
	}
}
