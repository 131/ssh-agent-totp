# Development

Read the [guideline](dev_guideline.md) first: boring is good.

## Build and tests

```sh
gofmt -l .
go vet ./... && go test ./... && GOOS=windows go vet ./...
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath \
  -ldflags "-H windowsgui -s -w -X main.version=<version>" -o ssh-agent-totp.exe .
```

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
| `specs/` | these documents |

## Translations

`internal/i18n/messages.json` holds `fr-fr` (default) and `en-us`. Go uses
`i18n.T` / `i18n.Errorf`; the page gets the catalog as `window.I18N` and
`{{key}}` placeholders in `ui/index.html`. Tests check that both catalogs have
the same keys and that every key used exists.

## Icon and manifest

The icon (`winres/icon.py`: a stopwatch whose crown is a padlock shackle) and
the Windows manifest are linked through `rsrc_windows_amd64.syso`. After
changing them, regenerate it:

```sh
(cd winres && python3 icon.py)
go run github.com/tc-hib/go-winres@v0.3.3 make --arch amd64
```
