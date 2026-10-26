// Package settings reads and writes the ssh-agent-totp settings file,
// %APPDATA%\ssh-agent-totp\settings.yml. See specs/SETTINGS.md.
package settings

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/131/ssh-agent-totp/internal/i18n"
)

const (
	DefaultUpstream = "https://github.com/131/ssh-agent-totp"
	DefaultChannel  = "production"
	DefaultTTL      = 86400
)

// Settings is the whole file. It holds preferences and caches, never secrets.
type Settings struct {
	Update         Update          `yaml:"update"`
	Language       string          `yaml:"language"`
	Tokens         Tokens          `yaml:"tokens"`
	RemoteChannels *RemoteChannels `yaml:"remote_channels,omitempty"`
}

// Update configures auto-update.
type Update struct {
	Enable   bool   `yaml:"enable"`
	Upstream string `yaml:"upstream"`
	Channel  string `yaml:"channel"`
}

// Tokens locates the token file (specs/TOKENS.md). An empty Path is
// tokens.crypt next to settings.yml; an empty Key is the first agent key.
type Tokens struct {
	Path string `yaml:"path,omitempty"`
	Key  string `yaml:"key"`
}

// RemoteChannels caches the upstream manifest.json (channel → release tag)
// for TTL seconds after Fetched.
type RemoteChannels struct {
	Upstream string            `yaml:"upstream"`
	TTL      int               `yaml:"ttl"`
	Fetched  time.Time         `yaml:"fetched"`
	List     map[string]string `yaml:"list"`
}

// Fresh reports whether the cache describes upstream and is still valid.
func (r *RemoteChannels) Fresh(upstream string, now time.Time) bool {
	return r != nil && r.Upstream == upstream && len(r.List) > 0 &&
		now.Before(r.Fetched.Add(time.Duration(r.TTL)*time.Second))
}

// Defaults are the settings of a missing file.
func Defaults() Settings {
	return Settings{Update: Update{Enable: true, Upstream: DefaultUpstream, Channel: DefaultChannel}, Language: i18n.Default}
}

// normalize restores defaults for empty values (and unknown languages).
func (s *Settings) normalize() {
	s.Update.Upstream = strings.TrimSuffix(strings.TrimSpace(s.Update.Upstream), "/")
	if s.Update.Upstream == "" {
		s.Update.Upstream = DefaultUpstream
	}
	if s.Update.Channel == "" {
		s.Update.Channel = DefaultChannel
	}
	s.Language = i18n.Normalize(s.Language)
	s.Tokens.Path, s.Tokens.Key = strings.TrimSpace(s.Tokens.Path), strings.TrimSpace(s.Tokens.Key)
}

// TokensPath is the token file of settings: tokens.path with environment
// variables expanded (%APPDATA% or $HOME), else tokens.crypt next to this file.
func (s *Store) TokensPath(settings Settings) string {
	if settings.Tokens.Path == "" {
		return filepath.Join(filepath.Dir(s.Path), "tokens.crypt")
	}
	return os.Expand(windowsVariables.ReplaceAllString(settings.Tokens.Path, "$${$1}"), os.Getenv)
}

var windowsVariables = regexp.MustCompile(`%([A-Za-z_][A-Za-z0-9_]*)%`)

const header = "# ssh-agent-totp settings: https://github.com/131/ssh-agent-totp/blob/master/specs/SETTINGS.md\n"

// Store is the settings file. A missing file means defaults; an unreadable
// one is reported and left untouched.
type Store struct {
	Path string
	mu   sync.Mutex
}

// DefaultStore is %APPDATA%\ssh-agent-totp\settings.yml (the user
// configuration directory elsewhere).
func DefaultStore() *Store {
	dir, err := os.UserConfigDir()
	if err != nil {
		return &Store{}
	}
	return &Store{Path: filepath.Join(dir, "ssh-agent-totp", "settings.yml")}
}

func (s *Store) read() (Settings, error) {
	settings := Defaults()
	if s.Path == "" {
		return settings, i18n.Errorf("err.settings.dir")
	}
	body, err := os.ReadFile(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return settings, nil
	}
	if err == nil {
		err = yaml.Unmarshal(body, &settings)
	}
	if err != nil {
		return Defaults(), i18n.Errorf("err.settings.read", s.Path, err)
	}
	settings.normalize()
	return settings, nil
}

// Load returns the settings, defaults on error.
func (s *Store) Load() (Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.read()
}

// Update applies change and saves the file. An unreadable file is never
// overwritten.
func (s *Store) Update(change func(*Settings)) (Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	settings, err := s.read()
	if err != nil {
		return settings, err
	}
	change(&settings)
	settings.normalize()
	body, err := yaml.Marshal(settings)
	if err != nil {
		return settings, err
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o700); err != nil {
		return settings, err
	}
	temporary := s.Path + ".tmp"
	if err := os.WriteFile(temporary, append([]byte(header), body...), 0o600); err != nil {
		return settings, err
	}
	return settings, os.Rename(temporary, s.Path)
}
