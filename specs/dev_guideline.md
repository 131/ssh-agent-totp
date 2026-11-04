# Development guideline

## Target platforms

**Target Windows, Linux and macOS through Wails. Cross-platform support is
ongoing work; changes must preserve the shared application contract rather
than diverging into Windows-only behavior.** Keep token operations, resident
tray, disposable GUI and interaction semantics shared. Isolate native adapters
where platform capabilities differ (clipboard, autostart, verification and
packaging); document unsupported capabilities and remaining verification.
A portable runtime does not by itself establish release readiness.

## Boring is good

Choose the dull, obvious solution, the one a reader understands at first
sight and nobody has to explain.

- **One way to do a thing.** One order (alphabetical), one file (sealed
  whole), one manifest per channel (`updates/<channel>.json`). No options for what nobody asked.
- **Standard formats and tools.** `otpauth://` links, YAML settings, RFC 6238,
  ssh-agent-crypt, ppackage, GoReleaser. The file must stay usable without the
  app: `ssh-agent-crypt -decrypt < tokens.crypt`.
- **No feature for a problem we do not have.** No sync, no lock timer, no
  per-field encryption, no drag and drop: when in doubt, leave it out.
- **Failing loudly beats guessing.** An unreadable file is reported and never
  overwritten; an unknown key is an error, not a silent default.
- **Small code, few dependencies.** The standard library first; a dependency
  must remove more code than it brings.
- **Plain UI.** Native-looking, compact, nothing moves under the pointer. A
  notification never shifts the layout.

When a change makes the project more clever, it should make it a lot more
useful. Otherwise, keep it boring.
