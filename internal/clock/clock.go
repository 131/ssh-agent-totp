// Package clock is the application's single, wall-clock-aligned tick source.
package clock

import "time"

const visualPeriod = 30

type Tick struct {
	Now     int64
	Modulus int64
	Percent float64
}

// Clock is the only periodic timer; each subscriber registers once before Run.
// The optional GUI bridge subscribes once and skips ticks while closed.
// There are no subscriber-specific timers.
type Clock struct {
	listeners []func(Tick)
}

func (c *Clock) Subscribe(listener func(Tick)) {
	c.listeners = append(c.listeners, listener)
}

func (c *Clock) Fire(now time.Time) {
	now = now.Truncate(time.Second)
	modulus := now.Unix() % visualPeriod
	tick := Tick{Now: now.UnixMilli(), Modulus: modulus, Percent: float64(modulus) * 100 / visualPeriod}

	for _, listener := range c.listeners {
		listener(tick)
	}
}

// Run fires every wall-clock second through post (e.g. the UI thread).
func (c *Clock) Run(post func(func())) {
	for {
		now := time.Now()
		time.Sleep(now.Truncate(time.Second).Add(time.Second).Sub(now))
		post(func() { c.Fire(time.Now()) })
	}
}
