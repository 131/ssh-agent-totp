package tokens

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestCommittedChangesNotifyWithUnlockedStore(t *testing.T) {
	fail := false
	store := &Store{Path: filepath.Join(t.TempDir(), "tokens.crypt"), Crypt: fake(&fail)}
	var snapshots [][]PinnedItem
	store.Changed = func() { snapshots = append(snapshots, store.Pinned()) }
	check := func(operation func() error, wantNotifications int) {
		t.Helper()
		before := len(snapshots)
		if err := operation(); err != nil {
			t.Fatal(err)
		}
		if len(snapshots)-before != wantNotifications {
			t.Fatalf("notifications: got %d, want %d", len(snapshots)-before, wantNotifications)
		}
	}
	check(store.Load, 1)
	uri := "otpauth://totp/Azure:test?secret=JBSWY3DPEHPK3PXP&pinned=true"
	check(func() error { _, err := store.Import(uri); return err }, 1)
	id := store.Pinned()[0].ID
	check(func() error { return store.Pin(id, false) }, 1)
	check(func() error { return store.Pin(id, true) }, 1)
	check(func() error { return store.Rename(id, "Azure/team", "test") }, 1)
	check(func() error { _, err := store.RenameFolder("Azure", "Work"); return err }, 1)
	if got := snapshots[len(snapshots)-1][0].Label; got != "Work / team / test" {
		t.Fatal(got)
	}
	check(func() error {
		_, err := store.Import("otpauth://totp/Work%2Fteam:test?secret=JBSWY3DPEHPK3PXP&pinned=true")
		return err
	}, 0)
	before, pins := len(snapshots), store.Pinned()
	fail = true
	if err := store.Pin(id, false); err == nil {
		t.Fatal("failed write accepted")
	}
	if len(snapshots) != before || !reflect.DeepEqual(store.Pinned(), pins) {
		t.Fatal("failed write published tentative pins")
	}
	if err := store.Load(); err == nil || len(snapshots) != before {
		t.Fatal("failed unlock notified")
	}
	fail = false
	check(func() error { return store.Delete(id) }, 1)
	if len(snapshots[len(snapshots)-1]) != 0 {
		t.Fatal("deleted pin remained in published snapshot")
	}
	// A lazy reload through Codes must notify too, not just explicit Unlock.
	writer := &Store{Path: store.Path, Crypt: fake(&fail)}
	if _, err := writer.Import(uri); err != nil {
		t.Fatal(err)
	}
	check(func() error { _, err := store.Codes(); return err }, 1)
	check(func() error { _, err := store.Codes(); return err }, 0)
}

func TestFailedClearLoadKeepsCommittedPins(t *testing.T) {
	fail := false
	store := &Store{Path: filepath.Join(t.TempDir(), "tokens.crypt"), Crypt: fake(&fail)}
	if _, err := store.Import("otpauth://totp/Old:test?secret=JBSWY3DPEHPK3PXP&pinned=true"); err != nil {
		t.Fatal(err)
	}
	pins, notified := store.Pinned(), 0
	store.Changed = func() { notified++ }
	if err := os.WriteFile(store.Path, []byte("otpauth://totp/New:test?secret=JBSWY3DPEHPK3PXP&pinned=true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	fail = true
	if err := store.Load(); err == nil {
		t.Fatal("failed clear-file sealing accepted")
	}
	if notified != 0 || !reflect.DeepEqual(store.Pinned(), pins) {
		t.Fatal("failed clear-file sealing replaced committed pins")
	}
}
