package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"math"
	"slices"
	"strconv"

	"github.com/gotd/td/tg"

	"go.octolab.org/toolset/indexit/internal/telegram/mapper"
	"go.octolab.org/toolset/indexit/internal/telegram/model"
	"go.octolab.org/toolset/indexit/internal/telegram/peers"
	"go.octolab.org/toolset/indexit/internal/telegram/uid"
)

// maxIDsPerRequest caps ids per messages.getMessages / channels.getMessages
// call.
const maxIDsPerRequest = 100

// albumReach is how far from its known members an album can still have one:
// an album holds up to ten messages, and the gap a deleted member leaves is
// shorter than that.
const albumReach = 9

type MessageOptions struct {
	Previews *PreviewWriter
	Peer     uid.PeerRef
	IDs      []int
	Limit    int
	// Emitted holds the messages already written in this run, by dialog UID
	// and ID, so that links into one album emit it once. Nil starts empty.
	Emitted map[string]bool
}

// fetched is a message read by ID, with its record.
type fetched struct {
	msg *tg.Message
	rec model.MessageRecord
}

// FetchMessagesByID fetches specific messages by MTProto ID from one dialog
// and emits them in the requested order. A message of an album brings the
// whole album, in message order, whichever member was asked for. Deleted or
// invisible ids and service messages are logged and skipped rather than
// failing the call: a partial result is still a result.
func FetchMessagesByID(ctx context.Context, api API, cache *peers.Cache, out Writer, opt MessageOptions, guard RateGuard) error {
	ids := dedupeIDs(opt.IDs)
	if len(ids) == 0 {
		return fmt.Errorf("no message ids to fetch")
	}
	resolved, err := ResolvePeer(ctx, api, cache, opt.Peer, guard)
	if err != nil {
		return err
	}
	emitted := opt.Emitted
	if emitted == nil {
		emitted = map[string]bool{}
	}
	reader := &messageReader{api: api, cache: cache, resolved: resolved, previews: opt.Previews, guard: guard}

	count := 0
	full := func() bool { return opt.Limit > 0 && count >= opt.Limit }
	emit := func(f fetched) error {
		key := resolved.UID + "/" + strconv.Itoa(f.msg.ID)
		if emitted[key] || full() {
			return nil
		}
		if err := out.Write(f.rec); err != nil {
			return err
		}
		emitted[key] = true
		count++
		return nil
	}
	for start := 0; start < len(ids) && !full(); {
		size := min(maxIDsPerRequest, len(ids)-start)
		if opt.Limit > 0 {
			size = min(size, opt.Limit-count)
		}
		chunk := ids[start : start+size]
		start += size
		got, skipped, err := reader.get(ctx, chunk)
		if err != nil {
			return err
		}
		for _, id := range chunk {
			f, ok := got[id]
			if !ok {
				reason := skipped[id]
				if reason == "" {
					reason = "not found"
				}
				slog.Default().Warn("message: skipped", "peer", resolved.UID, "id", id, "reason", reason)
				continue
			}
			if f.msg.GroupedID == 0 {
				if err := emit(f); err != nil {
					return err
				}
				continue
			}
			if emitted[resolved.UID+"/"+strconv.Itoa(id)] {
				continue
			}
			album, err := reader.album(ctx, f)
			if err != nil {
				return err
			}
			for _, member := range album {
				if err := emit(member); err != nil {
					return err
				}
			}
			if full() {
				break
			}
		}
	}
	slog.Default().Info("message: fetched", "peer", resolved.UID, "requested", len(ids), "got", count)
	return nil
}

// messageReader reads messages of one dialog by ID.
type messageReader struct {
	api      API
	cache    *peers.Cache
	resolved ResolvedPeer
	previews *PreviewWriter
	guard    RateGuard
	// asked holds every ID requested so far, read or not; read holds the
	// messages read and skipped why the others were skipped, so that albums
	// near each other share what was read for either.
	asked   map[int]bool
	read    map[int]fetched
	skipped map[int]string
}

// get reads ids, at most maxIDsPerRequest of them, and tells why the ones it
// could not read were skipped.
func (r *messageReader) get(ctx context.Context, ids []int) (map[int]fetched, map[int]string, error) {
	if r.asked == nil {
		r.asked, r.read, r.skipped = map[int]bool{}, map[int]fetched{}, map[int]string{}
	}
	got := make(map[int]fetched, len(ids))
	skipped := make(map[int]string)
	inputs := make([]tg.InputMessageClass, 0, len(ids))
	for _, id := range ids {
		inputs = append(inputs, &tg.InputMessageID{ID: id})
	}
	// Channels live in their own id space and require channels.getMessages;
	// messages.getMessages serves user and basic-chat dialogs.
	channel, isChannel := channelInput(r.resolved.Input)
	var result tg.MessagesMessagesClass
	err := r.guard.Do(ctx, func(ctx context.Context) error {
		var err error
		if isChannel {
			result, err = r.api.ChannelsGetMessages(ctx, &tg.ChannelsGetMessagesRequest{
				Channel: channel,
				ID:      inputs,
			})
			return err
		}
		result, err = r.api.MessagesGetMessages(ctx, inputs)
		return err
	})
	if err != nil {
		return nil, nil, err
	}
	// IDs of a failed request are not asked: a later album reads them again.
	for _, id := range ids {
		r.asked[id] = true
	}
	modified, ok := result.AsModified()
	if !ok {
		return got, skipped, nil
	}
	entities := entitiesFromLists(modified.GetUsers(), modified.GetChats())
	mapper.CacheEntities(r.cache, entities)
	for _, msg := range modified.GetMessages() {
		notEmpty, ok := msg.AsNotEmpty()
		if !ok {
			continue // MessageEmpty: reported as missing by the caller
		}
		// messages.getMessages has no peer parameter. Never label a
		// message from another dialog with the requested dialog's UID.
		peerUID, err := canonicalUID(notEmpty.GetPeerID())
		if err != nil || peerUID != r.resolved.UID {
			skipped[notEmpty.GetID()] = "different dialog"
			continue
		}
		msg, isContent := notEmpty.(*tg.Message)
		if !isContent {
			skipped[notEmpty.GetID()] = "service message"
			continue
		}
		if !messageInTopic(msg, r.resolved.TopicID) {
			skipped[msg.ID] = "different topic"
			continue
		}
		rec := mapper.Message(r.resolved.UID, r.resolved.TopicID, msg, entities)
		got[msg.ID] = fetched{msg: msg, rec: r.previews.Enrich(msg, rec)}
	}
	maps.Copy(r.read, got)
	maps.Copy(r.skipped, skipped)
	return got, skipped, nil
}

// album returns every message of the album of f in message order: the
// members read so far, and the ones it reads around them until no message
// within albumReach of the album is left unread. A member it cannot see
// between two others is named in a warning; one at either end leaves no trace,
// since an empty message tells no album.
func (r *messageReader) album(ctx context.Context, f fetched) ([]fetched, error) {
	group := f.msg.GroupedID
	members := func() map[int]fetched {
		out := map[int]fetched{}
		for id, m := range r.read {
			if m.msg.GroupedID == group {
				out[id] = m
			}
		}
		return out
	}
	// From one member, the first read covers any album of up to ten.
	low, high := f.msg.ID-2*albumReach, f.msg.ID+2*albumReach
	for {
		var ids []int
		for id := max(1, low); id <= min(high, math.MaxInt32); id++ {
			if !r.asked[id] {
				ids = append(ids, id)
			}
		}
		if len(ids) == 0 {
			break
		}
		for start := 0; start < len(ids); start += maxIDsPerRequest {
			if _, _, err := r.get(ctx, ids[start:min(start+maxIDsPerRequest, len(ids))]); err != nil {
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
				slog.Default().Warn("message: album may be incomplete", "peer", r.resolved.UID, "album", group, "error", err)
				return sortedMembers(members()), nil
			}
		}
		ids = slices.Collect(maps.Keys(members()))
		low, high = slices.Min(ids)-albumReach, slices.Max(ids)+albumReach
	}
	album := sortedMembers(members())
	for i := 1; i < len(album); i++ {
		for id := album[i-1].msg.ID + 1; id < album[i].msg.ID; id++ {
			// a message of its own, or of another dialog where IDs are the
			// account's and run through all its chats
			if _, other := r.read[id]; other || r.skipped[id] == "different dialog" {
				continue
			}
			reason := r.skipped[id]
			if reason == "" {
				reason = "not found"
			}
			slog.Default().Warn("message: album member skipped", "peer", r.resolved.UID, "album", group, "id", id, "reason", reason)
		}
	}
	return album, nil
}

func sortedMembers(members map[int]fetched) []fetched {
	album := slices.Collect(maps.Values(members))
	slices.SortFunc(album, func(a, b fetched) int { return a.msg.ID - b.msg.ID })
	return album
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
