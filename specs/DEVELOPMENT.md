# Development

Read the [guideline](dev_guideline.md): boring is good.

## Target platforms

**Windows, Linux and macOS (OS X) are the target platforms, through Wails.**
The [shared-platform guideline](dev_guideline.md#target-platforms) governs
changes. Windows is currently distributed; Linux/macOS distribution, native
verification, autostart and update support remain ongoing work. Preserve the
shared runtime/tray contract while completing those adapters.
CI vets, tests and builds on all three platforms; native lifecycle integration
is currently checked on Windows.

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
then runs the lifecycle check on Windows, including native minimize/hide and
reopening the same window/browser session.

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
frameless messages for drag/resize; minimize hides the session and Close
destroys it. Native minimize events also hide the session. Agent/file work
never runs on the UI thread.

`SSH_AGENT_TOTP_DEBUG=1` enables developer tools; WebView2 data lives in the
user cache directory under `ssh-agent-totp/webview`. Known Wails v3.1
deviation: Quit while the WebView2 controller is still initializing exits
with code 1 (the lifecycle script reports it as a warning).

## Tray contract verification

[TRAY.md](TRAY.md#contract-status) defines the shared platform contract.
`Tray.Install` publishes Open and Quit before startup. `tokens.Store.Changed`
notifies after successful load/commit with the store unlocked; the runtime
posts `Tray.Refresh` to the UI thread, also after language changes. Refresh
republishes through `SystemTray.SetMenu`, including the Linux D-Bus menu.
Primary activation opens/restores the GUI without a double-click timer; Wails
supplies the native context-menu path.

Verify on Windows, Linux and macOS, with disposable settings and tokens:

- Before unlock (including an unavailable agent), the first context-menu
  opening offers Open and Quit, without a preceding primary click.
- Successful unlock/import, pin/unpin, deletion and renaming update the
  published entries without a tray click; failed writes preserve the old menu.
- Primary click opens/restores the GUI immediately; repeated activation does
  not create another session. Closing it retains the tray.
- Right-click displays the native context menu independently of primary-click
  dispatch. Linux needs no programmatic `OpenMenu()` call.
- A pinned entry copies its current code after changes and with the GUI closed;
  Open and Quit remain available alongside pins.

## Layout

| Path | Responsibility |
| --- | --- |
| `app.go`, `main.go` | Services/API and bootstrap |
| `runtime.go`, `window_session.go` | Resident ownership and disposable GUI/bridge (Wails) |
| `ui/` | Embedded HTML, CSS and script, inlined into one page |
| `internal/{tokens,totp}` | Sealed store, RFC 6238, import/export |
| `internal/tray` | Tray icon frames, pinned menu and immediate primary activation |
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

## Concurrency regression tests

`internal/tray/tray_test.go` verifies initial Open/Quit entries, pin replacement
and removal, translated labels and immediate activation without opening a
native menu. `internal/tokens/changes_test.go` verifies notification after
load/import/pin/rename/delete and lazy reload, with the store readable by its
observer; failed writes/unlocks leave committed pins unchanged. `internal/update/update_test.go` holds signature
verification and restart while a channel change waits, and checks that a
verification failure never restarts and leaves the transaction retryable.

The updater serializes checks, preference changes and the complete install
transaction (tag check, download, digest/signature verification, restart).
`App.Handle("update_install")` calls this transaction once; it does not perform
an independently interleavable restart.

## Windows startup cleanup

`Runtime.Run` starts `internal/update.CleanupLegacy` off-thread, after Wails
has handled helper mode and the single-instance lock. Matching, scope and the
bounded retry policy are defined in [RELEASES.md](RELEASES.md#windows-update-leftovers).
Tests use temporary executable directories and fake retry waits; the Windows
test also holds a real file handle that denies deletion, then releases it
before the retry. No test cleans the operator's installation.
