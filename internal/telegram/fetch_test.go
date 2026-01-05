package telegram

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/gotd/td/tg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.octolab.org/toolset/indexit/internal/telegram/peers"
	"go.octolab.org/toolset/indexit/internal/telegram/uid"
)

func init() {
	// Silence lifecycle logging in tests; assertions inspect requests/records,
	// not stderr noise.
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// scriptedAPI replays a fixed list of responses per RPC; once exhausted, it
// returns an empty page so iteration terminates cleanly.
type scriptedAPI struct {
	historyPages []tg.MessagesMessagesClass
	repliesPages []tg.MessagesMessagesClass
	dialogsPages []tg.MessagesDialogsClass
	resolved     *tg.ContactsResolvedPeer

	historyReqs []*tg.MessagesGetHistoryRequest
	repliesReqs []*tg.MessagesGetRepliesRequest
	dialogsReqs []*tg.MessagesGetDialogsRequest

	topicsPages []*tg.MessagesForumTopics
	topicsReqs  []*tg.MessagesGetForumTopicsRequest
}

func (f *scriptedAPI) MessagesGetForumTopics(_ context.Context, req *tg.MessagesGetForumTopicsRequest) (*tg.MessagesForumTopics, error) {
	f.topicsReqs = append(f.topicsReqs, req)
	if len(f.topicsPages) == 0 {
		return &tg.MessagesForumTopics{}, nil
	}
	out := f.topicsPages[0]
	f.topicsPages = f.topicsPages[1:]
	return out, nil
}

func (f *scriptedAPI) MessagesGetHistory(_ context.Context, req *tg.MessagesGetHistoryRequest) (tg.MessagesMessagesClass, error) {
	f.historyReqs = append(f.historyReqs, req)
	if len(f.historyPages) == 0 {
		return &tg.MessagesMessages{}, nil
	}
	out := f.historyPages[0]
	f.historyPages = f.historyPages[1:]
	return out, nil
}

func (f *scriptedAPI) MessagesGetReplies(_ context.Context, req *tg.MessagesGetRepliesRequest) (tg.MessagesMessagesClass, error) {
	f.repliesReqs = append(f.repliesReqs, req)
	if len(f.repliesPages) == 0 {
		return &tg.MessagesMessages{}, nil
	}
	out := f.repliesPages[0]
	f.repliesPages = f.repliesPages[1:]
	return out, nil
}

func (f *scriptedAPI) MessagesGetDialogs(_ context.Context, req *tg.MessagesGetDialogsRequest) (tg.MessagesDialogsClass, error) {
	f.dialogsReqs = append(f.dialogsReqs, req)
	if len(f.dialogsPages) == 0 {
		return &tg.MessagesDialogs{}, nil
	}
	out := f.dialogsPages[0]
	f.dialogsPages = f.dialogsPages[1:]
	return out, nil
}

func (f *scriptedAPI) ContactsResolveUsername(context.Context, *tg.ContactsResolveUsernameRequest) (*tg.ContactsResolvedPeer, error) {
	return f.resolved, nil
}

func (f *scriptedAPI) AuthLogOut(context.Context) (*tg.AuthLoggedOut, error) {
	return &tg.AuthLoggedOut{}, nil
}

// recWriter captures emitted records for assertion.
type recWriter struct{ records []any }

func (w *recWriter) Write(v any) error { w.records = append(w.records, v); return nil }

func userMessage(id int, date time.Time, text string) *tg.Message {
	return &tg.Message{
		ID:      id,
		Date:    int(date.Unix()),
		Message: text,
		PeerID:  &tg.PeerChat{ChatID: 42},
		FromID:  &tg.PeerUser{UserID: 100},
	}
}

func msgPage(messages ...tg.MessageClass) *tg.MessagesMessages {
	return &tg.MessagesMessages{Messages: messages}
}

func TestFetchMessages_FiltersServiceMessages(t *testing.T) {
	api := &scriptedAPI{
		historyPages: []tg.MessagesMessagesClass{
			msgPage(
				userMessage(100, time.Unix(1700000000, 0), "hello"),
				// MessageActionTopicCreate at id == topic_id (synthetic).
				&tg.MessageService{ID: 7, Date: int(time.Unix(1690000000, 0).Unix()), PeerID: &tg.PeerChat{ChatID: 42}},
			),
		},
	}

	w := &recWriter{}
	err := FetchMessages(t.Context(), api, peers.New(), w, MessagesOptions{
		Peer: uid.PeerRef{Kind: uid.KindChat, ID: 42},
	}, RateGuard{})
	require.NoError(t, err)
	require.Len(t, w.records, 1, "MessageService must be filtered out (plan §6.3)")
}

func TestFetchMessages_AnchorIsNotAutoCursor(t *testing.T) {
	api := &scriptedAPI{
		historyPages: []tg.MessagesMessagesClass{
			msgPage(userMessage(500, time.Unix(1700000000, 0), "x")),
		},
	}

	err := FetchMessages(t.Context(), api, peers.New(), &recWriter{}, MessagesOptions{
		Peer: uid.PeerRef{Kind: uid.KindChat, ID: 42, HasAnchor: true, AnchorID: 999},
	}, RateGuard{})
	require.NoError(t, err)
	require.NotEmpty(t, api.historyReqs)
	for i, req := range api.historyReqs {
		assert.Equalf(t, 0, req.MaxID,
			"URL anchor must NOT be auto-promoted to max_id (plan §4.3); request %d", i)
	}
}

func TestFetchMessages_ExplicitMaxIDIsHonoured(t *testing.T) {
	api := &scriptedAPI{
		historyPages: []tg.MessagesMessagesClass{
			msgPage(userMessage(50, time.Unix(1700000000, 0), "x")),
		},
	}

	err := FetchMessages(t.Context(), api, peers.New(), &recWriter{}, MessagesOptions{
		Peer:  uid.PeerRef{Kind: uid.KindChat, ID: 42},
		MaxID: 123,
	}, RateGuard{})
	require.NoError(t, err)
	assert.Equal(t, 123, api.historyReqs[0].MaxID)
}

func TestFetchMessages_LimitHonoured(t *testing.T) {
	api := &scriptedAPI{
		historyPages: []tg.MessagesMessagesClass{
			msgPage(
				userMessage(10, time.Unix(1700000010, 0), "a"),
				userMessage(9, time.Unix(1700000009, 0), "b"),
				userMessage(8, time.Unix(1700000008, 0), "c"),
			),
		},
	}

	w := &recWriter{}
	err := FetchMessages(t.Context(), api, peers.New(), w, MessagesOptions{
		Peer:  uid.PeerRef{Kind: uid.KindChat, ID: 42},
		Limit: 2,
	}, RateGuard{})
	require.NoError(t, err)
	assert.Len(t, w.records, 2)
}

func TestFetchMessages_PaginatesUntilExhausted(t *testing.T) {
	api := &scriptedAPI{
		historyPages: []tg.MessagesMessagesClass{
			msgPage(
				userMessage(20, time.Unix(1700000020, 0), "p1-a"),
				userMessage(19, time.Unix(1700000019, 0), "p1-b"),
			),
			msgPage(userMessage(18, time.Unix(1700000018, 0), "p2")),
			// 3rd call (after empty) is the natural termination.
		},
	}

	w := &recWriter{}
	err := FetchMessages(t.Context(), api, peers.New(), w, MessagesOptions{
		Peer: uid.PeerRef{Kind: uid.KindChat, ID: 42},
	}, RateGuard{})
	require.NoError(t, err)
	assert.Len(t, w.records, 3)
	// Second call must use offset_id from the last record of page 1.
	require.GreaterOrEqual(t, len(api.historyReqs), 2)
	assert.Equal(t, 19, api.historyReqs[1].OffsetID)
}

func TestFetchMessages_TopicGoesThroughGetReplies(t *testing.T) {
	api := &scriptedAPI{
		repliesPages: []tg.MessagesMessagesClass{
			msgPage(userMessage(100, time.Unix(1700000000, 0), "in topic")),
		},
	}

	w := &recWriter{}
	err := FetchMessages(t.Context(), api, peers.New(), w, MessagesOptions{
		Peer: uid.PeerRef{Kind: uid.KindChat, ID: 42, HasTopic: true, TopicID: 7},
	}, RateGuard{})
	require.NoError(t, err)
	assert.Len(t, w.records, 1)
	assert.Empty(t, api.historyReqs, "topic fetch must not call MessagesGetHistory")
	require.NotEmpty(t, api.repliesReqs)
	assert.Equal(t, 7, api.repliesReqs[0].MsgID)
}

func TestFetchMessages_FromDateStopsIteration(t *testing.T) {
	base := time.Unix(1700000000, 0).UTC()
	api := &scriptedAPI{
		historyPages: []tg.MessagesMessagesClass{
			msgPage(
				userMessage(3, base.Add(2*time.Hour), "newer"), // emitted
				userMessage(2, base.Add(1*time.Hour), "edge"),  // emitted
				userMessage(1, base.Add(-1*time.Hour), "old"),  // skipped: before --from
			),
		},
	}

	w := &recWriter{}
	err := FetchMessages(t.Context(), api, peers.New(), w, MessagesOptions{
		Peer: uid.PeerRef{Kind: uid.KindChat, ID: 42},
		From: base,
	}, RateGuard{})
	require.NoError(t, err)
	assert.Len(t, w.records, 2, "messages older than --from must be skipped and stop iteration")
}

func TestFetchDialogs_LimitHonoured(t *testing.T) {
	api := &scriptedAPI{
		dialogsPages: []tg.MessagesDialogsClass{
			&tg.MessagesDialogs{
				Dialogs: []tg.DialogClass{
					&tg.Dialog{Peer: &tg.PeerChat{ChatID: 1}, TopMessage: 100},
					&tg.Dialog{Peer: &tg.PeerChat{ChatID: 2}, TopMessage: 101},
					&tg.Dialog{Peer: &tg.PeerChat{ChatID: 3}, TopMessage: 102},
				},
				Messages: []tg.MessageClass{
					userMessage(100, time.Unix(1700000000, 0), ""),
					userMessage(101, time.Unix(1700000001, 0), ""),
					userMessage(102, time.Unix(1700000002, 0), ""),
				},
				Chats: []tg.ChatClass{
					&tg.Chat{ID: 1, Title: "one"},
					&tg.Chat{ID: 2, Title: "two"},
					&tg.Chat{ID: 3, Title: "three"},
				},
			},
		},
	}

	w := &recWriter{}
	err := FetchDialogs(t.Context(), api, peers.New(), w, DialogsOptions{Limit: 2}, RateGuard{})
	require.NoError(t, err)
	assert.Len(t, w.records, 2)
}

func TestFetchDialogs_PopulatesPeerCache(t *testing.T) {
	api := &scriptedAPI{
		dialogsPages: []tg.MessagesDialogsClass{
			&tg.MessagesDialogs{
				Dialogs: []tg.DialogClass{
					&tg.Dialog{Peer: &tg.PeerChannel{ChannelID: 77}, TopMessage: 100},
				},
				Messages: []tg.MessageClass{userMessage(100, time.Unix(1700000000, 0), "")},
				Chats: []tg.ChatClass{
					&tg.Channel{ID: 77, AccessHash: 12345, Username: "demo", Title: "Demo"},
				},
			},
		},
	}

	cache := peers.New()
	err := FetchDialogs(t.Context(), api, cache, &recWriter{}, DialogsOptions{}, RateGuard{})
	require.NoError(t, err)

	entry, ok := cache.Get(uid.KindChannel, 77)
	require.True(t, ok, "FetchDialogs must populate the peer cache")
	assert.EqualValues(t, 12345, entry.AccessHash)
	assert.Equal(t, "demo", entry.Username)
}
