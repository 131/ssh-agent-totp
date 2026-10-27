// Package clock is the application's single, wall-clock-aligned tick source.
package clock

import "time"

type Tick struct {
	Now     int64
	Modulus int64
	Percent float64
}

// Clock is owned by the UI thread; subscribers register once at startup.
// The native timer calls Fire. There are no subscriber-specific timers.
type Clock struct {
	listeners []func(Tick)
}

func (c *Clock) Subscribe(listener func(Tick)) {
	c.listeners = append(c.listeners, listener)
}

func (c *Clock) Fire(now time.Time) {
	now = now.Truncate(time.Second)
	modulus := now.Unix() % 30
	tick := Tick{Now: now.UnixMilli(), Modulus: modulus, Percent: float64(modulus) * 100 / 30}
	for _, listener := range c.listeners {
		listener(tick)
	}
}
