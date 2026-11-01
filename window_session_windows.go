package main

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/131/ssh-agent-totp/internal/clock"
	"github.com/131/ssh-agent-totp/internal/desktop"
	"github.com/131/ssh-agent-totp/internal/i18n"
	"github.com/131/ssh-agent-totp/internal/tray"
	"github.com/131/ssh-agent-totp/ui"
	webview "github.com/jchv/go-webview2"
)

// WindowSession owns one GUI, frame and bridge. Callbacks retain this object,
// not the runtime's next window, and dispatch checks its closed state on the
// UI thread. Backend operations may finish after their session is closed.
type WindowSession struct {
	host   *desktop.Host
	view   webview.WebView
	frame  *desktop.Frame
	closed bool
}

func NewWindowSession(app *App, host *desktop.Host, close, afterModal func()) (*WindowSession, error) {
	data := ""
	if dir, err := os.UserCacheDir(); err == nil {
		data = filepath.Join(dir, "ssh-agent-totp", "webview")
	}
	view := webview.NewWithOptions(webview.WebViewOptions{
		Debug: os.Getenv("SSH_AGENT_TOTP_DEBUG") != "", AutoFocus: true, DataPath: data,
		WindowOptions: webview.WindowOptions{Hidden: true, Title: tray.WindowTitle, Width: desktop.Scale(360), Height: desktop.Scale(560), Center: true, IconId: tray.IconID},
	})
	if view == nil {
		return nil, i18n.Errorf("err.window")
	}
	s := &WindowSession{host: host, view: view}
	view.SetSize(int(desktop.Scale(280)), int(desktop.Scale(300)), webview.HintMin)
	s.frame = desktop.NewFrame(uintptr(view.Window()), close, afterModal)
	// UI-only callbacks belong to this session; the copied facade shares the
	// resident stores/services and never mutates their lifetime.
	viewApp := *app
	viewApp.Window = s.WindowAction
	if err := view.Bind("totp_call", func(id int, method string, args []json.RawMessage) {
		go func() {
			result, err := viewApp.Handle(method, args)
			message := ""
			if err != nil {
				message = err.Error()
			}
			payload, _ := json.Marshal(map[string]any{"id": id, "error": message, "result": result})
			s.dispatch(func() { s.view.Eval("window.totpDone(" + string(payload) + ")") })
		}()
	}); err != nil {
		s.Close()
		return nil, err
	}
	s.Reload()
	return s, nil
}

func (s *WindowSession) dispatch(f func()) {
	s.host.Dispatch(func() {
		if !s.closed {
			f()
		}
	})
}

func (s *WindowSession) WindowAction(action string) error {
	done := make(chan bool)
	s.host.Dispatch(func() { done <- (!s.closed && s.frame.Action(action)) })
	if !<-done {
		return i18n.Errorf("err.window")
	}
	return nil
}

func (s *WindowSession) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true
	s.frame.Detach()
	if err := s.view.Close(); err != nil {
		s.closed = false
		s.frame.Attach()
		return err
	}
	return nil
}

func (s *WindowSession) Show() {
	if !s.closed {
		s.frame.Show()
	}
}
func (s *WindowSession) Drain() {
	if !s.closed {
		s.view.ProcessDispatch()
	}
}
func (s *WindowSession) Reload() {
	if !s.closed {
		s.view.SetHtml(ui.Page(i18n.Current()))
	}
}
func (s *WindowSession) Tick(tick clock.Tick) {
	if !s.closed {
		payload, _ := json.Marshal([]any{tick.Modulus, tick.Percent, tick.Now})
		s.view.Eval("window.fireEvent?.('tick', ..." + string(payload) + ")")
	}
}
func (s *WindowSession) Notice(err error) {
	if !s.closed {
		message, _ := json.Marshal(err.Error())
		s.view.Eval("window.notice(" + string(message) + ", true)")
	}
}
