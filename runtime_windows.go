package main

import (
	"os/exec"

	"github.com/131/ssh-agent-totp/internal/clipboard"
	"github.com/131/ssh-agent-totp/internal/clock"
	"github.com/131/ssh-agent-totp/internal/desktop"
	"github.com/131/ssh-agent-totp/internal/tray"
)

// Runtime owns the resident services, host and clock. Its only optional
// resource is a disposable GUI session; closing that session does not quit.
type Runtime struct {
	app      *App
	host     *desktop.Host
	icon     *tray.Tray
	clock    clock.Clock
	window   *WindowSession
	changing bool
}

func NewRuntime(app *App) (*Runtime, error) {
	r := &Runtime{app: app}
	host, err := desktop.New(&r.clock, tray.WindowTitle)
	if err != nil {
		return nil, err
	}
	r.host = host
	r.icon = tray.Install(host.Window(), &r.clock)
	r.icon.Open, r.icon.Quit = r.OpenWindow, host.Quit
	r.icon.AfterModal = r.Drain
	r.icon.Pinned = app.Tokens.Pinned
	r.icon.CopyPinned = r.CopyPinned
	host.Open, host.ProcessThread = r.OpenWindow, r.Drain
	host.Message, host.OnDestroy = r.icon.HandleMessage, r.icon.Remove
	r.clock.Subscribe(func(tick clock.Tick) {
		if r.window != nil {
			r.window.Tick(tick)
		}
	})
	app.Copy = func(text string) error {
		if err := clipboard.Copy(host.Window(), text); err != nil {
			return err
		}
		host.Dispatch(r.icon.Copied)
		return nil
	}
	app.Reload = func() {
		host.Dispatch(func() {
			if r.window != nil {
				r.window.Reload()
			}
		})
	}
	app.Relaunch = func(path string) error {
		command := exec.Command(path, updatedFlag)
		if err := command.Start(); err != nil {
			return err
		}
		command.Process.Release()
		host.Dispatch(host.Quit)
		return nil
	}
	return r, nil
}

func (r *Runtime) Run() {
	defer r.icon.Remove()
	// This opportunistic load and GUI Unlock/Retry retain the existing agent policy.
	go func() { _, _ = r.app.Handle("unlock", nil) }()
	r.host.Run()
	r.CloseWindow()
}

func (r *Runtime) OpenWindow() {
	if r.changing || r.host.Closed() {
		return
	}
	if r.window != nil {
		if err := r.window.Show(); err != nil {
			r.host.Error(err)
		}
		return
	}
	r.changing = true
	defer func() { r.changing = false }()
	window, err := NewWindowSession(r.app, r.host, r.CloseWindow, r.Drain)
	if err != nil {
		if !r.host.Closed() {
			r.host.Error(err)
		}
		return
	}
	if r.host.Closed() {
		window.Close()
		return
	}
	r.window = window
	if err := window.Show(); err != nil {
		r.host.Error(err)
	}
}

func (r *Runtime) CloseWindow() {
	if r.window == nil || r.changing {
		return
	}
	r.changing = true
	defer func() { r.changing = false }()
	window := r.window
	r.window = nil
	if err := window.Close(); err != nil {
		r.window = window
		r.host.Error(err)
	}
}

func (r *Runtime) Drain() {
	if r.window != nil {
		r.window.Drain()
	}
}

func (r *Runtime) CopyPinned(id int) {
	go func() {
		code, err := r.app.Tokens.PinnedCode(id)
		if err == nil {
			err = r.app.Copy(code)
		}
		if err != nil {
			r.host.Dispatch(func() {
				if r.window == nil {
					r.host.Error(err)
				} else {
					r.window.Show()
					r.window.Notice(err)
				}
			})
		}
	}()
}
