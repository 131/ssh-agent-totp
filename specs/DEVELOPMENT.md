# Development

Read the [guideline](dev_guideline.md) first: boring is good.

## Build and tests

```sh
gofmt -l .
go vet ./... && go test ./... && GOOS=windows go vet ./...
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath \
  -ldflags "-H windowsgui -s -w -X main.version=<version>" -o ssh-agent-totp.exe .
```

## Native runtime and GUI session

The defining behavior is the [application contract](TRAY.md#application-contract):
the tray is permanent and the WebView is dynamic. The implementation has two
resource lifetimes, even though both run on one UI thread in one process.

| Owner/lifetime | Resources and responsibilities |
| --- | --- |
| Resident application | `App`, token/settings/startup/update services, single-instance mutex, native host HWND, clipboard ownership, tray icon/menu, clock and message loop |
| Optional GUI session | WebView controller and owned COM references, GUI HWND/frame, HTML/JS document, bindings and pending UI completions |

`main_windows.go` composes both. `internal/tray` provides the persistent host
and native loop, notification-area behavior and attached GUI frame.
`App` and its stores are constructed once; startup loads pinned tokens
outside the UI thread, independently of page unlock.

The host's native loop runs for the entire application lifetime. Open creates
a WebView on that same thread and attaches its frame to the host; Close
releases the browser and detaches/destroys the GUI without ending the loop.
Minimize only hides it. Single-instance activation targets the permanent
host, including when no GUI exists.

The native loop drains the current WebView's private binding completions
on WM_APP and after native modal loops. Replies and window actions retain
the originating view identity and are ignored if that session has closed;
they cannot address a new document after another Open. Backend operations
and persisted application state are not owned by that document. The tick
bridge registers once and evaluates JavaScript only while a session exists.

Browser processes exit asynchronously after controller Close. The host keeps
pumping messages during teardown and any subsequent Open; it does not wait
for process exit with a blocked UI thread. Tests observe eventual exit of the
closed browser tree, not an instantaneous disappearance inside Close.
The dependency still initializes COM at package initialization, and DLLs
can remain loaded. Neither IPC, process killing nor DLL unloading is needed.

## WebView integration and lifecycle checks

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
| `app.go`, `main_windows.go` | resident services and runtime/GUI composition |
| `ui/` | page, style and script, embedded and inlined into one document |
| `internal/i18n/messages.json` | every user-facing string, French and English, shared by Go and `ui/` |
| `internal/totp` | RFC 6238 codes, import formats, `otpauth://` export |
| `internal/tokens` | `tokens.crypt` ([TOKENS.md](TOKENS.md)), sealed through the agent |
| `internal/clipboard` | copy kept out of the clipboard history |
| `internal/update` | channels, download, `SHA256SUMS` and Authenticode checks |
| `internal/tray` | resident native host/loop, notification icon, activation and attached GUI frame |
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
