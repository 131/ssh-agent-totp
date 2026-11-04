# Tray

## Application contract

**Permanent tray, dynamic WebView.** `Runtime` owns the resident services,
Wails application, clipboard, clock and notification icon. An optional `WindowSession`
owns the GUI/frame/bridge, not the application. Both use one native process
and UI thread; there is no IPC.

| Action | GUI | Resident tray |
| --- | --- | --- |
| Launch, including sign-in | Absent; no browser processes | Starts |
| Open / double-click / second launch | Creates a session or restores the existing one | Keeps running; a second process exits |
| Close / native X / Escape at list root | Destroys its window and releases the browser | Keeps running |
| Minimize | Hides the session, retaining its browser | Keeps running |
| Quit | Releases any remaining session | Removes icon and exits |

Close releases the window and drops queued replies. Browser
processes exit asynchronously; DLLs may stay loaded. No process is killed
externally. The next Open loads a fresh page: saved tokens/pins/settings
persist, while navigation, dialogs and unsaved form contents do not.
Closing the GUI neither locks the token store nor restarts services. Backend
work may finish, but old replies/window actions cannot target a replacement
session. Only Quit ends the application. See [implementation](DEVELOPMENT.md#native-runtime-and-gui-session).

## Agent and pins

Startup tries to unlock/load the store off-thread. Before a successful load,
the menu contains Open and Quit only. Pins are inside the sealed token file,
so their names are unavailable before decryption. GUI Unlock/Retry checks
the agent and reloads the store on each new page.

Loaded tokens remain in resident memory. With an unchanged file, pinned
codes can still be copied after the agent becomes unavailable, even if a
new GUI page shows its lock screen. Reloading or rewriting the file needs
the agent/key; there is no automatic store lock when the GUI closes.

## Compromise

The standard Windows popup menu shows **token names only, no codes**:
full paths, alphabetically, one row per pin, without submenus or owner-draw.
The main window displays codes, folders and edit controls.

## Shortcuts

- Edit mode toggles pins with the same glyph and pressed state. Persistence
  and export/import are defined in [TOKENS.md](TOKENS.md#tray-pins).
- Each pin has an animated bitmap ring: opaque white background, gray track,
  soft blue arc. Open and Quit have no icons and follow the pinned rows.
- Clicking a pin computes its code at click time, using the same clipboard
  function as the GUI, outside clipboard history/cloud sync.
- Success flashes a checkmark for three seconds. Errors restore an existing GUI or use a native dialog when closed.
- Single/right-click opens the menu; double-click creates/restores the GUI.

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
intervals, accumulated frame counters or generic event bus. The one-shot
click/double-click timer is interaction handling, not another animation clock.
