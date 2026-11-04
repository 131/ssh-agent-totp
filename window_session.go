package main

import (
	"encoding/json"
	"os"
	"sync/atomic"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/131/ssh-agent-totp/internal/clock"
	"github.com/131/ssh-agent-totp/internal/i18n"
	"github.com/131/ssh-agent-totp/internal/tray"
	"github.com/131/ssh-agent-totp/ui"
)

// WindowSession owns one GUI and its bridge. Callbacks retain this object,
// not the runtime's next window. Backend operations may finish after their
// session is closed; their replies are then dropped.
type WindowSession struct {
	app    App
	view   *application.WebviewWindow
	closed atomic.Bool
}

// edges maps the UI resize zones to the toolkit's resize messages.
var edges = map[string]string{
	"top": "n-resize", "top-right": "ne-resize", "right": "e-resize", "bottom-right": "se-resize",
	"bottom": "s-resize", "bottom-left": "sw-resize", "left": "w-resize", "top-left": "nw-resize",
}

func NewWindowSession(app *App, wails *application.App, closed func(*WindowSession)) *WindowSession {
	// UI-only callbacks belong to this session; the copied facade shares the
	// resident stores/services and never mutates their lifetime.
	s := &WindowSession{app: *app}
	s.app.Window = s.WindowAction
	s.view = wails.Window.NewWithOptions(application.WebviewWindowOptions{
		Title: tray.WindowTitle, Width: 360, Height: 560, MinWidth: 280, MinHeight: 300,
		Frameless: true, Hidden: true, InitialPosition: application.WindowCentered,
		DevToolsEnabled: os.Getenv("SSH_AGENT_TOTP_DEBUG") != "",
		HTML:            ui.Page(i18n.Current()),
	})
	s.view.OnWindowEvent(events.Common.WindowClosing, func(*application.WindowEvent) {
		s.closed.Store(true)
		closed(s)
	})
	return s
}

// Message runs one bridge call: {"id", "method", "args"} from app.js.
func (s *WindowSession) Message(message string) {
	var call struct {
		ID     int               `json:"id"`
		Method string            `json:"method"`
		Args   []json.RawMessage `json:"args"`
	}
	if err := json.Unmarshal([]byte(message), &call); err != nil {
		return
	}
	go func() {
		result, err := s.app.Handle(call.Method, call.Args)
		text := ""
		if err != nil {
			text = err.Error()
		}
		payload, _ := json.Marshal(map[string]any{"id": call.ID, "error": text, "result": result})
		s.eval("window.totpDone(" + string(payload) + ")")
	}()
}

func (s *WindowSession) eval(js string) {
	if !s.closed.Load() {
		s.view.ExecJS(js)
	}
}

func (s *WindowSession) WindowAction(action string) error {
	if s.closed.Load() {
		return i18n.Errorf("err.window")
	}
	switch action {
	case "drag":
		application.InvokeSync(func() { s.view.HandleMessage("wails:drag") })
	case "minimize":
		s.view.Minimise()
	case "close":
		s.view.Close()
	default:
		edge, ok := edges[action]
		if !ok {
			return i18n.Errorf("err.window")
		}
		application.InvokeSync(func() { s.view.HandleMessage("wails:resize:" + edge) })
	}
	return nil
}

func (s *WindowSession) Close() { s.view.Close() }

func (s *WindowSession) Show() {
	if !s.closed.Load() {
		s.view.Restore()
		s.view.Show()
		s.view.Focus()
	}
}

func (s *WindowSession) Reload() {
	if !s.closed.Load() {
		s.view.SetHTML(ui.Page(i18n.Current()))
	}
}

func (s *WindowSession) Tick(tick clock.Tick) {
	payload, _ := json.Marshal([]any{tick.Modulus, tick.Percent, tick.Now})
	s.eval("window.fireEvent?.('tick', ..." + string(payload) + ")")
}

func (s *WindowSession) Notice(err error) {
	message, _ := json.Marshal(err.Error())
	s.eval("window.notice(" + string(message) + ", true)")
}
