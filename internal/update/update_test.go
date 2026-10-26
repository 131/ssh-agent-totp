package update

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/131/ssh-agent-totp/settings"
)

// releaseServer serves manifest.json (production → tag, beta → v2.0.0-beta.1)
// and the release files of tag. *hits counts manifest downloads.
func releaseServer(t *testing.T, tag string, asset []byte, sum string) (*httptest.Server, *int) {
	t.Helper()
	mux := http.NewServeMux()
	hits := new(int)
	mux.HandleFunc("/131/ssh-agent-totp/HEAD/manifest.json", func(w http.ResponseWriter, r *http.Request) {
		*hits++
		fmt.Fprintf(w, `{"production": %q, "beta": "v2.0.0-beta.1", "bogus": "../x", "notes": 3}`, tag)
	})
	mux.HandleFunc("/131/ssh-agent-totp/releases/download/"+tag+"/SHA256SUMS", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(sum + "  ssh-agent-totp-linux-amd64\n" + sum + "  " + AssetName + "\n"))
	})
	mux.HandleFunc("/131/ssh-agent-totp/releases/download/"+tag+"/"+AssetName, func(w http.ResponseWriter, r *http.Request) { w.Write(asset) })
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server, hits
}

func testUpdater(t *testing.T, server *httptest.Server, current, exe string, verify func(string, string) error) *Updater {
	return &Updater{GitHub: server.URL, Raw: server.URL, HTTP: server.Client(), Current: current,
		Settings: &settings.Store{Path: filepath.Join(t.TempDir(), "settings.yml")}, Now: time.Now, Verify: verify,
		Executable: func() (string, error) { return exe, nil }}
}

func TestUpdateCheck(t *testing.T) {
	server, _ := releaseServer(t, "v1.2.0", nil, "")
	for current, available := range map[string]bool{"v1.1.9": true, "1.2.0": false, "v1.3.0": false, "0.0.0-dev": false, "dev": false} {
		release, err := testUpdater(t, server, current, "", nil).Check()
		development := strings.Contains(current, "dev")
		if err != nil || release.Available != available || release.Development != development || release.Latest != "v1.2.0" || release.Channel != "production" {
			t.Fatalf("%s: %+v %v", current, release, err)
		}
		if !slices.Equal(release.Channels, []string{"production", "beta"}) {
			t.Fatalf("channels: %v", release.Channels)
		}
	}
}

func TestUpdateChannels(t *testing.T) {
	server, hits := releaseServer(t, "v1.2.0", nil, "")
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	u := testUpdater(t, server, "v1.2.0", "", nil)
	u.Now = func() time.Time { return now }

	if _, err := u.SetChannel("nightly"); err == nil {
		t.Fatal("unknown channel accepted")
	}
	release, err := u.SetChannel("beta")
	if err != nil || release.Channel != "beta" || release.Latest != "v2.0.0-beta.1" || !release.Available {
		t.Fatalf("beta: %+v %v", release, err)
	}
	if *hits != 1 {
		t.Fatalf("manifest fetched %d times within the TTL", *hits)
	}
	saved, _ := u.Settings.Load()
	if saved.Update.Channel != "beta" || saved.RemoteChannels.TTL != settings.DefaultTTL || saved.RemoteChannels.List["production"] != "v1.2.0" {
		t.Fatalf("settings: %+v", saved)
	}

	// A manual check ignores the TTL.
	if release, err := u.CheckNow(); err != nil || *hits != 2 || release.Upstream != settings.DefaultUpstream {
		t.Fatalf("manual check: %+v %v (%d hits)", release, err, *hits)
	}
	// Expired cache: refetched; a failed refetch keeps the stale list.
	now = now.Add(25 * time.Hour)
	u.Check()
	if *hits != 3 {
		t.Fatalf("expired cache not refreshed (%d)", *hits)
	}
	now = now.Add(25 * time.Hour)
	u.Raw = server.URL + "/missing"
	if release, err := u.Check(); err != nil || release.Latest != "v2.0.0-beta.1" {
		t.Fatalf("stale cache: %+v %v", release, err)
	}
	if _, err := u.CheckNow(); err == nil {
		t.Fatal("failed manual check must be reported")
	}
	// A beta build follows its channel to the next prerelease.
	u.Current = "v2.0.0-beta.0"
	if release, _ := u.Check(); !release.Available {
		t.Fatal("beta build must update to a newer beta")
	}
}

func TestUpdateInstall(t *testing.T) {
	asset := []byte("new binary")
	digest := sha256.Sum256(asset)
	server, _ := releaseServer(t, "v1.2.0", asset, hex.EncodeToString(digest[:]))
	exe := filepath.Join(t.TempDir(), "ssh-agent-totp.exe")
	os.WriteFile(exe, []byte("old binary"), 0o755)

	refused := errors.New("signer mismatch")
	if _, err := testUpdater(t, server, "v1.0.0", exe, func(string, string) error { return refused }).Install("v1.2.0"); err != refused {
		t.Fatalf("verify error expected, got %v", err)
	}
	if body, _ := os.ReadFile(exe); string(body) != "old binary" {
		t.Fatal("refused update must keep the running executable")
	}
	if _, err := os.Stat(exe + ".new"); !os.IsNotExist(err) {
		t.Fatal("refused candidate must be removed")
	}

	var verified string
	path, err := testUpdater(t, server, "v1.0.0", exe, func(candidate, current string) error { verified = candidate; return nil }).Install("v1.2.0")
	if err != nil || path != exe || verified != exe+".new" {
		t.Fatalf("install: %s %v (verified %s)", path, err, verified)
	}
	if body, _ := os.ReadFile(exe); string(body) != "new binary" {
		t.Fatal("executable not replaced")
	}
	if body, _ := os.ReadFile(exe + ".old"); string(body) != "old binary" {
		t.Fatal("previous executable not kept")
	}
}

func TestUpdateChecksumMismatch(t *testing.T) {
	server, _ := releaseServer(t, "v1.2.0", []byte("tampered"), "00"+hex.EncodeToString(make([]byte, 31)))
	exe := filepath.Join(t.TempDir(), "ssh-agent-totp.exe")
	os.WriteFile(exe, []byte("old binary"), 0o755)
	if _, err := testUpdater(t, server, "v1.0.0", exe, nil).Install("v1.2.0"); err == nil {
		t.Fatal("checksum mismatch accepted")
	}
	if body, _ := os.ReadFile(exe); string(body) != "old binary" {
		t.Fatal("executable replaced despite mismatch")
	}
}

func TestUpdateRefusesDowngrade(t *testing.T) {
	server, _ := releaseServer(t, "v1.2.0", []byte("old release"), "")
	for _, current := range []string{"v1.2.0", "v1.3.0", "0.0.0-dev"} {
		if _, err := testUpdater(t, server, current, "", nil).Install("v1.2.0"); err == nil {
			t.Fatalf("%s: install of v1.2.0 accepted", current)
		}
	}
}

func TestUpdateSettings(t *testing.T) {
	server, hits := releaseServer(t, "v1.2.0", nil, "")
	u := testUpdater(t, server, "v1.0.0", "", nil)
	u.Settings.Update(func(s *settings.Settings) { s.Update.Enable = false })
	if release, err := u.Check(); err != nil || release.Enabled || release.Available || *hits != 0 {
		t.Fatalf("disabled: %+v %v (%d hits)", release, err, *hits)
	}
	if _, err := u.Install("v1.2.0"); err == nil {
		t.Fatal("disabled updates must not install")
	}
	if release, err := u.SetEnabled(true); err != nil || !release.Enabled || !release.Available || *hits != 1 {
		t.Fatalf("enabled: %+v %v (%d hits)", release, err, *hits)
	}
	if release, err := u.SetEnabled(false); err != nil || release.Enabled || release.Available {
		t.Fatalf("disabled again: %+v %v", release, err)
	}
	if saved, _ := u.Settings.Load(); saved.Update.Enable {
		t.Fatal("update.enable not saved")
	}
	for _, upstream := range []string{"http://github.com/131/ssh-agent-totp", "https://gitlab.com/131/ssh-agent-totp", "https://github.com/131", "https://github.com/131/ssh-agent-totp/releases"} {
		u.Settings.Update(func(s *settings.Settings) { s.Update = settings.Update{Enable: true, Upstream: upstream} })
		if _, err := u.Check(); err == nil {
			t.Fatalf("upstream %s accepted", upstream)
		}
	}
	u.Settings.Update(func(s *settings.Settings) { s.Update.Upstream = "https://github.com/131/ssh-agent-totp.git/" })
	if release, err := u.Check(); err != nil || !release.Available || release.URL != server.URL+"/131/ssh-agent-totp/releases/tag/v1.2.0" {
		t.Fatalf("upstream with .git: %+v %v", release, err)
	}
}
