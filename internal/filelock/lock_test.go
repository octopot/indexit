package filelock

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestWaitCancellationAndRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.lock")
	release, e := Acquire(context.Background(), path)
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, e = Acquire(ctx, path)
	if !errors.Is(e, context.DeadlineExceeded) {
		t.Fatalf("%v", e)
	}
	release()
	release, e = Acquire(context.Background(), path)
	if e != nil {
		t.Fatal(e)
	}
	release()
}
