# Tray

## Compromise

The tray is a small shortcut menu, not another view of the authenticator.
It uses the standard Windows popup menu and shows **token names only, no
codes**. One pinned token occupies one row. There is no code preview,
multiline row, owner-drawn item, or custom menu styling.

Showing gray codes beside names would require styling the menu. If that
becomes necessary, the full application is already the right interface: it
shows codes, expiry rings, folders, search, and edit controls. Keep the tray
plain rather than recreating that interface.

## Shortcuts

- Pin/unpin individual tokens directly in the main window's edit mode.
  The same pin glyph toggles a pressed state (`aria-pressed`); there is no
  alternate unpin glyph.
  Persistence and export/import are defined in [TOKENS.md](TOKENS.md#tray-pins).
- Pinned entries use their full path, including the account when present,
  and sort alphabetically without submenus. Each entry has a small animated
  ring in the standard menu bitmap column: opaque white background, gray
  track, and soft blue arc, based on the main list’s expiry ring. Open and
  Quit have no item icon.
- Clicking a pinned entry computes the TOTP at click time and copies it
  through the same clipboard function as the main window. The code is
  never part of the menu label.
- A successful copy posts a silent notification and flashes a checkmark
  in the tray icon for three seconds. An error restores the full window
  and reports the failure there.
- Open and Quit follow the pinned entries. A single click or right-click
  on the tray icon opens this menu; double-click restores the full window.

## Icon

The existing stopwatch dial advances at one frame per second, using shared
icon resources and a window timer. Menu rings follow the same 30-second
phase at 1 fps, using separate ring resources in the native bitmap column.
The notification-area icon keeps its stopwatch and black tile. This visual
cycle is not the
period of a particular token. The animation remains under 25 lines for its
routine, timer setup and dispatch; no runtime drawing framework is needed.
Resource generation is documented in [DEVELOPMENT.md](DEVELOPMENT.md#icon-and-manifest).

## Shared motion and refresh contract

All TOTP progress indicators use the same convention, regardless of style:
main-list rings, the notification-area stopwatch, and pinned menu rings.

- Start at twelve o'clock.
- Advance clockwise, showing the elapsed fraction of the cycle (empty at
  the beginning, filling toward one full turn). Do not draw a shrinking
  remaining-time arc whose endpoint moves counterclockwise.
- Reset at the next cycle and continue indefinitely. The main list uses
  each token's actual period; native tray/menu visuals share a 30-second
  wall-clock cycle, quantized to one frame per second.
- At expiry, the main window fetches new codes and validity ends without
  requiring a click, refocus, or window reopen. Native popup menus and
  window move/resize loops must not strand an in-flight refresh reply.

Colors and glyphs may differ; origin, direction, and repeat behavior must not.

## One application tick

`internal/clock.Clock` is the single application clock. A native window timer
fires it at 1 Hz, including while the main window is hidden or a popup menu
is open. Subscribers register once at startup:

1. The notification-area icon selects its animation frame.
2. Pinned menu items reuse that frame's native bitmap variant.
3. The GUI receives `fireEvent("tick", modulus, percent, now)` and its list
   subscribes with `window.addEventListener("tick", ...)`.

`modulus` is Unix seconds modulo 30; `percent` is elapsed progress from 0 to
100 for that visual cycle; `now` is the same UTC timestamp in milliseconds,
quantized to a whole second. The list uses `now` with each token's actual
validity end and period. There are no per-view intervals or accumulated
frame counters, and no generic event-bus framework.

The short native timer that distinguishes single and double clicks is
one-shot interaction handling, not another periodic animation clock.
