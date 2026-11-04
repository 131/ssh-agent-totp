// Package tray owns the notification icon and native shortcut menu.
package tray

import (
	"embed"
	"fmt"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/131/ssh-agent-totp/internal/clock"
	"github.com/131/ssh-agent-totp/internal/i18n"
	"github.com/131/ssh-agent-totp/internal/tokens"
)

const WindowTitle = "ssh-agent-totp"

//go:embed frames/*.png
var frames embed.FS

func frame(name string) []byte {
	data, err := frames.ReadFile("frames/" + name + ".png")
	if err != nil {
		panic(err)
	}
	return data
}

type Tray struct {
	Pinned     func() []tokens.PinnedItem
	CopyPinned func(int)
	Open       func()
	Quit       func()

	icon *application.SystemTray
	menu *application.Menu

	mu         sync.Mutex
	pins       []*application.MenuItem
	flashUntil time.Time
}

func Install(app *application.App, ticks *clock.Clock) *Tray {
	t := &Tray{menu: app.Menu.New()}
	t.icon = app.SystemTray.New().SetIcon(frame("00"))
	t.icon.SetTooltip(WindowTitle)
	t.icon.OnClick(t.open)
	t.icon.OnDoubleClick(t.open)
	// Wails supplies the native context-menu path; Linux's desktop opens
	// the published D-Bus menu without a programmatic OpenMenu call.
	t.Refresh()
	ticks.Subscribe(t.spin)
	return t
}

func (t *Tray) open() {
	if t.Open != nil {
		t.Open()
	}
}

// Refresh publishes the current flat menu without opening it. Run on the UI
// thread after store/language changes, or before the tray starts.
func (t *Tray) Refresh() {
	var pins []tokens.PinnedItem
	if t.Pinned != nil {
		pins = t.Pinned()
	}
	t.menu.Clear()
	items := make([]*application.MenuItem, 0, len(pins))
	for _, pin := range pins {
		id := pin.ID
		item := t.menu.Add(pin.Label).SetBitmap(frame(menuFrame(time.Now())))
		item.OnClick(func(*application.Context) {
			if t.CopyPinned != nil {
				t.CopyPinned(id)
			}
		})
		items = append(items, item)
	}
	if len(pins) > 0 {
		t.menu.AddSeparator()
	}
	t.menu.Add(i18n.T("tray.open")).OnClick(func(*application.Context) { t.open() })
	t.menu.AddSeparator()
	t.menu.Add(i18n.T("tray.quit")).OnClick(func(*application.Context) { t.Quit() })
	t.mu.Lock()
	t.pins = items
	t.mu.Unlock()
	// SetMenu republishes the D-Bus tray on Linux; Menu.Update alone only
	// rebuilds a GTK menu, not the desktop's exported shortcut entries.
	t.icon.SetMenu(t.menu)
}

func menuFrame(now time.Time) string {
	return fmt.Sprintf("menu-%02d", now.Unix()%30)
}

// spin swaps embedded frames: one sector per second, no drawing loop.
func (t *Tray) spin(tick clock.Tick) {
	t.mu.Lock()
	name := fmt.Sprintf("%02d", tick.Modulus)
	if tick.Now < t.flashUntil.UnixMilli() && tick.Modulus%2 == 0 {
		name = "copied"
	}
	pins := t.pins
	t.mu.Unlock()
	t.icon.SetIcon(frame(name))
	ring := frame(fmt.Sprintf("menu-%02d", tick.Modulus))
	for _, item := range pins {
		item.SetBitmap(ring)
	}
}

// Copied gives feedback without opening the window or exposing the code.
func (t *Tray) Copied() {
	t.mu.Lock()
	t.flashUntil = time.Now().Add(3 * time.Second)
	t.mu.Unlock()
}
