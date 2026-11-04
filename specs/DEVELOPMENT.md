# Development

Read the [guideline](dev_guideline.md): boring is good.

## Build and tests

```sh
gofmt -l .
go vet ./... && go test ./... && GOOS=windows go vet ./...
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath \
  -ldflags "-H windowsgui -s -w -X main.version=<version>" -o ssh-agent-totp.exe .
```

Linux builds need the Wails GTK4/WebKitGTK headers
(`libgtk-4-dev libwebkitgtk-6.0-dev` on Debian/Ubuntu). On Windows, also test
the browser lifecycle:

```powershell
./scripts/test-webview-lifecycle.ps1 -Executable ./ssh-agent-totp.exe
```

The integration script uses temporary APPDATA, an unavailable agent and
updates disabled. Stop any other instance of the same build first (Wails'
single-instance lock is per application); never target the installed executable. Unit tests use fake crypt,
not a real agent. For manual checks use a throwaway APPDATA; a clear token
file is sealed on load. CI ensures the WebView2 runtime, verifies its installer signature if needed,
then runs the lifecycle check on Windows.

## Native runtime and GUI session

[TRAY.md](TRAY.md#application-contract) defines the resident-tray/dynamic-GUI
contract and [agent/pin behavior](TRAY.md#agent-and-pins).
`main.go` bootstraps `Runtime` (`runtime.go`), which owns one `App`, the
[Wails v3](https://v3.wails.io) application, tray, clock and optional
`WindowSession`. Wails provides the message loop, single-instance lock,
frameless window, WebView2/WebKit, tray and dialogs; `internal/tray` only
builds the icon frames and pinned menu. `WindowSession` (`window_session.go`)
owns one hidden-then-shown Wails window, its bridge and closed-state guard.
Close destroys that window and releases its browser; the next Open creates a
fresh one. Wails' `ApplicationShouldTerminateAfterLastWindowClosed`/
`DisableQuitOnLastWindowClosed` keep the tray resident.

The page is inline HTML (no asset server files, no generated bindings).
`app.js` posts `{id, method, args}` with `_wails.invoke` once Wails' runtime
config is ready; `RawMessageHandler` hands it to the session, which runs
`App.Handle` off-thread and replies through `ExecJS`. Replies are dropped after
Close. Window actions (drag, resize edges, minimize, close) map to Wails'
frameless messages. Agent/file work never runs on the UI thread.

`SSH_AGENT_TOTP_DEBUG=1` enables developer tools; WebView2 data lives in the
user cache directory under `ssh-agent-totp/webview`. Known Wails v3.1
deviation: Quit while the WebView2 controller is still initializing exits
with code 1 (the lifecycle script reports it as a warning).

## Layout

| Path | Responsibility |
| --- | --- |
| `app.go`, `main.go` | Services/API and bootstrap |
| `runtime.go`, `window_session.go` | Resident ownership and disposable GUI/bridge (Wails) |
| `ui/` | Embedded HTML, CSS and script, inlined into one page |
| `internal/{tokens,totp}` | Sealed store, RFC 6238, import/export |
| `internal/tray` | Tray icon frames and pinned menu |
| `internal/{clock,clipboard}` | Shared tick and private clipboard |
| `internal/{update,startup}` | Wails updater (channels, Authenticode) and startup shortcut |
| `internal/i18n`, `settings/` | Catalog and preferences |
| `winres/`, `scripts/` | Resources and Windows integration checks |

## Translations

`internal/i18n/messages.json` contains `fr-fr` (default) and `en-us`.
Go uses `i18n.T`/`Errorf`; the page receives `window.I18N` and `{{key}}`
substitutions. Tests check matching catalogs and all referenced keys.

## Icon and manifest

`winres/icon.py` defines the stopwatch/shackle, 30 tray frames, white/gray/blue
menu rings and copied checkmark as PNG under `internal/tray/frames`. The
manifest and application icon are linked through `rsrc_windows_amd64.syso`. Regenerate after editing them:

```sh
(cd winres && python3 icon.py)
go run github.com/tc-hib/go-winres@v0.3.3 make --arch amd64
```

Shared resources provide tray animation without runtime drawing; menu rows
use the standard bitmap column, not owner-draw. Motion/timing are defined in
[TRAY.md](TRAY.md#one-application-tick).

## Shared clock and GUI dispatch

One wall-clock-aligned goroutine (`clock.Run`) posts each second to the UI
thread and serves the tray, menu and optional page. No page interval
or subscriber-specific clock is installed; native popup and move/resize
loops must continue processing replies and binding completions.

## Header controls

Logo/brand then settings wrench are left; Add/Edit precede window controls on
the right. Pin/unpin uses one glyph and changes only its pressed style.
