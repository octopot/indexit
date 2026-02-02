package dashboard

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"go.octolab.org/toolset/indexit/internal/preview"
	"go.octolab.org/toolset/indexit/internal/telegram/peers"
)

// Photo is a photograph saved on disk.
type Photo struct {
	Source preview.Source
	Size   int64
	// Saved is the modification time of the file: when the service wrote it,
	// unless something copied or restored it since.
	Saved time.Time
}

// Peer names the dialog of a photograph, when the peer cache knows it.
type Peer struct {
	Title    string
	Username string
}

// Inventory is what the data directory holds at one moment.
type Inventory struct {
	At time.Time
	// Photos are the saved photographs, the latest first.
	Photos []Photo
	// Sources holds the URL paths of the registered photographs.
	Sources map[string]bool
	// Invalid counts the files that are neither a photograph nor a source,
	// such as an empty or foreign file.
	Invalid int
	// Peers holds the dialogs the peer cache knows, by "a<account>/<peer>".
	Peers map[string]Peer
	// Err is the first error of the scan, which then counts what it could.
	Err error
}

// Scan walks the data directory of store and reads the peer cache at
// peersPath. Below images/ and sources/ it follows no symbolic link, and it
// accepts only names that are valid photograph coordinates.
func Scan(store *preview.Store, peersPath string, now time.Time) *Inventory {
	inv := &Inventory{At: now, Sources: map[string]bool{}, Peers: map[string]Peer{}}
	fail := func(err error) {
		if inv.Err == nil {
			inv.Err = err
		}
	}
	walk := func(kind, suffix string, visit func(src preview.Source, d fs.DirEntry)) {
		root := filepath.Join(store.Root, kind)
		// the directory itself may be a link, as to another disk
		if real, err := filepath.EvalSymlinks(root); err == nil {
			root = real
		}
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				if !errors.Is(err, fs.ErrNotExist) || path != root {
					fail(err)
				}
				return nil
			}
			if d.IsDir() {
				return nil
			}
			// a download in progress or one a crash left behind
			if strings.HasPrefix(d.Name(), ".") {
				return nil
			}
			rel, err := filepath.Rel(root, path)
			name, found := strings.CutSuffix(filepath.ToSlash(rel), suffix)
			src, parsed := preview.Parse(preview.Prefix + name)
			if err != nil || !found || parsed != nil || !d.Type().IsRegular() {
				inv.Invalid++
				return nil
			}
			visit(src, d)
			return nil
		})
		if err != nil {
			fail(err)
		}
	}
	walk("sources", ".json", func(src preview.Source, _ fs.DirEntry) {
		inv.Sources[src.Path()] = true
	})
	walk("images", "", func(src preview.Source, d fs.DirEntry) {
		info, err := d.Info()
		if err != nil {
			fail(err)
			return
		}
		if info.Size() == 0 {
			inv.Invalid++
			return
		}
		inv.Photos = append(inv.Photos, Photo{Source: src, Size: info.Size(), Saved: info.ModTime()})
	})
	slices.SortFunc(inv.Photos, func(a, b Photo) int {
		if c := b.Saved.Compare(a.Saved); c != 0 {
			return c
		}
		return strings.Compare(a.Source.Path(), b.Source.Path())
	})
	if cache, err := peers.Load(peersPath); err != nil {
		fail(err)
	} else if cache.Account != nil {
		// Access hashes and titles belong to the account of the cache.
		prefix := "a" + strconv.FormatInt(cache.Account.ID, 10) + "/"
		kinds := map[string]string{"channel": "c", "chat": "g", "user": "u"}
		for _, entry := range cache.Entries {
			if kind, ok := kinds[string(entry.Kind)]; ok && (entry.Title != "" || entry.Username != "") {
				inv.Peers[prefix+kind+strconv.FormatInt(entry.ID, 10)] = Peer{Title: entry.Title, Username: entry.Username}
			}
		}
	}
	return inv
}

// peerKey is the key of the dialog of src in Inventory.Peers.
func peerKey(src preview.Source) string {
	return "a" + strconv.FormatInt(src.Account, 10) + "/" + src.Peer
}

// NotSaved lists the registered photographs that are not saved, sorted.
func (inv *Inventory) NotSaved() []string {
	saved := make(map[string]bool, len(inv.Photos))
	for _, photo := range inv.Photos {
		saved[photo.Source.Path()] = true
	}
	var paths []string
	for path := range inv.Sources {
		if !saved[path] {
			paths = append(paths, path)
		}
	}
	slices.Sort(paths)
	return paths
}

// Orphans counts the saved photographs without a source record.
func (inv *Inventory) Orphans() int {
	n := 0
	for _, photo := range inv.Photos {
		if !inv.Sources[photo.Source.Path()] {
			n++
		}
	}
	return n
}

// diskFree reports the bytes available to this user on the disk of path or
// of its closest existing parent.
func diskFree(path string) (uint64, bool) {
	for {
		if _, err := os.Stat(path); err == nil {
			return statfs(path)
		}
		parent := filepath.Dir(path)
		if parent == path {
			return 0, false
		}
		path = parent
	}
}
