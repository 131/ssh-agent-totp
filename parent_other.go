//go:build !windows

package main

import (
	"os"
	"time"
)

// waitParent lets the updated executable start once the previous instance,
// its parent, has released the single-instance lock.
func waitParent() {
	parent := os.Getppid()
	for attempt := 0; attempt < 75 && os.Getppid() == parent; attempt++ {
		time.Sleep(200 * time.Millisecond)
	}
}
