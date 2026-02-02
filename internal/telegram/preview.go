package telegram

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"

	"go.octolab.org/toolset/indexit/internal/preview"
	"go.octolab.org/toolset/indexit/internal/telegram/model"
	"go.octolab.org/toolset/indexit/internal/telegram/peers"
	"go.octolab.org/toolset/indexit/internal/telegram/uid"
)

// PreviewWriter adds local URLs using the message metadata already returned by
// Telegram. A URL is a function of the message alone: giving one writes
// nothing. A nil store leaves library callers' existing behavior unchanged.
type PreviewWriter struct {
	Store *preview.Store
}

// photoSource names the photograph of msg. Only posts of channels and
// supergroups have one: a URL that any account can fill needs the IDs that
// are the same for every account, and private chats and basic groups number
// their messages per account. ok is false for them.
func photoSource(msg *tg.Message) (src preview.Source, ok bool, err error) {
	peer, isChannel := msg.PeerID.(*tg.PeerChannel)
	if !isChannel {
		return src, false, nil
	}
	src = preview.Source{Channel: peer.ChannelID, Message: msg.ID}
	m, isPhoto := msg.Media.(*tg.MessageMediaPhoto)
	if !isPhoto || m.TTLSeconds != 0 {
		return src, true, errors.New("not a persistent cloud photograph")
	}
	photo, isPhoto := m.Photo.(*tg.Photo)
	if !isPhoto {
		return src, true, errors.New("photograph unavailable")
	}
	if size, _ := largestPhotoSize(photo); size == "" {
		return src, true, errors.New("photograph has no downloadable size")
	}
	src.Photo = photo.ID
	if _, err = preview.Parse(src.Path()); err != nil {
		return src, true, err
	}
	return src, true, nil
}

// Enrich adds the URL of the photograph of msg to rec.
func (p *PreviewWriter) Enrich(msg *tg.Message, rec model.MessageRecord) model.MessageRecord {
	if p == nil || p.Store == nil || rec.Media == nil || rec.Media.Type != "photo" {
		return rec
	}
	src, ok, err := photoSource(msg)
	switch {
	case !ok:
	case err != nil:
		rec.Media.PreviewError = err.Error()
	default:
		rec.Media.PreviewURL = p.Store.URL(src)
	}
	return rec
}

// Previews fills the photographs the local service has not saved yet. It
// finds a channel by itself, in this order: a channel it already found, the
// CLI's peer cache, which it only reads, the dialogs of the account with the
// archive, and the channel's cached username resolved and checked against
// its ID. What it found belongs to the account of the session.
type Previews struct {
	// Peers is the path of the CLI's peer cache.
	Peers string
	Log   *slog.Logger

	mu sync.Mutex
	// session is the fingerprint of the session the access hashes belong to.
	session string
	self    int64
	known   map[int64]int64
	// missed holds when a lookup of the dialogs last failed for a channel,
	// so that the photographs of one post do not read them again each.
	missed map[int64]time.Time
}

// missTTL is how long a channel the dialogs did not have is not looked for
// there again.
const missTTL = time.Minute

// Download writes the photograph of src to path. It reads the post again
// and checks that it still holds the photograph: preview.ErrSourceGone when
// the post or the photograph is gone or replaced, preview.ErrNotFound when
// the account cannot find the channel.
func (p *Previews) Download(ctx context.Context, api MediaAPI, session Fingerprinter, src preview.Source, path string, guard RateGuard) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if fp := session.Fingerprint(); fp != p.session || p.known == nil {
		p.session, p.self = fp, 0
		p.known, p.missed = map[int64]int64{}, map[int64]time.Time{}
	}
	tried := map[int64]bool{}
	try := func(hash int64) (bool, error) {
		if hash == 0 || tried[hash] {
			return false, nil
		}
		tried[hash] = true
		err := p.read(ctx, api, src, hash, path, guard)
		if tgerr.Is(err, "CHANNEL_INVALID", "CHANNEL_PRIVATE") {
			delete(p.known, src.Channel)
			return false, nil
		}
		if err == nil || errors.Is(err, preview.ErrSourceGone) {
			p.known[src.Channel] = hash
		}
		return true, err
	}
	if done, err := try(p.known[src.Channel]); done {
		return err
	}
	username := ""
	if cache, err := peers.Load(p.Peers); err != nil {
		p.logger().Warn("preview: cannot read the peer cache", "path", p.Peers, "error", err)
	} else if entry, ok := cache.Get(uid.KindChannel, src.Channel); ok {
		username = entry.Username
		if entry.AccessHash != 0 && p.owns(ctx, api, cache.Account, guard) {
			if done, err := try(entry.AccessHash); done {
				return err
			}
		}
	}
	if at, ok := p.missed[src.Channel]; ok && time.Since(at) < missTTL {
		return preview.ErrNotFound
	}
	if err := p.scanDialogs(ctx, api, src.Channel, guard); err != nil {
		return err
	}
	if done, err := try(p.known[src.Channel]); done {
		return err
	}
	if username != "" {
		hash, err := p.resolve(ctx, api, username, src.Channel, guard)
		if err != nil {
			return err
		}
		if done, err := try(hash); done {
			return err
		}
	}
	p.missed[src.Channel] = time.Now()
	return preview.ErrNotFound
}

func (p *Previews) logger() *slog.Logger {
	if p.Log == nil {
		return slog.Default()
	}
	return p.Log
}

// owns reports whether the peer cache of owner belongs to the account of the
// session: by the fingerprint the CLI stamped it with, or else by asking who
// the account is, once per session.
func (p *Previews) owns(ctx context.Context, api API, owner *peers.Account, guard RateGuard) bool {
	if owner == nil {
		return false
	}
	if p.session != "" && owner.AuthKeyID == p.session {
		return true
	}
	if p.self == 0 {
		self, err := currentAccount(ctx, api, guard)
		if err != nil {
			return false
		}
		p.self = self.ID
	}
	return owner.ID == p.self
}

// scanDialogs reads the dialogs of the account, the archive included, until
// it meets channel, and keeps the access hash of every channel it meets.
func (p *Previews) scanDialogs(ctx context.Context, api API, channel int64, guard RateGuard) error {
	for _, folder := range []int{0, 1} {
		req := &tg.MessagesGetDialogsRequest{OffsetPeer: &tg.InputPeerEmpty{}, Limit: defaultPageSize, FolderID: folder}
		for {
			var result tg.MessagesDialogsClass
			err := guard.Do(ctx, func(ctx context.Context) error {
				var err error
				result, err = api.MessagesGetDialogs(ctx, req)
				return err
			})
			if err != nil {
				return err
			}
			modified, ok := result.AsModified()
			if !ok {
				break
			}
			for _, chat := range modified.GetChats() {
				if c, ok := chat.(*tg.Channel); ok && !c.Min && c.AccessHash != 0 {
					p.known[c.ID] = c.AccessHash
				}
			}
			if _, found := p.known[channel]; found {
				return nil
			}
			dialogs := modified.GetDialogs()
			if _, slice := result.(*tg.MessagesDialogsSlice); !slice || len(dialogs) == 0 {
				break
			}
			last, ok := dialogs[len(dialogs)-1].(dialogRef)
			if !ok {
				break
			}
			entities := entitiesFromLists(modified.GetUsers(), modified.GetChats())
			next := &tg.MessagesGetDialogsRequest{OffsetPeer: inputPeerForOffset(last.GetPeer(), entities), OffsetID: last.GetTopMessage(), Limit: defaultPageSize, FolderID: folder}
			// IDs repeat across channels: the top message is the one of the
			// dialog's peer.
			for _, m := range modified.GetMessages() {
				if m, ok := m.AsNotEmpty(); ok && m.GetID() == next.OffsetID && samePeer(m.GetPeerID(), last.GetPeer()) {
					next.OffsetDate = m.GetDate()
				}
			}
			// an offset peer that cannot be addressed is empty on both pages
			if next.OffsetID == req.OffsetID && next.OffsetDate == req.OffsetDate && peerName(peerOf(next.OffsetPeer)) == peerName(peerOf(req.OffsetPeer)) {
				break
			}
			req = next
		}
	}
	return nil
}

// resolve returns the access hash of the channel username names, when it is
// still channel; a username can move to another chat.
func (p *Previews) resolve(ctx context.Context, api API, username string, channel int64, guard RateGuard) (int64, error) {
	var result *tg.ContactsResolvedPeer
	err := guard.Do(ctx, func(ctx context.Context) error {
		var err error
		result, err = api.ContactsResolveUsername(ctx, &tg.ContactsResolveUsernameRequest{Username: username})
		return err
	})
	if tgerr.Is(err, "USERNAME_NOT_OCCUPIED", "USERNAME_INVALID") {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	for _, chat := range result.Chats {
		if c, ok := chat.(*tg.Channel); ok && c.ID == channel && !c.Min {
			return c.AccessHash, nil
		}
	}
	return 0, nil
}

// read downloads the photograph of src from its post, addressing the
// channel with hash. It refreshes the file reference and checks the
// immutable photo ID.
func (p *Previews) read(ctx context.Context, api MediaAPI, src preview.Source, hash int64, path string, guard RateGuard) error {
	var result tg.MessagesMessagesClass
	err := guard.Do(ctx, func(ctx context.Context) error {
		var e error
		result, e = api.ChannelsGetMessages(ctx, &tg.ChannelsGetMessagesRequest{
			Channel: &tg.InputChannel{ChannelID: src.Channel, AccessHash: hash},
			ID:      []tg.InputMessageClass{&tg.InputMessageID{ID: src.Message}},
		})
		return e
	})
	if err != nil {
		return err
	}
	modified, ok := result.AsModified()
	if !ok {
		return preview.ErrSourceGone
	}
	for _, m := range modified.GetMessages() {
		msg, ok := m.(*tg.Message)
		if !ok || msg.ID != src.Message {
			continue
		}
		if peer, ok := msg.PeerID.(*tg.PeerChannel); !ok || peer.ChannelID != src.Channel {
			continue
		}
		media, ok := msg.Media.(*tg.MessageMediaPhoto)
		if !ok || media.TTLSeconds != 0 {
			return preview.ErrSourceGone
		}
		photo, ok := media.Photo.(*tg.Photo)
		if !ok || photo.ID != src.Photo {
			return preview.ErrSourceGone
		}
		target := resolveTarget(msg.ID, media)
		if target == nil {
			return errors.New("source photograph unavailable")
		}
		n, err := download(ctx, downloader.NewDownloader(), api, guard, target.location, path)
		if err != nil {
			return err
		}
		if n <= 0 || (target.size > 0 && n != target.size) {
			return errors.New("incomplete photograph download")
		}
		return nil
	}
	return preview.ErrSourceGone
}

func samePeer(a, b tg.PeerClass) bool {
	x, y := peerName(a), peerName(b)
	return x != "" && x == y
}

// peerName is the UID of peer, empty for none.
func peerName(peer tg.PeerClass) string {
	name, _ := canonicalUID(peer)
	return name
}

// peerOf is the peer an offset of messages.getDialogs names, nil for none.
func peerOf(input tg.InputPeerClass) tg.PeerClass {
	switch p := input.(type) {
	case *tg.InputPeerChannel:
		return &tg.PeerChannel{ChannelID: p.ChannelID}
	case *tg.InputPeerChat:
		return &tg.PeerChat{ChatID: p.ChatID}
	case *tg.InputPeerUser:
		return &tg.PeerUser{UserID: p.UserID}
	}
	return nil
}
