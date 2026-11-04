# Settings

Preferences live in `%APPDATA%\ssh-agent-totp\settings.yml`
(`os.UserConfigDir()`, `~/.config/ssh-agent-totp/settings.yml` elsewhere).
Missing files/keys use defaults; unreadable files are reported and never
overwritten. GUI edits write the file. It holds no
secrets: [tokens.crypt](TOKENS.md) is beside it; startup state is an OS shortcut.

## Example

```yaml
update:
  enable: true
  upstream: https://github.com/131/ssh-agent-totp
  channel: production
language: en-us
tokens:
  path: '%USERPROFILE%\OneDrive\tokens.crypt'
```

## Keys

| Key | Default | UI | Behavior |
| --- | --- | --- | --- |
| `language` | `fr-fr` | yes | `fr-fr` or `en-us`, applied at once to page/tray/errors; unknown values fall back to French |
| `tokens.key` | first agent key | yes | SSH SHA-256 fingerprint; changing it [reseals the store](TOKENS.md#changing-the-key) |
| `tokens.path` | adjacent `tokens.crypt` | no | Expands `%VARIABLE%`/`$VARIABLE`; default omitted from writes; displayed in settings/status bar |
| `update.enable` | `true` | yes | `false` disables GitHub requests and hides channel/check controls |
| `update.channel` | `production` | yes | `production` or `beta`, the [manifest](RELEASES.md#channels) followed; unknown values fall back to production |
| `update.upstream` | `https://github.com/131/ssh-agent-totp` | no | GitHub repository supplying manifests/releases; shown in settings |

The wrench after the logo opens settings: token file, agent key, language,
startup, channels and version/check controls. Export belongs to the Add dialog,
not settings; see [Export](TOKENS.md#export).

## Launch on startup (Windows)

*Launch on startup* manages `ssh-agent-totp.lnk` in the current user's Startup
known folder (`shell:startup`). Presence is the persisted boolean; there is
no YAML duplicate. Opening settings rereads it. Enable creates/updates the
shortcut; disable removes only that entry. Errors restore the toggle's prior
state; other entries are untouched.

The shortcut targets the executable with its directory as working directory
and starts the resident tray, without a browser until Open/double-click.
Auto-update preserves the path. Windows policy/Task Manager can override
execution; the switch controls registration. Dev checks redirect the folder
provider into test APPDATA, never the operator's actual startup folder.
