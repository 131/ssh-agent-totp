package tray

import (
	"testing"
	"time"
)

func testClicks(t *testing.T) (*clickHandler, <-chan func(), *int, *int) {
	t.Helper()
	queue := make(chan func(), 8)
	menus, opens := new(int), new(int)
	c := &clickHandler{delay: time.Millisecond, post: func(fn func()) { queue <- fn },
		menu: func() { *menus++ }, open: func() { *opens++ }}
	t.Cleanup(func() { c.mu.Lock(); c.cancel(); c.mu.Unlock() })
	return c, queue, menus, opens
}

func queuedClick(t *testing.T, queue <-chan func()) func() {
	t.Helper()
	select {
	case fn := <-queue:
		return fn
	case <-time.After(5 * time.Second):
		t.Fatal("timer did not post to UI")
		return nil
	}
}

func TestSingleClickWaitsForUI(t *testing.T) {
	c, queue, menus, opens := testClicks(t)
	c.singleClick()
	fn := queuedClick(t, queue)
	if *menus != 0 {
		t.Fatal("menu ran on timer goroutine")
	}
	fn()
	if *menus != 1 || *opens != 0 {
		t.Fatalf("menus=%d opens=%d", *menus, *opens)
	}
}

func TestDoubleClickCancelsAlreadyQueuedMenu(t *testing.T) {
	c, queue, menus, opens := testClicks(t)
	c.singleClick()
	stale := queuedClick(t, queue) // Timer already fired; Stop alone cannot help.
	c.doubleClick()
	c.singleClick() // Release after the double-click must not arm another timer.
	stale()
	if *menus != 0 || *opens != 1 {
		t.Fatalf("menus=%d opens=%d", *menus, *opens)
	}
	c.singleClick() // A subsequent real single click must work again.
	queuedClick(t, queue)()
	if *menus != 1 {
		t.Fatal("next single click was swallowed")
	}
}

func TestRightClickCancelsAlreadyQueuedMenu(t *testing.T) {
	c, queue, menus, _ := testClicks(t)
	c.singleClick()
	stale := queuedClick(t, queue)
	c.rightClick()
	stale()
	if *menus != 1 {
		t.Fatalf("right click opened %d menus", *menus)
	}
}

func TestNewSingleClickInvalidatesPreviousCallback(t *testing.T) {
	c, queue, menus, _ := testClicks(t)
	c.singleClick()
	stale := queuedClick(t, queue)
	c.singleClick()
	current := queuedClick(t, queue)
	stale()
	current()
	if *menus != 1 {
		t.Fatalf("opened %d menus", *menus)
	}
}

func TestDoubleClickDelay(t *testing.T) {
	if doubleClickDelay() <= 0 {
		t.Fatal("invalid system double-click interval")
	}
}
