// Package winres holds the application icon; icon.py regenerates it.
package winres

import _ "embed"

//go:embed icon.png
var Icon []byte
