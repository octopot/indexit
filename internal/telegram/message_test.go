package telegram

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/gotd/td/tg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.octolab.org/toolset/indexit/internal/telegram/model"
	"go.octolab.org/toolset/indexit/internal/telegram/peers"
	"go.octolab.org/toolset/indexit/internal/telegram/uid"
)

func TestFetchMessagesByID_ChannelRoutesAndOrders(t *testing.T) {
	api := &scriptedAPI{
		channelsPages: []tg.MessagesMessagesClass{
			// The response comes back out of the requested order.
			&tg.MessagesChannelMessages{Messages: []tg.MessageClass{
				channelMessage(46, "forty six"),
				channelMessage(11, "eleven"),
			}},
		},
	}
	cache := peers.New()
	cache.Put(peers.Entry{Kind: uid.KindChannel, ID: 77, AccessHash: 555})

	w := &recWriter{}
	err := FetchMessagesByID(t.Context(), api, cache, w, MessageOptions{
		Peer: uid.PeerRef{Kind: uid.KindChannel, ID: 77},
		IDs:  []int{11, 46, 999},
	}, RateGuard{})
	require.NoError(t, err)

	require.Len(t, api.channelsReqs, 1)
	assert.Empty(t, api.byIDReqs, "channel fetch must use channels.getMessages")
	channel, ok := api.channelsReqs[0].Channel.(*tg.InputChannel)
	require.True(t, ok)
	assert.EqualValues(t, 77, channel.ChannelID)
	assert.EqualValues(t, 555, channel.AccessHash)

	require.Len(t, w.records, 2, "missing id must be skipped, not fail the call")
	first := w.records[0].(model.MessageRecord)
	second := w.records[1].(model.MessageRecord)
	assert.Equal(t, 11, first.ID, "records must follow the requested order")
	assert.Equal(t, 46, second.ID)
	assert.Equal(t, "channel:77", first.DialogUID)
	assert.Equal(t, "eleven", first.Text)
}

func TestFetchMessagesByID_ChatUsesMessagesAPI(t *testing.T) {
	api := &scriptedAPI{
		byIDPages: []tg.MessagesMessagesClass{
			msgPage(userMessage(5, time.Unix(1700000005, 0), "hi")),
		},
	}

	w := &recWriter{}
	err := FetchMessagesByID(t.Context(), api, peers.New(), w, MessageOptions{
		Peer: uid.PeerRef{Kind: uid.KindChat, ID: 42},
		IDs:  []int{5},
	}, RateGuard{})
	require.NoError(t, err)

	require.Len(t, api.byIDReqs, 1)
	assert.Empty(t, api.channelsReqs, "chat fetch must not call channels.getMessages")
	input, ok := api.byIDReqs[0][0].(*tg.InputMessageID)
	require.True(t, ok)
	assert.Equal(t, 5, input.ID)
	assert.Len(t, w.records, 1)
}

func TestFetchMessagesByID_DedupesAndSkipsService(t *testing.T) {
	api := &scriptedAPI{
		byIDPages: []tg.MessagesMessagesClass{
			msgPage(
				userMessage(5, time.Unix(1700000005, 0), "content"),
				&tg.MessageService{ID: 7, Date: int(time.Unix(1700000007, 0).Unix()), PeerID: &tg.PeerChat{ChatID: 42}},
			),
		},
	}

	w := &recWriter{}
	err := FetchMessagesByID(t.Context(), api, peers.New(), w, MessageOptions{
		Peer: uid.PeerRef{Kind: uid.KindChat, ID: 42},
		IDs:  []int{5, 5, 7, 0, -1},
	}, RateGuard{})
	require.NoError(t, err)

	require.Len(t, api.byIDReqs, 1)
	assert.Len(t, api.byIDReqs[0], 2, "duplicates and non-positive ids must not reach the API")
	require.Len(t, w.records, 1, "service message must be skipped (plan §6)")
}

func TestFetchMessagesByID_ChunksLargeBatches(t *testing.T) {
	api := &scriptedAPI{}
	ids := make([]int, 150)
	for i := range ids {
		ids[i] = i + 1
	}

	for start := 0; start < len(ids); start += 100 {
		var messages []tg.MessageClass
		for i := min(start+100, len(ids)) - 1; i >= start; i-- {
			messages = append(messages, userMessage(ids[i], time.Unix(1700000000, 0), "content"))
		}
		api.byIDPages = append(api.byIDPages, msgPage(messages...))
	}
	w := &recWriter{}
	err := FetchMessagesByID(t.Context(), api, peers.New(), w, MessageOptions{
		Peer: uid.PeerRef{Kind: uid.KindChat, ID: 42},
		IDs:  ids,
	}, RateGuard{})
	require.NoError(t, err)

	require.Len(t, api.byIDReqs, 2)
	assert.Len(t, api.byIDReqs[0], 100)
	assert.Len(t, api.byIDReqs[1], 50)
	require.Len(t, w.records, 150)
	for i, record := range w.records {
		assert.Equal(t, i+1, record.(model.MessageRecord).ID)
	}
}

func TestFetchMessagesByID_LimitHonoured(t *testing.T) {
	api := &scriptedAPI{
		byIDPages: []tg.MessagesMessagesClass{
			msgPage(
				userMessage(1, time.Unix(1700000001, 0), "a"),
				userMessage(2, time.Unix(1700000002, 0), "b"),
			),
		},
	}

	w := &recWriter{}
	err := FetchMessagesByID(t.Context(), api, peers.New(), w, MessageOptions{
		Peer:  uid.PeerRef{Kind: uid.KindChat, ID: 42},
		IDs:   []int{1, 2},
		Limit: 1,
	}, RateGuard{})
	require.NoError(t, err)
	assert.Len(t, w.records, 1)
}

func TestFetchMessagesByID_NoIDs(t *testing.T) {
	err := FetchMessagesByID(t.Context(), &scriptedAPI{}, peers.New(), &recWriter{}, MessageOptions{
		Peer: uid.PeerRef{Kind: uid.KindChat, ID: 42},
		IDs:  []int{0, -1},
	}, RateGuard{})
	require.Error(t, err)
}

func channelMessage(id int, text string) *tg.Message {
	msg := userMessage(id, time.Unix(1700000000, 0), text)
	msg.PeerID = &tg.PeerChannel{ChannelID: 77}
	return msg
}

func TestFetchMessagesByID_RejectsOtherDialogs(t *testing.T) {
	for _, kind := range []uid.Kind{uid.KindUser, uid.KindChat, uid.KindChannel} {
		t.Run(string(kind), func(t *testing.T) {
			cache := peers.New()
			cache.Put(peers.Entry{Kind: kind, ID: 42, AccessHash: 555})
			correct := userMessage(1, time.Unix(1700000000, 0), "requested dialog")
			switch kind {
			case uid.KindUser:
				correct.PeerID = &tg.PeerUser{UserID: 42}
			case uid.KindChannel:
				correct.PeerID = &tg.PeerChannel{ChannelID: 42}
			default:
				// userMessage already builds a chat peer.
			}
			other := userMessage(2, time.Unix(1700000000, 0), "other dialog")
			other.PeerID = &tg.PeerChat{ChatID: 999}
			page := msgPage(other, correct)
			api := &scriptedAPI{
				byIDPages:     []tg.MessagesMessagesClass{page},
				channelsPages: []tg.MessagesMessagesClass{page},
			}
			w := &recWriter{}
			err := FetchMessagesByID(t.Context(), api, cache, w, MessageOptions{
				Peer: uid.PeerRef{Kind: kind, ID: 42}, IDs: []int{2, 1},
			}, RateGuard{})
			require.NoError(t, err)
			require.Len(t, w.records, 1)
			assert.Equal(t, 1, w.records[0].(model.MessageRecord).ID)
			assert.Equal(t, string(kind)+":42", w.records[0].(model.MessageRecord).DialogUID)
			if kind == uid.KindChannel {
				assert.Len(t, api.channelsReqs, 1)
				assert.Empty(t, api.byIDReqs)
			} else {
				assert.Len(t, api.byIDReqs, 1)
				assert.Empty(t, api.channelsReqs)
			}
		})
	}
}

func TestFetchMessagesByID_TopicScope(t *testing.T) {
	rootReply := channelMessage(11, "direct reply to topic root")
	rootHeader := &tg.MessageReplyHeader{ForumTopic: true}
	rootHeader.SetReplyToMsgID(7)
	rootReply.SetReplyTo(rootHeader)
	nestedReply := channelMessage(12, "reply within topic")
	nestedHeader := &tg.MessageReplyHeader{ForumTopic: true}
	nestedHeader.SetReplyToMsgID(11)
	nestedHeader.SetReplyToTopID(7)
	nestedReply.SetReplyTo(nestedHeader)
	other := channelMessage(13, "another topic")
	otherHeader := &tg.MessageReplyHeader{ForumTopic: true}
	otherHeader.SetReplyToMsgID(8)
	other.SetReplyTo(otherHeader)
	general := channelMessage(14, "general topic")
	generalReply := channelMessage(15, "reply in general topic")
	generalHeader := &tg.MessageReplyHeader{}
	generalHeader.SetReplyToMsgID(14)
	generalReply.SetReplyTo(generalHeader)

	for _, tc := range []struct {
		topic int
		want  []int
	}{
		{7, []int{11, 12}},
		{1, []int{14, 15}},
		{0, []int{11, 12, 13, 14, 15}},
	} {
		t.Run(fmt.Sprint(tc.topic), func(t *testing.T) {
			cache := peers.New()
			cache.Put(peers.Entry{Kind: uid.KindChannel, ID: 77, AccessHash: 555})
			api := &scriptedAPI{channelsPages: []tg.MessagesMessagesClass{
				msgPage(rootReply, nestedReply, other, general, generalReply),
			}}
			w := &recWriter{}
			err := FetchMessagesByID(t.Context(), api, cache, w, MessageOptions{
				Peer: uid.PeerRef{Kind: uid.KindChannel, ID: 77, TopicID: tc.topic},
				IDs:  []int{11, 12, 13, 14, 15},
			}, RateGuard{})
			require.NoError(t, err)
			var got []int
			for _, record := range w.records {
				msg := record.(model.MessageRecord)
				got = append(got, msg.ID)
				assert.Equal(t, tc.topic, msg.TopicID)
			}
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestFetchMessagesByID_LimitSkipsMissingAndStopsRequests(t *testing.T) {
	api := &scriptedAPI{byIDPages: []tg.MessagesMessagesClass{
		msgPage(&tg.MessageEmpty{ID: 1}),
		msgPage(&tg.MessageService{ID: 2, PeerID: &tg.PeerChat{ChatID: 42}}),
		msgPage(userMessage(3, time.Unix(1700000000, 0), "found")),
	}}
	ids := make([]int, 150)
	for i := range ids {
		ids[i] = i + 1
	}
	w := &recWriter{}
	err := FetchMessagesByID(t.Context(), api, peers.New(), w, MessageOptions{
		Peer: uid.PeerRef{Kind: uid.KindChat, ID: 42}, IDs: ids, Limit: 1,
	}, RateGuard{})
	require.NoError(t, err)
	require.Len(t, w.records, 1)
	assert.Equal(t, 3, w.records[0].(model.MessageRecord).ID)
	require.Len(t, api.byIDReqs, 3)
	for i, req := range api.byIDReqs {
		require.Len(t, req, 1)
		assert.Equal(t, i+1, req[0].(*tg.InputMessageID).ID)
	}
}

type messageErrorAPI struct {
	*scriptedAPI
	err error
}

func (a messageErrorAPI) MessagesGetMessages(context.Context, []tg.InputMessageClass) (tg.MessagesMessagesClass, error) {
	return nil, a.err
}

func (a messageErrorAPI) ChannelsGetMessages(context.Context, *tg.ChannelsGetMessagesRequest) (tg.MessagesMessagesClass, error) {
	return nil, a.err
}

type messageErrorWriter struct{ err error }

func (w messageErrorWriter) Write(any) error { return w.err }

func TestFetchMessagesByID_PropagatesErrors(t *testing.T) {
	failure := errors.New("fetch failed")
	for _, kind := range []uid.Kind{uid.KindChat, uid.KindChannel} {
		t.Run(string(kind), func(t *testing.T) {
			cache := peers.New()
			cache.Put(peers.Entry{Kind: kind, ID: 42, AccessHash: 555})
			err := FetchMessagesByID(t.Context(), messageErrorAPI{&scriptedAPI{}, failure}, cache, &recWriter{}, MessageOptions{
				Peer: uid.PeerRef{Kind: kind, ID: 42}, IDs: []int{1},
			}, RateGuard{})
			require.ErrorIs(t, err, failure)
		})
	}
	t.Run("writer", func(t *testing.T) {
		api := &scriptedAPI{byIDPages: []tg.MessagesMessagesClass{
			msgPage(userMessage(1, time.Unix(1700000000, 0), "found")),
		}}
		err := FetchMessagesByID(t.Context(), api, peers.New(), messageErrorWriter{failure}, MessageOptions{
			Peer: uid.PeerRef{Kind: uid.KindChat, ID: 42}, IDs: []int{1, 2}, Limit: 1,
		}, RateGuard{})
		require.ErrorIs(t, err, failure)
		assert.Len(t, api.byIDReqs, 1)
	})
	t.Run("cold peer", func(t *testing.T) {
		api := &scriptedAPI{}
		err := FetchMessagesByID(t.Context(), api, peers.New(), &recWriter{}, MessageOptions{
			Peer: uid.PeerRef{Kind: uid.KindChannel, ID: 42}, IDs: []int{1},
		}, RateGuard{})
		require.ErrorIs(t, err, ErrColdPeer)
		assert.Empty(t, api.channelsReqs)
	})
}
