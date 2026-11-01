# Development

Read the [guideline](dev_guideline.md) first: boring is good.

## Build and tests

```sh
gofmt -l .
go vet ./... && go test ./... && GOOS=windows go vet ./...
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath \
  -ldflags "-H windowsgui -s -w -X main.version=<version>" -o ssh-agent-totp.exe .
```

The tray owns an invisible native top-level window for dispatch, clipboard,
timers and single-instance activation. Startup does not create a browser.
Open/double-click creates the GUI on the same UI thread. Closing releases
its WebView2 controller and owned COM references, then destroys only the GUI
HWND; minimizing hides it. The native tray message loop runs throughout,
including while the browser's processes exit and before the next Open.
It drains the current WebView's binding completions on WM_APP and after
native modal loops. Results targeting a closed view are discarded rather
than delivered to a replacement window. The tick bridge subscribes once.
The dependency still initializes COM at package initialization, and DLLs
can remain loaded. No IPC, process killing or DLL unloading is involved.
Pinned tokens are loaded off-thread at startup, independently of the page.
An unavailable agent leaves the shortcuts locked; opening the GUI can retry.

`third_party/go-webview2/` is a local lifecycle patch of the pinned dependency;
its origin, license, COM ownership and added API are in
[PATCHES.md](../third_party/go-webview2/PATCHES.md). Tests for that nested module
run separately on Windows. An isolated Windows integration check verifies
startup, process exit after Close, repeated creation, second-instance
activation and rapid close/reopen:

```powershell
./scripts/test-webview-lifecycle.ps1 -Executable ./ssh-agent-totp.exe
```

Use a test build with an isolated mutex if another instance is running.
The script uses a temporary APPDATA, an unavailable agent and disabled
updates; it must not be run against the operator's installed executable.

The window uses the [WebView2 runtime](https://developer.microsoft.com/microsoft-edge/webview2/)
(shipped with Windows 10 and 11) through `github.com/jchv/go-webview2`, without
cgo. It is borderless: the page draws the title bar, Go moves and resizes the
window. Agent calls and code computation run in Go, off the UI thread.
`SSH_AGENT_TOTP_DEBUG=1` enables the developer tools. WebView2 data lives in the user cache directory (`ssh-agent-totp/webview`).

Tests never use a real agent: `internal/tokens` seals with a fake crypt. Try
the app against a throwaway `%APPDATA%` (`set APPDATA=…` before starting it)
with a clear `tokens.crypt`: it is sealed at the first start.

## Layout

| Path | |
|------|-|
| `app.go`, `main_windows.go` | UI methods and the window |
| `ui/` | page, style and script, embedded and inlined into one document |
| `internal/i18n/messages.json` | every user-facing string, French and English, shared by Go and `ui/` |
| `internal/totp` | RFC 6238 codes, import formats, `otpauth://` export |
| `internal/tokens` | `tokens.crypt` ([TOKENS.md](TOKENS.md)), sealed through the agent |
| `internal/clipboard` | copy kept out of the clipboard history |
| `internal/update` | channels, download, `SHA256SUMS` and Authenticode checks |
| `internal/tray` | notification area icon, single instance, borderless frame |
| `settings/` | `settings.yml` ([SETTINGS.md](SETTINGS.md)) |
| `winres/` | icon and Windows manifest |
| `third_party/go-webview2/` | pinned WebView2 wrapper with explicit browser teardown |
| `scripts/` | isolated Windows lifecycle integration check |
| `specs/` | these documents |

## Translations

`internal/i18n/messages.json` holds `fr-fr` (default) and `en-us`. Go uses
`i18n.T` / `i18n.Errorf`; the page gets the catalog as `window.I18N` and
`{{key}}` placeholders in `ui/index.html`. Tests check that both catalogs have
the same keys and that every key used exists.

## Icon and manifest

The icon (`winres/icon.py`: a stopwatch whose crown is a padlock shackle), its
30 small tray frames (`winres/frames/`), white/gray/blue menu ring variants, the
copied checkmark, and
the Windows manifest are linked through `rsrc_windows_amd64.syso`. After
changing them, regenerate it:

```sh
(cd winres && python3 icon.py)
go run github.com/tc-hib/go-winres@v0.3.3 make --arch amd64
```

The tray advances the existing dial once per second on a Win32 window timer.
`spin` plus its timer setup/dispatch is under 25 lines: shared icon resources,
no runtime drawing or animation framework. The 30-second cycle is a visual
clock, not an individual token's period (tokens may have different periods).

Menu items use the standard Windows bitmap column, not owner-drawn rows.
Their ring resources share the tray's animation phase; one
bitmap serves all pinned entries, and replaced bitmaps are released.

## Shared clock and GUI dispatch

The composition root creates one `internal/clock.Clock`. Native icon/menu
subscribers and the GUI bridge register once at startup; a single 1 Hz
Win32 timer emits the tick. The bridge skips closed windows. The GUI listens
to the global `tick` event rather than installing an interval of its own. Timing and direction are defined in
[TRAY.md](TRAY.md#one-application-tick).

Agent/file work stays off the GUI thread. Replies use a window-targeted
dispatch message, so native popup and move/resize message loops still
execute them. After a modal loop, the current WebView's binding completion
queue is drained as well. A thread-only wakeup consumed by the native menu must not
leave the list's refresh promise pending forever.

## Header controls

The brand/logo then the settings wrench form the left group. Add and Edit
are on the right, before the window controls. A token's pin tool keeps the
same icon in both states; only its pressed style changes.
