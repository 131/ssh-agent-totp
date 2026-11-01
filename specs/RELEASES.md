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
with `SHA256SUMS`. The GitHub `production` environment protects signing
credentials; it is not a release channel. After CI, assets and signature are
verified, promote the tag in [`manifest.json`](../manifest.json) and push.

## Channels

The root manifest maps channel names to release tags. The settings selector
lists its keys. The app reads
`https://raw.githubusercontent.com/<owner>/<repo>/HEAD/manifest.json` and
uses the [settings cache](SETTINGS.md#channels).

## Update

Each GUI document requests an automatic check; *Check for updates* explicitly
refreshes it. Tray-only startup does not trigger the page-driven check.
Settings/cached results belong to the resident service and outlive the page.
A newer followed version produces an install notification in the GUI.
The executable is downloaded beside the running one and accepted only when:

- SHA-256 matches the release `SHA256SUMS`;
- Windows trusts the Authenticode chain, including revocation;
- the signer matches the running executable's.

Installation replaces/restarts the executable; cleanup removes the previous
binary on the next start. Its directory must be writable. Downgrades are
not offered; switching back from beta waits for a newer production release.
Unsigned/development builds cannot self-update. `update.enable: false`
disables every GitHub request.

On Windows, `SSH_AGENT_TOTP_VERIFY_SIGNED=<signed exe> go test ./internal/update`
checks the actual signature-verification path.

## Installation feedback

The install action stays disabled with a spinner through download,
verification and restart. Failure restores it for retry. Toasts belong to the
currently open modal dialog, so their first click is actionable.
