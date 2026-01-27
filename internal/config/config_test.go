package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_DotEnvOverridesEnv(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	require.NoError(t, os.WriteFile(envPath, []byte("FOO=from_dotenv\n"), 0o600))

	t.Setenv("FOO", "from_shell")

	got, err := Load(Options{EnvFile: envPath, Explicit: true})
	require.NoError(t, err)
	assert.Equal(t, envPath, got.Path)
	assert.Equal(t, 1, got.Applied)
	assert.Equal(t, "from_dotenv", os.Getenv("FOO"))
}

func TestLoad_EnvFallsThroughForMissingKeys(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	require.NoError(t, os.WriteFile(envPath, []byte("OTHER=x\n"), 0o600))

	t.Setenv("FOO", "from_shell")

	_, err := Load(Options{EnvFile: envPath, Explicit: true})
	require.NoError(t, err)
	assert.Equal(t, "from_shell", os.Getenv("FOO"))
	assert.Equal(t, "x", os.Getenv("OTHER"))
}

func TestLoad_ExplicitEmptyDisablesDotEnv(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte("FOO=from_dotenv\n"), 0o600))

	t.Setenv("FOO", "from_shell")

	got, err := Load(Options{EnvFile: "", Explicit: true})
	require.NoError(t, err)
	assert.Empty(t, got.Path)
	assert.Equal(t, "from_shell", os.Getenv("FOO"))
}

func TestLoad_ExplicitMissingFileErrors(t *testing.T) {
	_, err := Load(Options{EnvFile: "/nonexistent/.env", Explicit: true})
	assert.Error(t, err)
}

func TestLoad_AutoDiscoversCwdDotEnv(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte("INDEXIT_TEST_AUTO=1\n"), 0o600))

	t.Setenv("INDEXIT_ENV_FILE", "")

	got, err := Load(Options{})
	require.NoError(t, err)
	abs, _ := filepath.Abs(filepath.Join(dir, ".env"))
	assert.Equal(t, abs, got.Path)
	assert.Equal(t, "1", os.Getenv("INDEXIT_TEST_AUTO"))
}

func TestLoad_NoFile(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("INDEXIT_ENV_FILE", "")
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)

	got, err := Load(Options{})
	require.NoError(t, err)
	assert.Empty(t, got.Path)
	assert.Equal(t, 0, got.Applied)
}

func TestLoad_IndexitEnvFile(t *testing.T) {
	dir := t.TempDir()
	custom := filepath.Join(dir, "custom.env")
	require.NoError(t, os.WriteFile(custom, []byte("INDEXIT_TEST_CUSTOM=ok\n"), 0o600))

	t.Setenv("INDEXIT_ENV_FILE", custom)

	got, err := Load(Options{})
	require.NoError(t, err)
	assert.Equal(t, custom, got.Path)
	assert.Equal(t, "ok", os.Getenv("INDEXIT_TEST_CUSTOM"))
}

func TestServiceIgnoresWorkingDirectoryButExportsSharedConfig(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("INDEXIT_ENV_FILE", "")
	t.Setenv("XDG_CONFIG_HOME", dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte("INDEXIT_DATA_DIR=wrong-directory\n"), 0600))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "indexit"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "indexit", ".env"), []byte("INDEXIT_DATA_DIR=shared-directory\n"), 0600))
	t.Setenv("INDEXIT_DATA_DIR", "")
	_, err := Load(Options{IgnoreWorkingDir: true})
	require.NoError(t, err)
	assert.Equal(t, "shared-directory", os.Getenv("INDEXIT_DATA_DIR"))
}
