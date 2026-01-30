package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// File is the optional config.toml in the indexit config directory, read by
// the CLI and the service alike. config.cue next to this file is its schema:
// keys outside it are errors.
type File struct {
	Service Service
}

// Service configures the local image service.
type Service struct {
	// Origin is the scheme, host and port of the photograph URLs written into
	// exports, without a trailing slash; empty when the file does not set it.
	Origin string
}

// origin mirrors #Service.origin in config.cue: http or https, a host name
// or an IPv4 address, an optional port from 1 to 65535 and nothing after
// them but a slash.
var origin = regexp.MustCompile(`^https?://[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?(\.[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?)*(:([1-9][0-9]{0,3}|[1-5][0-9]{4}|6[0-4][0-9]{3}|65[0-4][0-9]{2}|655[0-2][0-9]|6553[0-5]))?/?$`)

// schema holds the keys of config.cue, spelled exactly: go-toml matches
// struct fields regardless of case, a closed CUE definition does not.
var schema = map[string]any{"service": map[string]any{"origin": nil}}

// FilePath returns $XDG_CONFIG_HOME/indexit/config.toml, or
// ~/.config/indexit/config.toml without XDG_CONFIG_HOME: the directory of
// the shared .env.
func FilePath() (string, error) {
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		configHome = filepath.Join(home, ".config")
	}
	return filepath.Join(configHome, "indexit", "config.toml"), nil
}

// ReadFile decodes the config file at path strictly. A missing file is the
// zero File; an unknown key or a malformed value is an error that names the
// file and the key.
func ReadFile(path string) (File, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return File{}, nil
	}
	if err != nil {
		return File{}, err
	}
	var doc map[string]any
	if err = toml.Unmarshal(data, &doc); err != nil {
		return File{}, decodeError(path, err)
	}
	if keys := unknownKeys(doc, schema, ""); len(keys) > 0 {
		return File{}, fmt.Errorf("%s: unknown key %s", path, strings.Join(keys, ", "))
	}
	var raw struct {
		Service struct {
			Origin *string `toml:"origin"`
		} `toml:"service"`
	}
	if err = toml.NewDecoder(bytes.NewReader(data)).DisallowUnknownFields().Decode(&raw); err != nil {
		return File{}, decodeError(path, err)
	}
	var file File
	if v := raw.Service.Origin; v != nil {
		if !origin.MatchString(*v) {
			return File{}, fmt.Errorf("%s: service.origin must be an http or https origin, such as %q, not %q", path, "http://indexit.localhost:17840", *v)
		}
		file.Service.Origin = strings.TrimSuffix(*v, "/")
	}
	return file, nil
}

// unknownKeys lists the keys of doc that allowed does not spell, as dotted
// paths; a nil value in allowed admits any value.
func unknownKeys(doc, allowed map[string]any, prefix string) []string {
	var keys []string
	for key, value := range doc {
		want, known := allowed[key]
		if !known {
			keys = append(keys, prefix+key)
			continue
		}
		nested, isSchema := want.(map[string]any)
		table, isTable := value.(map[string]any)
		if isSchema && isTable {
			keys = append(keys, unknownKeys(table, nested, prefix+key+".")...)
		}
	}
	sort.Strings(keys)
	return keys
}

// decodeError names the file, the position and, for a value of the wrong
// type, the key of a go-toml error.
func decodeError(path string, err error) error {
	var strict *toml.StrictMissingError
	if errors.As(err, &strict) {
		keys := make([]string, 0, len(strict.Errors))
		for _, e := range strict.Errors {
			keys = append(keys, strings.Join(e.Key(), "."))
		}
		return fmt.Errorf("%s: unknown key %s", path, strings.Join(keys, ", "))
	}
	var decode *toml.DecodeError
	if errors.As(err, &decode) {
		row, column := decode.Position()
		key := strings.Join(decode.Key(), ".")
		if kind, ok := mismatch(decode.Error()); ok && key != "" {
			return fmt.Errorf("%s:%d:%d: %s cannot be a TOML %s", path, row, column, key, kind)
		}
		return fmt.Errorf("%s:%d:%d: %w", path, row, column, err)
	}
	return fmt.Errorf("%s: %w", path, err)
}

// mismatch extracts the TOML type from a go-toml type mismatch message.
func mismatch(message string) (string, bool) {
	_, rest, found := strings.Cut(message, "cannot decode TOML ")
	if !found {
		return "", false
	}
	kind, _, found := strings.Cut(rest, " into ")
	return kind, found
}
