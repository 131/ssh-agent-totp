package main

import (
	"encoding/json"
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
	data := ""
	if dir, err := os.UserCacheDir(); err == nil {
		data = filepath.Join(dir, "ssh-agent-totp", "webview")
	}
	w := webview.NewWithOptions(webview.WebViewOptions{
		Debug:     os.Getenv("SSH_AGENT_TOTP_DEBUG") != "",
		AutoFocus: true,
		DataPath:  data,
		WindowOptions: webview.WindowOptions{
			Title: tray.WindowTitle, Width: tray.Scale(360), Height: tray.Scale(560), Center: true, IconId: tray.IconID,
		},
	})
	if w == nil {
		os.Exit(1)
	}
	defer w.Destroy()
	w.SetSize(int(tray.Scale(280)), int(tray.Scale(300)), webview.HintMin)
	hwnd := uintptr(w.Window())
	var ticks clock.Clock
	icon := tray.Install(hwnd, &ticks)
	icon.Borderless()
	// Wake WebView2's own binding completions after a native modal loop.
	icon.AfterModal = func() { w.Dispatch(func() {}) }
	app.Copy = func(text string) error {
		if err := clipboard.Copy(hwnd, text); err != nil {
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
				message, _ := json.Marshal(err.Error())
				icon.Dispatch(func() { icon.Show(); w.Eval("window.notice(" + string(message) + ", true)") })
			}
		}()
	}
	app.Window = func(action string) error {
		done := make(chan bool)
		icon.Dispatch(func() { done <- icon.Frame(action) })
		if !<-done {
			return i18n.Errorf("err.window")
		}
		return nil
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
	// Agent/file calls run outside the UI thread; HWND messages return their results
	// even while a native popup or move/resize loop pumps window messages.
	w.Bind("totp_call", func(id int, method string, args []json.RawMessage) {
		go func() {
			result, err := app.Handle(method, args)
			message := ""
			if err != nil {
				message = err.Error()
			}
			payload, _ := json.Marshal(map[string]any{"id": id, "error": message, "result": result})
			icon.Dispatch(func() { w.Eval("window.totpDone(" + string(payload) + ")") })
		}()
	})
	// The GUI subscribes to the same 1 Hz tick as the native icons.
	ticks.Subscribe(func(tick clock.Tick) {
		payload, _ := json.Marshal([]any{tick.Modulus, tick.Percent, tick.Now})
		w.Eval("window.fireEvent?.('tick', ..." + string(payload) + ")")
	})
	app.Reload = func() { icon.Dispatch(func() { w.SetHtml(ui.Page(i18n.Current())) }) }
	w.SetHtml(ui.Page(i18n.Current()))
	w.Run()
}
