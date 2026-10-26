//go:build !windows

package clipboard

import "errors"

// Copy is only available on Windows; the UI falls back to the WebView clipboard.
func Copy(uintptr, string) error { return errors.New("clipboard: windows only") }
