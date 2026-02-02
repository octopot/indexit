package telegram

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.octolab.org/toolset/indexit/internal/preview"
	"go.octolab.org/toolset/indexit/internal/telegram/model"
	"go.octolab.org/toolset/indexit/internal/telegram/peers"
	"go.octolab.org/toolset/indexit/internal/telegram/uid"
)

func testPhoto(id int, group int64) *tg.Message {
	return &tg.Message{ID: id, GroupedID: group, PeerID: &tg.PeerChannel{ChannelID: 123}, Media: &tg.MessageMediaPhoto{Photo: &tg.Photo{ID: int64(-100 - id), AccessHash: 7, FileReference: []byte{1}, Sizes: []tg.PhotoSizeClass{&tg.PhotoSize{Type: "w", W: 1280, H: 900, Size: 4}}}}}
}

func TestPreviewURLNamesTheChannelPostAndPhoto(t *testing.T) {
	root := t.TempDir()
	p := &PreviewWriter{Store: &preview.Store{Root: root}}
	rec := p.Enrich(testPhoto(7, 0), model.MessageRecord{Media: &model.MediaDescriptor{Type: "photo"}})
	assert.Equal(t, preview.DefaultOrigin+"/preview/telegram/v1/c123/7/p-107.jpg", rec.Media.PreviewURL, "nothing identifies the account; the photo ID keeps its sign")
	assert.Empty(t, rec.Media.PreviewError)
	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	assert.Empty(t, entries, "giving a URL writes nothing")
}

func TestPreviewLeavesOutChatsAndUsers(t *testing.T) {
	p := &PreviewWriter{Store: &preview.Store{Root: t.TempDir()}}
	for _, peer := range []tg.PeerClass{&tg.PeerChat{ChatID: 5}, &tg.PeerUser{UserID: 6}} {
		msg := testPhoto(7, 0)
		msg.PeerID = peer
		rec := p.Enrich(msg, model.MessageRecord{Media: &model.MediaDescriptor{Type: "photo"}})
		assert.Empty(t, rec.Media.PreviewURL, "%T", peer)
		assert.Empty(t, rec.Media.PreviewError, "%T is not a failure", peer)
	}
}

func TestPreviewRejectsEphemeralPhotos(t *testing.T) {
	msg := testPhoto(7, 0)
	msg.Media.(*tg.MessageMediaPhoto).TTLSeconds = 5
	rec := (&PreviewWriter{Store: &preview.Store{}}).Enrich(msg, model.MessageRecord{Media: &model.MediaDescriptor{Type: "photo"}})
	assert.Empty(t, rec.Media.PreviewURL)
	assert.NotEmpty(t, rec.Media.PreviewError)
}

type session string

func (s session) Fingerprint() string { return string(s) }

// channelAPI serves the posts of channel 123 to the access hash 555 only,
// as Telegram does to one account.
type channelAPI struct {
	fileAPI

	posts    map[int]*tg.Message
	self     int64
	dialogs  map[int][]tg.MessagesDialogsClass
	username map[string]*tg.ContactsResolvedPeer

	reads, lookups, resolves, selves int
}

func (f *channelAPI) ChannelsGetMessages(_ context.Context, req *tg.ChannelsGetMessagesRequest) (tg.MessagesMessagesClass, error) {
	f.reads++
	if ch := req.Channel.(*tg.InputChannel); ch.ChannelID != 123 || ch.AccessHash != 555 {
		return nil, tgerr.New(400, "CHANNEL_INVALID")
	}
	out := &tg.MessagesChannelMessages{}
	for _, id := range req.ID {
		if msg, ok := f.posts[id.(*tg.InputMessageID).ID]; ok {
			out.Messages = append(out.Messages, msg)
		} else {
			out.Messages = append(out.Messages, &tg.MessageEmpty{ID: id.(*tg.InputMessageID).ID})
		}
	}
	return out, nil
}

func (f *channelAPI) MessagesGetDialogs(_ context.Context, req *tg.MessagesGetDialogsRequest) (tg.MessagesDialogsClass, error) {
	f.lookups++
	pages := f.dialogs[req.FolderID]
	if len(pages) == 0 {
		return &tg.MessagesDialogs{}, nil
	}
	f.dialogs[req.FolderID] = pages[1:]
	return pages[0], nil
}

func (f *channelAPI) ContactsResolveUsername(_ context.Context, req *tg.ContactsResolveUsernameRequest) (*tg.ContactsResolvedPeer, error) {
	f.resolves++
	if r, ok := f.username[req.Username]; ok {
		return r, nil
	}
	return nil, tgerr.New(400, "USERNAME_NOT_OCCUPIED")
}

func (f *channelAPI) UsersGetUsers(context.Context, []tg.InputUserClass) ([]tg.UserClass, error) {
	f.selves++
	return []tg.UserClass{&tg.User{ID: f.self}}, nil
}

func newChannelAPI() *channelAPI {
	return &channelAPI{fileAPI: fileAPI{content: []byte{0xFF, 0xD8, 0xFF, 0xD9}}, posts: map[int]*tg.Message{7: testPhoto(7, 0)}, self: 42, dialogs: map[int][]tg.MessagesDialogsClass{}}
}

var post = preview.Source{Channel: 123, Message: 7, Photo: -107}

func peerCache(t *testing.T, account peers.Account, entry peers.Entry) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "peers.json")
	cache := peers.New()
	cache.Account = &account
	cache.Put(entry)
	require.NoError(t, cache.Save(path))
	return path
}

func fill(t *testing.T, p *Previews, api *channelAPI, src preview.Source) error {
	t.Helper()
	path := filepath.Join(t.TempDir(), "photo")
	err := p.Download(t.Context(), api, session("abc"), src, path, RateGuard{})
	if err == nil {
		data, e := os.ReadFile(path)
		require.NoError(t, e)
		assert.Equal(t, api.content, data)
	}
	return err
}

func TestPreviewsFromThePeerCacheOnlyReadIt(t *testing.T) {
	path := peerCache(t, peers.Account{ID: 42, AuthKeyID: "abc"}, peers.Entry{Kind: uid.KindChannel, ID: 123, AccessHash: 555})
	before, err := os.ReadFile(path)
	require.NoError(t, err)
	api := newChannelAPI()
	p := &Previews{Peers: path}
	require.NoError(t, fill(t, p, api, post))
	assert.Equal(t, 1, api.reads)
	assert.Zero(t, api.lookups+api.resolves+api.selves, "a cache of this session needs no lookup")
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, before, after, "the peer cache is only read")

	// the channel it already found needs not even the cache
	require.NoError(t, os.Remove(path))
	api.posts[8] = testPhoto(8, 0)
	require.NoError(t, fill(t, p, api, preview.Source{Channel: 123, Message: 8, Photo: -108}))
	assert.Equal(t, 2, api.reads)
	assert.Zero(t, api.lookups+api.resolves)
}

func TestPreviewsTrustACacheOfTheSameAccount(t *testing.T) {
	path := peerCache(t, peers.Account{ID: 42, AuthKeyID: "another session"}, peers.Entry{Kind: uid.KindChannel, ID: 123, AccessHash: 555})
	api := newChannelAPI()
	require.NoError(t, fill(t, &Previews{Peers: path}, api, post))
	assert.Equal(t, 1, api.selves, "the account is asked once")
	assert.Zero(t, api.lookups)
}

func TestPreviewsFindTheChannelInTheArchive(t *testing.T) {
	// a cache of another account: its access hash is worthless here
	path := peerCache(t, peers.Account{ID: 1, AuthKeyID: "zzz"}, peers.Entry{Kind: uid.KindChannel, ID: 123, AccessHash: 999})
	api := newChannelAPI()
	other := &tg.Channel{ID: 9, AccessHash: 1}
	api.dialogs[0] = []tg.MessagesDialogsClass{&tg.MessagesDialogs{Dialogs: []tg.DialogClass{&tg.Dialog{Peer: &tg.PeerChannel{ChannelID: 9}}}, Chats: []tg.ChatClass{other}}}
	api.dialogs[1] = []tg.MessagesDialogsClass{&tg.MessagesDialogs{Chats: []tg.ChatClass{&tg.Channel{ID: 123, AccessHash: 555}}}}
	require.NoError(t, fill(t, &Previews{Peers: path}, api, post))
	assert.Equal(t, 2, api.lookups, "the main list, then the archive")
	assert.Equal(t, 1, api.reads, "the hash of another account is never tried")
}

func TestPreviewsDropAStaleHash(t *testing.T) {
	path := peerCache(t, peers.Account{ID: 42, AuthKeyID: "abc"}, peers.Entry{Kind: uid.KindChannel, ID: 123, AccessHash: 999})
	api := newChannelAPI()
	api.dialogs[0] = []tg.MessagesDialogsClass{&tg.MessagesDialogs{Chats: []tg.ChatClass{&tg.Channel{ID: 123, AccessHash: 555}}}}
	require.NoError(t, fill(t, &Previews{Peers: path}, api, post))
	assert.Equal(t, 2, api.reads)
}

func TestPreviewsResolveTheCachedUsername(t *testing.T) {
	path := peerCache(t, peers.Account{ID: 1}, peers.Entry{Kind: uid.KindChannel, ID: 123, Username: "example"})
	api := newChannelAPI()
	api.username = map[string]*tg.ContactsResolvedPeer{"example": {Peer: &tg.PeerChannel{ChannelID: 123}, Chats: []tg.ChatClass{&tg.Channel{ID: 123, AccessHash: 555}}}}
	require.NoError(t, fill(t, &Previews{Peers: path}, api, post))
	assert.Equal(t, 1, api.resolves)

	// a username that moved to another channel is not followed
	api.username["example"] = &tg.ContactsResolvedPeer{Peer: &tg.PeerChannel{ChannelID: 124}, Chats: []tg.ChatClass{&tg.Channel{ID: 124, AccessHash: 555}}}
	err := fill(t, &Previews{Peers: path}, api, post)
	assert.ErrorIs(t, err, preview.ErrNotFound)
}

func TestPreviewsNotFoundIsRememberedForAMoment(t *testing.T) {
	api := newChannelAPI()
	p := &Previews{Peers: filepath.Join(t.TempDir(), "none.json")}
	assert.ErrorIs(t, fill(t, p, api, post), preview.ErrNotFound)
	assert.ErrorIs(t, fill(t, p, api, post), preview.ErrNotFound)
	assert.Equal(t, 2, api.lookups, "the photographs of one post read the dialogs once")
	assert.Zero(t, api.reads)
	assert.True(t, strings.Contains(preview.ErrNotFound.Error(), "fetch the post on this computer"))
}

func TestPreviewsGoneOrReplaced(t *testing.T) {
	path := peerCache(t, peers.Account{ID: 42, AuthKeyID: "abc"}, peers.Entry{Kind: uid.KindChannel, ID: 123, AccessHash: 555})
	api := newChannelAPI()
	replaced := post
	replaced.Photo = -999
	assert.ErrorIs(t, fill(t, &Previews{Peers: path}, api, replaced), preview.ErrSourceGone)
	deleted := post
	deleted.Message = 8
	assert.ErrorIs(t, fill(t, &Previews{Peers: path}, api, deleted), preview.ErrSourceGone)
	assert.Empty(t, api.reqs, "nothing is downloaded")
}

func TestPreviewsReturnOtherErrors(t *testing.T) {
	path := peerCache(t, peers.Account{ID: 42, AuthKeyID: "abc"}, peers.Entry{Kind: uid.KindChannel, ID: 123, AccessHash: 555})
	api := newChannelAPI()
	api.fail = true
	err := fill(t, &Previews{Peers: path}, api, post)
	require.Error(t, err)
	assert.False(t, errors.Is(err, preview.ErrNotFound) || errors.Is(err, preview.ErrSourceGone))
}

func TestPreviewsPageTheDialogsByPeer(t *testing.T) {
	api := newChannelAPI()
	// two pages whose last dialogs share a top message ID and date
	page := func(channel int64) *tg.MessagesDialogsSlice {
		return &tg.MessagesDialogsSlice{Count: 300,
			Dialogs:  []tg.DialogClass{&tg.Dialog{Peer: &tg.PeerChannel{ChannelID: channel}, TopMessage: 50}},
			Messages: []tg.MessageClass{&tg.Message{ID: 50, Date: 1000, PeerID: &tg.PeerChannel{ChannelID: 1}}, &tg.Message{ID: 50, Date: 2000, PeerID: &tg.PeerChannel{ChannelID: channel}}},
			Chats:    []tg.ChatClass{&tg.Channel{ID: channel, AccessHash: 1}}}
	}
	api.dialogs[0] = []tg.MessagesDialogsClass{page(8), page(9), &tg.MessagesDialogs{Chats: []tg.ChatClass{&tg.Channel{ID: 123, AccessHash: 555}}}}
	var reqs []*tg.MessagesGetDialogsRequest
	recorder := &dialogRecorder{channelAPI: api, reqs: &reqs}
	p := &Previews{Peers: filepath.Join(t.TempDir(), "none.json")}
	require.NoError(t, p.Download(t.Context(), recorder, session("abc"), post, filepath.Join(t.TempDir(), "photo"), RateGuard{}))
	require.Len(t, reqs, 3, "equal IDs and dates of other peers do not end the scan")
	assert.Equal(t, 2000, reqs[1].OffsetDate, "the date of the dialog's own top message")
}

type dialogRecorder struct {
	*channelAPI
	reqs *[]*tg.MessagesGetDialogsRequest
}

func (d *dialogRecorder) MessagesGetDialogs(ctx context.Context, req *tg.MessagesGetDialogsRequest) (tg.MessagesDialogsClass, error) {
	*d.reqs = append(*d.reqs, req)
	return d.channelAPI.MessagesGetDialogs(ctx, req)
}

func TestPreviewsStopOnAStuckPage(t *testing.T) {
	api := newChannelAPI()
	// the last dialog is a channel the account lost: it cannot be an offset
	stuck := &tg.MessagesDialogsSlice{Count: 300,
		Dialogs: []tg.DialogClass{&tg.Dialog{Peer: &tg.PeerChannel{ChannelID: 8}, TopMessage: 50}},
		Chats:   []tg.ChatClass{&tg.ChannelForbidden{ID: 8}}}
	api.dialogs[0] = []tg.MessagesDialogsClass{stuck, stuck, stuck, stuck}
	p := &Previews{Peers: filepath.Join(t.TempDir(), "none.json")}
	assert.ErrorIs(t, p.Download(t.Context(), api, session("abc"), post, filepath.Join(t.TempDir(), "photo"), RateGuard{}), preview.ErrNotFound)
	assert.Equal(t, 3, api.lookups, "the second page repeats the first: the main list ends, then the archive")
}
