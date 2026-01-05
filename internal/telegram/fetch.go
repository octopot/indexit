package telegram

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	gotdpeer "github.com/gotd/td/telegram/message/peer"
	"github.com/gotd/td/tg"

	"go.octolab.org/toolset/indexit/internal/telegram/mapper"
	"go.octolab.org/toolset/indexit/internal/telegram/peers"
	"go.octolab.org/toolset/indexit/internal/telegram/uid"
)

const defaultPageSize = 100

type Writer interface {
	Write(any) error
}

// dialogRef exposes the peer/top-message pair shared by *tg.Dialog and
// *tg.DialogFolder. gotd/td v0.161.0 dropped these getters from the generated
// tg.DialogClass interface (only GetPinned remains), but both concrete types
// still implement them.
type dialogRef interface {
	GetPeer() tg.PeerClass
	GetTopMessage() int
}

type DialogsOptions struct {
	Limit    int
	PageSize int
}

type MessagesOptions struct {
	Peer     uid.PeerRef
	Limit    int
	PageSize int
	MinID    int
	MaxID    int
	From     time.Time
	To       time.Time
}

func FetchDialogs(ctx context.Context, api API, cache *peers.Cache, out Writer, opt DialogsOptions, guard RateGuard) error {
	limit := opt.Limit
	pageSize := normalizePageSize(opt.PageSize, limit)
	offsetPeer := tg.InputPeerClass(&tg.InputPeerEmpty{})
	offsetID := 0
	offsetDate := 0
	emitted := 0
	page := 0

	for {
		if limit > 0 && emitted >= limit {
			return nil
		}
		reqLimit := pageSize
		if limit > 0 && limit-emitted < reqLimit {
			reqLimit = limit - emitted
		}
		var result tg.MessagesDialogsClass
		err := guard.Do(ctx, func(ctx context.Context) error {
			var err error
			result, err = api.MessagesGetDialogs(ctx, &tg.MessagesGetDialogsRequest{
				OffsetDate: offsetDate,
				OffsetID:   offsetID,
				OffsetPeer: offsetPeer,
				Limit:      reqLimit,
			})
			return err
		})
		if err != nil {
			return err
		}
		modified, ok := result.AsModified()
		if !ok {
			return nil
		}
		dialogs := modified.GetDialogs()
		messages := modified.GetMessages()
		if len(dialogs) == 0 {
			return nil
		}

		entities := entitiesFromLists(modified.GetUsers(), modified.GetChats())
		mapper.CacheEntities(cache, entities)
		lastByID := lastMessagesByID(messages)
		page++
		pageGot := 0
		for _, dialog := range dialogs {
			ref, ok := dialog.(dialogRef)
			if !ok {
				continue
			}
			last := lastByID[ref.GetTopMessage()]
			rec, ok := mapper.Dialog(dialog, entities, last)
			if !ok {
				continue
			}
			if err := out.Write(rec); err != nil {
				return err
			}
			emitted++
			pageGot++
			if limit > 0 && emitted >= limit {
				slog.Default().Info("dialogs: page", "n", page, "got", pageGot, "total", emitted)
				return nil
			}
		}

		lastDialog, ok := dialogs[len(dialogs)-1].(dialogRef)
		if !ok {
			// Without a top message there is no cursor to advance, and
			// repeating the request would spin on the same page forever.
			slog.Default().Warn("dialogs: no cursor at page boundary, stopping",
				"n", page, "total", emitted)
			return nil
		}
		offsetID = lastDialog.GetTopMessage()
		offsetPeer = inputPeerForOffset(lastDialog.GetPeer(), entities)
		if last, ok := lastByID[offsetID]; ok {
			offsetDate = last.GetDate()
		} else {
			offsetDate = 0
		}
		slog.Default().Info("dialogs: page",
			"n", page, "got", pageGot, "total", emitted, "offset_id", offsetID)
	}
}

func FetchMessages(ctx context.Context, api API, cache *peers.Cache, out Writer, opt MessagesOptions, guard RateGuard) error {
	resolved, err := ResolvePeer(ctx, api, cache, opt.Peer, guard)
	if err != nil {
		return err
	}
	window := walkOptions{
		Limit:    opt.Limit,
		PageSize: opt.PageSize,
		MinID:    opt.MinID,
		MaxID:    opt.MaxID,
		From:     opt.From,
		To:       opt.To,
	}
	return walkMessages(ctx, api, cache, resolved, window, guard, "messages",
		func(_ context.Context, msg *tg.Message, entities gotdpeer.Entities) (bool, error) {
			if err := out.Write(mapper.Message(resolved.UID, resolved.TopicID, msg, entities)); err != nil {
				return false, err
			}
			return true, nil
		})
}

// walkOptions is the history window shared by the fetchers that walk one
// dialog: `fetch messages` and `fetch media` differ in what they do with a
// message, not in how the history is paged.
type walkOptions struct {
	Limit    int
	PageSize int
	MinID    int
	MaxID    int
	From     time.Time
	To       time.Time
}

// walkMessages pages one dialog — or one forum topic, when the resolved peer
// carries a topic anchor — and hands every content message to visit, newest
// first. The bool visit returns says whether the message counted against
// Limit: a message the caller skipped must not consume the budget, otherwise
// `--limit 10` on a media fetch would stop after ten *messages* rather than
// ten files.
func walkMessages(
	ctx context.Context,
	api API,
	cache *peers.Cache,
	resolved ResolvedPeer,
	opt walkOptions,
	guard RateGuard,
	scope string,
	visit func(context.Context, *tg.Message, gotdpeer.Entities) (bool, error),
) error {
	limit := opt.Limit
	pageSize := normalizePageSize(opt.PageSize, limit)
	// URL anchor (t.me/.../<msg>) is captured on PeerRef but NOT applied as a
	// cursor in PoC: users typically paste t.me links to identify a peer/topic,
	// not to window history before the linked message. See plan §4.3.
	maxID := opt.MaxID
	minID := opt.MinID
	offsetID := 0
	offsetDate := 0
	if !opt.To.IsZero() {
		offsetDate = int(opt.To.Unix())
	}
	emitted := 0
	page := 0

	for {
		if limit > 0 && emitted >= limit {
			return nil
		}
		reqLimit := pageSize
		if limit > 0 && limit-emitted < reqLimit {
			reqLimit = limit - emitted
		}
		var result tg.MessagesMessagesClass
		err := guard.Do(ctx, func(ctx context.Context) error {
			var err error
			if resolved.TopicID > 0 {
				result, err = api.MessagesGetReplies(ctx, &tg.MessagesGetRepliesRequest{
					Peer:       resolved.Input,
					MsgID:      resolved.TopicID,
					OffsetID:   offsetID,
					OffsetDate: offsetDate,
					Limit:      reqLimit,
					MaxID:      maxID,
					MinID:      minID,
				})
				return err
			}
			result, err = api.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
				Peer:       resolved.Input,
				OffsetID:   offsetID,
				OffsetDate: offsetDate,
				Limit:      reqLimit,
				MaxID:      maxID,
				MinID:      minID,
			})
			return err
		})
		if err != nil {
			return err
		}
		modified, ok := result.AsModified()
		if !ok {
			return nil
		}
		messages := modified.GetMessages()
		if len(messages) == 0 {
			return nil
		}

		entities := entitiesFromLists(modified.GetUsers(), modified.GetChats())
		mapper.CacheEntities(cache, entities)
		stop := false
		lastPageID := 0
		page++
		pageGot := 0
		for _, msg := range messages {
			if id := msg.GetID(); id > 0 {
				lastPageID = id
			}
			notEmpty, ok := msg.AsNotEmpty()
			if !ok {
				continue
			}
			// Skip MessageService (topic-created, joined, title-changed, etc.).
			// They are metadata, not user content. The most visible case is the
			// MessageActionTopicCreate that lives at id == topic_id and would
			// otherwise emit a record with empty text. See plan §6 and §15.
			full, isContent := notEmpty.(*tg.Message)
			if !isContent {
				continue
			}
			if !opt.To.IsZero() && messageTime(notEmpty).After(opt.To) {
				continue
			}
			if !opt.From.IsZero() && messageTime(notEmpty).Before(opt.From) {
				stop = true
				continue
			}
			counted, err := visit(ctx, full, entities)
			if err != nil {
				return err
			}
			offsetID = notEmpty.GetID()
			offsetDate = notEmpty.GetDate()
			if !counted {
				continue
			}
			emitted++
			pageGot++
			if limit > 0 && emitted >= limit {
				slog.Default().Info(scope+": page",
					"n", page, "got", pageGot, "total", emitted)
				return nil
			}
		}
		slog.Default().Info(scope+": page",
			"n", page, "got", pageGot, "total", emitted, "offset_id", offsetID)
		if stop {
			return nil
		}
		if lastPageID > 0 {
			offsetID = lastPageID
		}
		if offsetID == 0 {
			return nil
		}
	}
}

func normalizePageSize(pageSize, limit int) int {
	if pageSize <= 0 || pageSize > defaultPageSize {
		pageSize = defaultPageSize
	}
	if limit > 0 && limit < pageSize {
		pageSize = limit
	}
	if pageSize <= 0 {
		pageSize = defaultPageSize
	}
	return pageSize
}

func lastMessagesByID(messages []tg.MessageClass) map[int]tg.NotEmptyMessage {
	out := make(map[int]tg.NotEmptyMessage, len(messages))
	for _, msg := range messages {
		notEmpty, ok := msg.AsNotEmpty()
		if !ok {
			continue
		}
		out[notEmpty.GetID()] = notEmpty
	}
	return out
}

func inputPeerForOffset(peer tg.PeerClass, entities gotdpeer.Entities) tg.InputPeerClass {
	input, err := entities.ExtractPeer(peer)
	if err == nil {
		return input
	}
	if p, ok := peer.(*tg.PeerChat); ok {
		return &tg.InputPeerChat{ChatID: p.ChatID}
	}
	return &tg.InputPeerEmpty{}
}

func entitiesFromLists(users []tg.UserClass, chats []tg.ChatClass) gotdpeer.Entities {
	chatArray := tg.ChatClassArray(chats)
	return gotdpeer.NewEntities(
		tg.UserClassArray(users).UserToMap(),
		chatArray.ChatToMap(),
		chatArray.ChannelToMap(),
	)
}

func messageTime(msg tg.NotEmptyMessage) time.Time {
	return time.Unix(int64(msg.GetDate()), 0).UTC()
}

func ColdPeerHint(err error, value string) error {
	if !errors.Is(err, ErrColdPeer) {
		return err
	}
	return fmt.Errorf("peer %s not in cache; run 'indexit telegram fetch dialogs' first, or pass a @username / t.me/<nick> link instead of a numeric / t.me/c/ form", value)
}
