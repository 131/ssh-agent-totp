package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"syscall"

	"github.com/131/ssh-agent-totp/internal/clipboard"
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
	icon := tray.Install(hwnd)
	icon.Borderless()
	app.Copy = func(text string) error { return clipboard.Copy(hwnd, text) }
	icon.Pinned = app.Tokens.Pinned
	icon.CopyPinned = func(id int) {
		go func() {
			code, err := app.Tokens.PinnedCode(id)
			if err == nil {
				err = app.Copy(code)
			}
			if err != nil {
				message, _ := json.Marshal(err.Error())
				w.Dispatch(func() { icon.Show(); w.Eval("notice(" + string(message) + ", true)") })
			}
		}()
	}
	app.Window = func(action string) error {
		done := make(chan bool)
		w.Dispatch(func() { done <- icon.Frame(action) })
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
		w.Dispatch(icon.Quit)
		return nil
	}
	// Agent and file calls run outside the UI thread; results come back through Dispatch.
	w.Bind("totp_call", func(id int, method string, args []json.RawMessage) {
		go func() {
			result, err := app.Handle(method, args)
			message := ""
			if err != nil {
				message = err.Error()
			}
			payload, _ := json.Marshal(map[string]any{"id": id, "error": message, "result": result})
			w.Dispatch(func() { w.Eval("window.totpDone(" + string(payload) + ")") })
		}()
	})
	app.Reload = func() { w.Dispatch(func() { w.SetHtml(ui.Page(i18n.Current())) }) }
	w.SetHtml(ui.Page(i18n.Current()))
	w.Run()
}
