# Settings

The preferences live in one YAML file, outside of any repository:

```
%APPDATA%\ssh-agent-totp\settings.yml
```

(`os.UserConfigDir()`, so `~/.config/ssh-agent-totp/settings.yml` elsewhere.)
A missing file means defaults; a missing key takes its default. The file is
written when you change a setting from the settings panel and when the channel
list is refreshed ([releases](RELEASES.md)). An unreadable file is reported and
never overwritten: fix it or delete it.

It holds preferences and caches only, never secrets: the tokens are in
[`tokens.crypt`](TOKENS.md), next to it.

## Example

```yaml
# ssh-agent-totp settings: https://github.com/131/ssh-agent-totp/blob/master/specs/SETTINGS.md
update:
  enable: true
  upstream: https://github.com/131/ssh-agent-totp
  channel: production
language: en-us
tokens:
  key: SHA256:Xn0kQ7cTt3uY0d2bLr8yWq5vH1sJm4pZfGa6eNiC9oE
remote_channels:
  upstream: https://github.com/131/ssh-agent-totp
  ttl: 86400
  fetched: 2026-10-26T12:00:00Z
  list:
    beta: v0.1.0
    production: v0.1.0
```

## Keys

| Key | Default | Configurable in UI | |
|-----|---------|:------------------:|-|
| `language` | `fr-fr` | yes | `fr-fr` or `en-us`, for the window, notifications, errors and tray menu; applied at once. Unknown values fall back to `fr-fr`. |
| `tokens.key` | first agent key | yes | SHA-256 fingerprint of the SSH key sealing `tokens.crypt` ([changing the key](TOKENS.md#changing-the-key)). |
| `tokens.path` | `tokens.crypt` next to this file | no | Token file, not written while it is the default. `%VARIABLE%` and `$VARIABLE` are expanded, e.g. `%USERPROFILE%\OneDrive\tokens.crypt`. Shown in the settings panel and the status bar. |
| `update.enable` | `true` | yes | *Automatic updates* switch. `false` disables auto-update: no request to GitHub, channel and check are hidden. |
| `update.channel` | `production` | yes | Channel followed, one of the manifest keys. An unknown channel falls back to `production`. |
| `update.upstream` | `https://github.com/131/ssh-agent-totp` | no | GitHub repository providing `manifest.json` and the releases, `https://github.com/<owner>/<repo>`. Shown in the settings panel. |
| `remote_channels` | | no | Cache of the upstream `manifest.json`, managed by the application. |

The settings panel is the wrench after the brand/logo at the top left of the window. It also shows
the token file, the running and latest versions, an *Export* button
([export](TOKENS.md#export)) and a *Check for updates* button, which fetches
`manifest.json` again whatever the cache.

## Channels

The upstream [`manifest.json`](RELEASES.md#channels) is kept in
`remote_channels` for `ttl` seconds (default 86400, one day; lower it to follow
releases sooner) after `fetched`; *Check for updates* ignores it. When the
upstream changes, the cache is ignored. When GitHub is unreachable, a stale
cache of the same upstream is still used. Delete `remote_channels` to force a
refresh.
