package update

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const legacyCleanupAttempts = 6

// cleanupLegacy only removes regular siblings named <executable>.old.<digits>.
// Capture the startup leftovers once; retry only files that could not be removed.
func cleanupLegacy(executable string, remove func(string) error, wait func()) error {
	dir := filepath.Dir(executable)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read legacy update files: %w", err)
	}
	prefix := filepath.Base(executable) + ".old."
	var paths []string
	for _, entry := range entries {
		if entry.Type().IsRegular() && strings.HasPrefix(entry.Name(), prefix) && decimalTimestamp(strings.TrimPrefix(entry.Name(), prefix)) {
			paths = append(paths, filepath.Join(dir, entry.Name()))
		}
	}
	for attempt := 0; attempt < legacyCleanupAttempts && len(paths) > 0; attempt++ {
		var pending []string
		var failures []error
		for _, path := range paths {
			if err := remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				pending = append(pending, path)
				failures = append(failures, fmt.Errorf("remove %s: %w", filepath.Base(path), err))
			}
		}
		paths = pending
		if len(paths) == 0 {
			return nil
		}
		if attempt == legacyCleanupAttempts-1 {
			return errors.Join(failures...)
		}
		wait()
	}
	return nil
}

func decimalTimestamp(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
