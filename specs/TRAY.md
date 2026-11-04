# Tray

## Contract status

Windows, Linux and macOS share the tray contract below. The resident runtime
publishes its initial menu, refreshes pins on committed store changes and opens
the GUI on primary activation. Platform verification and Linux/macOS packaging,
autostart and update support remain ongoing; the released target is Windows.

## Application contract

**Permanent tray, dynamic WebView.** `Runtime` owns the resident services,
Wails application, clipboard, clock and notification icon. An optional `WindowSession`
owns the GUI/frame/bridge, not the application. Both use one native process
and UI thread; there is no IPC.

| Action | GUI | Resident tray |
| --- | --- | --- |
| Launch, including sign-in | Absent; no browser processes | Starts with Open and Quit already published |
| Primary (left) click / Open / second launch | Creates a session or restores the existing one | Keeps running; a second process exits |
| Close / native X / Escape at list root | Destroys its window and releases the browser | Keeps running |
| Minimize button / native minimize | Hides the session, retaining its browser | Keeps running |
| Quit | Releases any remaining session | Removes icon and exits |

The window is borderless: custom header and resize edges, without a CSS
outline or Windows frame decorations (shadow/rounded corners).

Close releases the window and drops queued replies. Browser
processes exit asynchronously; DLLs may stay loaded. No process is killed
externally. The next Open loads a fresh page: saved tokens/pins/settings
persist, while navigation, dialogs and unsaved form contents do not.
Closing the GUI neither locks the token store nor restarts services. Backend
work may finish, but old replies/window actions cannot target a replacement
session. Only Quit ends the application. See [implementation](DEVELOPMENT.md#native-runtime-and-gui-session).

## Agent and pins

**Publish Open and Quit before the tray becomes interactive, on Windows,
Linux and macOS.** Startup tries to unlock/load the store off-thread. Before a
successful load, these are the only entries: pins are inside the sealed token
file, so their names are unavailable before decryption. GUI Unlock/Retry checks
the agent and reloads the store on each new page.

**Refresh the published pin entries after successful unlock/load, import,
pin/unpin or deletion, without waiting for a tray click.** Renaming a token
or folder also refreshes its displayed shortcut path; changing the language
refreshes Open and Quit. Read the committed resident store, then publish the
menu update on the UI thread. Failed writes do not publish tentative changes.

Loaded tokens remain in resident memory. With an unchanged file, pinned
codes can still be copied after the agent becomes unavailable, even if a
new GUI page shows its lock screen. Reloading or rewriting the file needs
the agent/key; there is no automatic store lock when the GUI closes.

## Compromise

The native Windows/Linux/macOS context menu shows **token names only, no codes**:
full paths, alphabetically, one row per pin, without submenus or owner-draw.
The main window displays codes, folders and edit controls.

## Shortcuts

- Edit mode toggles pins with the same glyph and pressed state. Persistence
  and export/import are defined in [TOKENS.md](TOKENS.md#tray-pins).
- Each pin has an animated bitmap ring: opaque white background, gray track,
  soft blue arc. Open and Quit have no icons and follow the pinned rows.
- Clicking a pin computes its code at click time, using the same clipboard
  function as the GUI, with clipboard-history/cloud-sync exclusion on Windows.
- Success flashes a checkmark for three seconds. Errors restore an existing GUI or use a native dialog when closed.
- **Primary (left) click creates/restores the GUI on Windows, Linux and macOS.**
  It does not open the menu or wait for a double-click. Repeated activation
  restores the same session rather than creating another window.
- **Right-click uses the native desktop context menu, with its entries already
  published.** Menu availability and freshness do not depend on a preceding
  primary click or rebuilding the menu inside a click handler. A platform
  adapter may request the native popup when its toolkit requires it (Windows/macOS);
  Linux does not depend on programmatic `OpenMenu()` support.
- Menu construction, publication and bitmap refresh run on the UI thread.

## Icon

The stopwatch/black tile and menu rings use shared icon resources at 1 fps;
there is no runtime drawing. The tray cycle is 30 seconds, not a token's
individual period. `spin` and its timer setup/dispatch stay under 25 lines.
See [resource generation](DEVELOPMENT.md#icon-and-manifest).

## Shared motion and refresh contract

All progress rings grow clockwise from twelve o'clock, showing elapsed
progress and resetting at expiry. Colors/glyphs may differ; direction does
not. Native tray/menu rings share a 30-second cycle; the GUI uses each token's
period. Expiry refreshes codes without clicks, including during native popup
or move/resize loops; those loops must not strand a pending reply.

## One application tick

One wall-clock-aligned ticker fires `internal/clock.Clock` at 1 Hz on the UI thread, with or without a GUI.
Subscribers register once: tray frame, pinned-menu bitmap and optional GUI
bridge. The bridge skips closed sessions; the page listens for
`fireEvent("tick", modulus, percent, now)` through `window.addEventListener`.
`modulus` is Unix seconds modulo 30, `percent` is elapsed 0–100 progress, and
`now` is UTC milliseconds truncated to a whole second. There are no per-view
intervals, accumulated frame counters or generic event bus. Pin membership
and labels refresh on store/language changes, not on an animation timer.
