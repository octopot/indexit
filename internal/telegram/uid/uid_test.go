package uid

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseAccepted(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want PeerRef
	}{
		{"user numeric", "user:42", PeerRef{Kind: KindUser, ID: 42}},
		{"user username", "user:@telegram", PeerRef{Kind: KindUser, Username: "telegram"}},
		{"bare username", "@telegram", PeerRef{Kind: KindUsername, Username: "telegram"}},
		{"chat", "chat:55", PeerRef{Kind: KindChat, ID: 55}},
		{"bot api chat", "-55", PeerRef{Kind: KindChat, ID: 55}},
		{"channel", "channel:77", PeerRef{Kind: KindChannel, ID: 77}},
		{"bot api channel", "-10077", PeerRef{Kind: KindChannel, ID: 77}},
		{"channel topic", "channel:77:12", PeerRef{Kind: KindChannel, ID: 77, TopicID: 12, HasTopic: true}},
		{"bot api channel topic", "-10077:12", PeerRef{Kind: KindChannel, ID: 77, TopicID: 12, HasTopic: true}},
		{"public url", "https://t.me/telegram", PeerRef{Kind: KindUsername, Username: "telegram"}},
		{"public url anchor", "https://t.me/telegram/123", PeerRef{Kind: KindUsername, Username: "telegram", AnchorID: 123, HasAnchor: true}},
		{"public url topic", "https://t.me/telegram/7/123", PeerRef{Kind: KindUsername, Username: "telegram", TopicID: 7, HasTopic: true, AnchorID: 123, HasAnchor: true}},
		{"internal url anchor", "https://t.me/c/77/123", PeerRef{Kind: KindChannel, ID: 77, AnchorID: 123, HasAnchor: true}},
		{"internal url topic", "https://t.me/c/77/7/123", PeerRef{Kind: KindChannel, ID: 77, TopicID: 7, HasTopic: true, AnchorID: 123, HasAnchor: true}},
		{"four-letter username", "@beta", PeerRef{Kind: KindUsername, Username: "beta"}},
		{"four-letter user", "user:@beta", PeerRef{Kind: KindUser, Username: "beta"}},
		{"four-letter url", "https://t.me/beta", PeerRef{Kind: KindUsername, Username: "beta"}},
		{"four-letter url anchor", "https://t.me/beta/12", PeerRef{Kind: KindUsername, Username: "beta", AnchorID: 12, HasAnchor: true}},
		{"trim wrappers", " <`@telegram`> ", PeerRef{Kind: KindUsername, Username: "telegram"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.raw)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseRejects(t *testing.T) {
	for _, raw := range []string{"", "123", "telegram", "bot:1", "https://example.com/x", "https://t.me/c/1", "channel:x",
		"@abc", "https://t.me/abc", "user:@abc"} {
		t.Run(raw, func(t *testing.T) {
			_, err := Parse(raw)
			assert.Error(t, err)
		})
	}
}

func TestString(t *testing.T) {
	tests := []struct {
		ref  PeerRef
		want string
	}{
		{PeerRef{Kind: KindUser, ID: 42}, "user:42"},
		{PeerRef{Kind: KindUser, Username: "telegram"}, "user:@telegram"},
		{PeerRef{Kind: KindUsername, Username: "telegram"}, "@telegram"},
		{PeerRef{Kind: KindChat, ID: 55}, "chat:55"},
		{PeerRef{Kind: KindChannel, ID: 77}, "channel:77"},
		{PeerRef{Kind: KindChannel, ID: 77, TopicID: 12, HasTopic: true}, "channel:77:12"},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, tt.ref.String())
	}
}
