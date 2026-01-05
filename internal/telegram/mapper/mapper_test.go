package mapper

import (
	"testing"

	gotdpeer "github.com/gotd/td/telegram/message/peer"
	"github.com/gotd/td/tg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.octolab.org/toolset/indexit/internal/telegram/peers"
	"go.octolab.org/toolset/indexit/internal/telegram/uid"
)

func TestCacheEntities(t *testing.T) {
	cache := peers.New()
	entities := gotdpeer.NewEntities(
		map[int64]*tg.User{1: {ID: 1, AccessHash: 10, Username: "alice", FirstName: "Alice"}},
		map[int64]*tg.Chat{2: {ID: 2, Title: "Chat"}},
		map[int64]*tg.Channel{3: {ID: 3, AccessHash: 30, Username: "chan", Title: "Channel"}},
	)

	CacheEntities(cache, entities)

	user, ok := cache.Get(uid.KindUser, 1)
	require.True(t, ok)
	assert.Equal(t, int64(10), user.AccessHash)
	channel, ok := cache.Get(uid.KindChannel, 3)
	require.True(t, ok)
	assert.Equal(t, "chan", channel.Username)
}

func TestDialogMapsChannel(t *testing.T) {
	entities := gotdpeer.NewEntities(nil, nil, map[int64]*tg.Channel{
		3: {ID: 3, AccessHash: 30, Username: "chan", Title: "Channel", Megagroup: true, Forum: true},
	})

	record, ok := Dialog(&tg.Dialog{Peer: &tg.PeerChannel{ChannelID: 3}, UnreadCount: 2, Pinned: true}, entities, nil)
	require.True(t, ok)
	assert.Equal(t, "dialog", record.Kind)
	assert.Equal(t, "channel:3", record.UID)
	assert.Equal(t, "supergroup", record.PeerType)
	assert.True(t, record.IsForum)
}

func TestTopicMapsForumTopic(t *testing.T) {
	record := Topic("channel:3", &tg.ForumTopic{
		ID:          370,
		Title:       "Ваньково, июнь 2025",
		Date:        1700000000,
		TopMessage:  647,
		UnreadCount: 4,
		Closed:      true,
		Pinned:      true,
		My:          true,
	})
	require.NotNil(t, record)
	assert.Equal(t, "topic", record.Kind)
	assert.Equal(t, "channel:3", record.DialogUID)
	assert.Equal(t, 370, record.TopicID)
	assert.Equal(t, "Ваньково, июнь 2025", record.Title)
	assert.Equal(t, "2023-11-14T22:13:20Z", record.Date)
	assert.Equal(t, 647, record.TopMessage)
	assert.Equal(t, 4, record.Unread)
	assert.True(t, record.Closed)
	assert.True(t, record.Pinned)
	assert.True(t, record.My)
	assert.False(t, record.Hidden)
}

func TestTopicSkipsDeleted(t *testing.T) {
	assert.Nil(t, Topic("channel:3", &tg.ForumTopicDeleted{ID: 5}))
}
