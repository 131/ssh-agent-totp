// Package update replaces the running executable with the release its channel
// points to in the upstream manifest.json. See specs/RELEASES.md.
package update

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"

	"github.com/131/ssh-agent-totp/internal/i18n"
	"github.com/131/ssh-agent-totp/settings"
)

const (
	AssetName = "ssh-agent-totp-windows-amd64.exe"
	maxAsset  = 128 << 20
)

// Updater installs a release only when it is newer than Current, listed in
// SHA256SUMS and accepted by Verify (on Windows: trusted Authenticode
// signature from the signer of the running executable).
type Updater struct {
	// GitHub and Raw are the hosts serving releases and repository files.
	GitHub, Raw string
	HTTP        *http.Client
	Current     string
	Settings    *settings.Store
	Now         func() time.Time
	Executable  func() (string, error)
	// Verify checks the downloaded candidate against the running executable.
	Verify func(candidate, current string) error
}

// Release describes the update state shown by the UI.
type Release struct {
	Enabled   bool     `json:"enabled"`
	Current   string   `json:"current"`
	Channel   string   `json:"channel"`
	Channels  []string `json:"channels"`
	Latest    string   `json:"latest"`
	Available bool     `json:"available"`
	URL       string   `json:"url"`
	Upstream  string   `json:"upstream"`
	// Development builds never update: the UI links to the release instead.
	Development bool `json:"development"`
	// Error is the failed check, for the settings panel.
	Error string `json:"error,omitempty"`
}

func New(current string, store *settings.Store, verify func(candidate, current string) error) *Updater {
	return &Updater{GitHub: "https://github.com", Raw: "https://raw.githubusercontent.com",
		HTTP: &http.Client{Timeout: 2 * time.Minute}, Current: current, Settings: store,
		Now: time.Now, Executable: os.Executable, Verify: verify}
}

// repository returns /<owner>/<repo> of a https://github.com/<owner>/<repo>
// upstream.
func repository(upstream string) (string, error) {
	link, err := url.Parse(upstream)
	parts := []string{}
	if err == nil {
		parts = strings.Split(strings.Trim(strings.TrimSuffix(link.Path, ".git"), "/"), "/")
	}
	if err != nil || link.Scheme != "https" || link.Host != "github.com" || len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", i18n.Errorf("err.upstream", upstream)
	}
	return "/" + parts[0] + "/" + parts[1], nil
}

func (u *Updater) releases(upstream string) (string, error) {
	repo, err := repository(upstream)
	return u.GitHub + repo + "/releases", err
}

// channels returns the settings with the channel list, from the cache while
// its TTL runs (unless refresh), else from manifest.json. A failed automatic
// download falls back to a stale cache of the same upstream.
func (u *Updater) channels(refresh bool) (settings.Settings, error) {
	current, err := u.Settings.Load()
	if err != nil || !current.Update.Enable || (!refresh && current.RemoteChannels.Fresh(current.Update.Upstream, u.Now())) {
		return current, err
	}
	list, err := u.fetchManifest(current.Update.Upstream)
	if err != nil {
		if cache := current.RemoteChannels; !refresh && cache != nil && cache.Upstream == current.Update.Upstream && len(cache.List) > 0 {
			return current, nil
		}
		return current, err
	}
	ttl := settings.DefaultTTL
	if current.RemoteChannels != nil && current.RemoteChannels.TTL > 0 {
		ttl = current.RemoteChannels.TTL
	}
	return u.Settings.Update(func(s *settings.Settings) {
		s.RemoteChannels = &settings.RemoteChannels{Upstream: current.Update.Upstream, TTL: ttl, Fetched: u.Now().UTC(), List: list}
	})
}

func (u *Updater) fetchManifest(upstream string) (map[string]string, error) {
	repo, err := repository(upstream)
	if err != nil {
		return nil, err
	}
	response, err := u.HTTP.Get(u.Raw + repo + "/HEAD/manifest.json")
	if err != nil {
		return nil, err
	}
	if response.StatusCode == http.StatusNotFound {
		response.Body.Close()
		return nil, i18n.Errorf("err.manifest.missing", upstream)
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		return nil, i18n.Errorf("err.download", u.Raw+repo+"/HEAD/manifest.json", response.StatusCode)
	}
	defer response.Body.Close()
	var manifest map[string]any
	if err := json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&manifest); err != nil {
		return nil, i18n.Errorf("err.manifest", err)
	}
	list := map[string]string{}
	for channel, tag := range manifest {
		if tag, ok := tag.(string); ok && validTag(tag) {
			list[channel] = tag
		}
	}
	if len(list) == 0 {
		return nil, i18n.Errorf("err.manifest.empty")
	}
	return list, nil
}

func validTag(tag string) bool {
	_, err := semver.NewVersion(tag)
	return err == nil && !strings.ContainsAny(tag, "/?#%")
}

// newer reports whether tag may replace the running version: development
// builds (unparsable or -dev) never update, and downgrades are refused.
func (u *Updater) newer(tag string) bool {
	if u.development() {
		return false
	}
	current, _ := semver.NewVersion(u.Current)
	latest, err := semver.NewVersion(tag)
	return err == nil && latest.GreaterThan(current)
}

// development reports a build that is not a release (unparsable or -dev).
func (u *Updater) development() bool {
	current, err := semver.NewVersion(u.Current)
	return err != nil || strings.HasPrefix(current.Prerelease(), "dev")
}

// Check compares the tag of the configured channel (production when it is
// not in the list) with the running version. Disabled updates check nothing.
func (u *Updater) Check() (Release, error) { return u.check(false) }

// CheckNow is Check with a fresh manifest.json, ignoring the cache TTL.
func (u *Updater) CheckNow() (Release, error) { return u.check(true) }

func (u *Updater) check(refresh bool) (Release, error) {
	release := Release{Current: u.Current, Channel: settings.DefaultChannel, Development: u.development()}
	current, err := u.channels(refresh)
	release.Enabled, release.Upstream = current.Update.Enable, current.Update.Upstream
	if err != nil || !release.Enabled {
		return release, err
	}
	list := current.RemoteChannels.List
	for channel := range list {
		release.Channels = append(release.Channels, channel)
	}
	slices.SortFunc(release.Channels, func(a, b string) int {
		if (a == settings.DefaultChannel) != (b == settings.DefaultChannel) {
			if a == settings.DefaultChannel {
				return -1
			}
			return 1
		}
		return strings.Compare(a, b)
	})
	if _, ok := list[current.Update.Channel]; ok {
		release.Channel = current.Update.Channel
	}
	tag, ok := list[release.Channel]
	if !ok {
		return release, nil
	}
	base, _ := u.releases(current.Update.Upstream)
	release.Latest, release.URL = tag, base+"/tag/"+tag
	release.Available = u.newer(tag)
	return release, nil
}

// SetEnabled turns auto-update on or off (update.enable), then checks again.
func (u *Updater) SetEnabled(enable bool) (Release, error) {
	if _, err := u.Settings.Update(func(s *settings.Settings) { s.Update.Enable = enable }); err != nil {
		return Release{}, err
	}
	return u.Check()
}

// SetChannel follows channel, which must be in the channel list.
func (u *Updater) SetChannel(channel string) (Release, error) {
	current, err := u.channels(false)
	if err != nil {
		return Release{}, err
	}
	if !current.Update.Enable {
		return Release{}, i18n.Errorf("err.update.disabled")
	}
	if _, ok := current.RemoteChannels.List[channel]; !ok {
		return Release{}, i18n.Errorf("err.channel", channel)
	}
	if _, err := u.Settings.Update(func(s *settings.Settings) { s.Update.Channel = channel }); err != nil {
		return Release{}, err
	}
	return u.Check()
}

func (u *Updater) get(url string) (*http.Response, error) {
	response, err := u.HTTP.Get(url)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		return nil, i18n.Errorf("err.download", url, response.StatusCode)
	}
	return response, nil
}

func (u *Updater) checksum(base, tag string) (string, error) {
	response, err := u.get(base + "/download/" + tag + "/SHA256SUMS")
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	scanner := bufio.NewScanner(io.LimitReader(response.Body, 1<<20))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == AssetName && len(fields[0]) == 64 {
			return strings.ToLower(fields[0]), nil
		}
	}
	return "", i18n.Errorf("err.sums", AssetName)
}

// Install downloads tag next to the running executable, verifies it, then
// swaps the files; the previous executable is kept as <exe>.old until the next
// start. It returns the path to launch.
func (u *Updater) Install(tag string) (string, error) {
	if !validTag(tag) || !u.newer(tag) {
		return "", i18n.Errorf("err.downgrade", tag, u.Current)
	}
	config, err := u.Settings.Load()
	if err != nil {
		return "", err
	}
	if !config.Update.Enable {
		return "", i18n.Errorf("err.update.disabled")
	}
	base, err := u.releases(config.Update.Upstream)
	if err != nil {
		return "", err
	}
	expected, err := u.checksum(base, tag)
	if err != nil {
		return "", err
	}
	current, err := u.Executable()
	if err != nil {
		return "", err
	}
	candidate := current + ".new"
	if err := u.download(base+"/download/"+tag+"/"+AssetName, candidate, expected); err != nil {
		os.Remove(candidate)
		return "", err
	}
	if u.Verify != nil {
		if err := u.Verify(candidate, current); err != nil {
			os.Remove(candidate)
			return "", err
		}
	}
	previous := current + ".old"
	os.Remove(previous)
	if err := os.Rename(current, previous); err != nil {
		os.Remove(candidate)
		return "", err
	}
	if err := os.Rename(candidate, current); err != nil {
		os.Rename(previous, current)
		os.Remove(candidate)
		return "", err
	}
	return current, nil
}

func (u *Updater) download(url, path, expected string) error {
	response, err := u.get(url)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(file, hash), io.LimitReader(response.Body, maxAsset+1))
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if written > maxAsset {
		return i18n.Errorf("err.too.large")
	}
	if hex.EncodeToString(hash.Sum(nil)) != expected {
		return i18n.Errorf("err.checksum")
	}
	return nil
}

// Cleanup removes the executable replaced by a previous update.
func Cleanup() {
	if current, err := os.Executable(); err == nil {
		os.Remove(current + ".old")
	}
}
