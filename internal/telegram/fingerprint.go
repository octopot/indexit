package telegram

import (
	"context"
	"encoding/hex"
	"sync"

	gotdsession "github.com/gotd/td/session"
)

// Fingerprinter tells which session a running client uses.
type Fingerprinter interface {
	// Fingerprint is the hex auth key ID of the session, or "" when unknown.
	Fingerprint() string
}

// SessionRecorder is the session storage handed to gotd. It delegates to the
// real storage and remembers the auth key ID of the session the client
// actually runs on: the one of the first load, the restore, until the client
// stores another. Later loads, such as gotd's before it saves a session, pass
// through without changing it: the file may be mid-write or replaced by then.
// The key ID is not secret: it travels in the clear with every request of the
// session.
type SessionRecorder struct {
	storage gotdsession.Storage

	mu       sync.Mutex
	restored bool
	keyID    string
}

func NewSessionRecorder(storage gotdsession.Storage) *SessionRecorder {
	return &SessionRecorder{storage: storage}
}

// LoadSession records only the first load. A first load without a session
// leaves the fingerprint unknown.
func (r *SessionRecorder) LoadSession(ctx context.Context) ([]byte, error) {
	data, err := r.storage.LoadSession(ctx)
	r.mu.Lock()
	first := !r.restored
	r.restored = true
	r.mu.Unlock()
	if first && err == nil {
		r.record(ctx, data)
	}
	return data, err
}

// StoreSession records the session even when the write fails: the client
// keeps running on it either way.
func (r *SessionRecorder) StoreSession(ctx context.Context, data []byte) error {
	r.record(ctx, data)
	return r.storage.StoreSession(ctx, data)
}

func (r *SessionRecorder) Fingerprint() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.keyID
}

func (r *SessionRecorder) record(ctx context.Context, data []byte) {
	keyID := sessionKeyID(ctx, data)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.keyID = keyID
}

// sessionKeyID decodes the auth key ID out of encoded session data.
func sessionKeyID(ctx context.Context, data []byte) string {
	var memory gotdsession.StorageMemory
	if err := memory.StoreSession(ctx, data); err != nil {
		return ""
	}
	decoded, err := (&gotdsession.Loader{Storage: &memory}).Load(ctx)
	if err != nil || len(decoded.AuthKeyID) == 0 {
		return ""
	}
	return hex.EncodeToString(decoded.AuthKeyID)
}
