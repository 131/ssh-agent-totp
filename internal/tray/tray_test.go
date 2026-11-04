package tray

import (
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/131/ssh-agent-totp/internal/i18n"
	"github.com/131/ssh-agent-totp/internal/tokens"
)

func TestMenuPublishedBeforeUnlock(t *testing.T) {
	tray := &Tray{menu: application.NewMenu(), icon: &application.SystemTray{}}
	tray.Refresh()
	if tray.menu.ItemAt(0).Label() != i18n.T("tray.open") ||
		!tray.menu.ItemAt(1).IsSeparator() ||
		tray.menu.ItemAt(2).Label() != i18n.T("tray.quit") || tray.menu.ItemAt(3) != nil {
		t.Fatal("initial menu must contain only Open, separator, Quit")
	}
}

func TestRefreshPinsWithoutOpeningWindow(t *testing.T) {
	pins := []tokens.PinnedItem{{ID: 1, Label: "Azure / test"}}
	opened := 0
	tray := &Tray{menu: application.NewMenu(), icon: &application.SystemTray{},
		Pinned: func() []tokens.PinnedItem { return pins }, Open: func() { opened++ }}
	tray.Refresh()
	if tray.menu.ItemAt(0).Label() != pins[0].Label || len(tray.pins) != 1 {
		t.Fatal("pin was not published")
	}
	pins = []tokens.PinnedItem{{ID: 2, Label: "GitHub / test"}}
	tray.Refresh()
	if tray.menu.FindByLabel("Azure / test") != nil || tray.menu.ItemAt(0).Label() != pins[0].Label {
		t.Fatal("published menu retained a stale pin")
	}
	pins = nil
	tray.Refresh()
	if len(tray.pins) != 0 || tray.menu.ItemAt(0).Label() != i18n.T("tray.open") || opened != 0 {
		t.Fatal("refresh should remove pins without opening the window")
	}
	tray.open()
	tray.open()
	if opened != 2 {
		t.Fatal("primary activation must dispatch immediately without a timer")
	}
}

func TestRefreshMenuLanguage(t *testing.T) {
	defer i18n.Set(i18n.Default)
	tray := &Tray{menu: application.NewMenu(), icon: &application.SystemTray{}}
	for _, language := range []string{"en-us", "fr-fr"} {
		i18n.Set(language)
		tray.Refresh()
		if tray.menu.ItemAt(0).Label() != i18n.T("tray.open") || tray.menu.ItemAt(2).Label() != i18n.T("tray.quit") {
			t.Fatal("published menu labels did not follow the language")
		}
	}
}
