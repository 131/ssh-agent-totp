package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/131/ssh-agent-totp/internal/clipboard"
	"github.com/131/ssh-agent-totp/internal/clock"
	"github.com/131/ssh-agent-totp/internal/i18n"
	"github.com/131/ssh-agent-totp/internal/tray"
	"github.com/131/ssh-agent-totp/winres"
)

// Runtime owns the resident services, tray and clock. Its only optional
// resource is a disposable GUI session; closing that session does not quit.
type Runtime struct {
	app   *App
	wails *application.App
	icon  *tray.Tray
	clock clock.Clock

	mu     sync.Mutex
	window *WindowSession
}

func NewRuntime(app *App) *Runtime {
	r := &Runtime{app: app}
	data := ""
	if dir, err := os.UserCacheDir(); err == nil {
		data = filepath.Join(dir, "ssh-agent-totp", "webview")
	}
	r.wails = application.New(application.Options{
		Name: tray.WindowTitle,
		Icon: winres.Icon,
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID:               "ssh-agent-totp",
			OnSecondInstanceLaunch: func(application.SecondInstanceData) { r.OpenWindow() },
		},
		RawMessageHandler: func(window application.Window, message string, _ *application.OriginInfo) {
			if session := r.session(); session != nil && session.view.ID() == window.ID() {
				session.Message(message)
			}
		},
		Windows: application.WindowsOptions{DisableQuitOnLastWindowClosed: true, WebviewUserDataPath: data},
		Linux:   application.LinuxOptions{DisableQuitOnLastWindowClosed: true},
		Mac: application.MacOptions{
			ActivationPolicy: application.ActivationPolicyAccessory,
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
	})
	r.icon = tray.Install(r.wails, &r.clock)
	r.icon.Open, r.icon.Quit = r.OpenWindow, r.wails.Quit
	r.icon.Pinned = app.Tokens.Pinned
	r.icon.CopyPinned = r.CopyPinned
	r.clock.Subscribe(func(tick clock.Tick) {
		if session := r.session(); session != nil {
			session.Tick(tick)
		}
	})
	app.Open = r.wails.Browser.OpenURL
	app.Copy = func(text string) error {
		err := application.InvokeSyncWithError(func() error { return clipboard.Copy(text) })
		if errors.Is(err, errors.ErrUnsupported) {
			err = nil
			if !r.wails.Clipboard.SetText(text) {
				err = i18n.Errorf("err.copy")
			}
		}
		if err != nil {
			return err
		}
		r.icon.Copied()
		return nil
	}
	app.Reload = func() {
		if session := r.session(); session != nil {
			session.Reload()
		}
	}
	app.Relaunch = func(path string) error {
		command := exec.Command(path, updatedFlag)
		if err := command.Start(); err != nil {
			return err
		}
		command.Process.Release()
		r.wails.Quit()
		return nil
	}
	return r
}

func (r *Runtime) Run() error {
	// This opportunistic load and GUI Unlock/Retry retain the existing agent policy.
	go func() { _, _ = r.app.Handle("unlock", nil) }()
	go r.clock.Run(application.InvokeAsync)
	return r.wails.Run()
}

func (r *Runtime) session() *WindowSession {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.window
}

// OpenWindow creates or restores the GUI on the UI thread, which serializes
// concurrent requests (tray, menu, second launch).
func (r *Runtime) OpenWindow() { application.InvokeAsync(r.openWindow) }

func (r *Runtime) openWindow() {
	r.mu.Lock()
	window := r.window
	r.mu.Unlock()
	if window == nil {
		window = NewWindowSession(r.app, r.wails, r.closed)
		r.mu.Lock()
		r.window = window
		r.mu.Unlock()
	}
	window.Show()
}

func (r *Runtime) closed(session *WindowSession) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.window == session {
		r.window = nil
	}
}

func (r *Runtime) Error(err error) {
	r.wails.Dialog.Error().SetTitle(tray.WindowTitle).SetMessage(err.Error()).Show()
}

func (r *Runtime) CopyPinned(id int) {
	go func() {
		code, err := r.app.Tokens.PinnedCode(id)
		if err == nil {
			err = r.app.Copy(code)
		}
		if err == nil {
			return
		}
		if session := r.session(); session != nil {
			session.Show()
			session.Notice(err)
		} else {
			r.Error(err)
		}
	}()
}
