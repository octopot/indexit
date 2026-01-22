package peers

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.octolab.org/toolset/indexit/internal/telegram/uid"
)

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
