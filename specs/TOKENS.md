# Tokens

## The file

`%APPDATA%\ssh-agent-totp\tokens.crypt` is beside settings (relocatable through
[`tokens.path`](SETTINGS.md#keys)). [ssh-agent-crypt](https://github.com/131/ssh-agent-crypt)
seals the whole file with an agent key as one armored
`ssh-agent-crypt:v3:SHA256:<key fingerprint>.…` line. Names and pins are encrypted
too; backup/sync the file, but keep the SSH key. There is no master password
or separate file signature.

Clear store, Export and Import use the same standard format: one
[otpauth://totp/](https://github.com/google/google-authenticator/wiki/Key-Uri-Format)
URI per token, with optional blank lines and `#` comments:

```text
otpauth://totp/GitHub:fake-user?issuer=GitHub&secret=JBSWY3DPEHPK3PXP
```

## Link parameters

| Part | Default | Contract |
| --- | --- | --- |
| label | empty | URL-encoded `issuer:account`; account optional |
| `issuer` | label issuer | Free text, displayed bold; `/` creates folders; wins over label |
| `secret` | required | Base32 |
| `algorithm` | `SHA1` | SHA1, SHA256 or SHA512 |
| `digits` | `6` | 6–8 |
| `period` | `30` | Seconds, 1–3600 |

TOTP follows [RFC 6238](https://www.rfc-editor.org/rfc/rfc6238); HOTP counters
are unsupported. Default parameters are omitted. An issuerless account
containing `:` is encoded with a leading colon (`otpauth://totp/:a:b`).

## Order

Case-insensitive alphabetical: file by issuer/account, GUI by displayed name.
Rename to change ordering; there is no manual movement.

## Names and folders

Issuer/account are free text. Issuer `/` creates nested folders:
`Azure/admin` and `Azure/perso` share an Azure card with a count. Click replaces
the list with its contents; Escape, Backspace or Alt+Left returns. Search
shows full names across folders. A folder disappears when no names inhabit it.

- **An issuer with at least two tokens with accounts groups those accounts.**
  `Microsoft:a`, `Microsoft:b`, `Microsoft/c` share a Microsoft folder; a lone
  account remains a token card.
- **A token named like a folder goes inside it**, under its name; cards and
  folders do not share a name. Accountless duplicate issuers remain separate
  cards. Names match exactly (`Github` and `GitHub` differ), with alphabetical
  order inside each folder.

Edit-mode folder rename replaces that issuer prefix for all descendants and
own tokens in one write. Renaming to an existing path merges folders; an empty
path removes hierarchy, with own tokens retaining the folder's last name.

## Reading and writing

Strict URI/secret/algorithm/digits/period validation rejects the whole file
on one bad line. Errors disclose the line number, not its secret-bearing
content; an unreadable file is **never overwritten**. Fix it or restore the
sealed `tokens.crypt.bak`.

Each change seals the entire store and writes it atomically with mode 0600,
retaining the previous sealed version as `.bak`. Disk changes trigger reload.
A nonempty clear file is accepted and sealed on load with `tokens.key` or the
first agent key, without a backup of its clear contents.

GUI Unlock/Retry checks the agent and reloads on each new page; missing
agent/key produces its lock screen. Loaded tokens remain in memory when that
page closes. [TRAY.md](TRAY.md#agent-and-pins) defines cached pin availability.

## Editing by hand

Using the [ssh-agent-crypt CLI](https://github.com/131/ssh-agent-crypt#install):

```sh
ssh-agent-crypt -decrypt < tokens.crypt > tokens.txt
# edit tokens.txt
ssh-agent-crypt SHA256:<key> < tokens.txt > tokens.crypt
rm tokens.txt
```

The running app follows file changes. Prefer Import rather than leaving clear
`tokens.txt` on disk.

## Add

The Add dialog has Add entry, Batch import and Export tabs. Add entry takes
issuer, account, base32 secret (spaces/case ignored), algorithm, digits and
period. Same secret/issuer/account is a duplicate and is refused.

## Import

Batch import accepts mixed newline-separated formats:

| Format | Example |
| --- | --- |
| otpauth URI | `otpauth://totp/GitHub:fake-user?secret=JBSWY3DPEHPK3PXP&issuer=GitHub` |
| Google Authenticator export | `otpauth-migration://offline?data=…` |
| Bare base32 | `JBSWY3DPEHPK3PXP` |
| Label + secret/groups | `github 131 JBSW Y3DP EHPK 3PXP` |
| Explicit label | `AWS root: JBSWY3DPEHPK3PXP` or `AWS root:JBSWY3DPEHPK3PXP` |
| Issuer/account | `Azure:fake-user JBSWY3DPEHPK3PXP` |

The final word (16+ characters) or trailing groups of four supply a bare
secret. Earlier text is its label. Duplicates are skipped; HOTP and unsupported
lines are reported and remain in the input while valid entries are imported.

## Export

Export returns clear otpauth URIs with a warning, initially masked. The eye
reveals them; the copy tool or masked-text click copies them. Closing the tab
or dialog drops the text. Exported links import back unchanged.

## Changing the key

Selecting `tokens.key` reseals the store immediately. The previous key is
only needed for older backups.

## Tray pins

Edit mode toggles the same pin glyph's pressed state. `pinned=true` is part
of the sealed URI and survives export/import; absence means unpinned. Pins
do not alter GUI order. [TRAY.md](TRAY.md) defines the names-only shortcut menu.

## Live codes and progress

Expiry recomputes codes/validity without interaction, including during native
menus. Rings grow clockwise from twelve o'clock and reset per token period;
see the [shared motion contract](TRAY.md#shared-motion-and-refresh-contract).
