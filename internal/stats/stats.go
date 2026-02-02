// Package stats keeps the statistics log of the image service.
//
// The log is append-only: one JSON object per line in <dir>/YYYY-MM.jsonl,
// the month in UTC. Requests served from disk and failures are added up per
// minute; a download and a photograph Telegram no longer has are one line
// each. Several services on one data directory append to the same files, and
// every reader sees all of their lines. Nothing in it is needed to serve a
// photograph: a log that cannot be written only loses statistics.
package stats

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"go.octolab.org/toolset/indexit/internal/preview"
)

// Event kinds.
const (
	// Hits adds up the photographs served from disk in a minute.
	Hits = "hits"
	// Download is one photograph fetched from Telegram and saved.
	Download = "download"
	// Failures adds up the failed requests with one status in a minute.
	Failures = "failures"
	// Gone is a photograph Telegram answered it no longer has.
	Gone = "gone"
)

// Event is one line of the log.
type Event struct {
	At    time.Time `json:"at"`
	Event string    `json:"event"`
	// Count is the number of requests of Hits and Failures.
	Count int64 `json:"count,omitempty"`
	// Sent is the number of response body bytes of Hits and Download.
	Sent int64 `json:"sent,omitempty"`
	// Saved is the size of the photograph of a Download.
	Saved int64 `json:"saved,omitempty"`
	// Status is the HTTP status of Failures and of the response to a
	// Download.
	Status int `json:"status,omitempty"`
	// Path is the photograph of a Download or Gone, as in its URL.
	Path string `json:"path,omitempty"`
}

// Retention is how many months of the log a service keeps, the current one
// included.
const Retention = 13

var monthFile = regexp.MustCompile(`^(\d{4})-(\d{2})\.jsonl$`)

// Recorder turns the outcomes of photograph requests into log lines. Observe
// never blocks a request: lines that do not fit its queue are dropped.
type Recorder struct {
	dir string
	log *slog.Logger
	now func() time.Time

	mu       sync.Mutex
	hits     int64
	sent     int64
	failures map[int]int64
	gone     map[string]bool

	queue   chan Event
	dropped atomic.Int64
}

// NewRecorder returns a recorder that writes to dir.
func NewRecorder(dir string, log *slog.Logger) *Recorder {
	return &Recorder{dir: dir, log: log, now: time.Now, failures: map[int]int64{}, gone: map[string]bool{}, queue: make(chan Event, 256)}
}

// Observe records the outcome of one photograph request.
func (r *Recorder) Observe(o preview.Outcome) {
	path := o.Source.Path()
	switch o.Kind {
	case preview.Hit:
		r.mu.Lock()
		r.hits++
		r.sent += o.Sent
		r.mu.Unlock()
	case preview.Download:
		r.enqueue(Event{At: r.now().UTC(), Event: Download, Path: path, Status: o.Status, Sent: o.Sent, Saved: o.Downloaded})
	case preview.Failure:
		r.mu.Lock()
		r.failures[o.Status]++
		first := o.Status == http.StatusGone && !r.gone[path]
		if first {
			r.gone[path] = true
		}
		r.mu.Unlock()
		if first {
			r.enqueue(Event{At: r.now().UTC(), Event: Gone, Path: path})
		}
	}
}

// Dropped is the number of lines lost to a full queue.
func (r *Recorder) Dropped() int64 { return r.dropped.Load() }

func (r *Recorder) enqueue(e Event) {
	select {
	case r.queue <- e:
	default:
		r.dropped.Add(1)
	}
}

// Run writes the log until ctx ends, then writes what is left and returns.
// Outcomes observed after that are not written.
// It removes the months older than Retention when it starts and when a new
// month begins.
func (r *Recorder) Run(ctx context.Context) {
	r.prune()
	month := r.now().UTC().Format("2006-01")
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	var failed time.Time
	write := func(events []Event) {
		if err := r.write(events); err != nil && time.Since(failed) > time.Minute {
			failed = time.Now()
			r.log.Warn("stats: cannot write the log", "dir", r.dir, "error", err)
		}
	}
	for {
		select {
		case e := <-r.queue:
			write(append([]Event{e}, r.drain()...))
		case <-ticker.C:
			write(r.flush())
			month = r.rollover(month)
		case <-ctx.Done():
			write(append(r.drain(), r.flush()...))
			return
		}
	}
}

// rollover removes the months older than Retention once a new month begins
// and returns the current one.
func (r *Recorder) rollover(month string) string {
	if now := r.now().UTC().Format("2006-01"); now != month {
		r.prune()
		return now
	}
	return month
}

func (r *Recorder) drain() []Event {
	var events []Event
	for {
		select {
		case e := <-r.queue:
			events = append(events, e)
		default:
			return events
		}
	}
}

// flush takes the sums of the minute as events.
func (r *Recorder) flush() []Event {
	at := r.now().UTC()
	r.mu.Lock()
	defer r.mu.Unlock()
	var events []Event
	if r.hits > 0 {
		events = append(events, Event{At: at, Event: Hits, Count: r.hits, Sent: r.sent})
	}
	for _, status := range slices.Sorted(maps.Keys(r.failures)) {
		events = append(events, Event{At: at, Event: Failures, Status: status, Count: r.failures[status]})
	}
	r.hits, r.sent = 0, 0
	clear(r.failures)
	return events
}

// write appends events to the files of their months, each file with a
// single write.
func (r *Recorder) write(events []Event) error {
	if len(events) == 0 {
		return nil
	}
	files := map[string]*bytes.Buffer{}
	for _, e := range events {
		name := e.At.UTC().Format("2006-01") + ".jsonl"
		if files[name] == nil {
			files[name] = new(bytes.Buffer)
		}
		if err := json.NewEncoder(files[name]).Encode(e); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(r.dir, 0o700); err != nil {
		return err
	}
	var errs []error
	for name, lines := range files {
		f, err := os.OpenFile(filepath.Join(r.dir, name), os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		_, err = f.Write(lines.Bytes())
		errs = append(errs, err, f.Close())
	}
	return errors.Join(errs...)
}

func (r *Recorder) prune() {
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		return
	}
	oldest := oldestMonth(r.now())
	for _, entry := range entries {
		if monthFile.MatchString(entry.Name()) && entry.Name()[:7] < oldest {
			if err := os.Remove(filepath.Join(r.dir, entry.Name())); err != nil {
				r.log.Warn("stats: cannot remove an old month", "file", entry.Name(), "error", err)
			}
		}
	}
}

// Totals adds up the log.
type Totals struct {
	// Since is the time of the first line, zero for an empty log.
	Since time.Time
	// Served counts the photographs served, from disk or just downloaded.
	Served int64
	// Sent is the number of response body bytes of the photographs served.
	Sent int64
	// Downloads counts the photographs fetched from Telegram.
	Downloads int64
	// Downloaded is the size of the photographs fetched from Telegram.
	Downloaded int64
	// Failures counts the failed requests by HTTP status.
	Failures map[int]int64
	// Gone holds the photographs Telegram answered it no longer has.
	Gone map[string]bool
	// Malformed counts the lines that are not events.
	Malformed int64
}

// Reader adds up the log as it grows: each Update reads only the complete
// lines appended since the last one. A month file that is replaced, cut short
// or removed counts anew or no more, and so do the months older than
// Retention.
type Reader struct {
	dir   string
	now   func() time.Time
	mu    sync.Mutex
	files map[string]*monthLog
}

// monthLog is what one month file adds up to so far.
type monthLog struct {
	info   os.FileInfo
	offset int64
	totals Totals
}

// NewReader returns a reader of the log in dir.
func NewReader(dir string) *Reader {
	return &Reader{dir: dir, now: time.Now, files: map[string]*monthLog{}}
}

func newTotals() Totals { return Totals{Failures: map[int]int64{}, Gone: map[string]bool{}} }

// Update reads the new lines and returns the totals. A month that cannot be
// read is an error; it keeps what it counted before, and the others count.
func (r *Reader) Update() (Totals, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	entries, err := os.ReadDir(r.dir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return r.sum(), err
	}
	oldest := oldestMonth(r.now())
	seen := map[string]bool{}
	var errs []error
	for _, entry := range entries {
		name := entry.Name()
		if !monthFile.MatchString(name) || !entry.Type().IsRegular() || name[:7] < oldest {
			continue
		}
		seen[name] = true
		errs = append(errs, r.read(name))
	}
	for name := range r.files {
		if !seen[name] {
			delete(r.files, name)
		}
	}
	return r.sum(), errors.Join(errs...)
}

func (r *Reader) read(name string) error {
	f, err := os.Open(filepath.Join(r.dir, name))
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	month := r.files[name]
	if month == nil || !os.SameFile(month.info, info) || info.Size() < month.offset {
		// a new file, or one replaced or cut short, as by a restore: count
		// it from the start
		month = &monthLog{totals: newTotals()}
		r.files[name] = month
	}
	month.info = info
	if _, err = f.Seek(month.offset, io.SeekStart); err != nil {
		return err
	}
	in := bufio.NewReader(f)
	for {
		line, err := in.ReadBytes('\n')
		if errors.Is(err, io.EOF) {
			// a line still being written counts once it is complete
			return nil
		}
		if err != nil {
			return err
		}
		month.offset += int64(len(line))
		month.totals.add(line)
	}
}

func (t *Totals) add(line []byte) {
	var e Event
	if err := json.Unmarshal(line, &e); err != nil || e.At.IsZero() {
		t.Malformed++
		return
	}
	switch e.Event {
	case Hits:
		t.Served += e.Count
		t.Sent += e.Sent
	case Download:
		t.Downloads++
		t.Downloaded += e.Saved
		// the photograph is saved even when its response failed, as for a
		// range it could not meet
		switch {
		case e.Status < http.StatusBadRequest:
			t.Served++
			t.Sent += e.Sent
		case preview.Failed(e.Status):
			t.Failures[e.Status]++
		}
	case Failures:
		t.Failures[e.Status] += e.Count
	case Gone:
		if _, err := preview.Parse(e.Path); err != nil {
			t.Malformed++
			return
		}
		t.Gone[e.Path] = true
	default:
		t.Malformed++
		return
	}
	if t.Since.IsZero() || e.At.Before(t.Since) {
		t.Since = e.At
	}
}

func (r *Reader) sum() Totals {
	total := newTotals()
	for _, month := range r.files {
		t := month.totals
		if total.Since.IsZero() || !t.Since.IsZero() && t.Since.Before(total.Since) {
			total.Since = t.Since
		}
		total.Served += t.Served
		total.Sent += t.Sent
		total.Downloads += t.Downloads
		total.Downloaded += t.Downloaded
		total.Malformed += t.Malformed
		for status, n := range t.Failures {
			total.Failures[status] += n
		}
		for path := range t.Gone {
			total.Gone[path] = true
		}
	}
	return total
}

// oldestMonth is the first month of the log Retention keeps at now, as in
// its file names.
func oldestMonth(now time.Time) string {
	now = now.UTC()
	return time.Date(now.Year(), now.Month()-Retention+1, 1, 0, 0, 0, 0, time.UTC).Format("2006-01")
}
