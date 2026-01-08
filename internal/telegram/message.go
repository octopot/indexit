package telegram

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/gotd/td/tg"

	"go.octolab.org/toolset/indexit/internal/telegram/mapper"
	"go.octolab.org/toolset/indexit/internal/telegram/model"
	"go.octolab.org/toolset/indexit/internal/telegram/peers"
	"go.octolab.org/toolset/indexit/internal/telegram/uid"
)

// maxIDsPerRequest caps ids per messages.getMessages / channels.getMessages
// call.
const maxIDsPerRequest = 100

type MessageOptions struct {
	Peer  uid.PeerRef
	IDs   []int
	Limit int
}

// FetchMessagesByID fetches specific messages by MTProto ID from one dialog
// and emits them in the requested order. Deleted or invisible ids and service
// messages are logged and skipped rather than failing the call: a partial
// result is still a result.
func FetchMessagesByID(ctx context.Context, api API, cache *peers.Cache, out Writer, opt MessageOptions, guard RateGuard) error {
	ids := dedupeIDs(opt.IDs)
	if len(ids) == 0 {
		return fmt.Errorf("no message ids to fetch")
	}
	resolved, err := ResolvePeer(ctx, api, cache, opt.Peer, guard)
	if err != nil {
		return err
	}
	// Channels live in their own id space and require channels.getMessages;
	// messages.getMessages serves user and basic-chat dialogs.
	channel, isChannel := channelInput(resolved.Input)

	emitted := 0
	for start := 0; start < len(ids); {
		if opt.Limit > 0 && emitted >= opt.Limit {
			break
		}
		size := min(maxIDsPerRequest, len(ids)-start)
		if opt.Limit > 0 {
			size = min(size, opt.Limit-emitted)
		}
		chunk := ids[start : start+size]
		start += size
		records := make(map[int]model.MessageRecord, len(chunk))
		skipped := make(map[int]string)
		inputs := make([]tg.InputMessageClass, 0, len(chunk))
		for _, id := range chunk {
			inputs = append(inputs, &tg.InputMessageID{ID: id})
		}
		var result tg.MessagesMessagesClass
		err := guard.Do(ctx, func(ctx context.Context) error {
			var err error
			if isChannel {
				result, err = api.ChannelsGetMessages(ctx, &tg.ChannelsGetMessagesRequest{
					Channel: channel,
					ID:      inputs,
				})
				return err
			}
			result, err = api.MessagesGetMessages(ctx, inputs)
			return err
		})
		if err != nil {
			return err
		}
		modified, ok := result.AsModified()
		if !ok {
			continue
		}
		entities := entitiesFromLists(modified.GetUsers(), modified.GetChats())
		mapper.CacheEntities(cache, entities)
		for _, msg := range modified.GetMessages() {
			notEmpty, ok := msg.AsNotEmpty()
			if !ok {
				continue // MessageEmpty: reported as missing on emit below
			}
			// messages.getMessages has no peer parameter. Never label a
			// message from another dialog with the requested dialog's UID.
			peerUID, err := canonicalUID(notEmpty.GetPeerID())
			if err != nil || peerUID != resolved.UID {
				skipped[notEmpty.GetID()] = "different dialog"
				continue
			}
			msg, isContent := notEmpty.(*tg.Message)
			if !isContent {
				skipped[notEmpty.GetID()] = "service message"
				continue
			}
			if !messageInTopic(msg, resolved.TopicID) {
				skipped[msg.ID] = "different topic"
				continue
			}
			records[msg.ID] = mapper.Message(resolved.UID, resolved.TopicID, msg, entities)
		}
		for _, id := range chunk {
			rec, ok := records[id]
			if !ok {
				reason := skipped[id]
				if reason == "" {
					reason = "not found"
				}
				slog.Default().Warn("message: skipped", "peer", resolved.UID, "id", id, "reason", reason)
				continue
			}
			if err := out.Write(rec); err != nil {
				return err
			}
			emitted++
		}
	}
	slog.Default().Info("message: fetched", "peer", resolved.UID, "requested", len(ids), "got", emitted)
	return nil
}

func dedupeIDs(ids []int) []int {
	seen := make(map[int]struct{}, len(ids))
	out := make([]int, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func channelInput(input tg.InputPeerClass) (tg.InputChannelClass, bool) {
	p, ok := input.(*tg.InputPeerChannel)
	if !ok {
		return nil, false
	}
	return &tg.InputChannel{ChannelID: p.ChannelID, AccessHash: p.AccessHash}, true
}

// messageInTopic checks the returned message because getMessages does not
// accept a topic filter. General (1) has no forum reply header; other topics
// use reply_to_top_id, or reply_to_msg_id for direct replies to the topic root.
// See https://core.telegram.org/api/forum#interacting-within-topics.
func messageInTopic(msg *tg.Message, topicID int) bool {
	if topicID == 0 || msg.ID == topicID {
		return true
	}
	reply, ok := msg.ReplyTo.(*tg.MessageReplyHeader)
	if !ok || !reply.ForumTopic {
		return topicID == 1
	}
	if topID, ok := reply.GetReplyToTopID(); ok {
		return topID == topicID
	}
	return reply.ReplyToMsgID == topicID
}
