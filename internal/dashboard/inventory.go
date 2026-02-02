package dashboard

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"go.octolab.org/toolset/indexit/internal/preview"
	"go.octolab.org/toolset/indexit/internal/telegram/peers"
	"go.octolab.org/toolset/indexit/internal/telegram/uid"
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

// Inventory is what the image directory holds at one moment.
type Inventory struct {
	At time.Time
	// Photos are the saved photographs, the latest first.
	Photos []Photo
	// Invalid counts the files that are not a photograph, such as an empty
	// or foreign file.
	Invalid int
	// Peers holds the channels the peer cache knows, by "c<id>".
	Peers map[string]Peer
	// Err is the first error of the scan, which then counts what it could.
	Err error
}

// conflict matches the copy a cloud folder makes of a file two computers
// wrote at once, as "p123 (1).jpg".
var conflict = regexp.MustCompile(` \([0-9]+\)\.jpg$`)

// Scan walks the image directory of store and reads the peer cache at
// peersPath. Below the image directory it follows no symbolic link, and it
// accepts only names that are valid photograph coordinates.
func Scan(store *preview.Store, peersPath string, now time.Time) *Inventory {
	inv := &Inventory{At: now, Peers: map[string]Peer{}}
	fail := func(err error) {
		if inv.Err == nil {
			inv.Err = err
		}
	}
	root := store.ImageDir()
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
		// a download in progress, one a crash left behind, or a copy of a
		// conflict in a cloud folder, which the URLs never name
		if strings.HasPrefix(d.Name(), ".") || conflict.MatchString(d.Name()) {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		src, parsed := preview.Parse(preview.Prefix + filepath.ToSlash(rel))
		if err != nil || parsed != nil || !d.Type().IsRegular() {
			inv.Invalid++
			return nil
		}
		info, err := d.Info()
		if err != nil {
			fail(err)
			return nil
		}
		if info.Size() == 0 {
			inv.Invalid++
			return nil
		}
		inv.Photos = append(inv.Photos, Photo{Source: src, Size: info.Size(), Saved: info.ModTime()})
		return nil
	})
	if err != nil {
		fail(err)
	}
	slices.SortFunc(inv.Photos, func(a, b Photo) int {
		if c := b.Saved.Compare(a.Saved); c != 0 {
			return c
		}
		return strings.Compare(a.Source.Path(), b.Source.Path())
	})
	// Titles and usernames are the same for every account.
	if cache, err := peers.Load(peersPath); err != nil {
		fail(err)
	} else {
		for _, entry := range cache.Entries {
			if entry.Kind == uid.KindChannel && (entry.Title != "" || entry.Username != "") {
				inv.Peers["c"+strconv.FormatInt(entry.ID, 10)] = Peer{Title: entry.Title, Username: entry.Username}
			}
		}
	}
	return inv
}

// peerKey is the key of the channel of src in Inventory.Peers.
func peerKey(src preview.Source) string {
	return "c" + strconv.FormatInt(src.Channel, 10)
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
