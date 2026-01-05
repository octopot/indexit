package telegram

import (
	"context"
	"testing"

	"github.com/gotd/td/tg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.octolab.org/toolset/indexit/internal/telegram/model"
	"go.octolab.org/toolset/indexit/internal/telegram/peers"
	"go.octolab.org/toolset/indexit/internal/telegram/uid"
)

func forumPeer(t *testing.T) (*peers.Cache, uid.PeerRef) {
	t.Helper()
	cache := peers.New()
	cache.Put(peers.Entry{Kind: uid.KindChannel, ID: 42, AccessHash: 7})
	ref, err := uid.Parse("channel:42")
	require.NoError(t, err)
	return cache, ref
}

func topic(id int, title string, top int) *tg.ForumTopic {
	return &tg.ForumTopic{ID: id, Title: title, TopMessage: top, Date: 1700000000}
}

func TestFetchTopicsPaginates(t *testing.T) {
	cache, ref := forumPeer(t)
	api := &scriptedAPI{topicsPages: []*tg.MessagesForumTopics{
		{Count: 3, Topics: []tg.ForumTopicClass{topic(2, "Ваньково, июнь 2025", 20)}},
		{Count: 3, Topics: []tg.ForumTopicClass{topic(3, "Тула, август 2024", 10)}},
		{Count: 3, Topics: []tg.ForumTopicClass{}},
	}}
	out := &recWriter{}

	require.NoError(t, FetchTopics(context.Background(), api, cache, out, TopicsOptions{Peer: ref}, RateGuard{}))
	require.Len(t, out.records, 2)

	first, ok := out.records[0].(*model.TopicRecord)
	require.True(t, ok)
	assert.Equal(t, "topic", first.Kind)
	assert.Equal(t, "channel:42", first.DialogUID)
	assert.Equal(t, 2, first.TopicID)
	assert.Equal(t, "Ваньково, июнь 2025", first.Title)
	assert.Equal(t, 20, first.TopMessage)

	require.Len(t, api.topicsReqs, 3)
	assert.Equal(t, 0, api.topicsReqs[0].OffsetTopic)
	assert.Equal(t, 2, api.topicsReqs[1].OffsetTopic, "cursor moves to the last topic of the page")
	assert.Equal(t, 20, api.topicsReqs[1].OffsetID)
}

func TestFetchTopicsStopsOnCount(t *testing.T) {
	cache, ref := forumPeer(t)
	api := &scriptedAPI{topicsPages: []*tg.MessagesForumTopics{
		{Count: 1, Topics: []tg.ForumTopicClass{topic(2, "one", 20)}},
	}}
	out := &recWriter{}

	require.NoError(t, FetchTopics(context.Background(), api, cache, out, TopicsOptions{Peer: ref}, RateGuard{}))
	assert.Len(t, out.records, 1)
	assert.Len(t, api.topicsReqs, 1, "the reported count ends the walk without an extra round trip")
}

func TestFetchTopicsHonoursLimit(t *testing.T) {
	cache, ref := forumPeer(t)
	api := &scriptedAPI{topicsPages: []*tg.MessagesForumTopics{
		{Count: 9, Topics: []tg.ForumTopicClass{topic(2, "one", 20), topic(3, "two", 19)}},
	}}
	out := &recWriter{}

	require.NoError(t, FetchTopics(context.Background(), api, cache, out, TopicsOptions{Peer: ref, Limit: 1}, RateGuard{}))
	assert.Len(t, out.records, 1)
	assert.Equal(t, 1, api.topicsReqs[0].Limit)
}

func TestFetchTopicsSkipsDeleted(t *testing.T) {
	cache, ref := forumPeer(t)
	api := &scriptedAPI{topicsPages: []*tg.MessagesForumTopics{
		{Count: 2, Topics: []tg.ForumTopicClass{&tg.ForumTopicDeleted{ID: 5}, topic(2, "one", 20)}},
	}}
	out := &recWriter{}

	require.NoError(t, FetchTopics(context.Background(), api, cache, out, TopicsOptions{Peer: ref}, RateGuard{}))
	require.Len(t, out.records, 1)
	assert.Equal(t, 2, out.records[0].(*model.TopicRecord).TopicID)
}

func TestFetchTopicsRejectsNonForum(t *testing.T) {
	cache := peers.New()
	cache.Put(peers.Entry{Kind: uid.KindUser, ID: 1, AccessHash: 2})
	ref, err := uid.Parse("user:1")
	require.NoError(t, err)

	err = FetchTopics(context.Background(), &scriptedAPI{}, cache, &recWriter{}, TopicsOptions{Peer: ref}, RateGuard{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "user:1")
	assert.Contains(t, err.Error(), "topics")
}
