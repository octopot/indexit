// Package preview names Telegram photographs by URL and keeps the copies the
// local service saved. A URL holds only what any Telegram client sees in the
// post itself, so giving one writes nothing and any service can fill it.
package preview

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"image/jpeg"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.octolab.org/toolset/indexit/internal/config"
	"go.octolab.org/toolset/indexit/internal/filelock"
)

const Prefix = "/preview/telegram/v1/"

// DefaultOrigin is where the local service answers: .localhost names resolve
// to loopback without a hosts entry or any other system configuration.
const DefaultOrigin = "http://indexit.localhost:17840"

// ErrNotFound means the account of the session cannot see the channel.
var ErrNotFound = errors.New("channel not found for this Telegram account; fetch the post on this computer, then retry")

// ErrSourceGone means the post or its photograph is gone or replaced.
var ErrSourceGone = errors.New("original source photograph is no longer available")

// Source names one photograph of a channel or supergroup post: the same for
// every account that can see it.
type Source struct {
	Channel int64
	Message int
	Photo   int64
}

func (s Source) Path() string {
	return fmt.Sprintf("%sc%d/%d/p%d.jpg", Prefix, s.Channel, s.Message, s.Photo)
}

var route = regexp.MustCompile(`^/preview/telegram/v1/c([1-9][0-9]*)/([1-9][0-9]*)/p(-?[1-9][0-9]*)\.jpg$`)

func Parse(path string) (Source, error) {
	m := route.FindStringSubmatch(path)
	if m == nil {
		return Source{}, errors.New("invalid image address")
	}
	c, e1 := strconv.ParseInt(m[1], 10, 64)
	id, e2 := strconv.ParseInt(m[2], 10, 32)
	p, e3 := strconv.ParseInt(m[3], 10, 64)
	if e1 != nil || e2 != nil || e3 != nil {
		return Source{}, errors.New("invalid image coordinates")
	}
	return Source{Channel: c, Message: int(id), Photo: p}, nil
}

// Store keeps the photographs in Images, Root/images when empty, and the
// locks of this computer under Root; Origin, DefaultOrigin when empty, is the
// scheme, host and port of the URLs written into exports.
type Store struct{ Root, Images, Origin string }

// Default returns the store of the CLI and the service: the data directory
// from INDEXIT_DATA_DIR or XDG_DATA_HOME, the origin from config.toml.
func Default() (*Store, error) {
	path, err := config.FilePath()
	if err != nil {
		return nil, err
	}
	file, err := config.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return FromConfig(file)
}

// FromConfig returns the store of the CLI and the service for a config file
// already read: the data directory from INDEXIT_DATA_DIR or XDG_DATA_HOME.
func FromConfig(file config.File) (*Store, error) {
	origin := file.Service.Origin
	if origin == "" {
		origin = DefaultOrigin
	}
	root := os.Getenv("INDEXIT_DATA_DIR")
	if root == "" {
		root = os.Getenv("XDG_DATA_HOME")
		if root == "" {
			h, e := os.UserHomeDir()
			if e != nil {
				return nil, e
			}
			root = filepath.Join(h, ".local", "share")
		}
		root = filepath.Join(root, "indexit")
	}
	return &Store{Root: root, Origin: origin}, nil
}

// URL returns the permanent address of src.
func (s *Store) URL(src Source) string {
	if s.Origin == "" {
		return DefaultOrigin + src.Path()
	}
	return s.Origin + src.Path()
}

// ImageDir is the directory of the photographs, which may be shared with
// other computers.
func (s *Store) ImageDir() string {
	if s.Images != "" {
		return s.Images
	}
	return filepath.Join(s.Root, "images")
}

// ImagePath is the photograph of src: the path of its URL under ImageDir.
func (s *Store) ImagePath(src Source) string {
	return filepath.Join(s.ImageDir(), filepath.FromSlash(strings.TrimPrefix(src.Path(), Prefix)))
}

// lockStripes is how many lock files the downloads of this computer share:
// a photograph always takes the same one, and nothing is kept per picture.
const lockStripes = 64

func (s *Store) lockPath(src Source) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(src.Path()))
	return filepath.Join(s.Root, "locks", fmt.Sprintf("%02d.lock", h.Sum32()%lockStripes))
}

// Fetch writes a complete source photograph to path; the store publishes it
// only after Fetch succeeds. Fetch must check that the post still holds the
// photograph of src.
type Fetch func(context.Context, Source, string) error

// Outcome kinds of a photograph request.
const (
	// Hit is a photograph served from disk, also after waiting for another
	// process that was downloading it.
	Hit = "hit"
	// Download is a photograph fetched from Telegram, saved and served.
	Download = "download"
	// Failure is a request for valid coordinates that ended in an error.
	Failure = "failure"
)

// Outcome describes how the handler answered one request for a photograph.
type Outcome struct {
	Source Source
	Kind   string
	// Status is the HTTP status of the response.
	Status int
	// Sent counts the response body bytes: none for HEAD or 304.
	Sent int64
	// Downloaded is the size of the photograph a Download published.
	Downloaded int64
}

// Failed reports whether status is one the handler answers with when it
// cannot serve a photograph.
func Failed(status int) bool { return failure[status] }

var failure = map[int]bool{
	http.StatusNotFound: true, http.StatusGone: true, http.StatusInternalServerError: true,
	http.StatusBadGateway: true, http.StatusServiceUnavailable: true, http.StatusInsufficientStorage: true,
}

// errUnreadable is a saved photograph that cannot be read: it is never
// downloaded again, since a saved photograph is never overwritten.
var errUnreadable = errors.New("saved photograph cannot be read")

// open opens the saved photograph of src: nil without error when there is
// none.
func (s *Store) open(src Source) (*os.File, os.FileInfo, error) {
	f, err := os.Open(s.ImagePath(src))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, errUnreadable
	}
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() || st.Size() == 0 {
		f.Close()
		return nil, nil, errUnreadable
	}
	return f, st, nil
}

// ServeSaved answers with the saved photograph of src and reports whether
// there was one; it never downloads.
func (s *Store) ServeSaved(w http.ResponseWriter, r *http.Request, src Source) bool {
	f, st, err := s.open(src)
	if f == nil || err != nil {
		return false
	}
	defer f.Close()
	serveFile(w, r, f, st)
	return true
}

func serveFile(w http.ResponseWriter, r *http.Request, f *os.File, st os.FileInfo) {
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	http.ServeContent(w, r, st.Name(), st.ModTime(), f)
}

// serveDisk answers with the saved photograph of src and reports whether it
// answered: with the photograph, or with 503 for one it cannot read.
func (s *Store) serveDisk(w http.ResponseWriter, r *http.Request, src Source) bool {
	f, st, err := s.open(src)
	if err != nil {
		w.Header().Set("Cache-Control", "no-store")
		http.Error(w, err.Error()+"; check the image directory", http.StatusServiceUnavailable)
		return true
	}
	if f == nil {
		return false
	}
	defer f.Close()
	serveFile(w, r, f, st)
	return true
}

// Handler serves photographs, downloading the ones not saved yet. observe,
// when set, learns the outcome of every request for valid coordinates.
func (s *Store) Handler(fetch Fetch, observe func(Outcome)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.URL.Path == "/" {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			fmt.Fprintln(w, "indexit image service is running")
			return
		}
		src, err := Parse(r.URL.Path)
		if err != nil || r.URL.RawQuery != "" {
			http.NotFound(w, r)
			return
		}
		if observe != nil {
			counter := &countingWriter{ResponseWriter: w, status: http.StatusOK}
			outcome := Outcome{Source: src, Kind: Hit}
			defer func() {
				outcome.Status, outcome.Sent = counter.status, counter.sent
				switch {
				case outcome.Kind == Download:
					// it counts even when its response then fails
				case failure[outcome.Status]:
					outcome.Kind = Failure
				case outcome.Status >= http.StatusBadRequest:
					// a range or a condition the request could not meet is
					// neither a photograph served nor a failure of the service
					return
				}
				observe(outcome)
			}()
			w = counter
			s.serve(w, r, src, fetch, func(size int64) { outcome.Kind, outcome.Downloaded = Download, size })
			return
		}
		s.serve(w, r, src, fetch, func(int64) {})
	})
}

func (s *Store) serve(w http.ResponseWriter, r *http.Request, src Source, fetch Fetch, downloaded func(size int64)) {
	if s.serveDisk(w, r, src) {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	unlock, err := filelock.Acquire(ctx, s.lockPath(src))
	if err != nil {
		http.Error(w, "image download is busy; retry later", http.StatusServiceUnavailable)
		return
	}
	defer unlock()
	if s.serveDisk(w, r, src) {
		return
	}
	path := s.ImagePath(src)
	dir := filepath.Dir(path)
	if err = os.MkdirAll(dir, 0700); err != nil {
		http.Error(w, "image storage unavailable", http.StatusInsufficientStorage)
		return
	}
	// Another computer, or a service of another data directory, may share
	// the directory and be downloading the same photograph, so a temporary
	// file names its host and process. Only the ones of this host older than
	// any download can last are a crash's leftovers.
	prefix := fmt.Sprintf(".p%d.%s.", src.Photo, host())
	if entries, e := os.ReadDir(dir); e == nil {
		for _, entry := range entries {
			if info, e := entry.Info(); e == nil && strings.HasPrefix(entry.Name(), prefix) && time.Since(info.ModTime()) > staleAfter {
				_ = os.Remove(filepath.Join(dir, entry.Name()))
			}
		}
	}
	f, err := os.CreateTemp(dir, prefix+strconv.Itoa(os.Getpid())+".*")
	if err != nil {
		http.Error(w, "image storage unavailable", http.StatusInsufficientStorage)
		return
	}
	tmp := f.Name()
	f.Close()
	defer os.Remove(tmp)
	if err = fetch(ctx, src, tmp); err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			http.Error(w, ErrNotFound.Error(), http.StatusNotFound)
		case errors.Is(err, ErrSourceGone):
			http.Error(w, ErrSourceGone.Error(), http.StatusGone)
		case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
			http.Error(w, "Telegram operation timed out or is busy; retry later", http.StatusServiceUnavailable)
		default:
			http.Error(w, "source unavailable; check service logs and retry", http.StatusBadGateway)
		}
		return
	}
	size, err := checkJPEG(tmp)
	if err != nil {
		http.Error(w, "invalid or incomplete source photograph", http.StatusBadGateway)
		return
	}
	if err = publish(tmp, path); err != nil {
		http.Error(w, "cannot save image", http.StatusInsufficientStorage)
		return
	}
	downloaded(size)
	if !s.ServeSaved(w, r, src) {
		http.Error(w, "image storage unavailable", http.StatusInternalServerError)
	}
}

// staleAfter is the age of a temporary file no download can still be
// writing: one ends within two minutes.
const staleAfter = 10 * time.Minute

// checkJPEG syncs the file at path to disk and reports its size when it is a
// complete JPEG: one that decodes and ends with the end of image marker.
func checkJPEG(path string) (int64, error) {
	f, err := os.OpenFile(path, os.O_RDWR, 0600)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return 0, err
	}
	tail := make([]byte, 2)
	if st.Size() < 2 {
		return 0, errors.New("photograph too short")
	}
	if _, err = f.ReadAt(tail, st.Size()-2); err != nil {
		return 0, err
	}
	if !bytes.Equal(tail, []byte{0xFF, 0xD9}) {
		return 0, errors.New("not a complete JPEG")
	}
	if _, err = jpeg.Decode(bufio.NewReader(f)); err != nil {
		return 0, fmt.Errorf("not a complete JPEG: %w", err)
	}
	return st.Size(), f.Sync()
}

// publish moves the complete photograph at tmp to path and never replaces a
// photograph already there, as one another computer saved meanwhile: a hard
// link fails on an existing name. Where links are not supported it renames
// after a look; only another computer downloading the same photograph at the
// same moment can come between, and it writes the same bytes: the largest
// size of one photo ID.
func publish(tmp, path string) error {
	err := os.Link(tmp, path)
	if err == nil || errors.Is(err, os.ErrExist) {
		return nil
	}
	if _, e := os.Lstat(path); e == nil {
		return nil
	}
	return os.Rename(tmp, path)
}

// host names this computer in temporary file names.
func host() string {
	name, err := os.Hostname()
	if err != nil || name == "" {
		return "localhost"
	}
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' {
			return r
		}
		return '-'
	}, name)
}

// countingWriter remembers the status and counts the body bytes of a
// response.
type countingWriter struct {
	http.ResponseWriter
	status int
	sent   int64
	wrote  bool
}

func (c *countingWriter) WriteHeader(status int) {
	if !c.wrote {
		c.status, c.wrote = status, true
	}
	c.ResponseWriter.WriteHeader(status)
}

func (c *countingWriter) Write(p []byte) (int, error) {
	c.wrote = true
	n, err := c.ResponseWriter.Write(p)
	c.sent += int64(n)
	return n, err
}

func (c *countingWriter) Unwrap() http.ResponseWriter { return c.ResponseWriter }
