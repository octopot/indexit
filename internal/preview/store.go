// Package preview stores stable image coordinates separately from image bytes.
package preview

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

var ErrAccount = errors.New("image belongs to another Telegram account")
var ErrSourceGone = errors.New("original source photograph is no longer available")

type Source struct {
	Account    int64  `json:"account"`
	Peer       string `json:"peer"`
	Message    int    `json:"message"`
	Photo      int64  `json:"photo"`
	AccessHash int64  `json:"access_hash,omitempty"`
}

func (s Source) Path() string {
	return fmt.Sprintf("%sa%d/%s/%d/p%d.jpg", Prefix, s.Account, s.Peer, s.Message, s.Photo)
}

var route = regexp.MustCompile(`^/preview/telegram/v1/a([1-9][0-9]*)/([cgu][1-9][0-9]*)/([1-9][0-9]*)/p(-?[1-9][0-9]*)\.jpg$`)

func Parse(path string) (Source, error) {
	m := route.FindStringSubmatch(path)
	if m == nil {
		return Source{}, errors.New("invalid image address")
	}
	a, e1 := strconv.ParseInt(m[1], 10, 64)
	_, e2 := strconv.ParseInt(m[2][1:], 10, 64)
	id, e3 := strconv.ParseInt(m[3], 10, 32)
	p, e4 := strconv.ParseInt(m[4], 10, 64)
	if e1 != nil || e2 != nil || e3 != nil || e4 != nil {
		return Source{}, errors.New("invalid image coordinates")
	}
	return Source{Account: a, Peer: m[2], Message: int(id), Photo: p}, nil
}

// Store keeps sources and images under Root; Origin, DefaultOrigin when
// empty, is the scheme, host and port of the URLs written into exports.
type Store struct{ Root, Origin string }

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
func (s *Store) location(kind string, src Source) string {
	return filepath.Join(s.Root, kind, strings.TrimPrefix(src.Path(), Prefix))
}
func (s *Store) ImagePath(src Source) string { return s.location("images", src) }
func (s *Store) Save(src Source) error {
	if _, err := Parse(src.Path()); err != nil {
		return err
	}
	if saved, err := s.Read(src); err == nil && saved == src {
		return nil
	}
	data, err := json.Marshal(src)
	if err != nil {
		return err
	}
	path := s.location("sources", src) + ".json"
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".source-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
func (s *Store) Read(src Source) (Source, error) {
	data, err := os.ReadFile(s.location("sources", src) + ".json")
	if err != nil {
		return Source{}, err
	}
	var saved Source
	if err = json.Unmarshal(data, &saved); err != nil {
		return Source{}, err
	}
	if saved.Path() != src.Path() {
		return Source{}, errors.New("source coordinates mismatch")
	}
	return saved, nil
}

// Fetch writes a complete source photograph to path; the store publishes it
// only after Fetch succeeds. Fetch must verify both account and photo identity.
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
	http.StatusForbidden: true, http.StatusNotFound: true, http.StatusGone: true, http.StatusInternalServerError: true,
	http.StatusBadGateway: true, http.StatusServiceUnavailable: true, http.StatusInsufficientStorage: true,
}

// Saved reports whether the photograph of src is on disk.
func (s *Store) Saved(src Source) bool {
	st, err := os.Lstat(s.ImagePath(src))
	return err == nil && st.Mode().IsRegular() && st.Size() > 0
}

// ServeSaved answers with the saved photograph of src and reports whether
// there was one; it never downloads.
func (s *Store) ServeSaved(w http.ResponseWriter, r *http.Request, src Source) bool {
	path := s.ImagePath(src)
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() || st.Size() == 0 {
		return false
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	http.ServeContent(w, r, filepath.Base(path), st.ModTime(), f)
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
	path := s.ImagePath(src)
	if s.ServeSaved(w, r, src) {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	unlock, err := filelock.Acquire(ctx, s.location("locks", src)+".lock")
	if err != nil {
		http.Error(w, "image download is busy; retry later", http.StatusServiceUnavailable)
		return
	}
	defer unlock()
	if s.ServeSaved(w, r, src) {
		return
	}
	saved, err := s.Read(src)
	if err != nil {
		http.Error(w, "source metadata unavailable; fetch the message again", http.StatusNotFound)
		return
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		http.Error(w, "image storage unavailable", http.StatusInsufficientStorage)
		return
	}
	// A previous crash may have left a temporary download for this exact
	// photograph. The per-photo lock proves no live writer owns these files.
	prefix := fmt.Sprintf(".p%d-download-", src.Photo)
	if entries, e := os.ReadDir(filepath.Dir(path)); e == nil {
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), prefix) {
				_ = os.Remove(filepath.Join(filepath.Dir(path), entry.Name()))
			}
		}
	}
	f, err := os.CreateTemp(filepath.Dir(path), prefix+"*")
	if err != nil {
		http.Error(w, "image storage unavailable", http.StatusInsufficientStorage)
		return
	}
	tmp := f.Name()
	f.Close()
	defer os.Remove(tmp)
	if err = fetch(ctx, saved, tmp); err != nil {
		switch {
		case errors.Is(err, ErrAccount):
			http.Error(w, err.Error(), http.StatusForbidden)
		case errors.Is(err, ErrSourceGone):
			http.Error(w, err.Error(), http.StatusGone)
		case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
			http.Error(w, "Telegram operation timed out or is busy; retry later", http.StatusServiceUnavailable)
		default:
			http.Error(w, "source unavailable; check service logs and retry", http.StatusBadGateway)
		}
		return
	}
	f, err = os.OpenFile(tmp, os.O_RDWR, 0600)
	if err != nil {
		http.Error(w, "download incomplete", http.StatusBadGateway)
		return
	}
	header := make([]byte, 512)
	n, _ := f.Read(header)
	st, statErr := f.Stat()
	syncErr := f.Sync()
	f.Close()
	if statErr != nil || syncErr != nil || st.Size() == 0 || http.DetectContentType(header[:n]) != "image/jpeg" {
		http.Error(w, "invalid or incomplete source photograph", http.StatusBadGateway)
		return
	}
	if err = os.Rename(tmp, path); err != nil {
		http.Error(w, "cannot save image", http.StatusInsufficientStorage)
		return
	}
	downloaded(st.Size())
	if !s.ServeSaved(w, r, src) {
		http.Error(w, "image storage unavailable", http.StatusInternalServerError)
	}
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
