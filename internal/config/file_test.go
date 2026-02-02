package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFilePath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path, err := FilePath()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "indexit", "config.toml"), path)

	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", dir)
	path, err = FilePath()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, ".config", "indexit", "config.toml"), path)
}

func TestReadFile_MissingFileIsEmpty(t *testing.T) {
	file, err := ReadFile(filepath.Join(t.TempDir(), "config.toml"))
	require.NoError(t, err)
	assert.Equal(t, File{}, file)
}

// The fixtures are the ones `make config-vet` checks against config.cue, so
// the decoder and the schema accept and reject the same files.
func TestReadFile_SchemaFixtures(t *testing.T) {
	want := map[string]string{
		"default.toml":                  "http://indexit.localhost:17840",
		"empty.toml":                    "",
		"https-trailing-slash.toml":     "https://images.example:8443",
		"ipv4.toml":                     "http://127.0.0.1:17840",
		"mixed-case-host-max-port.toml": "http://Indexit.Localhost:65535",
		"no-port.toml":                  "http://indexit.localhost",
		"service-without-origin.toml":   "",
		"dashboard-off.toml":            "",
		"proxy-with-dashboard.toml":     "https://images.example",
	}
	valid, err := filepath.Glob(filepath.Join("testdata", "valid", "*.toml"))
	require.NoError(t, err)
	require.Len(t, valid, len(want))
	for _, path := range valid {
		file, err := ReadFile(path)
		require.NoError(t, err, path)
		origin, known := want[filepath.Base(path)]
		require.True(t, known, path)
		assert.Equal(t, origin, file.Service.Origin, path)
	}

	invalid, err := filepath.Glob(filepath.Join("testdata", "invalid", "*.toml"))
	require.NoError(t, err)
	require.NotEmpty(t, invalid)
	for _, path := range invalid {
		_, err := ReadFile(path)
		require.Error(t, err, path)
		assert.Contains(t, err.Error(), path, "the error names the file")
	}
}

func TestReadFile_Dashboard(t *testing.T) {
	for name, want := range map[string]bool{
		"empty.toml":                    true,
		"default.toml":                  true,
		"dashboard-off.toml":            false,
		"ipv4.toml":                     true,
		"mixed-case-host-max-port.toml": true,
		"https-trailing-slash.toml":     false,
		"proxy-with-dashboard.toml":     true,
	} {
		file, err := ReadFile(filepath.Join("testdata", "valid", name))
		require.NoError(t, err, name)
		assert.Equal(t, want, file.Service.DashboardEnabled(), name)
	}
}

func TestLocalHost(t *testing.T) {
	for _, host := range []string{"localhost", "LOCALHOST.", "indexit.localhost", "a.b.localhost", "127.0.0.1", "127.1.2.3", "::1", "[::1]"} {
		assert.True(t, LocalHost(host), host)
	}
	for _, host := range []string{"", "localhost.example", "notlocalhost", "example.com", "0.0.0.0", "192.168.1.1", "::", "10.0.0.1", "127.0.0.1.nip.io"} {
		assert.False(t, LocalHost(host), host)
	}
}

func TestReadFile_ErrorsNameTheKey(t *testing.T) {
	for name, key := range map[string]string{
		"unknown-service-key.toml":  "unknown key service.port",
		"unknown-table.toml":        "unknown key preview",
		"top-level-origin.toml":     "unknown key origin",
		"uppercase-table.toml":      "unknown key SERVICE",
		"uppercase-key.toml":        "unknown key service.ORIGIN",
		"case-duplicate.toml":       "unknown key service.Origin",
		"port-out-of-range.toml":    "service.origin must be an http or https origin",
		"path.toml":                 "service.origin must be an http or https origin",
		"not-a-string.toml":         "service.origin cannot be a TOML integer",
		"service-not-a-table.toml":  "service cannot be a TOML string",
		"dashboard-not-a-bool.toml": "service.dashboard cannot be a TOML string",
	} {
		path := filepath.Join("testdata", "invalid", name)
		_, err := ReadFile(path)
		require.Error(t, err, name)
		assert.True(t, strings.HasPrefix(err.Error(), path+":"), err.Error())
		assert.Contains(t, err.Error(), key, name)
	}
}

func TestReadFile_UnreadableFile(t *testing.T) {
	_, err := ReadFile(t.TempDir())
	assert.Error(t, err, "a directory is not a config file")
}

func TestReadFile_SchemaIsNextToTheDecoder(t *testing.T) {
	schema, err := os.ReadFile("config.cue")
	require.NoError(t, err)
	quoted := strings.ReplaceAll(origin.String(), `\`, `\\`)
	assert.Contains(t, string(schema), `origin?: string & =~"`+quoted+`"`,
		"config.cue and the origin pattern of ReadFile agree")
}

func TestUnknownKeys(t *testing.T) {
	doc := map[string]any{"service": map[string]any{"origin": "x", "Origin": "y", "port": 1}, "SERVICE": map[string]any{}, "other": 1}
	assert.Equal(t, []string{"SERVICE", "other", "service.Origin", "service.port"}, unknownKeys(doc, schema, ""))
	assert.Empty(t, unknownKeys(map[string]any{"service": map[string]any{"origin": "x", "dashboard": false}}, schema, ""))
	assert.Empty(t, unknownKeys(map[string]any{"service": "not a table"}, schema, ""), "types are left to the decoder")
}

func TestTemplate_IsTheDefaultConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, Template, 0o600))
	file, err := ReadFile(path)
	require.NoError(t, err, "the template passes the schema")
	assert.Equal(t, File{}, file, "the template changes no default")
	assert.True(t, file.Service.DashboardEnabled())

	// Every key of the schema is documented, so uncommenting it is all it takes.
	for table, keys := range schema {
		assert.Contains(t, string(Template), "["+table+"]\n")
		for key := range keys.(map[string]any) {
			assert.Contains(t, string(Template), "\n# "+key+" = ")
		}
	}
}

func TestEnsureFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "indexit", "config.toml")
	created, err := EnsureFile(path)
	require.NoError(t, err)
	assert.True(t, created)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, Template, data)
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	entries, err := os.ReadDir(filepath.Dir(path))
	require.NoError(t, err)
	assert.Len(t, entries, 1, "no temporary file is left behind")

	for _, content := range []string{"", "[service]\norigin = \"http://photos.localhost:17840\"\n"} {
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
		created, err = EnsureFile(path)
		require.NoError(t, err)
		assert.False(t, created)
		data, err = os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, content, string(data), "an existing file is never replaced")
	}
}

func TestEnsureFile_Concurrent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	results := make(chan bool, 8)
	for range cap(results) {
		go func() {
			created, err := EnsureFile(path)
			assert.NoError(t, err)
			results <- created
		}()
	}
	won := 0
	for range cap(results) {
		if <-results {
			won++
		}
	}
	assert.Equal(t, 1, won, "exactly one process writes the file")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, Template, data)
}

func TestEnsureFile_Unwritable(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	created, err := EnsureFile(filepath.Join(dir, "indexit", "config.toml"))
	assert.Error(t, err)
	assert.False(t, created)
}

func TestCreateExclusive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	created, err := createExclusive(path)
	require.NoError(t, err)
	assert.True(t, created)
	created, err = createExclusive(path)
	require.NoError(t, err)
	assert.False(t, created, "a file that exists is left be")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, Template, data)
}
