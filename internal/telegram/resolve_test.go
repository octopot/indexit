package telegram

import (
	"context"
	"errors"
	"testing"

	"github.com/gotd/td/tg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.octolab.org/toolset/indexit/internal/telegram/peers"
	"go.octolab.org/toolset/indexit/internal/telegram/uid"
)

func TestResolvePeerUsesCacheForNumericChannel(t *testing.T) {
	cache := peers.New()
	cache.Put(peers.Entry{Kind: uid.KindChannel, ID: 42, AccessHash: 7})

	resolved, err := ResolvePeer(context.Background(), fakeAPI{}, cache, uid.PeerRef{
		Kind:     uid.KindChannel,
		ID:       42,
		HasTopic: true,
		TopicID:  100,
	}, RateGuard{})

	require.NoError(t, err)
	input, ok := resolved.Input.(*tg.InputPeerChannel)
	require.True(t, ok)
	assert.EqualValues(t, 42, input.ChannelID)
	assert.EqualValues(t, 7, input.AccessHash)
	assert.Equal(t, "channel:42", resolved.UID)
	assert.Equal(t, 100, resolved.TopicID)
}

func TestResolvePeerRejectsColdNumericChannel(t *testing.T) {
	_, err := ResolvePeer(context.Background(), fakeAPI{}, peers.New(), uid.PeerRef{
		Kind: uid.KindChannel,
		ID:   42,
	}, RateGuard{})

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrColdPeer))
}

func TestResolvePeerResolvesUsernameAndCachesPeer(t *testing.T) {
	cache := peers.New()

	resolved, err := ResolvePeer(context.Background(), fakeAPI{
		resolved: &tg.ContactsResolvedPeer{
			Peer: &tg.PeerChannel{ChannelID: 42},
			Chats: []tg.ChatClass{
				&tg.Channel{ID: 42, AccessHash: 7, Username: "public", Title: "Public"},
			},
		},
	}, cache, uid.PeerRef{Kind: uid.KindUsername, Username: "public"}, RateGuard{})

	require.NoError(t, err)
	input, ok := resolved.Input.(*tg.InputPeerChannel)
	require.True(t, ok)
	assert.EqualValues(t, 42, input.ChannelID)
	assert.EqualValues(t, 7, input.AccessHash)
	assert.Equal(t, "channel:42", resolved.UID)

	entry, ok := cache.Get(uid.KindChannel, 42)
	require.True(t, ok)
	assert.EqualValues(t, 7, entry.AccessHash)
	assert.Equal(t, "public", entry.Username)
}

type fakeAPI struct {
	resolved *tg.ContactsResolvedPeer
}

func (f fakeAPI) ContactsResolveUsername(context.Context, *tg.ContactsResolveUsernameRequest) (*tg.ContactsResolvedPeer, error) {
	return f.resolved, nil
}

func (f fakeAPI) MessagesGetDialogs(context.Context, *tg.MessagesGetDialogsRequest) (tg.MessagesDialogsClass, error) {
	return nil, errors.New("unexpected MessagesGetDialogs call")
}

func (f fakeAPI) MessagesGetHistory(context.Context, *tg.MessagesGetHistoryRequest) (tg.MessagesMessagesClass, error) {
	return nil, errors.New("unexpected MessagesGetHistory call")
}

func (f fakeAPI) MessagesGetReplies(context.Context, *tg.MessagesGetRepliesRequest) (tg.MessagesMessagesClass, error) {
	return nil, errors.New("unexpected MessagesGetReplies call")
}

func (f fakeAPI) MessagesGetForumTopics(context.Context, *tg.MessagesGetForumTopicsRequest) (*tg.MessagesForumTopics, error) {
	return nil, errors.New("unexpected MessagesGetForumTopics call")
}

func (f fakeAPI) AuthLogOut(context.Context) (*tg.AuthLoggedOut, error) {
	return nil, errors.New("unexpected AuthLogOut call")
}

func (f fakeAPI) ChannelsGetMessages(context.Context, *tg.ChannelsGetMessagesRequest) (tg.MessagesMessagesClass, error) {
	return nil, errors.New("unexpected ChannelsGetMessages call")
}

func (f fakeAPI) MessagesGetMessages(context.Context, []tg.InputMessageClass) (tg.MessagesMessagesClass, error) {
	return nil, errors.New("unexpected MessagesGetMessages call")
}
