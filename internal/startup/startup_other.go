//go:build !windows

package startup

import "errors"

func DefaultStore() *Store {
	return &Store{Folder: func() (string, error) {
		return "", errors.New("launch on startup is only available on Windows")
	}}
}
