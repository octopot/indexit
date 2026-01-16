package telegram

import (
	"context"
	"fmt"
	"testing"

	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.octolab.org/toolset/indexit/internal/telegram/model"
	"go.octolab.org/toolset/indexit/internal/telegram/peers"
	"go.octolab.org/toolset/indexit/internal/telegram/uid"
)

// peerAPI answers the peer-card RPCs from fixed tables and records the calls.
// Every other RPC fails through the embedded fakeAPI.
type peerAPI struct {
	fakeAPI

	resolved map[string]*tg.ContactsResolvedPeer
	channels map[int64]*tg.MessagesChatFull
	chats    map[int64]*tg.MessagesChatFull
	invites  map[string]tg.ChatInviteClass
	errs     map[string]error // keyed by username, invite hash, or "channel:<id>"

	calls []string
}

func (f *peerAPI) ContactsResolveUsername(_ context.Context, req *tg.ContactsResolveUsernameRequest) (*tg.ContactsResolvedPeer, error) {
	f.calls = append(f.calls, "resolve:"+req.Username)
	if err, ok := f.errs[req.Username]; ok {
		return nil, err
	}
	if out, ok := f.resolved[req.Username]; ok {
		return out, nil
	}
	return nil, tgerr.New(400, "USERNAME_NOT_OCCUPIED")
}

func (f *peerAPI) ChannelsGetFullChannel(_ context.Context, input tg.InputChannelClass) (*tg.MessagesChatFull, error) {
	channel, ok := input.(*tg.InputChannel)
	if !ok {
		return nil, fmt.Errorf("unexpected input %T", input)
	}
	key := fmt.Sprintf("channel:%d", channel.ChannelID)
	f.calls = append(f.calls, fmt.Sprintf("full:%s:%d", key, channel.AccessHash))
	if err, ok := f.errs[key]; ok {
		return nil, err
	}
	if out, ok := f.channels[channel.ChannelID]; ok {
		return out, nil
	}
	return nil, tgerr.New(400, "CHANNEL_INVALID")
}

func (f *peerAPI) MessagesGetFullChat(_ context.Context, id int64) (*tg.MessagesChatFull, error) {
	f.calls = append(f.calls, fmt.Sprintf("full:chat:%d", id))
	if out, ok := f.chats[id]; ok {
		return out, nil
	}
	return nil, tgerr.New(400, "CHAT_ID_INVALID")
}

func (f *peerAPI) MessagesCheckChatInvite(_ context.Context, hash string) (tg.ChatInviteClass, error) {
	f.calls = append(f.calls, "invite:"+hash)
	if err, ok := f.errs[hash]; ok {
		return nil, err
	}
	if out, ok := f.invites[hash]; ok {
		return out, nil
	}
	return nil, tgerr.New(400, "INVITE_HASH_INVALID")
}

// The fixture world: a broadcast channel 10 and its discussion supergroup 20,
// a gigagroup 30, a basic chat 40, a user 50 and a bot 60.
var (
	newsChannel = &tg.Channel{
		ID: 10, AccessHash: 110, Title: "News", Username: "news_channel",
		Broadcast: true, Verified: true, Date: 1600000000,
		Usernames: []tg.Username{
			{Username: "news_channel", Active: true, Editable: true},
			{Username: "news_old", Active: false},
			{Username: "news_short", Active: true},
			{Username: "NEWS_CHANNEL", Active: true},
		},
		RestrictionReason: []tg.RestrictionReason{
			{Platform: "ios", Reason: "porn", Text: "Unavailable here"},
			{Platform: "android", Reason: "porn", Text: "Unavailable here"},
		},
	}
	newsChat  = &tg.Channel{ID: 20, AccessHash: 120, Title: "News chat", Username: "news_chat", Megagroup: true, Date: 1600000100}
	betaChan  = &tg.Channel{ID: 70, AccessHash: 170, Title: "Beta", Username: "beta", Broadcast: true}
	bigGroup  = &tg.Channel{ID: 30, AccessHash: 130, Title: "Big group", Username: "big_group", Gigagroup: true}
	basicChat = &tg.Chat{ID: 40, Title: "Family", ParticipantsCount: 3, Date: 1500000000}
	someUser  = &tg.User{ID: 50, AccessHash: 150, FirstName: "Some", LastName: "User", Username: "some_user", Scam: true}
	someBot   = &tg.User{ID: 60, AccessHash: 160, FirstName: "Helper", Username: "helper_bot", Bot: true}
)

func peerWorld() *peerAPI {
	return &peerAPI{
		resolved: map[string]*tg.ContactsResolvedPeer{
			"news_channel": {Peer: &tg.PeerChannel{ChannelID: 10}, Chats: []tg.ChatClass{newsChannel}},
			"news_chat":    {Peer: &tg.PeerChannel{ChannelID: 20}, Chats: []tg.ChatClass{newsChat}},
			"big_group":    {Peer: &tg.PeerChannel{ChannelID: 30}, Chats: []tg.ChatClass{bigGroup}},
			"beta":         {Peer: &tg.PeerChannel{ChannelID: 70}, Chats: []tg.ChatClass{betaChan}},
			"some_user":    {Peer: &tg.PeerUser{UserID: 50}, Users: []tg.UserClass{someUser}},
			"helper_bot":   {Peer: &tg.PeerUser{UserID: 60}, Users: []tg.UserClass{someBot}},
		},
		channels: map[int64]*tg.MessagesChatFull{
			10: {
				FullChat: &tg.ChannelFull{ID: 10, About: "Daily news", ParticipantsCount: 1000, LinkedChatID: 20},
				Chats:    []tg.ChatClass{newsChannel, newsChat},
			},
			20: {
				FullChat: &tg.ChannelFull{ID: 20, About: "Discuss the news", ParticipantsCount: 300, LinkedChatID: 10},
				Chats:    []tg.ChatClass{newsChat, newsChannel},
			},
			70: {
				FullChat: &tg.ChannelFull{ID: 70, About: "Short name", ParticipantsCount: 5},
				Chats:    []tg.ChatClass{betaChan},
			},
			30: {
				FullChat: &tg.ChannelFull{ID: 30, About: "Many people", ParticipantsCount: 250000},
				Chats:    []tg.ChatClass{bigGroup},
			},
		},
		chats: map[int64]*tg.MessagesChatFull{
			40: {
				FullChat: &tg.ChatFull{ID: 40, About: "Just us", Participants: &tg.ChatParticipants{
					ChatID: 40,
					Participants: []tg.ChatParticipantClass{
						&tg.ChatParticipant{UserID: 50}, &tg.ChatParticipantCreator{UserID: 51},
					},
				}},
				Chats: []tg.ChatClass{basicChat},
				Users: []tg.UserClass{someUser},
			},
		},
		invites: map[string]tg.ChatInviteClass{
			"AlreadyIn": &tg.ChatInviteAlready{Chat: newsChat},
			"PeekIn":    &tg.ChatInvitePeek{Chat: newsChannel, Expires: 1700000000},
			"Closed": &tg.ChatInvite{
				Channel: true, Megagroup: true, Title: "Closed club", About: "Members only",
				ParticipantsCount: 42,
			},
			"AskFirst": &tg.ChatInvite{
				Channel: true, Broadcast: true, RequestNeeded: true, Title: "Gated", ParticipantsCount: 7,
			},
		},
		errs: map[string]error{
			"Expired":     tgerr.New(400, "INVITE_HASH_EXPIRED"),
			"channel:99":  tgerr.New(400, "CHANNEL_PRIVATE"),
			"broken_nick": tgerr.New(400, "USERNAME_INVALID"),
		},
	}
}

func fetchPeerRecords(t *testing.T, api *peerAPI, cache *peers.Cache, refs ...string) ([]*model.PeerRecord, PeerStats) {
	t.Helper()
	out := &recWriter{}
	stats, err := FetchPeers(context.Background(), api, cache, out, PeerOptions{Refs: refs}, RateGuard{})
	require.NoError(t, err)
	require.Len(t, out.records, len(refs), "one record per ref")
	records := make([]*model.PeerRecord, 0, len(out.records))
	for i, r := range out.records {
		rec, ok := r.(*model.PeerRecord)
		require.True(t, ok)
		assert.Equal(t, "peer", rec.Kind)
		assert.Equal(t, refs[i], rec.Ref, "records follow the order of the refs")
		records = append(records, rec)
	}
	return records, stats
}

func TestFetchPeersBroadcastChannelWithLinkedChat(t *testing.T) {
	api := peerWorld()
	cache := peers.New()

	recs, stats := fetchPeerRecords(t, api, cache, "@news_channel")
	rec := recs[0]

	assert.Nil(t, rec.Error)
	assert.Equal(t, "channel:10", rec.UID)
	assert.Equal(t, "channel", rec.PeerType)
	assert.EqualValues(t, 10, rec.PeerID)
	assert.Equal(t, "News", rec.Title)
	assert.Equal(t, "news_channel", rec.Username)
	assert.Equal(t, []string{"news_short"}, rec.Usernames, "inactive and duplicate usernames are dropped")
	assert.Equal(t, "Daily news", rec.About)
	assert.Equal(t, 1000, rec.ParticipantsCount)
	assert.Equal(t, "channel:20", rec.LinkedChatUID)
	assert.Equal(t, &model.LinkedChat{UID: "channel:20", Title: "News chat", Username: "news_chat"}, rec.LinkedChat)
	assert.True(t, rec.Verified)
	assert.Equal(t, "Unavailable here", rec.Restricted)
	assert.Equal(t, "2020-09-13T12:26:40Z", rec.Date)
	assert.Nil(t, rec.Invite)
	assert.Equal(t, PeerStats{Fetched: 1}, stats)

	assert.Equal(t, []string{"resolve:news_channel", "full:channel:10:110"}, api.calls,
		"the linked chat comes from the same response, no second request")
	linked, ok := cache.Get(uid.KindChannel, 20)
	require.True(t, ok, "the linked chat is cached")
	assert.EqualValues(t, 120, linked.AccessHash)
}

func TestFetchPeersMegagroupLinksBackToChannel(t *testing.T) {
	recs, _ := fetchPeerRecords(t, peerWorld(), peers.New(), "https://t.me/news_chat")
	rec := recs[0]

	assert.Nil(t, rec.Error)
	assert.Equal(t, "supergroup", rec.PeerType)
	assert.Equal(t, "channel:20", rec.UID)
	assert.Equal(t, 300, rec.ParticipantsCount)
	assert.Equal(t, "channel:10", rec.LinkedChatUID)
	require.NotNil(t, rec.LinkedChat)
	assert.Equal(t, "news_channel", rec.LinkedChat.Username)
	assert.Empty(t, rec.Usernames)
}

func TestFetchPeersGigagroupIsSupergroup(t *testing.T) {
	recs, _ := fetchPeerRecords(t, peerWorld(), peers.New(), "https://t.me/s/big_group")
	rec := recs[0]

	assert.Nil(t, rec.Error)
	assert.Equal(t, "supergroup", rec.PeerType)
	assert.Equal(t, 250000, rec.ParticipantsCount)
	assert.Empty(t, rec.LinkedChatUID)
	assert.Nil(t, rec.LinkedChat)
	assert.Empty(t, rec.Date, "a zero date is omitted")
}

func TestFetchPeersFourLetterUsername(t *testing.T) {
	api := peerWorld()
	recs, stats := fetchPeerRecords(t, api, peers.New(), "@beta", "https://t.me/beta", "t.me/s/beta")
	for _, rec := range recs {
		assert.Nil(t, rec.Error)
		assert.Equal(t, "channel:70", rec.UID)
		assert.Equal(t, "beta", rec.Username)
		assert.Equal(t, "Short name", rec.About)
	}
	assert.Equal(t, PeerStats{Fetched: 3}, stats)
}

func TestFetchPeersBasicChat(t *testing.T) {
	api := peerWorld()
	recs, _ := fetchPeerRecords(t, api, peers.New(), "chat:40", "-40")

	for _, rec := range recs {
		assert.Nil(t, rec.Error)
		assert.Equal(t, "chat:40", rec.UID)
		assert.Equal(t, "chat", rec.PeerType)
		assert.Equal(t, "Family", rec.Title)
		assert.Equal(t, "Just us", rec.About)
		assert.Equal(t, 2, rec.ParticipantsCount, "the member list wins over the chat's own counter")
		assert.Equal(t, "2017-07-14T02:40:00Z", rec.Date)
	}
	assert.Equal(t, []string{"full:chat:40", "full:chat:40"}, api.calls)
}

func TestFetchPeersUserAndBot(t *testing.T) {
	api := peerWorld()
	recs, stats := fetchPeerRecords(t, api, peers.New(), "@some_user", "user:@helper_bot")

	assert.Equal(t, "user", recs[0].PeerType)
	assert.Equal(t, "user:50", recs[0].UID)
	assert.EqualValues(t, 50, recs[0].PeerID)
	assert.Equal(t, "Some User", recs[0].Title)
	assert.Equal(t, "some_user", recs[0].Username)
	assert.True(t, recs[0].Scam)
	assert.Equal(t, "bot", recs[1].PeerType)
	assert.Equal(t, "user:60", recs[1].UID)
	assert.Equal(t, "helper_bot", recs[1].Username)
	for _, rec := range recs {
		require.NotNil(t, rec.Error, "a person is never a success for a channel consumer")
		assert.Equal(t, PeerErrNotChannel, rec.Error.Code)
	}
	assert.Equal(t, PeerStats{Failed: 2}, stats)
	assert.Equal(t, []string{"resolve:some_user", "resolve:helper_bot"}, api.calls, "users get no full request")
}

func TestFetchPeersUserRefRejectsChannel(t *testing.T) {
	recs, _ := fetchPeerRecords(t, peerWorld(), peers.New(), "user:@news_channel")
	require.NotNil(t, recs[0].Error)
	assert.Equal(t, PeerErrNotFound, recs[0].Error.Code)
}

func TestFetchPeersInviteAlreadyMember(t *testing.T) {
	api := peerWorld()
	recs, _ := fetchPeerRecords(t, api, peers.New(), "https://t.me/+AlreadyIn")
	rec := recs[0]

	assert.Nil(t, rec.Error)
	assert.Equal(t, "channel:20", rec.UID)
	assert.Equal(t, "supergroup", rec.PeerType)
	assert.Equal(t, "Discuss the news", rec.About)
	assert.Equal(t, &model.InviteDescriptor{Hash: "AlreadyIn", Member: true}, rec.Invite)
	assert.Equal(t, []string{"invite:AlreadyIn", "full:channel:20:120"}, api.calls)
}

func TestFetchPeersInvitePeek(t *testing.T) {
	api := peerWorld()
	recs, stats := fetchPeerRecords(t, api, peers.New(), "https://t.me/joinchat/PeekIn")
	rec := recs[0]

	assert.Nil(t, rec.Error, "a peek invite carries the chat identity: not invite_not_member")
	assert.Equal(t, "channel:10", rec.UID)
	assert.Equal(t, "channel", rec.PeerType)
	assert.Equal(t, "Daily news", rec.About)
	assert.Equal(t, "channel:20", rec.LinkedChatUID)
	assert.Equal(t, &model.InviteDescriptor{
		Hash: "PeekIn", Member: false, Peek: true, Expires: "2023-11-14T22:13:20Z",
	}, rec.Invite)
	assert.Equal(t, PeerStats{Fetched: 1}, stats)
	assert.Equal(t, []string{"invite:PeekIn", "full:channel:10:110"}, api.calls)
}

func TestFetchPeersInviteNotMember(t *testing.T) {
	api := peerWorld()
	recs, stats := fetchPeerRecords(t, api, peers.New(), "https://t.me/+Closed", "https://t.me/+AskFirst")

	closed := recs[0]
	require.NotNil(t, closed.Error)
	assert.Equal(t, PeerErrInviteNotMember, closed.Error.Code)
	assert.Empty(t, closed.UID, "the preview reveals no ID")
	assert.Zero(t, closed.PeerID)
	assert.Equal(t, "supergroup", closed.PeerType)
	assert.Equal(t, "Closed club", closed.Title)
	assert.Equal(t, "Members only", closed.About)
	assert.Equal(t, 42, closed.ParticipantsCount)
	assert.Equal(t, &model.InviteDescriptor{Hash: "Closed", Member: false}, closed.Invite)

	gated := recs[1]
	require.NotNil(t, gated.Error)
	assert.Equal(t, PeerErrInviteNotMember, gated.Error.Code)
	assert.Equal(t, "channel", gated.PeerType)
	assert.Equal(t, &model.InviteDescriptor{Hash: "AskFirst", Member: false, RequestNeeded: true}, gated.Invite)

	assert.Equal(t, PeerStats{Failed: 2}, stats)
	assert.Equal(t, []string{"invite:Closed", "invite:AskFirst"}, api.calls, "a preview is never followed by a join")
}

func TestFetchPeersInviteExpired(t *testing.T) {
	recs, _ := fetchPeerRecords(t, peerWorld(), peers.New(), "https://t.me/+Expired", "https://t.me/+Unknown")
	for _, rec := range recs {
		require.NotNil(t, rec.Error)
		assert.Equal(t, PeerErrInviteExpired, rec.Error.Code)
		require.NotNil(t, rec.Invite)
		assert.False(t, rec.Invite.Member)
	}
}

func TestFetchPeersColdPeer(t *testing.T) {
	api := peerWorld()
	recs, stats := fetchPeerRecords(t, api, peers.New(), "channel:10", "-10010", "https://t.me/c/10", "user:50")

	for _, rec := range recs {
		require.NotNil(t, rec.Error)
		assert.Equal(t, PeerErrColdPeer, rec.Error.Code)
		assert.Contains(t, rec.Error.Message, "fetch dialogs")
	}
	assert.Equal(t, PeerStats{Failed: 4}, stats)
	assert.Empty(t, api.calls, "a cold numeric peer costs no request")
}

func TestFetchPeersWarmNumericPeers(t *testing.T) {
	api := peerWorld()
	cache := peers.New()
	cache.Put(peers.Entry{Kind: uid.KindChannel, ID: 10, AccessHash: 110})
	cache.Put(peers.Entry{Kind: uid.KindUser, ID: 50, AccessHash: 150, Title: "Some User", Username: "some_user"})

	recs, stats := fetchPeerRecords(t, api, cache, "channel:10", "https://t.me/c/10", "user:50")
	assert.Equal(t, "News", recs[0].Title)
	assert.Equal(t, "channel:10", recs[1].UID)
	assert.Equal(t, "user", recs[2].PeerType)
	assert.Equal(t, "some_user", recs[2].Username)
	require.NotNil(t, recs[2].Error)
	assert.Equal(t, PeerErrNotChannel, recs[2].Error.Code)
	assert.Equal(t, PeerStats{Fetched: 2, Failed: 1}, stats)
	assert.Equal(t, []string{"full:channel:10:110", "full:channel:10:110"}, api.calls)
}

func TestFetchPeersNotFound(t *testing.T) {
	recs, _ := fetchPeerRecords(t, peerWorld(), peers.New(), "@nobody_here", "@broken_nick")
	for _, rec := range recs {
		require.NotNil(t, rec.Error)
		assert.Equal(t, PeerErrNotFound, rec.Error.Code)
		assert.Empty(t, rec.UID)
	}
}

func TestFetchPeersPartialFailureKeepsOrder(t *testing.T) {
	api := peerWorld()
	cache := peers.New()
	cache.Put(peers.Entry{Kind: uid.KindChannel, ID: 99, AccessHash: 199, Title: "Private"})

	recs, stats := fetchPeerRecords(t, api, cache,
		"not a ref", "@nobody_here", "@news_chat", "channel:99", "channel:77", "https://t.me/+Closed", "@helper_bot")

	codes := make([]string, 0, len(recs))
	for _, rec := range recs {
		if rec.Error == nil {
			codes = append(codes, "")
			continue
		}
		codes = append(codes, rec.Error.Code)
	}
	assert.Equal(t, []string{
		PeerErrBadRef, PeerErrNotFound, "", PeerErrRPC, PeerErrColdPeer, PeerErrInviteNotMember, PeerErrNotChannel,
	}, codes)
	assert.Equal(t, "channel:99", recs[3].UID, "a failed full request keeps the known identity")
	assert.Equal(t, "Private", recs[3].Title)
	assert.Contains(t, recs[3].Error.Message, "CHANNEL_PRIVATE")
	assert.Equal(t, PeerStats{Fetched: 1, Failed: 6, BadRefs: 1}, stats)
}

func TestFetchPeersStopsOnCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	out := &recWriter{}
	_, err := FetchPeers(ctx, peerWorld(), peers.New(), out, PeerOptions{Refs: []string{"@news_channel"}}, RateGuard{})
	require.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, out.records)
}

func TestParsePeerRef(t *testing.T) {
	tests := map[string]uid.PeerRef{
		"@news_channel":                     {Kind: uid.KindUsername, Username: "news_channel"},
		"t.me/news_channel":                 {Kind: uid.KindUsername, Username: "news_channel"},
		"www.telegram.me/s/news_channel":    {Kind: uid.KindUsername, Username: "news_channel"},
		"https://t.me/c/10":                 {Kind: uid.KindChannel, ID: 10},
		"https://t.me/c/10/5":               {Kind: uid.KindChannel, ID: 10, AnchorID: 5, HasAnchor: true},
		"-10010":                            {Kind: uid.KindChannel, ID: 10},
		"t.me/+AbCd":                        {Kind: uid.KindInvite, Invite: "AbCd"},
		"https://www.t.me/joinchat/AbCd_-1": {Kind: uid.KindInvite, Invite: "AbCd_-1"},
	}
	for raw, want := range tests {
		t.Run(raw, func(t *testing.T) {
			got, err := ParsePeerRef(raw)
			require.NoError(t, err)
			assert.Equal(t, want, got)
		})
	}

	for _, raw := range []string{"", "news", "https://t.me/c/x", "https://t.me/c/0", "https://example.com/c/10"} {
		_, err := ParsePeerRef(raw)
		assert.Error(t, err, raw)
	}
}
