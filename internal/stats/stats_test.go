package stats

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.octolab.org/toolset/indexit/internal/preview"
)

var (
	photo = preview.Source{Account: 42, Peer: "c123", Message: 7, Photo: 9}
	other = preview.Source{Account: 42, Peer: "c123", Message: 8, Photo: 10}
)

func discard() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// record runs a recorder on dir, feeds it outcomes and stops it.
func record(t *testing.T, dir string, at time.Time, outcomes ...preview.Outcome) *Recorder {
	t.Helper()
	r := NewRecorder(dir, discard())
	r.now = func() time.Time { return at }
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { r.Run(ctx); close(done) }()
	for _, o := range outcomes {
		r.Observe(o)
	}
	cancel()
	<-done
	return r
}

func TestRecorderAndReader(t *testing.T) {
	dir := t.TempDir()
	at := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	record(t, dir, at,
		preview.Outcome{Source: photo, Kind: preview.Download, Status: 200, Sent: 100, Downloaded: 100},
		preview.Outcome{Source: photo, Kind: preview.Hit, Status: 200, Sent: 100},
		preview.Outcome{Source: photo, Kind: preview.Hit, Status: 304},
		preview.Outcome{Source: other, Kind: preview.Failure, Status: 410, Sent: 20},
		preview.Outcome{Source: other, Kind: preview.Failure, Status: 410, Sent: 20},
		preview.Outcome{Source: other, Kind: preview.Failure, Status: 503, Sent: 20},
	)
	reader := NewReader(dir)
	totals, err := reader.Update()
	require.NoError(t, err)
	assert.Equal(t, at, totals.Since)
	assert.Equal(t, int64(3), totals.Served)
	assert.Equal(t, int64(200), totals.Sent)
	assert.Equal(t, int64(1), totals.Downloads)
	assert.Equal(t, int64(100), totals.Downloaded)
	assert.Equal(t, map[int]int64{410: 2, 503: 1}, totals.Failures)
	assert.Equal(t, map[string]bool{other.Path(): true}, totals.Gone, "a photograph gone twice is one line")
	assert.Zero(t, totals.Malformed)

	// A second service on the same directory: the reader sees its lines too,
	// and only those.
	record(t, dir, at.Add(time.Minute), preview.Outcome{Source: photo, Kind: preview.Hit, Status: 200, Sent: 100})
	totals, err = reader.Update()
	require.NoError(t, err)
	assert.Equal(t, int64(4), totals.Served)
	assert.Equal(t, int64(300), totals.Sent)
	assert.Equal(t, map[int]int64{410: 2, 503: 1}, totals.Failures)
}

func TestReaderWaitsForCompleteLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "2026-10.jsonl")
	line := `{"at":"2026-10-21T12:00:00Z","event":"hits","count":2,"sent":10}` + "\n"
	require.NoError(t, os.WriteFile(path, []byte(line+line[:20]), 0o600))
	reader := NewReader(dir)
	totals, err := reader.Update()
	require.NoError(t, err)
	assert.Equal(t, int64(2), totals.Served, "half a line is not read yet")

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
	require.NoError(t, err)
	_, err = f.WriteString(line[20:] + "not json\n" + `{"at":"2026-10-21T12:00:00Z","event":"gone","path":"../../etc"}` + "\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())
	totals, err = reader.Update()
	require.NoError(t, err)
	assert.Equal(t, int64(4), totals.Served, "the line counts once complete")
	assert.Equal(t, int64(2), totals.Malformed)
	assert.Empty(t, totals.Gone)
}

func TestReaderOfMissingLog(t *testing.T) {
	totals, err := NewReader(filepath.Join(t.TempDir(), "stats")).Update()
	require.NoError(t, err)
	assert.True(t, totals.Since.IsZero())
	assert.Zero(t, totals.Served)
}

func TestRecorderKeepsRetentionMonths(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"2025-09.jsonl", "2025-10.jsonl", "2026-10.jsonl", "notes.txt"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), nil, 0o600))
	}
	record(t, dir, time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC))
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	assert.Equal(t, []string{"2025-10.jsonl", "2026-10.jsonl", "notes.txt"}, names)
}

func TestRecorderNeverBlocks(t *testing.T) {
	r := NewRecorder(t.TempDir(), discard())
	var wg sync.WaitGroup
	for range 1000 {
		wg.Go(func() {
			r.Observe(preview.Outcome{Source: photo, Kind: preview.Download, Status: 200})
		})
	}
	wg.Wait()
	assert.Equal(t, int64(1000-cap(r.queue)), r.Dropped(), "nothing writes the queue, so the rest is dropped")
}

func TestRecorderSurvivesAnUnwritableLog(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(dir, nil, 0o600))
	r := record(t, dir, time.Now(), preview.Outcome{Source: photo, Kind: preview.Download, Status: 200})
	assert.Zero(t, r.Dropped())
}

func TestReaderFollowsRemovedAndReplacedMonths(t *testing.T) {
	dir := t.TempDir()
	reader := NewReader(dir)
	reader.now = func() time.Time { return time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC) }
	line := func(n int) string {
		return `{"at":"2026-10-21T12:00:00Z","event":"hits","count":` + string(rune('0'+n)) + `,"sent":1}` + "\n"
	}
	october, september := filepath.Join(dir, "2026-10.jsonl"), filepath.Join(dir, "2026-09.jsonl")
	require.NoError(t, os.WriteFile(october, []byte(line(2)+line(3)), 0o600))
	require.NoError(t, os.WriteFile(september, []byte(line(4)), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "2025-09.jsonl"), []byte(line(9)), 0o600))
	totals, err := reader.Update()
	require.NoError(t, err)
	assert.Equal(t, int64(9), totals.Served, "a month older than the retention does not count")

	// replaced by a shorter copy, as by a restore
	tmp := october + ".tmp"
	require.NoError(t, os.WriteFile(tmp, []byte(line(1)), 0o600))
	require.NoError(t, os.Rename(tmp, october))
	totals, err = reader.Update()
	require.NoError(t, err)
	assert.Equal(t, int64(5), totals.Served, "a replaced month counts anew")

	// replaced by a file of the same size
	require.NoError(t, os.WriteFile(tmp, []byte(line(7)), 0o600))
	require.NoError(t, os.Rename(tmp, october))
	totals, err = reader.Update()
	require.NoError(t, err)
	assert.Equal(t, int64(11), totals.Served)

	require.NoError(t, os.RemoveAll(dir))
	totals, err = reader.Update()
	require.NoError(t, err)
	assert.Zero(t, totals.Served, "deleting stats resets the counts")
	assert.True(t, totals.Since.IsZero())
}

func TestRecorderPrunesWhenAMonthBegins(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"2025-10.jsonl", "2026-10.jsonl"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), nil, 0o600))
	}
	r := NewRecorder(dir, discard())
	r.now = func() time.Time { return time.Date(2026, 10, 31, 23, 59, 0, 0, time.UTC) }
	assert.Equal(t, "2026-10", r.rollover("2026-10"))
	assert.FileExists(t, filepath.Join(dir, "2025-10.jsonl"), "still within the retention")
	r.now = func() time.Time { return time.Date(2026, 11, 1, 0, 0, 30, 0, time.UTC) }
	assert.Equal(t, "2026-11", r.rollover("2026-10"))
	assert.NoFileExists(t, filepath.Join(dir, "2025-10.jsonl"), "a running service prunes when a month begins")
	assert.FileExists(t, filepath.Join(dir, "2026-10.jsonl"))

	assert.Equal(t, "2025-10", oldestMonth(time.Date(2026, 10, 31, 23, 59, 0, 0, time.UTC)))
	assert.Equal(t, "2025-11", oldestMonth(time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)))
	assert.Equal(t, "2025-02", oldestMonth(time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)))
}

func TestColdRangeIsADownloadButNotServed(t *testing.T) {
	dir := t.TempDir()
	at := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	record(t, dir, at,
		preview.Outcome{Source: photo, Kind: preview.Download, Status: 416, Sent: 30, Downloaded: 100},
		preview.Outcome{Source: other, Kind: preview.Download, Status: 500, Sent: 25, Downloaded: 90},
	)
	totals, err := NewReader(dir).Update()
	require.NoError(t, err)
	assert.Equal(t, int64(2), totals.Downloads)
	assert.Equal(t, int64(190), totals.Downloaded)
	assert.Zero(t, totals.Served, "an error body is no photograph")
	assert.Zero(t, totals.Sent)
	assert.Equal(t, map[int]int64{500: 1}, totals.Failures)
}
