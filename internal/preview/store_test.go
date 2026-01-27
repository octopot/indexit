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
	return Source{Account: 42, Peer: "c123", Message: 7, Photo: -9223372036854775808, AccessHash: 99}
}
func request(h http.Handler, path string) *httptest.ResponseRecorder {
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequestWithContext(context.Background(), "GET", path, nil))
	return r
}
func TestCoordinates(t *testing.T) {
	src := testSource()
	got, e := Parse(src.Path())
	if e != nil || got.Photo != src.Photo || got.Account != src.Account {
		t.Fatalf("%+v %v", got, e)
	}
	for _, p := range []string{Prefix + "a0/c1/1/p1.jpg", Prefix + "a1/c1/1/p9223372036854775808.jpg", Prefix + "a1/c1/1/p-0.jpg", Prefix + "a1/c1/../p1.jpg", Prefix + "a1/x1/1/p1.jpg"} {
		if _, e := Parse(p); e == nil {
			t.Errorf("accepted %s", p)
		}
	}
}
func TestConcurrentColdThenOfflineAfterRestart(t *testing.T) {
	store := &Store{Root: t.TempDir()}
	src := testSource()
	if e := store.Save(src); e != nil {
		t.Fatal(e)
	}
	data := jpegBytes(t)
	var calls atomic.Int32
	h := store.Handler(func(ctx context.Context, s Source, p string) error {
		calls.Add(1)
		time.Sleep(40 * time.Millisecond)
		if s.AccessHash != 99 {
			t.Error("lost source metadata")
		}
		return os.WriteFile(p, data, 0600)
	})
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
	// Metadata, session and network are no longer available; a new process/store
	// must still serve exactly the original bytes.
	if e := os.RemoveAll(filepath.Join(store.Root, "sources")); e != nil {
		t.Fatal(e)
	}
	reopened := &Store{Root: store.Root}
	offline := reopened.Handler(func(context.Context, Source, string) error {
		t.Error("network called for cached photo")
		return errors.New("logged out")
	})
	r := request(offline, src.Path())
	if r.Code != 200 || !bytes.Equal(r.Body.Bytes(), data) {
		t.Fatal("lost cached image")
	}
	replacement := src
	replacement.Photo++
	if r = request(offline, replacement.Path()); r.Code != 404 {
		t.Fatalf("replacement silently substituted: %d", r.Code)
	}
}
func TestFailureDoesNotPublishPartialAndRetries(t *testing.T) {
	for _, bad := range []bool{true, false} {
		store := &Store{Root: t.TempDir()}
		src := testSource()
		if e := store.Save(src); e != nil {
			t.Fatal(e)
		}
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
		})
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
	h := (&Store{Root: t.TempDir()}).Handler(func(context.Context, Source, string) error { t.Fatal("unexpected fetch"); return nil })
	for _, path := range []string{"/images/", testSource().Path() + "?url=https://example.org", "/preview/telegram/v1/"} {
		if r := request(h, path); r.Code != 404 {
			t.Fatal(r.Code)
		}
	}
	if r := request(h, "/"); r.Code != 200 {
		t.Fatal(r.Code)
	}
}

func TestSourceSaveDoesNotRewriteUnchangedMetadata(t *testing.T) {
	store := &Store{Root: t.TempDir()}
	src := testSource()
	if err := store.Save(src); err != nil {
		t.Fatal(err)
	}
	path := store.location("sources", src) + ".json"
	old := time.Unix(1000, 0)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(src); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(old) {
		t.Fatal("unchanged source rewritten")
	}
	src.AccessHash++
	if err := store.Save(src); err != nil {
		t.Fatal(err)
	}
	got, err := store.Read(src)
	if err != nil || got.AccessHash != src.AccessHash {
		t.Fatal("updated access hash lost")
	}
}
func TestStatusAndStaleDownloadCleanup(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
	}{{ErrAccount, 403}, {ErrSourceGone, 410}, {context.DeadlineExceeded, 503}} {
		store := &Store{Root: filepath.Join(t.TempDir(), "[data]")}
		src := testSource()
		if err := store.Save(src); err != nil {
			t.Fatal(err)
		}
		dir := filepath.Dir(store.ImagePath(src))
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		stale := filepath.Join(dir, ".p-9223372036854775808-download-dead.part")
		if err := os.WriteFile(stale, []byte("old partial"), 0600); err != nil {
			t.Fatal(err)
		}
		h := store.Handler(func(context.Context, Source, string) error { return tc.err })
		r := request(h, src.Path())
		if r.Code != tc.status || r.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("%d %v", r.Code, r.Header())
		}
		if _, err := os.Stat(stale); !os.IsNotExist(err) {
			t.Fatal("crash leftover not removed")
		}
	}
}
