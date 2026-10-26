# Tokens

## The file

Every token lives in one file, next to the settings:

```
%APPDATA%\ssh-agent-totp\tokens.crypt
%APPDATA%\ssh-agent-totp\settings.yml
```

`tokens.crypt` is a text file sealed as a whole by a key of your SSH agent
with [ssh-agent-crypt](https://github.com/131/ssh-agent-crypt): one armored
line, `ssh-agent-crypt:v3:SHA256:<key fingerprint>.…`. Nothing is readable
without that key, names included, so you may back the file up or sync it like
any other (`tokens.path` in the [settings](SETTINGS.md) moves it). There is no
other protection: no master password, no signature of the file.

Unsealed, it is one standard
[`otpauth://totp/`](https://github.com/google/google-authenticator/wiki/Key-Uri-Format)
link per token, nothing else. The file, the *Export* tab and the *Batch
import* box speak the same format:

```
# ssh-agent-totp tokens: https://github.com/131/ssh-agent-totp/blob/master/specs/TOKENS.md
otpauth://totp/Azure/fleurent%28admin%29:?issuer=Azure%2Ffleurent%28admin%29&secret=KRSXG5CTMVRXEZLU
otpauth://totp/Azure/fleurent%28perso%29:?issuer=Azure%2Ffleurent%28perso%29&secret=GEZDGNBVGY3TQOJQ
otpauth://totp/github%20131:?digits=8&issuer=github+131&secret=MFRGGZDFMZTWQ2LK
otpauth://totp/GitHub:fake-user?issuer=GitHub&secret=JBSWY3DPEHPK3PXP
```

## Link parameters

| Part | Default | |
|------|---------|-|
| label | | `issuer:account`, URL-encoded. The account is shown next to the name and is optional. |
| `issuer` | | Service name, shown in bold. Any text; `/` makes [folders](#names-and-folders). Wins over the label. |
| `secret` | | Base32 secret. Required. |
| `algorithm` | `SHA1` | `SHA1`, `SHA256` or `SHA512`. |
| `digits` | `6` | 6 to 8. |
| `period` | `30` | Seconds, 1 to 3600. |

Only TOTP ([RFC 6238](https://www.rfc-editor.org/rfc/rfc6238)) is supported:
no HOTP counters. Default values are not written. Without an issuer, an
account containing `:` is written with a leading colon (`otpauth://totp/:a:b`)
so it reads back as an account.

## Order

Alphabetical, case-insensitive: the file on `issuer` then `account`, the
list on the name shown. There is no other order and no way to move a token:
rename it.

## Names and folders

`issuer` and `account` are free text. In `issuer`, `/` separates folders:
`Azure/fleurent(perso)` and `Azure/fleurent(admin)` show as an `Azure` card,
the size of a token, with its token count. Click it to open the folder: its
list replaces the main one, with *‹ Azure* on top to come back (Escape,
Backspace or Alt+Left too). Folders nest (`Work/Azure/admin`). A search lists
every match, whatever its folder, under its full name. Folders exist only
through names: renaming the last token out of a folder removes it.

Two more rules, for display only (the file keeps `issuer` and `account`):

- **An issuer with at least 2 tokens with an account becomes a folder of its
  accounts.** `Microsoft:a`, `Microsoft:b` and `Microsoft/c` show as a
  `Microsoft` folder holding `a`, `b` and `c`. With a single account, the
  token stays as it is: issuer in bold, account next to it.
- **A token named like a folder goes inside it**, under its name: `Azure`
  (no account) next to `Azure/CN` and `Azure/admin` is the third entry of the
  `Azure` folder. A card and a folder never share a name.

Tokens without an account are never grouped (two `GitHub` stay two cards: give
them an account). Names are compared exactly: `Github` and `GitHub` are two
issuers. Inside a folder, entries are in alphabetical order on the name shown.

In edit mode, a folder card has a rename tool: the new path replaces the
folder in the `issuer` of every token inside, sub-folders and the folder's own
tokens included (`Azure` → `Cloud/Azure` moves `Azure/fleurent(perso)` to
`Cloud/Azure/fleurent(perso)`, and `Azure:x` to `Cloud/Azure:x`). An
existing folder name merges both; an empty name takes the tokens out of every
folder (the folder's own tokens keep its last part as issuer). One write, like
any change.

## Reading and writing

- The file is read strictly: blank lines and `#` comments are ignored, any
  other line must be a valid `otpauth://totp/` link (secret, algorithm, digits
  and period checked). One bad line makes the whole file unreadable; the error
  gives its line number, never its content. The file is **never
  overwritten**: fix it, or restore `tokens.crypt.bak`.
- Without the agent or its key, the app shows the lock screen; *Retry* once
  the agent is back.
- Every change (import, rename, delete, new key) seals the whole file again
  and rewrites it atomically, with mode 0600; the previous version is kept as
  `tokens.crypt.bak`, sealed as well.
- The file is reloaded when it changes on disk.

## Editing by hand

Unseal, edit, seal again, with the
[ssh-agent-crypt CLI](https://github.com/131/ssh-agent-crypt#install):

```sh
ssh-agent-crypt -decrypt < tokens.crypt > tokens.txt
# edit tokens.txt
ssh-agent-crypt SHA256:<key> < tokens.txt > tokens.crypt
rm tokens.txt
```

The running app follows the change. A `tokens.crypt` written in clear is
accepted once: it is sealed at the next load with the configured key
(`tokens.key`, else the first key of the agent), without a `.bak` copy of the
clear version. Do not leave the clear `tokens.txt` around: prefer the import
box.

## Add

The `+` button opens three tabs: *Add entry*, *Batch import* and, on the
right, *Export*.

*Add entry* takes one token: issuer (`/` for folders), account, base32 secret
(spaces and case do not matter), algorithm (SHA1, SHA256, SHA512), digits (6
to 8) and period (seconds, default 30). A token already present (same secret,
issuer and account) is refused.

## Import

*Batch import* takes everything at once in one box, one token per line, in any
mix:

| Format | Example |
|--------|---------|
| `otpauth://totp/` link | `otpauth://totp/GitHub:fake-user?secret=JBSWY3DPEHPK3PXP&issuer=GitHub` |
| Google Authenticator export | `otpauth-migration://offline?data=…` (several tokens per link) |
| bare secret | `JBSWY3DPEHPK3PXP` |
| label and secret | `github 131 JBSW Y3DP EHPK 3PXP` |
| explicit label | `AWS root: JBSWY3DPEHPK3PXP`, `AWS root:JBSWY3DPEHPK3PXP` |
| issuer and account | `Azure:fleurent JBSWY3DPEHPK3PXP` |

A bare secret is the last word (16 characters or more) or the trailing groups
of 4 characters; what comes before is the label (the issuer, or
`issuer:account`). Tokens already present (same secret, issuer and account)
are skipped. HOTP entries and lines not understood are reported and left in
the box, the rest is imported.

## Export

*Export* (last tab of `+`) shows the unsealed file, one `otpauth://totp/`
link per line: secrets in clear, to import into another device or keep as a
backup. A warning comes first; the text is masked until shown (eye), copied by
the copy tool or a click on the masked text, and dropped when the tab or the
dialog closes. The links import back as they are.

## Changing the key

The SSH key chosen in the settings (`tokens.key`) seals the file. Choosing
another one seals it again with the new key at once; the former key is no
longer needed, except to open older `.bak` copies.
