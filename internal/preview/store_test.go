package preview

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func jpegBytes(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	if e := jpeg.Encode(&b, image.NewRGBA(image.Rect(0, 0, 8, 8)), nil); e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}
func testSource() Source {
	return Source{Channel: 123, Message: 7, Photo: -9223372036854775808}
}
func saved(store *Store, src Source) bool {
	st, err := os.Lstat(store.ImagePath(src))
	return err == nil && st.Mode().IsRegular() && st.Size() > 0
}
func request(h http.Handler, path string) *httptest.ResponseRecorder {
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequestWithContext(context.Background(), "GET", path, nil))
	return r
}
func TestCoordinates(t *testing.T) {
	src := testSource()
	got, e := Parse(src.Path())
	if e != nil || got != src || src.Path() != "/preview/telegram/v1/c123/7/p-9223372036854775808.jpg" {
		t.Fatalf("%s %+v %v", src.Path(), got, e)
	}
	for _, p := range []string{Prefix + "c0/1/p1.jpg", Prefix + "c1/1/p9223372036854775808.jpg", Prefix + "c1/1/p-0.jpg", Prefix + "c1/../p1.jpg",
		Prefix + "g1/1/p1.jpg", Prefix + "u1/1/p1.jpg", Prefix + "a1/c1/1/p1.jpg", Prefix + "c1/1/p1 (1).jpg"} {
		if _, e := Parse(p); e == nil {
			t.Errorf("accepted %s", p)
		}
	}
}
func TestURL(t *testing.T) {
	src := testSource()
	if got := (&Store{}).URL(src); got != "http://indexit.localhost:17840"+src.Path() {
		t.Errorf("empty origin: %s", got)
	}
	if got := (&Store{Origin: "https://images.example:8443"}).URL(src); got != "https://images.example:8443"+src.Path() {
		t.Errorf("configured origin: %s", got)
	}
}
func TestDefaultOrigin(t *testing.T) {
	config := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", config)
	t.Setenv("INDEXIT_DATA_DIR", t.TempDir())
	if s, e := Default(); e != nil || s.Origin != DefaultOrigin {
		t.Fatalf("without config.toml: %+v %v", s, e)
	}
	path := filepath.Join(config, "indexit", "config.toml")
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(path, []byte("[service]\norigin = \"http://photos.localhost:8080/\"\n"), 0600); e != nil {
		t.Fatal(e)
	}
	if s, e := Default(); e != nil || s.Origin != "http://photos.localhost:8080" {
		t.Fatalf("from config.toml: %+v %v", s, e)
	}
	if e := os.WriteFile(path, []byte("[service]\nurl = \"http://photos.localhost:8080\"\n"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := Default(); e == nil || !strings.Contains(e.Error(), path+": unknown key service.url") {
		t.Fatalf("unknown key: %v", e)
	}
}
func TestConcurrentColdThenOfflineAfterRestart(t *testing.T) {
	store := &Store{Root: t.TempDir()}
	src := testSource()
	data := jpegBytes(t)
	var calls atomic.Int32
	h := store.Handler(func(ctx context.Context, s Source, p string) error {
		calls.Add(1)
		time.Sleep(40 * time.Millisecond)
		if s != src {
			t.Error("lost coordinates")
		}
		return os.WriteFile(p, data, 0600)
	}, nil)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			r := request(h, src.Path())
			if r.Code != 200 || !bytes.Equal(r.Body.Bytes(), data) {
				t.Errorf("%d %s", r.Code, r.Body.String())
			}
		})
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("downloads: %d", calls.Load())
	}
	// Session and network are no longer available; a new process/store must
	// still serve exactly the original bytes.
	reopened := &Store{Root: store.Root}
	offline := reopened.Handler(func(_ context.Context, s Source, _ string) error {
		if s == src {
			t.Error("network called for cached photo")
		}
		return ErrSourceGone
	}, nil)
	r := request(offline, src.Path())
	if r.Code != 200 || !bytes.Equal(r.Body.Bytes(), data) {
		t.Fatal("lost cached image")
	}
	replacement := src
	replacement.Photo++
	if r = request(offline, replacement.Path()); r.Code != 410 {
		t.Fatalf("replacement silently substituted: %d", r.Code)
	}
}
func TestFailureDoesNotPublishPartialAndRetries(t *testing.T) {
	for _, bad := range []bool{true, false} {
		store := &Store{Root: t.TempDir()}
		src := testSource()
		var calls int
		data := jpegBytes(t)
		h := store.Handler(func(_ context.Context, _ Source, p string) error {
			calls++
			if calls == 1 {
				if err := os.WriteFile(p, []byte("partial"), 0600); err != nil {
					return err
				}
				if bad {
					return errors.New("network down")
				}
				return nil
			}
			return os.WriteFile(p, data, 0600)
		}, nil)
		if r := request(h, src.Path()); r.Code != 502 {
			t.Fatalf("%d", r.Code)
		}
		if _, e := os.Stat(store.ImagePath(src)); !os.IsNotExist(e) {
			t.Fatal("published failed download")
		}
		if r := request(h, src.Path()); r.Code != 200 {
			t.Fatal(r.Code)
		}
	}
}
func TestNoExternalSourcesOrDirectoryListing(t *testing.T) {
	h := (&Store{Root: t.TempDir()}).Handler(func(context.Context, Source, string) error { t.Fatal("unexpected fetch"); return nil }, nil)
	for _, path := range []string{"/images/", testSource().Path() + "?url=https://example.org", "/preview/telegram/v1/"} {
		if r := request(h, path); r.Code != 404 {
			t.Fatal(r.Code)
		}
	}
	if r := request(h, "/"); r.Code != 200 {
		t.Fatal(r.Code)
	}
}

func TestStatusAndStaleDownloadCleanup(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
	}{{ErrNotFound, 404}, {ErrSourceGone, 410}, {context.DeadlineExceeded, 503}} {
		store := &Store{Root: filepath.Join(t.TempDir(), "[data]")}
		src := testSource()
		dir := filepath.Dir(store.ImagePath(src))
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		// a crash of this computer left one; another computer writes the other
		own := filepath.Join(dir, ".p-9223372036854775808."+host()+".1.dead.part")
		live := filepath.Join(dir, ".p-9223372036854775808."+host()+".2.live")
		other := filepath.Join(dir, ".p-9223372036854775808.other-mac.3.live")
		for _, path := range []string{own, live, other} {
			if err := os.WriteFile(path, []byte("partial"), 0600); err != nil {
				t.Fatal(err)
			}
		}
		old := time.Now().Add(-time.Hour)
		for _, path := range []string{own, other} {
			if err := os.Chtimes(path, old, old); err != nil {
				t.Fatal(err)
			}
		}
		h := store.Handler(func(context.Context, Source, string) error { return tc.err }, nil)
		r := request(h, src.Path())
		if r.Code != tc.status || r.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("%d %v", r.Code, r.Header())
		}
		if _, err := os.Stat(own); !os.IsNotExist(err) {
			t.Fatal("crash leftover not removed")
		}
		if _, err := os.Stat(other); err != nil {
			t.Fatal("removed a temporary file of another computer")
		}
		if _, err := os.Stat(live); err != nil {
			t.Fatal("removed a download in progress of another service of this computer")
		}
	}
}

func TestImageDirectoryIsApart(t *testing.T) {
	root, images := t.TempDir(), filepath.Join(t.TempDir(), "Google Drive", "images")
	store := &Store{Root: root, Images: images}
	src := testSource()
	data := jpegBytes(t)
	h := store.Handler(func(_ context.Context, _ Source, p string) error { return os.WriteFile(p, data, 0600) }, nil)
	if r := request(h, src.Path()); r.Code != 200 {
		t.Fatal(r.Code)
	}
	want := filepath.Join(images, "c123", "7", "p-9223372036854775808.jpg")
	if got, err := os.ReadFile(want); err != nil || !bytes.Equal(got, data) {
		t.Fatalf("the photograph is not the URL's path under the image directory: %v", err)
	}
	locks, err := filepath.Glob(filepath.Join(root, "locks", "*.lock"))
	if err != nil || len(locks) != 1 || store.lockPath(src) != locks[0] {
		t.Fatalf("the lock is not one of the shared ones in the data directory: %v", locks)
	}
	entries, _ := os.ReadDir(filepath.Dir(want))
	if len(entries) != 1 {
		t.Fatalf("left files next to the photograph: %v", entries)
	}
	// another computer, sharing the directory, without a session
	other := (&Store{Root: t.TempDir(), Images: images}).Handler(func(context.Context, Source, string) error {
		t.Error("network called for a photograph another computer saved")
		return errors.New("no session")
	}, nil)
	if r := request(other, src.Path()); r.Code != 200 || !bytes.Equal(r.Body.Bytes(), data) {
		t.Fatal(r.Code)
	}
}

func TestUnreadablePhotographIsNotDownloadedAgain(t *testing.T) {
	store := &Store{Root: t.TempDir()}
	src := testSource()
	path := store.ImagePath(src)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	for name, make := range map[string]func() error{
		"empty":       func() error { return os.WriteFile(path, nil, 0600) },
		"no access":   func() error { return os.WriteFile(path, jpegBytes(t), 0) },
		"a directory": func() error { return os.Mkdir(path, 0700) },
	} {
		t.Run(name, func(t *testing.T) {
			if os.Getuid() == 0 && name == "no access" {
				t.Skip("root reads anything")
			}
			_ = os.RemoveAll(path)
			if err := make(); err != nil {
				t.Fatal(err)
			}
			var outcomes []Outcome
			h := store.Handler(func(context.Context, Source, string) error {
				t.Error("downloaded over a saved photograph")
				return nil
			}, func(o Outcome) { outcomes = append(outcomes, o) })
			if r := request(h, src.Path()); r.Code != 503 {
				t.Fatalf("%d %s", r.Code, r.Body.String())
			}
			if len(outcomes) != 1 || outcomes[0].Kind != Failure {
				t.Fatalf("%+v", outcomes)
			}
		})
	}
}

func TestPublishNeverOverwrites(t *testing.T) {
	dir := t.TempDir()
	path, tmp := filepath.Join(dir, "p1.jpg"), filepath.Join(dir, ".p1.tmp")
	for _, f := range []struct{ path, data string }{{path, "first"}, {tmp, "second"}} {
		if err := os.WriteFile(f.path, []byte(f.data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := publish(tmp, path); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "first" {
		t.Fatalf("overwritten: %q", got)
	}
}

func TestIncompleteJPEGIsNotPublished(t *testing.T) {
	data := jpegBytes(t)
	for name, body := range map[string][]byte{"cut short": data[:len(data)-10], "not a JPEG": []byte("<html>error</html>"), "markers only": {0xFF, 0xD8, 0xFF, 0xD9}} {
		t.Run(name, func(t *testing.T) {
			store := &Store{Root: t.TempDir()}
			src := testSource()
			h := store.Handler(func(_ context.Context, _ Source, p string) error { return os.WriteFile(p, body, 0600) }, nil)
			if r := request(h, src.Path()); r.Code != 502 {
				t.Fatal(r.Code)
			}
			if saved(store, src) {
				t.Fatal("published an incomplete photograph")
			}
		})
	}
}

func TestOutcomes(t *testing.T) {
	store := &Store{Root: t.TempDir()}
	src := testSource()
	data := jpegBytes(t)
	gone := src
	gone.Message++
	var outcomes []Outcome
	h := store.Handler(func(_ context.Context, s Source, p string) error {
		if s.Message == gone.Message {
			return ErrSourceGone
		}
		return os.WriteFile(p, data, 0600)
	}, func(o Outcome) { outcomes = append(outcomes, o) })
	size := int64(len(data))
	request(h, src.Path())
	request(h, src.Path())
	head := httptest.NewRecorder()
	h.ServeHTTP(head, httptest.NewRequestWithContext(context.Background(), "HEAD", src.Path(), nil))
	cached := httptest.NewRecorder()
	revalidate := httptest.NewRequestWithContext(context.Background(), "GET", src.Path(), nil)
	revalidate.Header.Set("If-Modified-Since", time.Now().Add(time.Hour).UTC().Format(http.TimeFormat))
	h.ServeHTTP(cached, revalidate)
	request(h, gone.Path())
	request(h, Prefix+"invalid")
	if !saved(store, src) || saved(store, gone) {
		t.Fatal("Saved disagrees with the disk")
	}
	want := []Outcome{
		{Source: src, Kind: Download, Status: 200, Sent: size, Downloaded: size},
		{Source: src, Kind: Hit, Status: 200, Sent: size},
		{Source: src, Kind: Hit, Status: 200},
		{Source: src, Kind: Hit, Status: 304},
		{Source: gone, Kind: Failure, Status: 410, Sent: int64(len(ErrSourceGone.Error()) + 1)},
	}
	if len(outcomes) != len(want) {
		t.Fatalf("outcomes: %+v", outcomes)
	}
	for i := range want {
		got := outcomes[i]
		if got != want[i] {
			t.Errorf("outcome %d: %+v, want %+v", i, got, want[i])
		}
	}
}
func TestServeSavedNeverDownloads(t *testing.T) {
	store := &Store{Root: t.TempDir()}
	src := testSource()
	r := httptest.NewRecorder()
	if store.ServeSaved(r, httptest.NewRequestWithContext(context.Background(), "GET", "/", nil), src) {
		t.Fatal("served a photograph that is not saved")
	}
	if e := os.MkdirAll(filepath.Dir(store.ImagePath(src)), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(store.ImagePath(src), jpegBytes(t), 0600); e != nil {
		t.Fatal(e)
	}
	r = httptest.NewRecorder()
	if !store.ServeSaved(r, httptest.NewRequestWithContext(context.Background(), "GET", "/", nil), src) || r.Code != 200 {
		t.Fatal("did not serve a saved photograph")
	}
}
func TestDownloadCountsWhenTheResponseFails(t *testing.T) {
	store := &Store{Root: t.TempDir()}
	src := testSource()
	data := jpegBytes(t)
	var outcomes []Outcome
	h := store.Handler(func(_ context.Context, _ Source, p string) error { return os.WriteFile(p, data, 0600) },
		func(o Outcome) { outcomes = append(outcomes, o) })
	r := httptest.NewRequestWithContext(context.Background(), "GET", src.Path(), nil)
	r.Header.Set("Range", "bytes=999999-")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusRequestedRangeNotSatisfiable {
		t.Fatalf("status %d", w.Code)
	}
	if len(outcomes) != 1 || outcomes[0].Kind != Download || outcomes[0].Downloaded != int64(len(data)) {
		t.Fatalf("outcomes: %+v", outcomes)
	}
	// the same range on the saved photograph is neither served nor a failure
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusRequestedRangeNotSatisfiable || len(outcomes) != 1 {
		t.Fatalf("%d %+v", w.Code, outcomes)
	}
}
