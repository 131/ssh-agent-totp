//go:build !windows

package tray

import "time"

// Windows is the released target; other toolkits use a 500 ms fallback.
func doubleClickDelay() time.Duration { return 500 * time.Millisecond }
