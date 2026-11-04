# Releases and auto-update

## Publishing a release

From a clean `master`, [ppackage](https://github.com/131/ppackage) aligns root
version markers and creates the version commit/lightweight tag:

```sh
ppackage version patch   # or minor, major, X.Y.Z, X.Y.Z-beta.N
git push origin master vX.Y.Z
```

Push the exact tag: `--follow-tags` does not send lightweight tags. Never
create release tags or GitHub releases by hand. `-beta.N` tags are prereleases.
The `release` workflow runs tests, then [GoReleaser](../.goreleaser.yaml) builds
`ssh-agent-totp-windows-amd64.exe`, signs via `SIGNING_SERVER` and publishes it
with `SHA256SUMS`; `wails3 updater manifest` then attaches its
[Wails update manifest](https://github.com/wailsapp/wails/blob/v3.1.0/docs/mpress/content/reference/update-manifest.md)
`update.json` (SHA-512 digest, absolute asset URL). The GitHub `production`
environment protects signing credentials; it is not a release channel.

## Channels

A channel is a static manifest committed upstream, `updates/<channel>.json`:
[production](../updates/production.json) and [beta](../updates/beta.json),
the list of the settings selector. Once CI, assets and signature are verified,
promote a release by committing its manifest, then push:

```sh
curl -fsSL https://github.com/131/ssh-agent-totp/releases/download/vX.Y.Z/update.json -o updates/production.json
```

The app reads
`https://raw.githubusercontent.com/<owner>/<repo>/HEAD/updates/<channel>.json`
through the Wails updater `endpoint` provider. A missing manifest means
nothing to install.

Versions up to v0.2.0 still follow the root [`manifest.json`](../manifest.json)
(channel → tag) and `SHA256SUMS`: keep promoting there too until no such
installation remains.

## Update

Each GUI document requests a check; *Check for updates* runs it again.
Tray-only startup does not trigger the page-driven check. A newer followed
version produces an install notification in the GUI. The
[Wails updater](https://github.com/wailsapp/wails/blob/v3.1.0/docs/mpress/content/guides/updater.md) downloads
the executable beside the running one; it is accepted only when:

- the manifest carries a digest and the download matches it;
- Windows trusts the Authenticode chain, including revocation;
- the signer matches the running executable's.

On restart, a Wails helper process waits for the application to exit, swaps
the executable and starts it again. Its directory must be writable. Downgrades
are not offered; switching back from beta waits for a newer production release.
Unsigned/development builds cannot self-update. `update.enable: false`
disables every GitHub request.

On Windows, `SSH_AGENT_TOTP_VERIFY_SIGNED=<signed exe> go test ./internal/update`
checks the actual signature-verification path.

## Installation feedback

The install action stays disabled with a spinner through download,
verification and restart. Failure restores it for retry. Toasts belong to the
currently open modal dialog, so their first click is actionable.
