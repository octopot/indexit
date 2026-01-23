package peers

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.octolab.org/toolset/indexit/internal/telegram/uid"
)

func TestCacheAccountRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "peers.json")
	cache := New()
	cache.Account = &Account{ID: 7, AuthKeyID: "00000000000000ff"}
	cache.Put(Entry{Kind: uid.KindUser, ID: 50, AccessHash: 150})
	require.NoError(t, cache.Save(path))
	require.NoError(t, cache.Save(path), "a second save replaces the file")

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"account": {`)
	loaded, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, &Account{ID: 7, AuthKeyID: "00000000000000ff"}, loaded.Account)
	assert.Equal(t, 1, loaded.Len())

	files, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, files, 1, "no temporary file is left behind")
	info, err := os.Stat(path)
	require.NoError(t, err)
	if runtime.GOOS != "windows" {
		assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
	}
}

func TestCacheLegacyFileHasNoAccount(t *testing.T) {
	path := filepath.Join(t.TempDir(), "peers.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"entries":{"user:5":{"kind":"user","id":5,"access_hash":50,"last_seen":"2026-01-01T00:00:00Z"}}}`), 0600))

	loaded, err := Load(path)
	require.NoError(t, err)
	assert.Nil(t, loaded.Account)
	assert.Equal(t, 1, loaded.Len())

	loaded.Reset()
	assert.Nil(t, loaded.Account)
	assert.Zero(t, loaded.Len())
}

func TestCacheIgnoresRetiredSelfField(t *testing.T) {
	path := filepath.Join(t.TempDir(), "peers.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"entries":{"user:8":{"kind":"user","id":8,"self":true,"last_seen":"2026-01-01T00:00:00Z"}}}`), 0600))

	loaded, err := Load(path)
	require.NoError(t, err)
	entry, ok := loaded.Get(uid.KindUser, 8)
	require.True(t, ok, "an old cache file with self still loads")
	assert.Zero(t, entry.AccessHash)
}

func TestCacheRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "peers.json")
	cache := New()
	cache.Put(Entry{Kind: uid.KindChannel, ID: 42, AccessHash: 99, Username: "telegram"})
	require.NoError(t, cache.Save(path))

	loaded, err := Load(path)
	require.NoError(t, err)
	entry, ok := loaded.Get(uid.KindChannel, 42)
	require.True(t, ok)
	assert.Equal(t, int64(99), entry.AccessHash)
	assert.Equal(t, "telegram", entry.Username)
}
