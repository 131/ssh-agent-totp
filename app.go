package main

import (
	"encoding/json"

	sshagentcrypt "github.com/131/ssh-agent-crypt"

	"github.com/131/ssh-agent-totp/internal/i18n"
	"github.com/131/ssh-agent-totp/internal/tokens"
	"github.com/131/ssh-agent-totp/internal/totp"
	"github.com/131/ssh-agent-totp/internal/update"
	"github.com/131/ssh-agent-totp/settings"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

// App exposes the UI methods; Open shows a URL in the default browser.
type App struct {
	Tokens  *tokens.Store
	Open    func(string) error
	Agent   func() string
	Updater *update.Updater
	// Relaunch starts the updated executable and ends this instance.
	Relaunch func(path string) error
	// Reload renders the page again, in the current language.
	Reload   func()
	Settings *settings.Store
	// Copy puts a code on the clipboard, out of the Windows history.
	Copy func(string) error
	// Window moves, resizes, minimizes or closes the borderless window.
	Window func(action string) error
}

func NewApp(open func(string) error) *App {
	store := settings.DefaultStore()
	current, _ := store.Load()
	i18n.Set(current.Language)
	agent := sshagentcrypt.DefaultAgentAddress
	return &App{
		Tokens: &tokens.Store{Path: store.TokensPath(current), Key: current.Tokens.Key, Crypt: tokens.AgentCrypt(agent)},
		Open:   open, Agent: agent, Updater: update.New(version, store, update.Authenticode), Settings: store,
	}
}

func arg[T any](args []json.RawMessage, index int) (T, error) {
	var value T
	if index >= len(args) {
		return value, i18n.Errorf("err.argument")
	}
	err := json.Unmarshal(args[index], &value)
	return value, err
}

// Unlocked describes the token file once unsealed.
type Unlocked struct {
	Path  string        `json:"path"`
	Key   string        `json:"key"`
	Codes []tokens.Code `json:"codes"`
}

// Handle runs one UI method with JSON arguments.
func (a *App) Handle(method string, args []json.RawMessage) (any, error) {
	switch method {
	case "version":
		return version, nil
	case "copy":
		text, err := arg[string](args, 0)
		if err != nil {
			return nil, err
		}
		if a.Copy == nil {
			return nil, i18n.Errorf("err.copy")
		}
		return nil, a.Copy(text)
	case "window":
		action, err := arg[string](args, 0)
		if err != nil {
			return nil, err
		}
		if a.Window == nil {
			return nil, i18n.Errorf("err.window")
		}
		return nil, a.Window(action)
	case "unlock":
		// The agent must answer before any secret is unsealed: an unreachable
		// agent is the lock screen, not a list of failed tokens.
		if _, err := tokens.AgentKeys(a.Agent()); err != nil {
			return nil, err
		}
		if err := a.Tokens.Load(); err != nil {
			return nil, err
		}
		codes, err := a.Tokens.Codes()
		return Unlocked{Path: a.Tokens.Path, Key: a.Tokens.Key, Codes: codes}, err
	case "codes":
		return a.Tokens.Codes()
	case "import":
		text, err := arg[string](args, 0)
		if err != nil {
			return nil, err
		}
		return a.Tokens.Import(text)
	case "rename":
		id, err := arg[int](args, 0)
		if err != nil {
			return nil, err
		}
		issuer, err := arg[string](args, 1)
		if err != nil {
			return nil, err
		}
		account, err := arg[string](args, 2)
		if err != nil {
			return nil, err
		}
		pinned, err := arg[bool](args, 3)
		if err != nil {
			return nil, err
		}
		return nil, a.Tokens.Edit(id, issuer, account, &pinned)
	case "add":
		// issuer, account, secret, algorithm, digits, period
		var token totp.Token
		var err error
		for index, target := range []*string{&token.Issuer, &token.Account, &token.Secret, &token.Algorithm} {
			if *target, err = arg[string](args, index); err != nil {
				return nil, err
			}
		}
		if token.Digits, err = arg[int](args, 4); err != nil {
			return nil, err
		}
		if token.Period, err = arg[int](args, 5); err != nil {
			return nil, err
		}
		return nil, a.Tokens.Add(token)
	case "rename_folder":
		from, err := arg[string](args, 0)
		if err != nil {
			return nil, err
		}
		to, err := arg[string](args, 1)
		if err != nil {
			return nil, err
		}
		return a.Tokens.RenameFolder(from, to)
	case "delete":
		id, err := arg[int](args, 0)
		if err != nil {
			return nil, err
		}
		return nil, a.Tokens.Delete(id)
	case "export":
		return a.Tokens.Export()
	case "agent_keys":
		return tokens.AgentKeys(a.Agent())
	case "set_key":
		// The key seals new secrets; existing ones are sealed again with it.
		key, err := arg[string](args, 0)
		if err != nil {
			return nil, err
		}
		count, err := a.Tokens.Reseal(key)
		if err != nil {
			return nil, err
		}
		if _, err := a.Settings.Update(func(s *settings.Settings) { s.Tokens.Key = key }); err != nil {
			return nil, err
		}
		return count, nil
	case "set_language":
		language, err := arg[string](args, 0)
		if err != nil {
			return nil, err
		}
		if !i18n.Supported(language) {
			return nil, i18n.Errorf("err.language", language)
		}
		if _, err := a.Settings.Update(func(s *settings.Settings) { s.Language = language }); err != nil {
			return nil, err
		}
		i18n.Set(language)
		if a.Reload != nil {
			a.Reload()
		}
		return nil, nil
	case "update_check":
		// update_check(true) is the manual check: manifest.json is fetched again.
		// A failed check still describes the settings, with its error.
		check := a.Updater.Check
		if refresh, _ := arg[bool](args, 0); refresh {
			check = a.Updater.CheckNow
		}
		release, err := check()
		if err != nil {
			release.Error = err.Error()
		}
		return release, nil
	case "update_enable":
		enable, err := arg[bool](args, 0)
		if err != nil {
			return nil, err
		}
		release, err := a.Updater.SetEnabled(enable)
		if err != nil {
			release.Error = err.Error()
		}
		return release, nil
	case "update_channel":
		channel, err := arg[string](args, 0)
		if err != nil {
			return nil, err
		}
		return a.Updater.SetChannel(channel)
	case "update_install":
		tag, err := arg[string](args, 0)
		if err != nil {
			return nil, err
		}
		path, err := a.Updater.Install(tag)
		if err != nil {
			return nil, err
		}
		if a.Relaunch == nil {
			return nil, i18n.Errorf("err.restart")
		}
		return nil, a.Relaunch(path)
	case "update_open":
		// Development builds cannot update: open the release page of the channel.
		release, err := a.Updater.Check()
		if err != nil {
			return nil, err
		}
		if release.URL == "" {
			return nil, i18n.Errorf("err.update.none")
		}
		return nil, a.Open(release.URL)
	}
	return nil, i18n.Errorf("err.method", method)
}
