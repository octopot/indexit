package telegram

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/gotd/td/tg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.octolab.org/toolset/indexit/internal/telegram/model"
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
	channelsPages []tg.MessagesMessagesClass
	byIDPages     []tg.MessagesMessagesClass
	historyPages  []tg.MessagesMessagesClass
	repliesPages  []tg.MessagesMessagesClass
	dialogsPages  []tg.MessagesDialogsClass
	resolved      *tg.ContactsResolvedPeer

	channelsReqs []*tg.ChannelsGetMessagesRequest
	byIDReqs     [][]tg.InputMessageClass
	historyReqs  []*tg.MessagesGetHistoryRequest
	repliesReqs  []*tg.MessagesGetRepliesRequest
	dialogsReqs  []*tg.MessagesGetDialogsRequest

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

func (f *scriptedAPI) ChannelsGetMessages(_ context.Context, req *tg.ChannelsGetMessagesRequest) (tg.MessagesMessagesClass, error) {
	f.channelsReqs = append(f.channelsReqs, req)
	if len(f.channelsPages) == 0 {
		return &tg.MessagesMessages{}, nil
	}
	out := f.channelsPages[0]
	f.channelsPages = f.channelsPages[1:]
	return out, nil
}

func (f *scriptedAPI) MessagesGetMessages(_ context.Context, ids []tg.InputMessageClass) (tg.MessagesMessagesClass, error) {
	f.byIDReqs = append(f.byIDReqs, ids)
	if len(f.byIDPages) == 0 {
		return &tg.MessagesMessages{}, nil
	}
	out := f.byIDPages[0]
	f.byIDPages = f.byIDPages[1:]
	return out, nil
}

func (f *scriptedAPI) ContactsResolveUsername(context.Context, *tg.ContactsResolveUsernameRequest) (*tg.ContactsResolvedPeer, error) {
	return f.resolved, nil
}

func (f *scriptedAPI) AuthLogOut(context.Context) (*tg.AuthLoggedOut, error) {
	return &tg.AuthLoggedOut{}, nil
}

func (f *scriptedAPI) ChannelsGetFullChannel(context.Context, tg.InputChannelClass) (*tg.MessagesChatFull, error) {
	return nil, fmt.Errorf("unexpected ChannelsGetFullChannel call")
}

func (f *scriptedAPI) MessagesGetFullChat(context.Context, int64) (*tg.MessagesChatFull, error) {
	return nil, fmt.Errorf("unexpected MessagesGetFullChat call")
}

func (f *scriptedAPI) MessagesCheckChatInvite(context.Context, string) (tg.ChatInviteClass, error) {
	return nil, fmt.Errorf("unexpected MessagesCheckChatInvite call")
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

func TestFetchMessages_PaginationDeduplicatesMessages(t *testing.T) {
	page := func(ids ...int) tg.MessagesMessagesClass {
		messages := make([]tg.MessageClass, 0, len(ids))
		for _, id := range ids {
			messages = append(messages, userMessage(id, time.Unix(1700000000, 0), "same text"))
		}
		return msgPage(messages...)
	}
	for _, tc := range []struct {
		name        string
		pages       []tg.MessagesMessagesClass
		limit       int
		wantIDs     []int
		wantOffsets []int
		wantLimits  []int
		wantErr     string
	}{
		{
			name:        "overlapping pages and repeated IDs within a page",
			pages:       []tg.MessagesMessagesClass{page(20, 20, 19), page(20, 19, 18), page(20, 18, 17)},
			wantIDs:     []int{20, 19, 18, 17},
			wantOffsets: []int{0, 19, 18, 17},
			wantLimits:  []int{3, 3, 3, 3},
		},
		{
			name:        "limit counts unique messages",
			pages:       []tg.MessagesMessagesClass{page(20, 20, 19), page(19, 18), page(17)},
			limit:       4,
			wantIDs:     []int{20, 19, 18, 17},
			wantOffsets: []int{0, 19, 18},
			wantLimits:  []int{3, 2, 1},
		},
		{
			name:        "cursor uses the smallest ID regardless of page order",
			pages:       []tg.MessagesMessagesClass{page(20, 18, 19), page(18, 17)},
			wantIDs:     []int{20, 18, 19, 17},
			wantOffsets: []int{0, 18, 17},
			wantLimits:  []int{3, 3, 3},
		},
		{
			name:        "repeated page fails without another request",
			pages:       []tg.MessagesMessagesClass{page(20, 19), page(20, 19), page(18)},
			wantIDs:     []int{20, 19},
			wantOffsets: []int{0, 19},
			wantLimits:  []int{3, 3},
			wantErr:     "messages: pagination did not advance past message ID 19",
		},
		{
			name:        "cursor cannot move backwards",
			pages:       []tg.MessagesMessagesClass{page(20, 19), page(21, 20)},
			wantIDs:     []int{20, 19},
			wantOffsets: []int{0, 19},
			wantLimits:  []int{3, 3},
			wantErr:     "messages: pagination did not advance past message ID 19",
		},
		{
			name:        "nonempty page without usable IDs fails",
			pages:       []tg.MessagesMessagesClass{page(0, -1)},
			wantOffsets: []int{0},
			wantLimits:  []int{3},
			wantErr:     "messages: pagination did not advance past message ID 0",
		},
	} {
		for _, topic := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/topic_%t", tc.name, topic), func(t *testing.T) {
				api := &scriptedAPI{}
				ref := uid.PeerRef{Kind: uid.KindChat, ID: 42}
				if topic {
					ref.HasTopic, ref.TopicID = true, 7
					api.repliesPages = tc.pages
				} else {
					api.historyPages = tc.pages
				}
				out := &recWriter{}
				err := FetchMessages(t.Context(), api, peers.New(), out, MessagesOptions{
					Peer: ref, Limit: tc.limit, PageSize: 3,
				}, RateGuard{})
				if tc.wantErr != "" {
					require.EqualError(t, err, tc.wantErr)
				} else {
					require.NoError(t, err)
				}
				var ids, offsets, limits []int
				for _, record := range out.records {
					ids = append(ids, record.(model.MessageRecord).ID)
				}
				for _, req := range api.historyReqs {
					offsets = append(offsets, req.OffsetID)
					limits = append(limits, req.Limit)
					assert.Zero(t, req.OffsetDate, "ID pagination must not exclude messages from the same second")
				}
				for _, req := range api.repliesReqs {
					offsets = append(offsets, req.OffsetID)
					limits = append(limits, req.Limit)
					assert.Zero(t, req.OffsetDate)
					assert.Equal(t, 7, req.MsgID)
				}
				assert.Equal(t, tc.wantIDs, ids)
				assert.Equal(t, tc.wantOffsets, offsets)
				assert.Equal(t, tc.wantLimits, limits)
				if topic {
					assert.Empty(t, api.historyReqs)
				} else {
					assert.Empty(t, api.repliesReqs)
				}
			})
		}
	}
}

func TestFetchMessages_FilteredPagesAdvance(t *testing.T) {
	base := time.Unix(1700000000, 0).UTC()
	for _, topic := range []bool{false, true} {
		t.Run(fmt.Sprintf("topic_%t", topic), func(t *testing.T) {
			pages := []tg.MessagesMessagesClass{
				msgPage(&tg.MessageEmpty{ID: 24}, &tg.MessageService{ID: 23}),
				msgPage(userMessage(22, base.Add(time.Hour), "after --to")),
				msgPage(userMessage(21, base, "within the window")),
				msgPage(userMessage(20, base.Add(-time.Hour), "before --from")),
				msgPage(userMessage(19, base.Add(-2*time.Hour), "must not request this page")),
			}
			api := &scriptedAPI{}
			ref := uid.PeerRef{Kind: uid.KindChat, ID: 42}
			if topic {
				ref.HasTopic, ref.TopicID = true, 7
				api.repliesPages = pages
			} else {
				api.historyPages = pages
			}
			out := &recWriter{}
			err := FetchMessages(t.Context(), api, peers.New(), out, MessagesOptions{
				Peer: ref, From: base.Add(-time.Minute), To: base.Add(time.Minute),
			}, RateGuard{})
			require.NoError(t, err)
			require.Len(t, out.records, 1)
			assert.Equal(t, 21, out.records[0].(model.MessageRecord).ID)
			var offsets, dates []int
			for _, req := range api.historyReqs {
				offsets = append(offsets, req.OffsetID)
				dates = append(dates, req.OffsetDate)
			}
			for _, req := range api.repliesReqs {
				offsets = append(offsets, req.OffsetID)
				dates = append(dates, req.OffsetDate)
			}
			assert.Equal(t, []int{0, 23, 22, 21}, offsets)
			assert.Equal(t, []int{int(base.Add(time.Minute).Unix()), 0, 0, 0}, dates)
		})
	}
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

func TestFetchDialogs_PaginationDeduplicatesPeers(t *testing.T) {
	for _, limit := range []int{0, 4} {
		t.Run(fmt.Sprintf("limit_%d", limit), func(t *testing.T) {
			pinned := &tg.Dialog{Peer: &tg.PeerChannel{ChannelID: 42}, TopMessage: 104, Pinned: true}
			other := &tg.Dialog{Peer: &tg.PeerChat{ChatID: 7}, TopMessage: 103}
			api := &scriptedAPI{
				dialogsPages: []tg.MessagesDialogsClass{
					&tg.MessagesDialogsSlice{
						Count:   4,
						Dialogs: []tg.DialogClass{pinned, other},
						Chats: []tg.ChatClass{
							&tg.Channel{ID: 42, AccessHash: 12345, Title: "Sparkle"},
							&tg.Chat{ID: 7, Title: "Other"},
						},
					},
					// Overlapping pages may repeat both pinned and ordinary dialogs.
					// Even a page of duplicates must not consume the output limit
					// or prevent us from reading later pages.
					&tg.MessagesDialogsSlice{
						Count:   4,
						Dialogs: []tg.DialogClass{pinned, other},
						Chats: []tg.ChatClass{
							&tg.Channel{ID: 42, AccessHash: 12345, Title: "Sparkle"},
							&tg.Chat{ID: 7, Title: "Other"},
						},
					},
					&tg.MessagesDialogs{
						Dialogs: []tg.DialogClass{
							&tg.Dialog{Peer: &tg.PeerChat{ChatID: 42}, TopMessage: 102},
							&tg.Dialog{Peer: &tg.PeerChat{ChatID: 9}, TopMessage: 101},
						},
						Chats: []tg.ChatClass{
							&tg.Chat{ID: 42, Title: "Sparkle"},
							&tg.Chat{ID: 9, Title: "Last"},
						},
					},
				},
			}
			w := &recWriter{}
			err := FetchDialogs(t.Context(), api, peers.New(), w, DialogsOptions{Limit: limit, PageSize: 2}, RateGuard{})
			require.NoError(t, err)
			var ids, titles []string
			for _, record := range w.records {
				dialog := record.(model.DialogRecord)
				ids = append(ids, dialog.UID)
				titles = append(titles, dialog.Title)
			}
			assert.Equal(t, []string{"channel:42", "chat:7", "chat:42", "chat:9"}, ids)
			assert.Equal(t, []string{"Sparkle", "Other", "Sparkle", "Last"}, titles,
				"distinct peers with identical titles must be preserved")
			require.GreaterOrEqual(t, len(api.dialogsReqs), 3)
			assert.False(t, api.dialogsReqs[0].ExcludePinned, "include pinned dialogs on the first page")
			for _, req := range api.dialogsReqs[1:] {
				assert.True(t, req.ExcludePinned, "exclude pinned dialogs on subsequent pages")
			}
			assert.Equal(t, 2, api.dialogsReqs[2].Limit, "duplicates must not count towards the limit")
		})
	}
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
