package telegram

import (
	"context"
	"errors"
	"fmt"
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
	Store *preview.Store
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
	if p == nil || p.Store == nil || rec.Media == nil || rec.Media.Type != "photo" {
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
		return p.Store.URL(src), nil
	}
	url, err := put(msg)
	if err != nil {
		rec.Media.PreviewError = err.Error()
		return rec
	}
	rec.Media.PreviewURL = url
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
