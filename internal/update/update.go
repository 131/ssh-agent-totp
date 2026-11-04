// Package update follows a release channel with the Wails updater: the
// static manifest updates/<channel>.json committed upstream names the release,
// Wails downloads, checks its digest and swaps the executable on restart.
// See specs/RELEASES.md.
package update

import (
	"context"
	"io"
	"net/url"
	"slices"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/wailsapp/wails/v3/pkg/updater"
	"github.com/wailsapp/wails/v3/pkg/updater/providers/endpoint"

	"github.com/131/ssh-agent-totp/internal/i18n"
	"github.com/131/ssh-agent-totp/settings"
)

// Channels are the manifests published in updates/ upstream, production first.
var Channels = []string{settings.DefaultChannel, "beta"}

// Updater installs a release only when the Wails updater offers it (newer,
// digest from the manifest) and Verify accepts it (on Windows: trusted
// Authenticode signature from the signer of the running executable).
type Updater struct {
	Wails *updater.Updater
	// Raw serves the repository files (the channel manifests).
	Raw      string
	Current  string
	Settings *settings.Store
	// Executable is the running executable, compared by Verify.
	Executable func() (string, error)
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

// Init configures the Wails updater without its window: the settings panel
// is the update UI. Development builds report any release as newer, for the
// release link, and never install it.
func (u *Updater) Init() error {
	current := strings.TrimPrefix(u.Current, "v")
	if u.development() {
		current = "0.0.0"
	}

	return u.Wails.Init(updater.Config{
		CurrentVersion: current,
		Providers:      []updater.Provider{feed{u}},
		Window:         updater.WindowNone,
	})
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

// channel is the followed channel, production when unknown.
func channel(current settings.Settings) string {
	if slices.Contains(Channels, current.Update.Channel) {
		return current.Update.Channel
	}

	return settings.DefaultChannel
}

// feed is the endpoint provider of the configured upstream and channel,
// read again at each check.
type feed struct{ u *Updater }

func (f feed) Name() string { return "endpoint" }

func (f feed) provider() (*endpoint.Provider, error) {
	current, err := f.u.Settings.Load()
	if err != nil {
		return nil, err
	}

	if !current.Update.Enable {
		return nil, i18n.Errorf("err.update.disabled")
	}

	repo, err := repository(current.Update.Upstream)
	if err != nil {
		return nil, err
	}

	return endpoint.New(endpoint.Config{URL: f.u.Raw + repo + "/HEAD/updates/{{channel}}.json", Channel: channel(current)})
}

func (f feed) Check(ctx context.Context, request updater.CheckRequest) (*updater.Release, error) {
	provider, err := f.provider()
	if err != nil {
		return nil, err
	}

	release, err := provider.Check(ctx, request)
	// Without a digest the Wails updater would install unchecked bytes.
	if release != nil && (release.Verification == nil || len(release.Verification.Digest) == 0) {
		return nil, i18n.Errorf("err.digest", release.Version)
	}

	return release, err
}

func (f feed) Download(ctx context.Context, release *updater.Release, dst io.Writer, progress func(int64, int64)) error {
	provider, err := f.provider()
	if err != nil {
		return err
	}

	return provider.Download(ctx, release, dst, progress)
}

// development reports a build that is not a release (unparsable or -dev).
func (u *Updater) development() bool {
	current, err := semver.NewVersion(u.Current)
	return err != nil || strings.HasPrefix(current.Prerelease(), "dev")
}

// Check asks the followed channel for a newer release. Disabled updates
// check nothing.
func (u *Updater) Check() (Release, error) {
	release := Release{Current: u.Current, Channel: settings.DefaultChannel, Development: u.development()}
	current, err := u.Settings.Load()
	release.Enabled, release.Upstream = current.Update.Enable, current.Update.Upstream
	if err != nil || !release.Enabled {
		return release, err
	}

	release.Channel, release.Channels = channel(current), Channels
	found, err := u.Wails.Check(context.Background())
	if err != nil || found == nil {
		return release, err
	}

	repo, err := repository(current.Update.Upstream)
	if err != nil {
		return release, err
	}

	release.Latest = "v" + found.Version
	release.Available = !release.Development
	release.URL = "https://github.com" + repo + "/releases/tag/" + release.Latest
	return release, nil
}

// SetEnabled turns auto-update on or off (update.enable), then checks again.
func (u *Updater) SetEnabled(enable bool) (Release, error) {
	if _, err := u.Settings.Update(func(s *settings.Settings) { s.Update.Enable = enable }); err != nil {
		return Release{}, err
	}

	return u.Check()
}

// SetChannel follows channel, which must be one of Channels.
func (u *Updater) SetChannel(channel string) (Release, error) {
	current, err := u.Settings.Load()
	if err != nil {
		return Release{}, err
	}

	if !current.Update.Enable {
		return Release{}, i18n.Errorf("err.update.disabled")
	}

	if !slices.Contains(Channels, channel) {
		return Release{}, i18n.Errorf("err.channel", channel)
	}

	if _, err := u.Settings.Update(func(s *settings.Settings) { s.Update.Channel = channel }); err != nil {
		return Release{}, err
	}

	return u.Check()
}

// Install downloads tag, still offered by the channel, and verifies it; the
// Wails updater then swaps it in on Restart.
func (u *Updater) Install(tag string) error {
	release, err := u.Check()
	if err != nil {
		return err
	}

	if !release.Available || release.Latest != tag {
		return i18n.Errorf("err.downgrade", tag, u.Current)
	}

	if err := u.Wails.DownloadAndInstall(context.Background()); err != nil {
		return err
	}

	if u.Verify == nil {
		return nil
	}

	current, err := u.Executable()
	if err != nil {
		return err
	}

	return u.Verify(u.Wails.DownloadedPath(), current)
}

// Restart quits; the Wails helper swaps the executable and starts it again.
func (u *Updater) Restart() error {
	return u.Wails.Restart(context.Background())
}
