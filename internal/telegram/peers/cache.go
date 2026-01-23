package peers

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.octolab.org/toolset/indexit/internal/telegram/uid"
)

type Entry struct {
	Kind       uid.Kind  `json:"kind"`
	ID         int64     `json:"id"`
	AccessHash int64     `json:"access_hash,omitempty"`
	Username   string    `json:"username,omitempty"`
	Title      string    `json:"title,omitempty"`
	LastSeen   time.Time `json:"last_seen"`
}

// Account is the Telegram account that owns the access hashes of a cache:
// they are issued per account and are worthless to any other. AuthKeyID
// fingerprints the session the owner was last verified with, so an unchanged
// session needs no request to trust the cache again.
type Account struct {
	ID        int64  `json:"id"`
	AuthKeyID string `json:"auth_key_id,omitempty"`
}

// Cache is the peer cache of one account. A cache without Account, as written
// before the owner was recorded, cannot be shown to belong to anyone.
type Cache struct {
	Account *Account         `json:"account,omitempty"`
	Entries map[string]Entry `json:"entries"`
}

func New() *Cache {
	return &Cache{Entries: map[string]Entry{}}
}

func Load(path string) (*Cache, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return New(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read peer cache: %w", err)
	}
	var cache Cache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, fmt.Errorf("decode peer cache: %w", err)
	}
	if cache.Entries == nil {
		cache.Entries = map[string]Entry{}
	}
	return &cache, nil
}

// Save writes the cache atomically: a temporary file in the same directory
// replaces the old one, so a failed write never leaves half a cache behind.
func (c *Cache) Save(path string) error {
	if c == nil {
		return nil
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create peer cache directory: %w", err)
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("encode peer cache: %w", err)
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("write peer cache: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write peer cache: %w", err)
	}
	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write peer cache: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write peer cache: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write peer cache: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("write peer cache: %w", err)
	}
	return nil
}

// Reset drops every entry and the owner: what the cache knew is not to be
// trusted.
func (c *Cache) Reset() {
	c.Account = nil
	c.Entries = map[string]Entry{}
}

func (c *Cache) Get(kind uid.Kind, id int64) (Entry, bool) {
	if c == nil {
		return Entry{}, false
	}
	entry, ok := c.Entries[key(kind, id)]
	return entry, ok
}

func (c *Cache) Len() int {
	if c == nil {
		return 0
	}
	return len(c.Entries)
}

func (c *Cache) Put(entry Entry) {
	if c.Entries == nil {
		c.Entries = map[string]Entry{}
	}
	entry.LastSeen = time.Now().UTC()
	c.Entries[key(entry.Kind, entry.ID)] = entry
}

func key(kind uid.Kind, id int64) string {
	return fmt.Sprintf("%s:%d", kind, id)
}
