package telegram

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/gotd/td/tg"

	"go.octolab.org/toolset/indexit/internal/telegram/mapper"
	"go.octolab.org/toolset/indexit/internal/telegram/peers"
	"go.octolab.org/toolset/indexit/internal/telegram/uid"
)

type TopicsOptions struct {
	Peer     uid.PeerRef
	Limit    int
	PageSize int
}

// FetchTopics lists the forum topics of one dialog through
// messages.getForumTopics. A dialog UID that carries a topic anchor is accepted
// and ignored: the anchor names one topic, this walks all of them.
func FetchTopics(ctx context.Context, api API, cache *peers.Cache, out Writer, opt TopicsOptions, guard RateGuard) error {
	resolved, err := ResolvePeer(ctx, api, cache, opt.Peer, guard)
	if err != nil {
		return err
	}
	if _, ok := resolved.Input.(*tg.InputPeerChannel); !ok {
		return fmt.Errorf("peer %s is not a supergroup: only a forum supergroup has topics", resolved.UID)
	}

	limit := opt.Limit
	pageSize := normalizePageSize(opt.PageSize, limit)
	offsetDate := 0
	offsetID := 0
	offsetTopic := 0
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
		var result *tg.MessagesForumTopics
		err := guard.Do(ctx, func(ctx context.Context) error {
			var err error
			result, err = api.MessagesGetForumTopics(ctx, &tg.MessagesGetForumTopicsRequest{
				Peer:        resolved.Input,
				OffsetDate:  offsetDate,
				OffsetID:    offsetID,
				OffsetTopic: offsetTopic,
				Limit:       reqLimit,
			})
			return err
		})
		if err != nil {
			return err
		}
		if len(result.Topics) == 0 {
			return nil
		}

		entities := entitiesFromLists(result.Users, result.Chats)
		mapper.CacheEntities(cache, entities)
		dates := messageDates(result.Messages)
		page++
		pageGot := 0
		var last *tg.ForumTopic
		for _, topic := range result.Topics {
			full, ok := topic.(*tg.ForumTopic)
			if !ok {
				continue
			}
			last = full
			record := mapper.Topic(resolved.UID, full)
			if record == nil {
				continue
			}
			if err := out.Write(record); err != nil {
				return err
			}
			emitted++
			pageGot++
			if limit > 0 && emitted >= limit {
				slog.Default().Info("topics: page", "n", page, "got", pageGot, "total", emitted)
				return nil
			}
		}
		slog.Default().Info("topics: page",
			"n", page, "got", pageGot, "total", emitted, "count", result.Count)
		if last == nil {
			return nil
		}
		// The cursor is the last topic of the page: its ID, its last message and
		// that message's date. Telegram orders topics by last activity, so a page
		// that fails to move the cursor means the walk is over.
		nextTopic, nextID, nextDate := last.ID, last.TopMessage, dates[last.TopMessage]
		if nextTopic == offsetTopic && nextID == offsetID && nextDate == offsetDate {
			return nil
		}
		offsetTopic, offsetID, offsetDate = nextTopic, nextID, nextDate
		if emitted >= result.Count && result.Count > 0 {
			return nil
		}
	}
}

// messageDates indexes message dates by ID, to resolve the pagination cursor
// without a second round trip.
func messageDates(messages []tg.MessageClass) map[int]int {
	out := make(map[int]int, len(messages))
	for _, msg := range messages {
		notEmpty, ok := msg.AsNotEmpty()
		if !ok {
			continue
		}
		out[notEmpty.GetID()] = notEmpty.GetDate()
	}
	return out
}
