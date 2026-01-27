package telegram

import (
	"context"
	"errors"
	"testing"

	"github.com/gotd/td/tg"

	"go.octolab.org/toolset/indexit/internal/preview"
	"go.octolab.org/toolset/indexit/internal/telegram/model"
	"go.octolab.org/toolset/indexit/internal/telegram/peers"
)

type previewAPI struct {
	API
	history tg.MessagesMessagesClass
	err     error
	reads   int
}

func (f *previewAPI) MessagesGetHistory(context.Context, *tg.MessagesGetHistoryRequest) (tg.MessagesMessagesClass, error) {
	f.reads++
	return f.history, f.err
}
func testPhoto(id int, group int64) *tg.Message {
	return &tg.Message{ID: id, GroupedID: group, PeerID: &tg.PeerChannel{ChannelID: 123}, Media: &tg.MessageMediaPhoto{Photo: &tg.Photo{ID: int64(-100 - id), Sizes: []tg.PhotoSizeClass{&tg.PhotoSize{Type: "w", W: 1280, H: 900, Size: 1234}}}}}
}
func TestPreviewSingleNeedsNoAdditionalAPI(t *testing.T) {
	api := &previewAPI{}
	store := &preview.Store{Root: t.TempDir()}
	cache := peers.New()
	cache.Account = &peers.Account{ID: 42}
	p := &PreviewWriter{Store: store}
	resolved := ResolvedPeer{Input: &tg.InputPeerChannel{ChannelID: 123, AccessHash: 99}, UID: "channel:123"}
	rec := p.Enrich(context.Background(), api, cache, resolved, testPhoto(7, 0), model.MessageRecord{Media: &model.MediaDescriptor{Type: "photo"}}, RateGuard{})
	if api.reads != 0 || rec.Media.PreviewURL == "" || rec.Media.PreviewError != "" {
		t.Fatalf("%+v reads %d", rec.Media, api.reads)
	}
	src, e := preview.Parse(rec.Media.PreviewURL[len(preview.Origin):])
	if e != nil {
		t.Fatal(e)
	}
	saved, e := store.Read(src)
	if e != nil || saved.AccessHash != 99 {
		t.Fatalf("%+v %v", saved, e)
	}
}
func TestPreviewAlbumFromMiddleAndPartial(t *testing.T) {
	cache := peers.New()
	cache.Account = &peers.Account{ID: 42}
	p := &PreviewWriter{Store: &preview.Store{Root: t.TempDir()}}
	resolved := ResolvedPeer{Input: &tg.InputPeerChannel{ChannelID: 123}, UID: "channel:123"}
	api := &previewAPI{history: &tg.MessagesMessages{Messages: []tg.MessageClass{testPhoto(9, 55), testPhoto(8, 55), testPhoto(7, 55)}}}
	rec := p.Enrich(context.Background(), api, cache, resolved, testPhoto(8, 55), model.MessageRecord{Text: "keep caption", Media: &model.MediaDescriptor{Type: "photo"}}, RateGuard{})
	a := rec.Media.Album
	if !a.Complete || len(a.Photos) != 3 || a.Photos[0].MessageID != 7 || a.Photos[2].MessageID != 9 {
		t.Fatalf("%+v", a)
	}
	p.albums = nil
	api.err = errors.New("network unavailable")
	rec = p.Enrich(context.Background(), api, cache, resolved, testPhoto(8, 55), model.MessageRecord{Text: "keep caption", Media: &model.MediaDescriptor{Type: "photo"}}, RateGuard{})
	if rec.Text != "keep caption" || rec.Media.Album.Complete || rec.Media.Album.Error == "" || len(rec.Media.Album.Photos) != 1 {
		t.Fatalf("%+v", rec)
	}
	api.err = nil
	api.history = &tg.MessagesMessagesSlice{Count: 300, Messages: []tg.MessageClass{testPhoto(8, 55)}}
	rec = p.Enrich(context.Background(), api, cache, resolved, testPhoto(8, 55), model.MessageRecord{Media: &model.MediaDescriptor{Type: "photo"}}, RateGuard{})
	if rec.Media.Album.Complete || rec.Media.Album.Error == "" {
		t.Fatal("silent partial album")
	}
}
func TestPreviewRejectsEphemeralPhotos(t *testing.T) {
	msg := testPhoto(7, 0)
	msg.Media.(*tg.MessageMediaPhoto).TTLSeconds = 5
	if _, e := photoSource(42, &tg.InputPeerChannel{ChannelID: 123}, msg); e == nil {
		t.Fatal("TTL photo exposed")
	}
}

type previewDownloadAPI struct {
	MediaAPI
	account int64
	message *tg.Message
}

func (f *previewDownloadAPI) UsersGetUsers(context.Context, []tg.InputUserClass) ([]tg.UserClass, error) {
	return []tg.UserClass{&tg.User{ID: f.account}}, nil
}
func (f *previewDownloadAPI) ChannelsGetMessages(context.Context, *tg.ChannelsGetMessagesRequest) (tg.MessagesMessagesClass, error) {
	return &tg.MessagesMessages{Messages: []tg.MessageClass{f.message}}, nil
}
func TestDownloadPreviewRejectsChangedAccountAndPhotoBeforeDownload(t *testing.T) {
	src := preview.Source{Account: 42, Peer: "c123", Message: 7, Photo: -107}
	for _, tc := range []struct {
		name    string
		account int64
		photo   int64
	}{{"changed account", 43, -107}, {"replaced photo", 42, -108}} {
		t.Run(tc.name, func(t *testing.T) {
			msg := testPhoto(7, 0)
			msg.Media.(*tg.MessageMediaPhoto).Photo.(*tg.Photo).ID = tc.photo
			api := &previewDownloadAPI{account: tc.account, message: msg}
			if e := DownloadPreview(t.Context(), api, src, t.TempDir()+"/out.jpg", RateGuard{}); e == nil {
				t.Fatal("download allowed for wrong identity")
			}
		})
	}
}

func TestPreviewAlbumMemoizedAndMixedVideoAnchor(t *testing.T) {
	cache := peers.New()
	cache.Account = &peers.Account{ID: 42}
	p := &PreviewWriter{Store: &preview.Store{Root: t.TempDir()}}
	resolved := ResolvedPeer{Input: &tg.InputPeerChannel{ChannelID: 123}, UID: "channel:123"}
	video := testPhoto(8, 55)
	video.Media = &tg.MessageMediaDocument{}
	api := &previewAPI{history: &tg.MessagesMessages{Messages: []tg.MessageClass{testPhoto(7, 55), video, testPhoto(9, 55)}}}
	for _, msg := range []*tg.Message{video, testPhoto(7, 55), testPhoto(9, 55)} {
		typ := "photo"
		if msg.ID == 8 {
			typ = "video"
		}
		rec := p.Enrich(t.Context(), api, cache, resolved, msg, model.MessageRecord{Media: &model.MediaDescriptor{Type: typ}}, RateGuard{})
		if rec.Media.Album == nil || !rec.Media.Album.Complete || len(rec.Media.Album.Photos) != 2 {
			t.Fatalf("%+v", rec.Media)
		}
	}
	if api.reads != 1 {
		t.Fatalf("history requests: %d", api.reads)
	}
}
