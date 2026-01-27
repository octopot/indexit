// Package config loads runtime configuration for indexit.
//
// The chain is documented in the PoC plan §5.1: an optional .env file is
// resolved via the lookup order below and its values override the process
// environment for keys it defines.
//
// Lookup order:
//  1. EnvFile (typically from --env-file). Explicit empty string disables loading.
//  2. INDEXIT_ENV_FILE environment variable.
//  3. ./.env in the current working directory.
//  4. $XDG_CONFIG_HOME/indexit/.env (fallback $HOME/.config/indexit/.env).
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// LoadResult reports the outcome of Load.
type LoadResult struct {
	Path    string
	Applied int
}

// Options controls Load.
type Options struct {
	// IgnoreWorkingDir makes background services independent of the launch directory.
	IgnoreWorkingDir bool
	// EnvFile, if non-empty, forces this exact path. When Explicit is true and
	// EnvFile is empty, loading is disabled.
	EnvFile string
	// Explicit means the caller passed --env-file (even if the value is empty).
	Explicit bool
}

// Load resolves the .env path and applies its values via os.Setenv. Values
// from the .env file win over any pre-existing environment values for the
// same key (project plan §5.1).
func Load(opts Options) (LoadResult, error) {
	if opts.Explicit && opts.EnvFile == "" {
		return LoadResult{}, nil
	}
	path, err := resolvePath(opts)
	if err != nil {
		return LoadResult{}, err
	}
	if path == "" {
		return LoadResult{}, nil
	}
	pairs, err := parseFile(path)
	if err != nil {
		return LoadResult{Path: path}, fmt.Errorf("parse %s: %w", path, err)
	}
	for k, v := range pairs {
		if err := os.Setenv(k, v); err != nil {
			return LoadResult{Path: path}, fmt.Errorf("set %s: %w", k, err)
		}
	}
	return LoadResult{Path: path, Applied: len(pairs)}, nil
}

func resolvePath(opts Options) (string, error) {
	if opts.EnvFile != "" {
		abs, err := filepath.Abs(opts.EnvFile)
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(abs); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return "", fmt.Errorf("--env-file points to missing file: %s", abs)
			}
			return "", err
		}
		return abs, nil
	}
	if env := os.Getenv("INDEXIT_ENV_FILE"); env != "" {
		abs, err := filepath.Abs(env)
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(abs); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return "", fmt.Errorf("INDEXIT_ENV_FILE points to missing file: %s", abs)
			}
			return "", err
		}
		return abs, nil
	}
	if cwd, err := os.Getwd(); err == nil && !opts.IgnoreWorkingDir {
		candidate := filepath.Join(cwd, ".env")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", nil
		}
		configHome = filepath.Join(home, ".config")
	}
	candidate := filepath.Join(configHome, "indexit", ".env")
	if _, err := os.Stat(candidate); err == nil {
		return candidate, nil
	}
	return "", nil
}
