# Development

Read the [guideline](dev_guideline.md): boring is good.

## Build and tests

```sh
gofmt -l .
go vet ./... && go test ./... && GOOS=windows go vet ./...
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath \
  -ldflags "-H windowsgui -s -w -X main.version=<version>" -o ssh-agent-totp.exe .
```

On Windows, also test the patched nested module and browser lifecycle:

```powershell
Push-Location third_party/go-webview2
go vet -unsafeptr=false ./...; go test ./...
Pop-Location
./scripts/test-webview-lifecycle.ps1 -Executable ./ssh-agent-totp.exe
```

The integration script uses temporary APPDATA, an unavailable agent and
updates disabled. Use an isolated-mutex test build when another instance is
running; never target the installed executable. Unit tests use fake crypt,
not a real agent. For manual checks use a throwaway APPDATA; a clear token
file is sealed on load. CI runs the nested tests and lifecycle check on Windows.

## Native runtime and GUI session

[TRAY.md](TRAY.md#application-contract) defines the resident-tray/dynamic-GUI
contract and [agent/pin behavior](TRAY.md#agent-and-pins).
`main_windows.go` composes one `App` and its stores, an invisible native host,
tray and clock. `internal/tray` owns the persistent message loop and attached
GUI frame. Each Open creates or restores one WebView on the same UI thread;
Close detaches/destroys only that GUI and releases its browser references.

Agent/file work runs off-thread; results return through HWND dispatch so
native modal loops still deliver them. WM_APP and modal-loop completion drain
the current WebView's private binding queue. Replies/window actions retain
the originating view identity, and the one-time tick bridge skips closed
views. No UI thread waits for browser-process exit. DLLs/COM may remain loaded.

`third_party/go-webview2/` is the pinned local lifecycle patch; provenance,
license, COM ownership, Close/dispatch API and scoped vet setting are in
[PATCHES.md](../third_party/go-webview2/PATCHES.md). The borderless WebView2
window uses no cgo. `SSH_AGENT_TOTP_DEBUG=1` enables developer tools; runtime
data lives in the user cache directory under `ssh-agent-totp/webview`.

## Layout

| Path | Responsibility |
| --- | --- |
| `app.go`, `main_windows.go` | Services, API and runtime/GUI composition |
| `ui/` | Embedded HTML, CSS and script, inlined into one page |
| `internal/{tokens,totp}` | Sealed store, RFC 6238, import/export |
| `internal/{tray,clock,clipboard}` | Native host/menu/frame, shared tick, private clipboard |
| `internal/{update,startup}` | Verified updates and startup shortcut |
| `internal/i18n`, `settings/` | Catalog and preferences |
| `winres/`, `third_party/`, `scripts/` | Resources, patched wrapper, Windows integration check |

## Translations

`internal/i18n/messages.json` contains `fr-fr` (default) and `en-us`.
Go uses `i18n.T`/`Errorf`; the page receives `window.I18N` and `{{key}}`
substitutions. Tests check matching catalogs and all referenced keys.

## Icon and manifest

`winres/icon.py` defines the stopwatch/shackle, 30 tray frames, white/gray/blue
menu rings and copied checkmark. The manifest/icons are linked through
`rsrc_windows_amd64.syso`. Regenerate after editing them:

```sh
(cd winres && python3 icon.py)
go run github.com/tc-hib/go-winres@v0.3.3 make --arch amd64
```

Shared resources provide tray animation without runtime drawing; menu rows
use the standard bitmap column, not owner-draw. Pins share one frame bitmap,
and replaced bitmaps are freed. Motion/timing are defined in
[TRAY.md](TRAY.md#one-application-tick).

## Shared clock and GUI dispatch

One native timer serves the tray, menu and optional page. No page interval
or subscriber-specific clock is installed; native popup and move/resize
loops must continue processing replies and binding completions.

## Header controls

Logo/brand then settings wrench are left; Add/Edit precede window controls on
the right. Pin/unpin uses one glyph and changes only its pressed style.
