package update

import (
	"os"
	"time"
)

// CleanupLegacy retries Wails' Windows rename-aside leftovers off the UI thread.
// The helper or kernel can still hold the old executable when the new app starts.
func CleanupLegacy() error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	return cleanupLegacy(executable, os.Remove, func() { time.Sleep(2 * time.Second) })
}
