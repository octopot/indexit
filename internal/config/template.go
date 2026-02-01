package config

import (
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Template is the config file the service writes when there is none: every
// key commented out with its default, so later defaults still apply.
//
//go:embed template.toml
var Template []byte

// EnsureFile writes Template to path unless a file is already there, and
// reports whether it did. It never replaces a file, not even an empty one: the
// template appears under its name complete or not at all, unless the file
// system lacks hard links, and of two processes creating it at once one wins
// and the other leaves it be.
func EnsureFile(path string) (bool, error) {
	if _, err := os.Lstat(path); err == nil || !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return false, err
	}
	tmp, err := os.CreateTemp(dir, ".config-*.toml")
	if err != nil {
		return false, err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err = tmp.Write(Template); err != nil {
		_ = tmp.Close()
		return false, err
	}
	if err = tmp.Sync(); err != nil {
		_ = tmp.Close()
		return false, err
	}
	if err = tmp.Close(); err != nil {
		return false, err
	}
	// A hard link publishes the finished file and, unlike a rename, fails
	// instead of replacing one that appeared in the meantime.
	err = os.Link(tmp.Name(), path)
	if errors.Is(err, os.ErrExist) {
		return false, nil
	}
	if err != nil {
		// Some file systems, such as exFAT or a network home, have no hard
		// links: create the file exclusively and write it in place.
		return createExclusive(path)
	}
	return true, nil
}

func createExclusive(path string) (bool, error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("create %s: %w", path, err)
	}
	_, err = f.Write(Template)
	if err == nil {
		err = f.Sync()
	}
	if e := f.Close(); err == nil {
		err = e
	}
	if err != nil {
		_ = os.Remove(path)
		return false, fmt.Errorf("create %s: %w", path, err)
	}
	return true, nil
}
