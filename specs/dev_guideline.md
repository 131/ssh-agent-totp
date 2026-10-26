# Development guideline

## Boring is good

Choose the dull, obvious solution, the one a reader understands at first
sight and nobody has to explain.

- **One way to do a thing.** One order (alphabetical), one file (sealed
  whole), one channel list (`manifest.json`). No options for what nobody asked.
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
