package telegram

import (
	"context"
	"errors"
	"log/slog"

	"github.com/gotd/td/tg"

	"go.octolab.org/toolset/indexit/internal/telegram/peers"
)

// RefillHint tells how to rebuild a peer cache that started empty.
const RefillHint = "run 'indexit telegram fetch dialogs' to refill it"

// PeerCache is the peer cache of one command run. Access hashes are issued
// per account, so its entries reach the fetchers only through Bind, which
// checks inside the running client that the cache belongs to the account of
// the session. It is saved only under an owner the run verified.
type PeerCache struct {
	path    string
	session Fingerprinter
	cache   *peers.Cache
	owner   *peers.Account
	log     *slog.Logger
}

// OpenPeerCache loads the peer cache at path for the client session told by
// session. Nothing in it is trusted until Bind.
func OpenPeerCache(path string, session Fingerprinter, log *slog.Logger) (*PeerCache, error) {
	cache, err := peers.Load(path)
	if err != nil {
		return nil, err
	}
	if log == nil {
		log = slog.Default()
	}
	return &PeerCache{path: path, session: session, cache: cache, log: log}, nil
}

// Path is where the cache lives.
func (c *PeerCache) Path() string { return c.path }

// Len is the number of entries the cache holds now.
func (c *PeerCache) Len() int { return c.cache.Len() }

// Bind returns the cache for the account of the running session.
//
// A cache stamped with the fingerprint of the session the client runs on is
// trusted without a request. Otherwise one users.getUsers call asks who the account
// is: the same account keeps its entries and the stamp is refreshed; another
// account, or a cache with no recorded owner, starts empty with a warning. An
// unknown fingerprint is never stamped, so the next run verifies again. If
// the account cannot be told, the run goes on with an empty cache that is
// never saved. Only the end of the context is an error.
func (c *PeerCache) Bind(ctx context.Context, api API, guard RateGuard) (*peers.Cache, error) {
	if c.owner != nil {
		return c.cache, nil
	}
	fingerprint := c.session.Fingerprint()
	if owner := c.cache.Account; owner != nil && fingerprint != "" && owner.AuthKeyID == fingerprint {
		c.owner = owner
		c.log.Info("cache: bound", "peers", c.cache.Len(), "account", owner.ID, "verified", "session")
		return c.cache, nil
	}

	self, err := currentAccount(ctx, api, guard)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		c.log.Warn("cache: cannot tell the account, running without the peer cache and leaving it as is",
			"path", c.path, "error", err)
		c.cache.Reset()
		return c.cache, nil
	}
	switch previous := c.cache.Account; {
	case previous != nil && previous.ID == self.ID:
	case previous != nil:
		c.log.Warn("cache: the peer cache belongs to another account, starting empty; "+RefillHint,
			"path", c.path, "owner", previous.ID, "account", self.ID)
		c.cache.Reset()
	case c.cache.Len() > 0:
		c.log.Warn("cache: the peer cache records no account, starting empty; "+RefillHint,
			"path", c.path, "account", self.ID)
		c.cache.Reset()
	}
	c.cache.Account = &peers.Account{ID: self.ID, AuthKeyID: fingerprint}
	c.owner = c.cache.Account
	c.log.Info("cache: bound", "peers", c.cache.Len(), "account", self.ID, "verified", "request")
	return c.cache, nil
}

// Save writes the cache when this run verified its owner and reports whether
// it did. Without a verified owner the file is left untouched.
func (c *PeerCache) Save() (bool, error) {
	if c.owner == nil {
		return false, nil
	}
	return true, c.cache.Save(c.path)
}

func currentAccount(ctx context.Context, api API, guard RateGuard) (*tg.User, error) {
	var users []tg.UserClass
	err := guard.Do(ctx, func(ctx context.Context) error {
		var err error
		users, err = api.UsersGetUsers(ctx, []tg.InputUserClass{&tg.InputUserSelf{}})
		return err
	})
	if err != nil {
		return nil, err
	}
	for _, u := range users {
		if user, ok := u.(*tg.User); ok {
			return user, nil
		}
	}
	return nil, errors.New("telegram returned no user for the account")
}
