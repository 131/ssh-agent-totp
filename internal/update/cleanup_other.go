//go:build !windows

package update

// CleanupLegacy is Windows-only: other platforms do not use rename-aside files.
func CleanupLegacy() error { return nil }
