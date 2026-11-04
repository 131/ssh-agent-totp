package tray

import (
	"sync"
	"time"
)

// clickHandler distinguishes clicks before posting native work to the UI.
// A generation also cancels callbacks that have already left their timer.
type clickHandler struct {
	delay      time.Duration
	post       func(func())
	menu, open func()
	mu         sync.Mutex
	timer      *time.Timer
	generation uint64
	double     bool
}

// cancel is called with mu held.
func (c *clickHandler) cancel() {
	c.generation++
	if c.timer != nil {
		c.timer.Stop()
		c.timer = nil
	}
}

func (c *clickHandler) singleClick() {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Windows sends a release after the double-click event.
	if c.double {
		c.double = false
		return
	}
	c.cancel()
	generation := c.generation
	c.timer = time.AfterFunc(c.delay, func() {
		c.post(func() {
			c.mu.Lock()
			valid := c.generation == generation
			if valid {
				c.timer = nil
			}
			c.mu.Unlock()
			if valid {
				c.menu()
			}
		})
	})
}

func (c *clickHandler) doubleClick() {
	c.mu.Lock()
	c.cancel()
	c.double = true
	c.mu.Unlock()
	c.open()
}

func (c *clickHandler) rightClick() {
	c.mu.Lock()
	c.cancel()
	c.mu.Unlock()
	c.menu()
}
