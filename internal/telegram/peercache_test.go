package telegram

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	gotdsession "github.com/gotd/td/session"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.octolab.org/toolset/indexit/internal/telegram/peers"
	"go.octolab.org/toolset/indexit/internal/telegram/uid"
)

// accountAPI answers the self lookup as the given account and counts it.
type accountAPI struct {
	fakeAPI

	id    int64
	err   error
	calls int
}

func (f *accountAPI) UsersGetUsers(_ context.Context, ids []tg.InputUserClass) ([]tg.UserClass, error) {
	f.calls++
	if len(ids) != 1 {
		return nil, tgerr.New(400, "USER_ID_INVALID")
	}
	if _, ok := ids[0].(*tg.InputUserSelf); !ok {
		return nil, tgerr.New(400, "USER_ID_INVALID")
	}
	if f.err != nil {
		return nil, f.err
	}
	return []tg.UserClass{&tg.User{ID: f.id, AccessHash: 1, Self: true}}, nil
}

// cacheWorld is a session file and a peer cache file side by side.
type cacheWorld struct {
	session string
	peers   string
	log     bytes.Buffer
}

func newCacheWorld(t *testing.T) *cacheWorld {
	dir := t.TempDir()
	return &cacheWorld{session: filepath.Join(dir, "session.json"), peers: filepath.Join(dir, "peers.json")}
}

// login writes a session whose auth key ID ends with key, as a login does.
func (w *cacheWorld) login(t *testing.T, key byte) string {
	t.Helper()
	saveSession(t, &gotdsession.FileStorage{Path: w.session}, key)
	return fmt.Sprintf("%016x", key)
}

func saveSession(t *testing.T, storage gotdsession.Storage, key byte) {
	t.Helper()
	id := []byte{0, 0, 0, 0, 0, 0, 0, key}
	require.NoError(t, (&gotdsession.Loader{Storage: storage}).Save(context.Background(),
		&gotdsession.Data{DC: 2, AuthKey: bytes.Repeat([]byte{key}, 256), AuthKeyID: id}))
}

// restore is what a client does on start: it loads the session file through
// the recorder.
func (w *cacheWorld) restore(t *testing.T) *SessionRecorder {
	t.Helper()
	recorder := NewSessionRecorder(&gotdsession.FileStorage{Path: w.session})
	_, err := recorder.LoadSession(context.Background())
	require.NoError(t, err)
	return recorder
}

func (w *cacheWorld) write(t *testing.T, cache *peers.Cache) {
	t.Helper()
	require.NoError(t, cache.Save(w.peers))
}

func (w *cacheWorld) read(t *testing.T) *peers.Cache {
	t.Helper()
	cache, err := peers.Load(w.peers)
	require.NoError(t, err)
	return cache
}

// run is one command run: restore the session, open, bind, learn a peer,
// save.
func (w *cacheWorld) run(t *testing.T, api API) (*peers.Cache, bool) {
	t.Helper()
	return w.runOn(t, w.restore(t), api)
}

func (w *cacheWorld) runOn(t *testing.T, session Fingerprinter, api API) (*peers.Cache, bool) {
	t.Helper()
	pc, err := OpenPeerCache(w.peers, session, slog.New(slog.NewTextHandler(&w.log, nil)))
	require.NoError(t, err)
	cache, err := pc.Bind(context.Background(), api, RateGuard{})
	require.NoError(t, err)
	cache.Put(peers.Entry{Kind: uid.KindChannel, ID: 99, AccessHash: 199})
	saved, err := pc.Save()
	require.NoError(t, err)
	return cache, saved
}

func ownedCache(account int64, fingerprint string) *peers.Cache {
	cache := peers.New()
	cache.Account = &peers.Account{ID: account, AuthKeyID: fingerprint}
	cache.Put(peers.Entry{Kind: uid.KindUser, ID: 50, AccessHash: 150})
	return cache
}

func TestPeerCacheFingerprintHitCostsNoRequest(t *testing.T) {
	w := newCacheWorld(t)
	fingerprint := w.login(t, 1)
	w.write(t, ownedCache(7, fingerprint))
	api := &accountAPI{id: 7}

	cache, saved := w.run(t, api)
	assert.Zero(t, api.calls, "an unchanged session needs no self lookup")
	assert.True(t, saved)
	_, ok := cache.Get(uid.KindUser, 50)
	assert.True(t, ok)
	assert.NotContains(t, w.log.String(), "level=WARN")
}

func TestPeerCacheSameAccountAfterRelogin(t *testing.T) {
	w := newCacheWorld(t)
	old := w.login(t, 1)
	w.write(t, ownedCache(7, old))
	fresh := w.login(t, 2)
	api := &accountAPI{id: 7}

	w.run(t, api)
	assert.Equal(t, 1, api.calls)
	stored := w.read(t)
	assert.Equal(t, &peers.Account{ID: 7, AuthKeyID: fresh}, stored.Account, "the stamp follows the new session")
	_, ok := stored.Get(uid.KindUser, 50)
	assert.True(t, ok, "the same account keeps its cache across a re-login")
	assert.NotContains(t, w.log.String(), "level=WARN")
}

func TestPeerCacheAnotherAccountStartsEmpty(t *testing.T) {
	w := newCacheWorld(t)
	w.write(t, ownedCache(7, w.login(t, 1)))
	fingerprint := w.login(t, 2)
	api := &accountAPI{id: 8}

	cache, saved := w.run(t, api)
	assert.True(t, saved)
	_, ok := cache.Get(uid.KindUser, 50)
	assert.False(t, ok, "a foreign access hash never reaches a fetcher")
	assert.Contains(t, w.log.String(), "belongs to another account")
	assert.Contains(t, w.log.String(), "fetch dialogs")
	assert.Contains(t, w.log.String(), "owner=7 account=8")

	stored := w.read(t)
	assert.Equal(t, &peers.Account{ID: 8, AuthKeyID: fingerprint}, stored.Account)
	assert.Equal(t, 1, stored.Len(), "only what this account learnt is kept")
}

func TestPeerCacheOwnerlessCacheFromAnotherAccount(t *testing.T) {
	w := newCacheWorld(t)
	// Written by account A before the owner was recorded, now loaded under B.
	legacy := peers.New()
	legacy.Put(peers.Entry{Kind: uid.KindUser, ID: 50, AccessHash: 150})
	w.write(t, legacy)
	fingerprint := w.login(t, 2)
	api := &accountAPI{id: 8}

	cache, saved := w.run(t, api)
	assert.True(t, saved)
	_, ok := cache.Get(uid.KindUser, 50)
	assert.False(t, ok, "an ownerless cache is not trusted")
	assert.Contains(t, w.log.String(), "records no account")
	assert.Equal(t, &peers.Account{ID: 8, AuthKeyID: fingerprint}, w.read(t).Account)
}

func TestPeerCacheNewFileIsStampedQuietly(t *testing.T) {
	w := newCacheWorld(t)
	fingerprint := w.login(t, 1)
	api := &accountAPI{id: 7}

	w.run(t, api)
	assert.Equal(t, 1, api.calls)
	assert.NotContains(t, w.log.String(), "level=WARN")
	assert.Equal(t, &peers.Account{ID: 7, AuthKeyID: fingerprint}, w.read(t).Account)
}

func TestPeerCacheUnknownAccountRunsWithoutCacheAndSavesNothing(t *testing.T) {
	w := newCacheWorld(t)
	w.write(t, ownedCache(7, "stale"))
	before, err := os.ReadFile(w.peers)
	require.NoError(t, err)
	w.login(t, 2)
	api := &accountAPI{err: tgerr.New(401, "AUTH_KEY_UNREGISTERED")}

	cache, saved := w.run(t, api)
	assert.False(t, saved)
	_, ok := cache.Get(uid.KindUser, 50)
	assert.False(t, ok, "the run goes on with an empty cache")
	assert.Contains(t, w.log.String(), "cannot tell the account")
	after, err := os.ReadFile(w.peers)
	require.NoError(t, err)
	assert.Equal(t, before, after, "the file is left as is")
}

func TestPeerCacheSelfLookupOnlyOnFingerprintMiss(t *testing.T) {
	w := newCacheWorld(t)
	w.login(t, 1)
	api := &accountAPI{id: 7}

	w.run(t, api)
	w.run(t, api)
	w.run(t, api)
	assert.Equal(t, 1, api.calls, "runs on the same session trust the stamp")

	w.login(t, 2)
	w.run(t, api)
	w.run(t, api)
	assert.Equal(t, 2, api.calls, "a new session costs one lookup")
}

func TestPrintStatusShowsPeerCacheOwner(t *testing.T) {
	var out bytes.Buffer
	require.NoError(t, PrintStatus(&out, Status{
		SessionPath:    "session.json",
		Authorized:     true,
		User:           &tg.User{ID: 8, FirstName: "Bea"},
		PeerCachePath:  "peers.json",
		PeerCacheOwner: &peers.Account{ID: 7},
	}))
	assert.Contains(t, out.String(), "peer cache: peers.json\n")
	assert.Contains(t, out.String(), "peer cache account: 7 (another account: the next fetch starts it empty)\n")
}

func TestSessionRecorderTracksTheSessionInUse(t *testing.T) {
	w := newCacheWorld(t)
	w.login(t, 1)
	recorder := w.restore(t)
	assert.Equal(t, "0000000000000001", recorder.Fingerprint(), "the restored session")

	// The client stores a new session, e.g. after a key exchange.
	saveSession(t, recorder, 2)
	assert.Equal(t, "0000000000000002", recorder.Fingerprint(), "the stored session wins at once")

	// Another process replaces the file: the client still runs on its own key.
	w.login(t, 3)
	assert.Equal(t, "0000000000000002", recorder.Fingerprint())
}

func TestSessionRecorderConcurrentStore(t *testing.T) {
	w := newCacheWorld(t)
	w.login(t, 1)
	recorder := w.restore(t)
	done := make(chan error, 1)
	go func() {
		done <- (&gotdsession.Loader{Storage: recorder}).Save(context.Background(),
			&gotdsession.Data{DC: 2, AuthKey: bytes.Repeat([]byte{2}, 256), AuthKeyID: []byte{0, 0, 0, 0, 0, 0, 0, 2}})
	}()
	got := recorder.Fingerprint()
	assert.Contains(t, []string{"0000000000000001", "0000000000000002"}, got, "never a torn or empty read")
	require.NoError(t, <-done)
	assert.Equal(t, "0000000000000002", recorder.Fingerprint())
}

func TestPeerCacheUsesTheLoadedSessionNotTheFile(t *testing.T) {
	w := newCacheWorld(t)
	loaded := w.login(t, 1)
	w.write(t, ownedCache(7, "0000000000000003"))
	recorder := w.restore(t)
	// After restore the file is replaced with the session whose key the cache
	// was stamped with; trusting the file would skip the check.
	w.login(t, 3)
	api := &accountAPI{id: 8}

	cache, _ := w.runOn(t, recorder, api)
	assert.Equal(t, 1, api.calls, "the loaded session misses the stamp")
	_, ok := cache.Get(uid.KindUser, 50)
	assert.False(t, ok)
	assert.Equal(t, &peers.Account{ID: 8, AuthKeyID: loaded}, w.read(t).Account)
}

type noFingerprint struct{}

func (noFingerprint) Fingerprint() string { return "" }

func TestPeerCacheUnknownFingerprintIsNeverStamped(t *testing.T) {
	w := newCacheWorld(t)
	w.write(t, ownedCache(7, ""))
	api := &accountAPI{id: 7}

	w.runOn(t, noFingerprint{}, api)
	w.runOn(t, noFingerprint{}, api)
	assert.Equal(t, 2, api.calls, "an empty stamp never matches")
	stored := w.read(t)
	assert.Equal(t, &peers.Account{ID: 7}, stored.Account, "the owner is stamped without a key")
	_, ok := stored.Get(uid.KindUser, 50)
	assert.True(t, ok)
}

func TestSessionRecorderIgnoresLaterLoads(t *testing.T) {
	w := newCacheWorld(t)
	w.login(t, 1)
	recorder := w.restore(t)
	w.login(t, 2)
	// gotd loads the storage again before it saves the active session.
	_, err := recorder.LoadSession(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "0000000000000001", recorder.Fingerprint(), "the client still runs on the restored session")

	w.write(t, ownedCache(7, "0000000000000002"))
	api := &accountAPI{id: 8}
	w.runOn(t, recorder, api)
	assert.Equal(t, 1, api.calls, "the cache of the replaced file is not trusted")
	assert.Equal(t, &peers.Account{ID: 8, AuthKeyID: "0000000000000001"}, w.read(t).Account)

	saveSession(t, recorder, 3)
	assert.Equal(t, "0000000000000003", recorder.Fingerprint(), "a store still moves it")
}

func TestSessionRecorderFirstLoadWithoutSession(t *testing.T) {
	w := newCacheWorld(t)
	recorder := NewSessionRecorder(&gotdsession.FileStorage{Path: w.session})
	_, err := recorder.LoadSession(context.Background())
	require.Error(t, err)
	w.login(t, 1)
	_, err = recorder.LoadSession(context.Background())
	require.NoError(t, err)
	assert.Empty(t, recorder.Fingerprint(), "unknown until the client stores a session")
}
