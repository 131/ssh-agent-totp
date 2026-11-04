//go:build !windows

package clipboard

import "errors"

// Copy has no history exclusion outside Windows: callers use the GUI toolkit.
func Copy(string) error { return errors.ErrUnsupported }
