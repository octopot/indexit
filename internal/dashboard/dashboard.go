// Package dashboard serves the statistics page of the image service: what
// the image directory holds and what the service served, at / and as JSON.
//
// The page lists every saved photograph, so it answers only requests that
// name this machine (localhost, a .localhost name or a loopback IP address)
// and came neither through a proxy nor from a script of another site.
package dashboard

import (
	"bytes"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"html/template"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"go.octolab.org/toolset/indexit/internal/config"
	"go.octolab.org/toolset/indexit/internal/preview"
	"go.octolab.org/toolset/indexit/internal/stats"
)

//go:embed web
var web embed.FS

// Interval is how often the dashboard looks at the data directory again; a
// download makes it look sooner.
const Interval = 30 * time.Second

const policy = "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self'; connect-src 'self'; " +
	"base-uri 'none'; form-action 'none'; frame-ancestors 'none'; object-src 'none'"

// Dashboard keeps the latest look at the data directory and serves it.
type Dashboard struct {
	Store *preview.Store
	// Peers is the path of the peer cache, which names the dialogs.
	Peers string
	// Log reads the statistics log.
	Log *stats.Reader
	// Dropped, when set, counts the statistics this process lost.
	Dropped    func() int64
	Version    string
	ConfigPath string
	Logger     *slog.Logger

	state  atomic.Pointer[state]
	kick   chan struct{}
	assets assets
	page   *template.Template
}

type state struct {
	inv    *Inventory
	totals stats.Totals
	// logErr says why the totals may be short.
	logErr error
}

type assets struct {
	// paths maps an asset name to its URL path, which changes with its content.
	paths map[string]string
	// files maps a URL path to the content and type of an asset.
	files map[string]asset
}

type asset struct {
	data []byte
	kind string
}

// New prepares a dashboard; Run keeps it current.
func New(store *preview.Store, peers string, log *stats.Reader) *Dashboard {
	d := &Dashboard{Store: store, Peers: peers, Log: log, Logger: slog.Default(), kick: make(chan struct{}, 1)}
	d.assets = assets{paths: map[string]string{}, files: map[string]asset{}}
	for key, name := range map[string]string{"CSS": "app.css", "JS": "app.js", "Icon": "icon.svg"} {
		data, err := web.ReadFile("web/" + name)
		if err != nil {
			panic(err)
		}
		sum := sha256.Sum256(data)
		url := "/assets/" + hex.EncodeToString(sum[:6]) + "/" + name
		d.assets.paths[key] = url
		d.assets.files[url] = asset{data: data, kind: mime.TypeByExtension(path.Ext(name))}
	}
	d.page = template.Must(template.New("page.html.tmpl").Funcs(template.FuncMap{
		"iso":  func(t time.Time) string { return t.Format(time.RFC3339) },
		"date": dateText,
		"pct":  func(f float64) string { return strconv.FormatFloat(f, 'f', 1, 64) },
		"cell": func() int { return cell },
	}).ParseFS(web, "web/page.html.tmpl"))
	return d
}

// Run looks at the data directory now, every Interval and soon after Refresh,
// until ctx ends.
func (d *Dashboard) Run(ctx context.Context) {
	ticker := time.NewTicker(Interval)
	defer ticker.Stop()
	for {
		d.update()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-d.kick:
			// a burst of downloads is one look
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
			}
		}
	}
}

// Refresh asks Run to look again soon, as after a download.
func (d *Dashboard) Refresh() {
	select {
	case d.kick <- struct{}{}:
	default:
	}
}

func (d *Dashboard) update() {
	inv := Scan(d.Store, d.Peers, time.Now())
	if inv.Err != nil {
		d.Logger.Warn("dashboard: incomplete scan", "error", inv.Err)
	}
	totals, err := d.Log.Update()
	if err != nil {
		d.Logger.Warn("dashboard: cannot read the statistics log", "error", err)
	}
	d.state.Store(&state{inv: inv, totals: totals, logErr: err})
}

func (d *Dashboard) dropped() int64 {
	if d.Dropped == nil {
		return 0
	}
	return d.Dropped()
}

// Handler serves the dashboard and passes every other request to next, which
// also answers / for a request from outside this machine.
func (d *Dashboard) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch {
		case p == "/":
			if r.Method != http.MethodGet && r.Method != http.MethodHead || !local(r, true) {
				next.ServeHTTP(w, r)
				return
			}
			d.serve(w, r, d.html)
		case p == "/api/v1/stats", strings.HasPrefix(p, "/assets/"), strings.HasPrefix(p, "/thumbnails/"):
			if r.Method != http.MethodGet && r.Method != http.MethodHead {
				w.Header().Set("Allow", "GET, HEAD")
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			if !local(r, false) {
				http.Error(w, "the dashboard answers requests from this machine only", http.StatusForbidden)
				return
			}
			switch {
			case p == "/api/v1/stats":
				d.serve(w, r, d.json)
			case strings.HasPrefix(p, "/assets/"):
				d.asset(w, r)
			default:
				d.thumbnail(w, r)
			}
		default:
			next.ServeHTTP(w, r)
		}
	})
}

func (d *Dashboard) serve(w http.ResponseWriter, r *http.Request, write func(http.ResponseWriter, *http.Request, *state)) {
	h := w.Header()
	h.Set("Content-Security-Policy", policy)
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "no-store")
	write(w, r, d.state.Load())
}

func (d *Dashboard) html(w http.ResponseWriter, r *http.Request, s *state) {
	var inv *Inventory
	var totals stats.Totals
	if s != nil {
		inv, totals = s.inv, s.totals
	}
	v := d.build(inv, totals, time.Now())
	if s != nil && s.logErr != nil {
		v.LogErr = s.logErr.Error()
	}
	var page bytes.Buffer
	if err := d.page.Execute(&page, v); err != nil {
		d.Logger.Error("dashboard: cannot render", "error", err)
		http.Error(w, "cannot render the dashboard", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(page.Bytes())
}

func (d *Dashboard) asset(w http.ResponseWriter, r *http.Request) {
	a, ok := d.assets.files[r.URL.Path]
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", a.kind)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(a.data))
}

// thumbnail serves a saved photograph to the page; unlike the photograph URL,
// it never downloads one that is missing.
func (d *Dashboard) thumbnail(w http.ResponseWriter, r *http.Request) {
	src, err := preview.Parse(preview.Prefix + strings.TrimPrefix(r.URL.Path, "/thumbnails/"))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if err != nil || r.URL.RawQuery != "" || !d.inImages(src) || !d.Store.ServeSaved(w, r, src) {
		http.NotFound(w, r)
	}
}

// inImages reports whether the photograph of src is a file in the image
// directory, not a link to one elsewhere; the directory itself may be a link.
func (d *Dashboard) inImages(src preview.Source) bool {
	root, err := filepath.EvalSymlinks(d.Store.ImageDir())
	if err != nil {
		return false
	}
	real, err := filepath.EvalSymlinks(d.Store.ImagePath(src))
	return err == nil && real == filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(src.Path(), preview.Prefix)))
}

// forwarded holds the headers that proxies add, besides X-Forwarded-*.
var forwarded = map[string]bool{
	"Forwarded": true, "Via": true, "X-Real-Ip": true, "X-Client-Ip": true, "True-Client-Ip": true,
	"Cf-Connecting-Ip": true, "Fastly-Client-Ip": true, "X-Original-Forwarded-For": true, "X-Envoy-External-Address": true,
}

// local reports whether r names this machine and came straight from it: a
// proxy adds forwarding headers, and a script of another site says so in
// Sec-Fetch-Site. A link from another site may still open the page.
func local(r *http.Request, page bool) bool {
	host, _, err := net.SplitHostPort(r.Host)
	if err != nil {
		host = r.Host
	}
	if !config.LocalHost(host) {
		return false
	}
	for name := range r.Header {
		if forwarded[name] || strings.HasPrefix(name, "X-Forwarded-") {
			return false
		}
	}
	// another port of a .localhost name is another origin of the same site
	if site := r.Header.Get("Sec-Fetch-Site"); site == "cross-site" || site == "same-site" {
		return page && r.Header.Get("Sec-Fetch-Mode") == "navigate" && r.Header.Get("Sec-Fetch-Dest") == "document"
	}
	return true
}

func dateText(t time.Time) string {
	t = t.Local()
	if t.Year() == time.Now().Year() {
		return t.Format("2 Jan, 15:04")
	}
	return t.Format("2 Jan 2006, 15:04")
}

// apiStats is the JSON of /api/v1/stats. Telegram IDs are strings: they do
// not fit the numbers of JavaScript.
type apiStats struct {
	Version     string      `json:"version"`
	Ready       bool        `json:"ready"`
	ScannedAt   *time.Time  `json:"scanned_at,omitempty"`
	ScanError   string      `json:"scan_error,omitempty"`
	StatsError  string      `json:"stats_error,omitempty"`
	Photographs apiPhotos   `json:"photographs"`
	Requests    apiRequests `json:"requests"`
	DiskFree    *int64      `json:"disk_free_bytes,omitempty"`
	Recent      []apiPhoto  `json:"recent"`
	Dialogs     []apiDialog `json:"dialogs"`
}

type apiPhotos struct {
	Saved   int   `json:"saved"`
	Bytes   int64 `json:"bytes"`
	Invalid int   `json:"invalid_files"`
}

type apiRequests struct {
	Since           *time.Time       `json:"since,omitempty"`
	Served          int64            `json:"served"`
	SentBytes       int64            `json:"sent_bytes"`
	Downloads       int64            `json:"downloads"`
	DownloadedBytes int64            `json:"downloaded_bytes"`
	Failures        map[string]int64 `json:"failures"`
	GoneNotSaved    int              `json:"gone_without_copy"`
	Dropped         int64            `json:"dropped_events"`
}

type apiPhoto struct {
	URL      string    `json:"url"`
	Channel  string    `json:"channel"`
	Message  int       `json:"message"`
	Photo    string    `json:"photo"`
	Bytes    int64     `json:"bytes"`
	SavedAt  time.Time `json:"saved_at"`
	Dialog   string    `json:"dialog"`
	Telegram string    `json:"telegram_url,omitempty"`
}

type apiDialog struct {
	Channel     string `json:"channel"`
	Title       string `json:"title"`
	Photographs int    `json:"photographs"`
	Bytes       int64  `json:"bytes"`
}

func (d *Dashboard) json(w http.ResponseWriter, r *http.Request, s *state) {
	out := apiStats{Version: d.Version, Recent: []apiPhoto{}, Dialogs: []apiDialog{}}
	out.Requests.Failures = map[string]int64{}
	out.Requests.Dropped = d.dropped()
	if s != nil {
		inv, totals := s.inv, s.totals
		out.Ready = true
		out.ScannedAt = &inv.At
		if inv.Err != nil {
			out.ScanError = inv.Err.Error()
		}
		if s.logErr != nil {
			out.StatsError = s.logErr.Error()
		}
		saved := map[string]bool{}
		dialogs := map[string]*apiDialog{}
		var order []string
		for _, photo := range inv.Photos {
			saved[photo.Source.Path()] = true
			out.Photographs.Bytes += photo.Size
			key := peerKey(photo.Source)
			if dialogs[key] == nil {
				dialogs[key] = &apiDialog{Channel: strconv.FormatInt(photo.Source.Channel, 10), Title: peerTitle(inv, photo.Source)}
				order = append(order, key)
			}
			dialogs[key].Photographs++
			dialogs[key].Bytes += photo.Size
		}
		for _, key := range order {
			out.Dialogs = append(out.Dialogs, *dialogs[key])
		}
		out.Photographs.Saved = len(inv.Photos)
		out.Photographs.Invalid = inv.Invalid
		if free, ok := diskFree(d.Store.ImageDir()); ok {
			n := int64(min(free, 1<<63-1))
			out.DiskFree = &n
		}
		for _, photo := range inv.Photos[:min(10, len(inv.Photos))] {
			out.Recent = append(out.Recent, apiPhoto{
				URL: d.Store.URL(photo.Source), Channel: strconv.FormatInt(photo.Source.Channel, 10),
				Message: photo.Source.Message, Photo: strconv.FormatInt(photo.Source.Photo, 10), Bytes: photo.Size,
				SavedAt: photo.Saved, Dialog: peerTitle(inv, photo.Source), Telegram: telegramLink(inv, photo.Source),
			})
		}
		if !totals.Since.IsZero() {
			out.Requests.Since = &totals.Since
		}
		out.Requests.Served, out.Requests.SentBytes = totals.Served, totals.Sent
		out.Requests.Downloads, out.Requests.DownloadedBytes = totals.Downloads, totals.Downloaded
		for status, n := range totals.Failures {
			out.Requests.Failures[strconv.Itoa(status)] = n
		}
		for p := range totals.Gone {
			if !saved[p] {
				out.Requests.GoneNotSaved++
			}
		}
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(append(data, '\n'))
}
