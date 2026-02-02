package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.octolab.org/toolset/indexit/internal/preview"
	"go.octolab.org/toolset/indexit/internal/stats"
)

var (
	saved    = preview.Source{Channel: 123, Message: 7, Photo: 9_007_199_254_740_993}
	unsaved  = preview.Source{Channel: 123, Message: 8, Photo: 11}
	stranger = preview.Source{Channel: 5, Message: 3, Photo: 12}
)

// fixture lays out an image directory with saved, foreign and skipped files
// and a peer cache that knows channel 123.
func fixture(t *testing.T) (*preview.Store, string) {
	t.Helper()
	store := &preview.Store{Root: t.TempDir()}
	write := func(path string, data []byte, at time.Time) {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, data, 0o600))
		require.NoError(t, os.Chtimes(path, at, at))
	}
	now := time.Now()
	write(store.ImagePath(saved), []byte("\xff\xd8\xff\xe0 jpeg"), now.Add(-time.Hour))
	write(store.ImagePath(stranger), []byte("\xff\xd8\xff\xe0 other"), now.Add(-2*time.Hour))
	images := store.ImageDir()
	write(filepath.Join(images, "c123", "9", "p1.jpg"), nil, now)                               // empty: invalid
	write(filepath.Join(images, "notes.txt"), []byte("x"), now)                                 // foreign: invalid
	write(filepath.Join(images, "a42", "c123", "7", "p1.jpg"), []byte("x"), now)                // another layout: invalid
	write(filepath.Join(images, "c123", "7", ".p9.mac.1.x"), []byte("partial"), now)            // in progress: skipped
	write(filepath.Join(images, "c123", "7", "p9007199254740993 (1).jpg"), []byte("copy"), now) // cloud conflict: skipped
	require.NoError(t, os.Symlink(store.ImagePath(saved), filepath.Join(images, "c123", "7", "p13.jpg")))
	peers := filepath.Join(t.TempDir(), "peers.json")
	write(peers, []byte(`{"account":{"id":42},"entries":{
		"channel:123":{"kind":"channel","id":123,"access_hash":555,"username":"example","title":"Example <b>channel</b>"},
		"chat:5":{"kind":"chat","id":5,"title":"A basic group, not channel 5"}}}`), now)
	return store, peers
}

func newDashboard(t *testing.T) (*Dashboard, *bool) {
	t.Helper()
	store, peers := fixture(t)
	dir := filepath.Join(store.Root, "stats")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	month := time.Now().UTC().Format("2006-01")
	log := `{"at":"` + time.Now().UTC().Format(time.RFC3339) + `","event":"download","path":"` + saved.Path() + `","sent":10,"saved":10}` + "\n" +
		`{"at":"` + time.Now().UTC().Format(time.RFC3339) + `","event":"hits","count":29,"sent":290}` + "\n" +
		`{"at":"` + time.Now().UTC().Format(time.RFC3339) + `","event":"gone","path":"` + unsaved.Path() + `"}` + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, month+".jsonl"), []byte(log), 0o600))
	d := New(store, peers, stats.NewReader(dir))
	d.Version = "v0.3.1"
	d.update()
	called := new(bool)
	return d, called
}

func get(t *testing.T, h http.Handler, path string, header map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://indexit.localhost:17840"+path, nil)
	for k, v := range header {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestScan(t *testing.T) {
	store, peers := fixture(t)
	inv := Scan(store, peers, time.Now())
	require.NoError(t, inv.Err)
	require.Len(t, inv.Photos, 2, "a symbolic link, an empty and a foreign file are no photographs")
	assert.Equal(t, saved.Path(), inv.Photos[0].Source.Path(), "the latest first")
	assert.Equal(t, 4, inv.Invalid, "temporary files and conflict copies are not counted")
	assert.Equal(t, "Example <b>channel</b>", peerTitle(inv, saved))
	assert.Equal(t, "Channel 5", peerTitle(inv, stranger), "a basic group is not a channel")
	assert.Equal(t, "https://t.me/example/7", telegramLink(inv, saved))
	assert.Equal(t, "https://t.me/c/5/3", telegramLink(inv, stranger), "a link for members")
}

func TestScanOfASeparateImageDirectory(t *testing.T) {
	store, peers := fixture(t)
	moved := filepath.Join(t.TempDir(), "Google Drive", "images")
	require.NoError(t, os.MkdirAll(filepath.Dir(moved), 0o700))
	require.NoError(t, os.Rename(store.ImageDir(), moved))
	store.Images = moved
	assert.Len(t, Scan(store, peers, time.Now()).Photos, 2)
	d := New(store, peers, stats.NewReader(t.TempDir()))
	w := get(t, d.Handler(http.NotFoundHandler()), "/thumbnails/c123/7/p9007199254740993.jpg", nil)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestScanFollowsALinkedDirectory(t *testing.T) {
	store, peers := fixture(t)
	moved := filepath.Join(t.TempDir(), "images")
	require.NoError(t, os.Rename(filepath.Join(store.Root, "images"), moved))
	require.NoError(t, os.Symlink(moved, filepath.Join(store.Root, "images")))
	assert.Len(t, Scan(store, peers, time.Now()).Photos, 2, "images/ on another disk")
	d := New(store, peers, stats.NewReader(t.TempDir()))
	w := get(t, d.Handler(http.NotFoundHandler()), "/thumbnails/c123/7/p9007199254740993.jpg", nil)
	assert.Equal(t, http.StatusOK, w.Code, "a thumbnail from images/ on another disk")
}

func TestStatsErrorIsShown(t *testing.T) {
	d, _ := newDashboard(t)
	dir := filepath.Join(d.Store.Root, "stats")
	month := filepath.Join(dir, time.Now().UTC().Format("2006-01")+".jsonl")
	require.NoError(t, os.Chmod(month, 0))
	t.Cleanup(func() { _ = os.Chmod(month, 0o600) })
	d.Log = stats.NewReader(dir)
	d.update()
	h := d.Handler(http.NotFoundHandler())
	assert.Contains(t, get(t, h, "/", nil).Body.String(), "could not be read in full")
	assert.Contains(t, get(t, h, "/api/v1/stats", nil).Body.String(), `"stats_error"`)
}

func TestScanOfEmptyDirectory(t *testing.T) {
	inv := Scan(&preview.Store{Root: filepath.Join(t.TempDir(), "none")}, filepath.Join(t.TempDir(), "none.json"), time.Now())
	assert.NoError(t, inv.Err)
	assert.Empty(t, inv.Photos)
}

func TestPage(t *testing.T) {
	d, called := newDashboard(t)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { *called = true; _, _ = w.Write([]byte("next")) })
	h := d.Handler(next)

	w := get(t, h, "/", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.False(t, *called)
	body := w.Body.String()
	assert.Equal(t, "text/html; charset=utf-8", w.Header().Get("Content-Type"))
	assert.Contains(t, w.Header().Get("Content-Security-Policy"), "script-src 'self'")
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	assert.NotRegexp(t, regexp.MustCompile(`\sstyle=|<style|\son[a-z]+=|<script>`), body, "the policy allows no inline style or script")
	assert.Contains(t, body, "Example &lt;b&gt;channel&lt;/b&gt;", "titles are escaped")
	assert.Contains(t, body, `data-count="2"`)
	assert.Contains(t, body, "/thumbnails/c123/7/p9007199254740993.jpg")
	assert.Contains(t, body, "30 requests per download on average")
	assert.Contains(t, body, "gone from Telegram with no local copy")
	assert.Contains(t, body, "from 2 channels")
	for _, gone := range []string{"registered", "unsaved", "source record"} {
		assert.NotContains(t, body, gone)
	}
	assert.Contains(t, body, "The calendar fills in once")
	assert.NotContains(t, body, "Table of the days", "no table without a calendar")
	for _, path := range regexp.MustCompile(`/assets/[0-9a-f]+/[a-z]+\.[a-z]+`).FindAllString(body, -1) {
		a := get(t, h, path, nil)
		assert.Equal(t, http.StatusOK, a.Code, path)
		assert.Contains(t, a.Header().Get("Cache-Control"), "immutable", path)
	}
}

func TestAPI(t *testing.T) {
	d, _ := newDashboard(t)
	h := d.Handler(http.NotFoundHandler())
	w := get(t, h, "/api/v1/stats", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.NotContains(t, w.Body.String(), "access_hash")
	assert.NotContains(t, w.Body.String(), "555")
	var out map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	recent := out["recent"].([]any)[0].(map[string]any)
	assert.Equal(t, "9007199254740993", recent["photo"], "IDs are strings, beyond the integers of JavaScript")
	assert.Equal(t, "123", recent["channel"])
	assert.NotContains(t, recent, "account")
	photographs := out["photographs"].(map[string]any)
	assert.NotContains(t, photographs, "registered")
	requests := out["requests"].(map[string]any)
	assert.InDelta(t, 30, requests["served"], 0)
	assert.InDelta(t, 1, requests["gone_without_copy"], 0)

	assert.Equal(t, http.StatusNotFound, get(t, h, "/api/v1/unsaved", nil).Code)
}

func TestThumbnailsNeverDownload(t *testing.T) {
	d, called := newDashboard(t)
	h := d.Handler(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { *called = true }))
	w := get(t, h, "/thumbnails/c123/7/p9007199254740993.jpg", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	for _, path := range []string{"/thumbnails/c123/8/p11.jpg", "/thumbnails/../notes.txt", "/thumbnails/c123/7/p9007199254740993.jpg?x", "/thumbnails/c123/7/p13.jpg", "/thumbnails/a42/c123/7/p1.jpg"} {
		w = get(t, h, path, nil)
		assert.Equal(t, http.StatusNotFound, w.Code, path)
	}
	assert.False(t, *called, "a missing photograph is not fetched")
}

func TestAccess(t *testing.T) {
	d, called := newDashboard(t)
	h := d.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { *called = true }))
	for _, tc := range []struct {
		host   string
		header map[string]string
		ok     bool
	}{
		{"indexit.localhost:17840", nil, true},
		{"LOCALHOST", nil, true},
		{"127.0.0.1:8080", nil, true},
		{"[::1]:17840", nil, true},
		{"photos.localhost.:17840", nil, true},
		{"images.example", nil, false},
		{"localhost.example:17840", nil, false},
		{"192.168.1.2:17840", nil, false},
		{"localhost", map[string]string{"X-Forwarded-For": "203.0.113.1"}, false},
		{"localhost", map[string]string{"Via": "1.1 proxy"}, false},
		{"localhost", map[string]string{"X-Forwarded-Port": "443"}, false},
		{"localhost", map[string]string{"X-Forwarded-Prefix": "/photos"}, false},
		{"localhost", map[string]string{"Forwarded": "for=203.0.113.1"}, false},
		{"localhost", map[string]string{"Cf-Connecting-Ip": "203.0.113.1"}, false},
		{"localhost", map[string]string{"X-Real-Ip": "203.0.113.1"}, false},
		{"localhost", map[string]string{"X-Forwarded-Host": "images.example"}, false},
		{"localhost", map[string]string{"Sec-Fetch-Site": "cross-site", "Sec-Fetch-Mode": "cors"}, false},
		{"localhost", map[string]string{"Sec-Fetch-Site": "same-origin", "Sec-Fetch-Mode": "cors"}, true},
		{"localhost", map[string]string{"Sec-Fetch-Site": "same-site", "Sec-Fetch-Mode": "cors"}, false},
		{"localhost", map[string]string{"Sec-Fetch-Site": "none", "Sec-Fetch-Mode": "navigate"}, true},
	} {
		r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/stats", nil)
		r.Host = tc.host
		for k, v := range tc.header {
			r.Header.Set(k, v)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if tc.ok {
			assert.Equal(t, http.StatusOK, w.Code, "%s %v", tc.host, tc.header)
		} else {
			assert.Equal(t, http.StatusForbidden, w.Code, "%s %v", tc.host, tc.header)
		}

		// The page itself falls back to the health line.
		*called = false
		r = httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
		r.Host = tc.host
		for k, v := range tc.header {
			r.Header.Set(k, v)
		}
		h.ServeHTTP(httptest.NewRecorder(), r)
		assert.Equal(t, !tc.ok, *called, "%s %v", tc.host, tc.header)
	}

	// A link from another site opens the page, a script there cannot read it.
	*called = false
	w := get(t, h, "/", map[string]string{"Sec-Fetch-Site": "cross-site", "Sec-Fetch-Mode": "navigate", "Sec-Fetch-Dest": "document"})
	assert.Equal(t, http.StatusOK, w.Code)
	assert.False(t, *called)
	*called = false
	get(t, h, "/", map[string]string{"Sec-Fetch-Site": "cross-site", "Sec-Fetch-Mode": "cors", "Sec-Fetch-Dest": "empty"})
	assert.True(t, *called)

	// every route of the page answers the same way
	for _, path := range []string{"/api/v1/stats", "/thumbnails/c123/7/p9007199254740993.jpg", d.assets.paths["CSS"]} {
		for _, header := range []map[string]string{{"X-Real-Ip": "203.0.113.1"}, {"X-Forwarded-Host": "images.example"}, {"Sec-Fetch-Site": "cross-site"}} {
			assert.Equal(t, http.StatusForbidden, get(t, h, path, header).Code, "%s %v", path, header)
		}
		assert.Equal(t, http.StatusOK, get(t, h, path, nil).Code, path)
	}

	r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "http://localhost/api/v1/stats", nil)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	assert.Equal(t, http.StatusNotFound, get(t, h, "/assets/000000000000/app.css", nil).Code)
}

func TestPageBeforeTheFirstScan(t *testing.T) {
	store, peers := fixture(t)
	d := New(store, peers, stats.NewReader(t.TempDir()))
	w := get(t, d.Handler(http.NotFoundHandler()), "/", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "Counting photographs")
}

func TestActivity(t *testing.T) {
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.Local)
	young := buildActivity([]Photo{{Saved: now.AddDate(0, 0, -3)}, {Saved: now}}, now)
	assert.Nil(t, young.Calendar, "two weeks of files before a calendar")
	assert.Equal(t, 4, young.Days)
	assert.Equal(t, "2 days", young.Active)

	var photos []Photo
	for i := range 30 {
		for range i % 4 {
			photos = append(photos, Photo{Saved: now.AddDate(0, 0, -i)})
		}
	}
	old := buildActivity(photos, now)
	require.NotNil(t, old.Calendar)
	require.Len(t, old.Calendar.Weeks, calendarWeeks)
	last := old.Calendar.Weeks[calendarWeeks-1].Days
	assert.Len(t, last, 3, "the current week ends today, a Wednesday")
	assert.Contains(t, old.Calendar.Label, "photographs saved on")
	levels := map[int]bool{}
	for _, w := range old.Calendar.Weeks {
		for _, d := range w.Days {
			levels[d.Level] = true
		}
	}
	assert.True(t, levels[0] && levels[4], "%v", levels)
}

func TestFormats(t *testing.T) {
	assert.Equal(t, "0", count(0))
	assert.Equal(t, "999", count(999))
	assert.Equal(t, "1,000", count(1000))
	assert.Equal(t, "12,345,678", count(12345678))
	assert.Equal(t, amount{"512", "B"}, bytesAmount(512))
	assert.Equal(t, amount{"2.2", "MB"}, bytesAmount(2188196))
	assert.Equal(t, amount{"251", "GB"}, bytesAmount(250729472000))
	assert.Equal(t, amount{"1", "GB"}, bytesAmount(999999999))
	assert.Equal(t, amount{"99,999", ""}, countAmount(99999))
	assert.Equal(t, amount{"123", "K"}, countAmount(123456))
	assert.Equal(t, amount{"1.2", "M"}, countAmount(1234567))
	assert.Equal(t, "8.8%", percent(0.088))
	assert.Equal(t, "27%", percent(0.268))
	assert.Equal(t, "1 day", plural(1, "day"))
	assert.Equal(t, "1,001 days", plural(1001, "day"))
	assert.True(t, strings.HasPrefix(home(filepath.Join(os.Getenv("HOME"), ".local", "share")), "~/"))
}
