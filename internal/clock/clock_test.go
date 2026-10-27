package clock

import (
	"testing"
	"time"
)

func TestSharedTickRepeatsAcrossCycles(t *testing.T) {
	var clock Clock
	var icon, gui []Tick
	clock.Subscribe(func(tick Tick) { icon = append(icon, tick) })
	clock.Subscribe(func(tick Tick) { gui = append(gui, tick) })
	for _, second := range []int64{0, 7, 29, 30, 45, 59, 60, 90, 120} {
		clock.Fire(time.Unix(second, 750000000))
	}
	for i, tick := range icon {
		if tick != gui[i] || tick.Now%1000 != 0 || tick.Modulus != tick.Now/1000%30 || tick.Percent != float64(tick.Modulus)*100/30 {
			t.Fatalf("subscriber clocks diverged: %+v %+v", tick, gui[i])
		}
	}
}
