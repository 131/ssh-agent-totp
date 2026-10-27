package tray

import "sync"

type uiDispatch struct {
	mu    sync.Mutex
	queue []func()
}

// Dispatch posts to the window, so native modal loops also deliver callbacks.
// PostThreadMessage wakeups can be consumed by those loops instead of Run.
func (t *Tray) Dispatch(f func()) {
	t.dispatch.mu.Lock()
	t.dispatch.queue = append(t.dispatch.queue, f)
	t.dispatch.mu.Unlock()
	procPostMessage.Call(t.hwnd, wmDispatch, 0, 0)
}

func (t *Tray) runDispatched() {
	t.dispatch.mu.Lock()
	queue := t.dispatch.queue
	t.dispatch.queue = nil
	t.dispatch.mu.Unlock()
	for _, f := range queue {
		f()
	}
}
