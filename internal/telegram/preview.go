package telegram

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"

	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/tg"

	"go.octolab.org/toolset/indexit/internal/preview"
	"go.octolab.org/toolset/indexit/internal/telegram/model"
	"go.octolab.org/toolset/indexit/internal/telegram/peers"
)

// PreviewWriter adds local URLs using the message metadata already returned by
// Telegram. A nil store leaves library callers' existing behavior unchanged.
type PreviewWriter struct {
	Store  *preview.Store
	albums map[string]*model.PhotoAlbum
}

func photoSource(account int64, input tg.InputPeerClass, msg *tg.Message) (preview.Source, error) {
	src := preview.Source{Account: account, Message: msg.ID}
	switch p := input.(type) {
	case *tg.InputPeerChannel:
		src.Peer = fmt.Sprintf("c%d", p.ChannelID)
		src.AccessHash = p.AccessHash
	case *tg.InputPeerChat:
		src.Peer = fmt.Sprintf("g%d", p.ChatID)
	case *tg.InputPeerUser:
		src.Peer = fmt.Sprintf("u%d", p.UserID)
		src.AccessHash = p.AccessHash
	case *tg.InputPeerSelf:
		src.Peer = fmt.Sprintf("u%d", account)
	default:
		return src, errors.New("unsupported photo peer")
	}
	m, ok := msg.Media.(*tg.MessageMediaPhoto)
	if !ok || m.TTLSeconds != 0 {
		return src, errors.New("not a persistent cloud photograph")
	}
	photo, ok := m.Photo.(*tg.Photo)
	if !ok {
		return src, errors.New("photograph unavailable")
	}
	size, _ := largestPhotoSize(photo)
	if size == "" {
		return src, errors.New("photograph has no downloadable size")
	}
	src.Photo = photo.ID
	if _, err := preview.Parse(src.Path()); err != nil {
		return src, err
	}
	return src, nil
}
func (p *PreviewWriter) Enrich(ctx context.Context, api API, cache *peers.Cache, resolved ResolvedPeer, msg *tg.Message, rec model.MessageRecord, guard RateGuard) model.MessageRecord {
	if p == nil || p.Store == nil || rec.Media == nil || (rec.Media.Type != "photo" && msg.GroupedID == 0) {
		return rec
	}
	if cache.Account == nil {
		rec.Media.PreviewError = "Telegram account could not be verified"
		return rec
	}
	put := func(m *tg.Message) (string, error) {
		src, err := photoSource(cache.Account.ID, resolved.Input, m)
		if err != nil {
			return "", err
		}
		if err = p.Store.Save(src); err != nil {
			return "", err
		}
		return src.URL(), nil
	}
	var url string
	if rec.Media.Type == "photo" {
		var err error
		url, err = put(msg)
		if err != nil {
			rec.Media.PreviewError = err.Error()
			return rec
		}
		rec.Media.PreviewURL = url
	}
	if msg.GroupedID == 0 {
		return rec
	}
	key := fmt.Sprintf("%d/%s/%d", cache.Account.ID, resolved.UID, msg.GroupedID)
	if album, known := p.albums[key]; known {
		rec.Media.Album = album
		return rec
	}
	album := &model.PhotoAlbum{Photos: []model.PreviewPhoto{}}
	if url != "" {
		album.Photos = append(album.Photos, model.PreviewPhoto{MessageID: msg.ID, URL: url})
	}
	rec.Media.Album = album
	// Read a history window centered on the selected item. Count/offset metadata
	// proves the ends of history; a matching group touching an unproven window
	// edge is explicitly partial rather than silently called complete.
	var result tg.MessagesMessagesClass
	err := guard.Do(ctx, func(ctx context.Context) error {
		var e error
		result, e = api.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{Peer: resolved.Input, OffsetID: msg.ID, AddOffset: -50, Limit: 100})
		return e
	})
	if err != nil {
		album.Error = "cannot read album: " + err.Error()
		return rec
	}
	modified, ok := result.AsModified()
	if !ok {
		album.Error = "album history unavailable"
		return rec
	}
	messages := modified.GetMessages()
	found := map[int]*tg.Message{msg.ID: msg}
	low, high := msg.ID, msg.ID
	for _, m := range messages {
		if m, ok := m.(*tg.Message); ok && m.GroupedID == msg.GroupedID {
			peer, e := canonicalUID(m.PeerID)
			if e == nil && peer == resolved.UID {
				found[m.ID] = m
				low = min(low, m.ID)
				high = max(high, m.ID)
			}
		}
	}
	before, after := false, false
	for _, m := range messages {
		if m, ok := m.AsNotEmpty(); ok {
			before = before || m.GetID() < low
			after = after || m.GetID() > high
		}
	}
	switch v := result.(type) {
	case *tg.MessagesMessages:
		before = true
		after = true
	case *tg.MessagesMessagesSlice:
		if off, ok := v.GetOffsetIDOffset(); ok {
			after = after || off == 0
			before = before || off+len(messages) >= v.Count
		}
	case *tg.MessagesChannelMessages:
		if off, ok := v.GetOffsetIDOffset(); ok {
			after = after || off == 0
			before = before || off+len(messages) >= v.Count
		}
	}
	ids := make([]int, 0, len(found))
	for id := range found {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	album.Photos = []model.PreviewPhoto{}
	for _, id := range ids {
		if _, ok := found[id].Media.(*tg.MessageMediaPhoto); !ok {
			continue
		}
		url, e := put(found[id])
		if e != nil {
			album.Error = "some album photos unavailable: " + e.Error()
			continue
		}
		album.Photos = append(album.Photos, model.PreviewPhoto{MessageID: id, URL: url})
	}
	album.Complete = before && after && album.Error == ""
	if !album.Complete && album.Error == "" {
		album.Error = "album extends beyond the verified history window"
	}
	if album.Complete {
		if p.albums == nil {
			p.albums = make(map[string]*model.PhotoAlbum)
		}
		if len(album.Photos) == 0 {
			rec.Media.Album = nil
			p.albums[key] = nil
		} else {
			p.albums[key] = album
		}
	}
	return rec
}

// DownloadPreview refreshes file_reference and checks immutable photo identity.
// Cached serving never calls this function.
func DownloadPreview(ctx context.Context, api MediaAPI, src preview.Source, path string, guard RateGuard) error {
	self, err := currentAccount(ctx, api, guard)
	if err != nil {
		return err
	}
	if self.ID != src.Account {
		return preview.ErrAccount
	}
	id, err := strconv.ParseInt(src.Peer[1:], 10, 64)
	if err != nil {
		return err
	}
	inputs := []tg.InputMessageClass{&tg.InputMessageID{ID: src.Message}}
	var result tg.MessagesMessagesClass
	err = guard.Do(ctx, func(ctx context.Context) error {
		var e error
		if src.Peer[0] == 'c' {
			result, e = api.ChannelsGetMessages(ctx, &tg.ChannelsGetMessagesRequest{Channel: &tg.InputChannel{ChannelID: id, AccessHash: src.AccessHash}, ID: inputs})
		} else {
			result, e = api.MessagesGetMessages(ctx, inputs)
		}
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
		var peer string
		switch p := msg.PeerID.(type) {
		case *tg.PeerChannel:
			peer = fmt.Sprintf("c%d", p.ChannelID)
		case *tg.PeerChat:
			peer = fmt.Sprintf("g%d", p.ChatID)
		case *tg.PeerUser:
			peer = fmt.Sprintf("u%d", p.UserID)
		}
		if peer != src.Peer {
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
