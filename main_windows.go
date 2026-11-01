package main

import (
	"encoding/json"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"syscall"

	"github.com/131/ssh-agent-totp/internal/clipboard"
	"github.com/131/ssh-agent-totp/internal/clock"
	"github.com/131/ssh-agent-totp/internal/i18n"
	"github.com/131/ssh-agent-totp/internal/tray"
	"github.com/131/ssh-agent-totp/internal/update"
	"github.com/131/ssh-agent-totp/ui"
	webview "github.com/jchv/go-webview2"
)

func openBrowser(link string) error {
	command := exec.Command("rundll32", "url.dll,FileProtocolHandler", link)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return command.Start()
}

// updatedFlag marks the first start after an update, while the previous
// instance is still closing.
const updatedFlag = "--updated"

func main() {
	if tray.ActivateExisting(slices.Contains(os.Args[1:], updatedFlag)) {
		return
	}

	update.Cleanup()
	app := NewApp(openBrowser)
	var ticks clock.Clock
	icon, err := tray.New(&ticks)
	if err != nil {
		log.Fatal(err)
	}
	var w webview.WebView
	changing := false

	app.Copy = func(text string) error {
		if err := clipboard.Copy(icon.Window(), text); err != nil {
			return err
		}

		icon.Dispatch(icon.Copied)
		return nil
	}

	icon.Pinned = app.Tokens.Pinned
	icon.CopyPinned = func(id int) {
		go func() {
			code, err := app.Tokens.PinnedCode(id)
			if err == nil {
				err = app.Copy(code)
			}
			if err != nil {
				icon.Dispatch(func() {
					if w == nil {
						icon.Error(err)
					} else {
						icon.Show()
						message, _ := json.Marshal(err.Error())
						w.Eval("window.notice(" + string(message) + ", true)")
					}
				})
			}
		}()
	}

	app.Relaunch = func(path string) error {
		command := exec.Command(path, updatedFlag)
		if err := command.Start(); err != nil {
			return err
		}

		command.Process.Release()
		icon.Dispatch(icon.Quit)
		return nil
	}
	// Pinned shortcuts must not depend on the HTML unlock screen being opened.
	// An unavailable agent leaves the menu locked until the GUI can retry.
	go func() { _, _ = app.Handle("unlock", nil) }()

	icon.Close = func() {
		if w == nil || changing {
			return
		}
		changing = true
		defer func() { changing = false }()
		old := w
		w = nil
		icon.Detach()
		if err := old.Close(); err != nil {
			w = old
			icon.Attach(uintptr(old.Window()))
			icon.Error(err)
		}
	}
	icon.ProcessThread = func() {
		if w != nil {
			w.ProcessDispatch()
		}
	}
	icon.AfterModal = icon.ProcessThread
	// Subscribe once: recreating a browser must not add another tick listener.
	ticks.Subscribe(func(tick clock.Tick) {
		if w != nil {
			payload, _ := json.Marshal([]any{tick.Modulus, tick.Percent, tick.Now})
			w.Eval("window.fireEvent?.('tick', ..." + string(payload) + ")")
		}
	})
	app.Reload = func() {
		icon.Dispatch(func() {
			if w != nil {
				w.SetHtml(ui.Page(i18n.Current()))
			}
		})
	}

	icon.Open = func() {
		if w != nil || changing {
			return
		}
		changing = true
		defer func() { changing = false }()
		data := ""
		if dir, err := os.UserCacheDir(); err == nil {
			data = filepath.Join(dir, "ssh-agent-totp", "webview")
		}
		w = webview.NewWithOptions(webview.WebViewOptions{
			Debug:     os.Getenv("SSH_AGENT_TOTP_DEBUG") != "",
			AutoFocus: true,
			DataPath:  data,
			WindowOptions: webview.WindowOptions{
				Title:  tray.WindowTitle,
				Width:  tray.Scale(360),
				Height: tray.Scale(560),
				Center: true,
				IconId: tray.IconID,
			},
		})
		if w == nil {
			icon.Error(i18n.Errorf("err.window"))
			return
		}

		w.SetSize(int(tray.Scale(280)), int(tray.Scale(300)), webview.HintMin)
		icon.Attach(uintptr(w.Window()))
		view := w
		viewApp := *app
		viewApp.Window = func(action string) error {
			done := make(chan bool)
			icon.Dispatch(func() { done <- (w == view && icon.Frame(action)) })
			if !<-done {
				return i18n.Errorf("err.window")
			}

			return nil
		}

		// Agent/file calls run outside the UI thread; HWND messages return their results
		// even while a native popup or move/resize loop pumps window messages.
		w.Bind("totp_call", func(id int, method string, args []json.RawMessage) {
			go func() {
				result, err := viewApp.Handle(method, args)
				message := ""
				if err != nil {
					message = err.Error()
				}
				payload, _ := json.Marshal(map[string]any{"id": id, "error": message, "result": result})
				icon.Dispatch(func() {
					if w == view {
						view.Eval("window.totpDone(" + string(payload) + ")")
					}
				})
			}()
		})

		w.SetHtml(ui.Page(i18n.Current()))
	}
	icon.Run()
	icon.Close()
}
