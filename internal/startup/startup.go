// Package startup manages this user's single application shortcut.
// The shortcut itself is the persisted boolean, not a duplicate YAML flag.
package startup

import (
	"errors"
	"os"
	"path/filepath"
)

const ShortcutName = "ssh-agent-totp.lnk"

type Store struct {
	Folder         func() (string, error)
	Executable     func() (string, error)
	CreateShortcut func(path, target string) error
}

func (s *Store) path() (string, error) {
	folder, err := s.Folder()
	if err != nil {
		return "", err
	}

	return filepath.Join(folder, ShortcutName), nil
}

func (s *Store) Enabled() (bool, error) {
	path, err := s.path()
	if err != nil {
		return false, err
	}

	_, err = os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}

	return err == nil, err
}

func (s *Store) SetEnabled(enabled bool) (bool, error) {
	path, err := s.path()
	if err != nil {
		return false, err
	}

	if !enabled {
		err := os.Remove(path)
		if errors.Is(err, os.ErrNotExist) {
			err = nil
		}
		return false, err
	}

	target, err := s.Executable()
	if err != nil {
		return false, err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return false, err
	}

	if err := s.CreateShortcut(path, target); err != nil {
		return false, err
	}

	return true, nil
}
