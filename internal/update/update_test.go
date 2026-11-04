package update

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/updater"

	"github.com/131/ssh-agent-totp/settings"
)

// host is the Wails application seen by the updater: no window, no quit.
type host struct{ available func() }

func (h host) Emit(event string, _ ...any) bool {
	if event == updater.EventUpdateAvailable && h.available != nil {
		h.available()
	}
	return true
}
func (host) OnEvent(string, func(any)) func()                      { return func() {} }
func (host) OpenWindow(updater.WindowOptions) updater.WindowHandle { return nil }
func (host) Quit()                                                 {}

// releaseServer serves updates/production.json (version) and
// updates/beta.json (2.0.0-beta.1), both naming asset with digest of sum.
// *hits counts manifest downloads.
func releaseServer(t *testing.T, version string, asset, sum []byte) (*httptest.Server, *int) {
	t.Helper()
	if sum == nil {
		digest := sha256.Sum256(asset)
		sum = digest[:]
	}
	manifest := func(version string) string {
		return fmt.Sprintf(`{"schemaVersion": 1, "version": %q, "artifacts": [{"url": "../releases/download/v%s/ssh-agent-totp-windows-amd64.exe", "digestAlgo": "sha256", "digest": %q}]}`,
			version, version, base64.StdEncoding.EncodeToString(sum))
	}
	mux := http.NewServeMux()
	hits := new(int)
	for channel, version := range map[string]string{"production": version, "beta": "2.0.0-beta.1"} {
		mux.HandleFunc("/131/ssh-agent-totp/HEAD/updates/"+channel+".json", func(w http.ResponseWriter, r *http.Request) {
			*hits++
			w.Write([]byte(manifest(version)))
		})
	}
	mux.HandleFunc("/131/ssh-agent-totp/HEAD/releases/download/", func(w http.ResponseWriter, r *http.Request) { w.Write(asset) })
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server, hits
}

func testUpdater(t *testing.T, server *httptest.Server, current, exe string, verify func(string, string) error) *Updater {
	u := &Updater{restart: func() error { return nil }, Wails: updater.New(host{}), Raw: server.URL, Current: current,
		Settings: &settings.Store{Path: filepath.Join(t.TempDir(), "settings.yml")}, Verify: verify,
		Executable: func() (string, error) { return exe, nil }}
	if err := u.Init(); err != nil {
		t.Fatal(err)
	}
	return u
}

func TestUpdateCheck(t *testing.T) {
	server, _ := releaseServer(t, "1.2.0", nil, nil)
	for current, latest := range map[string]string{"v1.1.9": "v1.2.0", "1.2.0": "", "v1.3.0": "", "0.0.0-dev": "v1.2.0", "dev": "v1.2.0"} {
		release, err := testUpdater(t, server, current, "", nil).Check()
		development := strings.Contains(current, "dev")
		if err != nil || release.Available != (latest != "" && !development) || release.Development != development || release.Latest != latest || release.Channel != "production" {
			t.Fatalf("%s: %+v %v", current, release, err)
		}
		if !slices.Equal(release.Channels, []string{"production", "beta"}) {
			t.Fatalf("channels: %v", release.Channels)
		}
	}
}

func TestUpdateChannels(t *testing.T) {
	server, _ := releaseServer(t, "1.2.0", nil, nil)
	u := testUpdater(t, server, "v1.2.0", "", nil)
	if _, err := u.SetChannel("nightly"); err == nil {
		t.Fatal("unknown channel accepted")
	}
	release, err := u.SetChannel("beta")
	if err != nil || release.Channel != "beta" || release.Latest != "v2.0.0-beta.1" || !release.Available {
		t.Fatalf("beta: %+v %v", release, err)
	}
	if saved, _ := u.Settings.Load(); saved.Update.Channel != "beta" {
		t.Fatalf("settings: %+v", saved)
	}
	// Back to production: nothing newer, no downgrade.
	if release, err := u.SetChannel("production"); err != nil || release.Available {
		t.Fatalf("production: %+v %v", release, err)
	}
	// A missing manifest (404) means nothing to install yet.
	u.Raw = server.URL + "/missing"
	if release, err := u.Check(); err != nil || release.Available {
		t.Fatalf("missing manifest: %+v %v", release, err)
	}
}

func TestUpdateInstall(t *testing.T) {
	asset := []byte("new binary")
	server, _ := releaseServer(t, "1.2.0", asset, nil)
	exe := filepath.Join(t.TempDir(), "ssh-agent-totp.exe")
	os.WriteFile(exe, []byte("old binary"), 0o755)

	refused := errors.New("signer mismatch")
	if err := testUpdater(t, server, "v1.0.0", exe, func(string, string) error { return refused }).Install("v1.2.0"); err != refused {
		t.Fatalf("verify error expected, got %v", err)
	}

	var verified, checked string
	u := testUpdater(t, server, "v1.0.0", exe, func(candidate, current string) error { verified, checked = candidate, current; return nil })
	if err := u.Install("v1.2.0"); err != nil || verified != u.Wails.DownloadedPath() || checked != exe {
		t.Fatalf("install: %v (verified %s against %s)", err, verified, checked)
	}
	if body, _ := os.ReadFile(verified); string(body) != "new binary" {
		t.Fatal("staged executable")
	}
	if body, _ := os.ReadFile(exe); string(body) != "old binary" {
		t.Fatal("the executable is swapped on restart only")
	}
}

func TestUpdateDigest(t *testing.T) {
	server, _ := releaseServer(t, "1.2.0", []byte("tampered"), make([]byte, sha256.Size))
	if err := testUpdater(t, server, "v1.0.0", "", nil).Install("v1.2.0"); err == nil {
		t.Fatal("digest mismatch accepted")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/131/ssh-agent-totp/HEAD/updates/production.json", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"schemaVersion": 1, "version": "1.2.0", "artifacts": [{"url": "x.exe"}]}`))
	})
	unsigned := httptest.NewServer(mux)
	defer unsigned.Close()
	if _, err := testUpdater(t, unsigned, "v1.0.0", "", nil).Check(); err == nil {
		t.Fatal("manifest without digest accepted")
	}
}

func TestUpdateRefusesDowngrade(t *testing.T) {
	server, _ := releaseServer(t, "1.2.0", []byte("old release"), nil)
	for _, current := range []string{"v1.2.0", "v1.3.0", "0.0.0-dev"} {
		if err := testUpdater(t, server, current, "", nil).Install("v1.2.0"); err == nil {
			t.Fatalf("%s: install of v1.2.0 accepted", current)
		}
	}
	if err := testUpdater(t, server, "v1.0.0", "", nil).Install("v1.1.0"); err == nil {
		t.Fatal("tag not offered by the channel accepted")
	}
}

func TestUpdateSettings(t *testing.T) {
	server, hits := releaseServer(t, "1.2.0", nil, nil)
	u := testUpdater(t, server, "v1.0.0", "", nil)
	u.Settings.Update(func(s *settings.Settings) { s.Update.Enable = false })
	if release, err := u.Check(); err != nil || release.Enabled || release.Available || *hits != 0 {
		t.Fatalf("disabled: %+v %v (%d hits)", release, err, *hits)
	}
	if err := u.Install("v1.2.0"); err == nil {
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
	if release, err := u.Check(); err != nil || !release.Available || release.URL != "https://github.com/131/ssh-agent-totp/releases/tag/v1.2.0" {
		t.Fatalf("upstream with .git: %+v %v", release, err)
	}
}

func TestUpdateInstallSerializesChannelThroughRestart(t *testing.T) {
	server, _ := releaseServer(t, "1.2.0", []byte("production binary"), nil)
	verified, allowVerify := make(chan struct{}), make(chan struct{})
	restarting, allowRestart := make(chan struct{}), make(chan struct{})
	u := testUpdater(t, server, "v1.0.0", "current.exe", func(candidate, _ string) error {
		data, err := os.ReadFile(candidate)
		if err != nil || string(data) != "production binary" {
			return fmt.Errorf("wrong download: %q, %v", data, err)
		}
		close(verified)
		<-allowVerify
		return nil
	})
	u.restart = func() error { close(restarting); <-allowRestart; return nil }
	// Always release workers on assertion failure, too.
	defer func() {
		select {
		case <-allowVerify:
		default:
			close(allowVerify)
		}
		select {
		case <-allowRestart:
		default:
			close(allowRestart)
		}
	}()
	installed := make(chan error, 1)
	go func() { installed <- u.Install("v1.2.0") }()
	waitUpdate(t, verified, "signature verification")
	changed := make(chan error, 1)
	started := make(chan struct{})
	go func() { close(started); _, err := u.SetChannel("beta"); changed <- err }()
	waitUpdate(t, started, "channel request")
	stillWaiting := func(stage string) {
		t.Helper()
		select {
		case err := <-changed:
			t.Fatalf("channel changed during %s: %v", stage, err)
		case <-time.After(30 * time.Millisecond):
		}
		current, err := u.Settings.Load()
		if err != nil || current.Update.Channel != "production" {
			t.Fatalf("channel during %s: %+v, %v", stage, current.Update, err)
		}
	}
	stillWaiting("verification")
	close(allowVerify)
	waitUpdate(t, restarting, "restart")
	stillWaiting("restart")
	close(allowRestart)
	if err := <-installed; err != nil {
		t.Fatal(err)
	}
	if err := <-changed; err != nil {
		t.Fatal(err)
	}
	if saved, err := u.Settings.Load(); err != nil || saved.Update.Channel != "beta" {
		t.Fatalf("channel request not applied after transaction: %+v, %v", saved.Update, err)
	}
}

func waitUpdate(t *testing.T, signal <-chan struct{}, stage string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", stage)
	}
}

func TestUpdateVerificationFailureDoesNotRestart(t *testing.T) {
	server, _ := releaseServer(t, "1.2.0", []byte("binary"), nil)
	rejected := errors.New("signer rejected")
	u := testUpdater(t, server, "v1.0.0", "current.exe", func(string, string) error { return rejected })
	restarts := 0
	u.restart = func() error { restarts++; return nil }
	if err := u.Install("v1.2.0"); !errors.Is(err, rejected) || restarts != 0 {
		t.Fatalf("rejected signature: %v, restarts=%d", err, restarts)
	}
	// The failed transaction releases the lock and can be retried.
	u.Verify = func(string, string) error { return nil }
	if err := u.Install("v1.2.0"); err != nil || restarts != 1 {
		t.Fatalf("retry: %v, restarts=%d", err, restarts)
	}
}

func TestUpdateInstallKeepsTheCheckedRelease(t *testing.T) {
	mux := http.NewServeMux()
	for channel, version := range map[string]string{"production": "1.2.0", "beta": "2.0.0-beta.1"} {
		digest := sha256.Sum256([]byte(version))
		manifest := fmt.Sprintf(`{"schemaVersion":1,"version":%q,"artifacts":[{"url":"../releases/%s.exe","digestAlgo":"sha256","digest":%q}]}`,
			version, version, base64.StdEncoding.EncodeToString(digest[:]))
		mux.HandleFunc("/131/ssh-agent-totp/HEAD/updates/"+channel+".json", func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, manifest) })
		mux.HandleFunc("/131/ssh-agent-totp/HEAD/releases/"+version+".exe", func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, version) })
	}
	server := httptest.NewServer(mux)
	defer server.Close()
	u := &Updater{Raw: server.URL, Current: "v1.0.0", Settings: &settings.Store{Path: filepath.Join(t.TempDir(), "settings.yml")},
		Executable: func() (string, error) { return "current.exe", nil }, restart: func() error { return nil },
		Verify: func(candidate, _ string) error {
			data, err := os.ReadFile(candidate)
			if err != nil || string(data) != "1.2.0" {
				return fmt.Errorf("requested v1.2.0, downloaded %q (%v)", data, err)
			}
			return nil
		}}
	started, changed := make(chan struct{}), make(chan error, 1)
	var once sync.Once
	u.Wails = updater.New(host{available: func() {
		// Force a channel request after Wails has selected production but before
		// the install caller has received Check's result and started downloading.
		once.Do(func() {
			go func() { close(started); _, err := u.SetChannel("beta"); changed <- err }()
			waitUpdate(t, started, "concurrent channel request")
			select {
			case err := <-changed:
				t.Fatalf("pending release replaced before download: %v", err)
			case <-time.After(30 * time.Millisecond):
			}
		})
	}})
	if err := u.Init(); err != nil {
		t.Fatal(err)
	}
	if err := u.Install("v1.2.0"); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-changed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("channel request was not released")
	}
}
