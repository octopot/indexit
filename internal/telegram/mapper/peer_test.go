package mapper

import (
	"encoding/json"
	"testing"

	gotdpeer "github.com/gotd/td/telegram/message/peer"
	"github.com/gotd/td/tg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.octolab.org/toolset/indexit/internal/telegram/model"
	"go.octolab.org/toolset/indexit/internal/telegram/peers"
	"go.octolab.org/toolset/indexit/internal/telegram/uid"
)

func TestPeerUserWithPersonalChannel(t *testing.T) {
	user := &tg.User{
		ID: 50, AccessHash: 150, FirstName: "Some", LastName: "User", Username: "some_user",
		Phone: "15550100", Premium: true, Verified: true,
		Usernames: []tg.Username{{Username: "some_user", Active: true, Editable: true}, {Username: "some_alias", Active: true}},
	}
	full := &tg.UserFull{ID: 50, About: "Writes things"}
	full.SetPersonalChannelID(70)
	full.SetBirthday(tg.Birthday{Day: 9, Month: 3})
	full.SetStarsRating(tg.StarsRating{Level: 2, Stars: 1500})
	full.SetSendPaidMessagesStars(25)
	entities := gotdpeer.NewEntities(nil, nil, map[int64]*tg.Channel{
		70: {ID: 70, AccessHash: 170, Title: "Some blog", Usernames: []tg.Username{{Username: "some_blog", Active: true, Editable: true}}},
	})

	rec := PeerUser(user)
	ApplyUserFull(&rec, full, entities)

	assert.Equal(t, "user:50", rec.UID)
	assert.Equal(t, PeerTypeUser, rec.PeerType)
	assert.Equal(t, "Some User", rec.Title)
	assert.Equal(t, "Some", rec.FirstName)
	assert.Equal(t, "User", rec.LastName)
	assert.Equal(t, []string{"some_alias"}, rec.Usernames)
	assert.Equal(t, "15550100", rec.Phone, "the phone is emitted as returned")
	assert.True(t, rec.Premium)
	assert.True(t, rec.Verified)
	assert.Equal(t, "Writes things", rec.About)
	assert.Equal(t, "channel:70", rec.PersonalChannelUID)
	assert.Equal(t, &model.LinkedChat{UID: "channel:70", Title: "Some blog", Username: "some_blog"}, rec.PersonalChannel)
	assert.Equal(t, "--03-09", rec.Birthday, "a birthday without a year")
	assert.Equal(t, &model.StarsRating{Level: 2, Stars: 1500}, rec.StarsRating)
	require.NotNil(t, rec.SendPaidMessagesStars)
	assert.EqualValues(t, 25, *rec.SendPaidMessagesStars)
	assert.Nil(t, rec.Bot)
	assert.Nil(t, rec.Business)
}

func TestApplyUserFullPaidMessagePrice(t *testing.T) {
	for _, tt := range []struct {
		name string
		set  bool
		size int64
		want string
	}{
		{name: "absent: messages are free", want: `{}`},
		{name: "zero: paid, but this account is exempt", set: true, want: `{"send_paid_messages_stars":0}`},
		{name: "positive: the price", set: true, size: 25, want: `{"send_paid_messages_stars":25}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			full := &tg.UserFull{ID: 50}
			if tt.set {
				full.SetSendPaidMessagesStars(tt.size)
			}
			var rec model.PeerRecord
			ApplyUserFull(&rec, full, gotdpeer.NewEntities(nil, nil, nil))
			data, err := json.Marshal(struct {
				Stars *int64 `json:"send_paid_messages_stars,omitempty"`
			}{rec.SendPaidMessagesStars})
			require.NoError(t, err)
			assert.JSONEq(t, tt.want, string(data))
		})
	}
}

func TestApplyUserFullKeepsMissingPersonalChannel(t *testing.T) {
	full := &tg.UserFull{ID: 50}
	full.SetPersonalChannelID(70)
	rec := PeerUser(&tg.User{ID: 50})
	ApplyUserFull(&rec, full, gotdpeer.NewEntities(nil, nil, nil))

	assert.Equal(t, "channel:70", rec.PersonalChannelUID)
	assert.Nil(t, rec.PersonalChannel)
}

func TestPeerUserBotWithCommands(t *testing.T) {
	user := &tg.User{
		ID: 60, AccessHash: 160, FirstName: "Helper", Username: "helper_bot", Bot: true,
		BotNochats: true, BotInlinePlaceholder: "Search…", LangCode: "en",
	}
	user.SetBotActiveUsers(1200)
	full := &tg.UserFull{ID: 60, About: "I help"}
	full.SetBotInfo(tg.BotInfo{
		Description:      "A helpful bot",
		PrivacyPolicyURL: "https://example.org/privacy",
		Commands: []tg.BotCommand{
			{Command: "start", Description: "Start over"},
			{Command: "help"},
		},
	})

	rec := PeerUser(user)
	ApplyUserFull(&rec, full, gotdpeer.NewEntities(nil, nil, nil))

	assert.Equal(t, PeerTypeBot, rec.PeerType)
	assert.Equal(t, "I help", rec.About)
	assert.Equal(t, "en", rec.LangCode)
	require.NotNil(t, rec.Bot)
	assert.Equal(t, "A helpful bot", rec.Bot.Description)
	assert.Equal(t, "https://example.org/privacy", rec.Bot.PrivacyPolicyURL)
	assert.Equal(t, []model.BotCommand{{Command: "start", Description: "Start over"}, {Command: "help"}}, rec.Bot.Commands)
	assert.Equal(t, 1200, rec.Bot.ActiveUsers)
	assert.Equal(t, "Search…", rec.Bot.InlinePlaceholder)

	data, err := json.Marshal(rec.Bot)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"description": "A helpful bot",
		"commands": [{"command": "start", "description": "Start over"}, {"command": "help"}],
		"privacy_policy_url": "https://example.org/privacy",
		"active_users": 1200,
		"inline_placeholder": "Search…",
		"can_join_groups": false,
		"can_read_all_group_messages": false,
		"has_main_app": false
	}`, string(data), "known false capabilities survive JSON")
}

func TestApplyUserFullBusinessAndBirthday(t *testing.T) {
	full := &tg.UserFull{ID: 80}
	birthday := tg.Birthday{Day: 1, Month: 12}
	birthday.SetYear(1990)
	full.SetBirthday(birthday)
	location := tg.BusinessLocation{Address: "1 Main St"}
	location.SetGeoPoint(&tg.GeoPoint{Lat: 0, Long: 30.5})
	full.SetBusinessLocation(location)
	full.SetBusinessWorkHours(tg.BusinessWorkHours{
		TimezoneID: "Europe/Berlin",
		WeeklyOpen: []tg.BusinessWeeklyOpen{{StartMinute: 540, EndMinute: 1080}},
	})
	full.SetBusinessIntro(tg.BusinessIntro{Title: "Welcome", Description: "Ask us"})

	rec := PeerUser(&tg.User{ID: 80, FirstName: "Shop"})
	ApplyUserFull(&rec, full, gotdpeer.NewEntities(nil, nil, nil))

	assert.Equal(t, "1990-12-01", rec.Birthday)
	require.NotNil(t, rec.Business)
	data, err := json.Marshal(rec.Business)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"location": {"address": "1 Main St", "latitude": 0, "longitude": 30.5},
		"work_hours": {"timezone_id": "Europe/Berlin", "weekly_open": [{"start_minute": 540, "end_minute": 1080}]},
		"intro": {"title": "Welcome", "description": "Ask us"}
	}`, string(data), "a zero coordinate is a real one")
}

func TestPeerUserDeleted(t *testing.T) {
	rec := PeerUser(&tg.User{ID: 90, Deleted: true})
	ApplyUserFull(&rec, &tg.UserFull{ID: 90}, gotdpeer.NewEntities(nil, nil, nil))

	data, err := json.Marshal(rec)
	require.NoError(t, err)
	assert.JSONEq(t, `{"_kind":"peer","ref":"","uid":"user:90","peer_type":"user","peer_id":90,"deleted":true}`, string(data),
		"a deleted account, or one hidden by privacy, is identity alone")
}

func TestCacheUserMinNeverContributesAccessHash(t *testing.T) {
	cache := peers.New()
	CacheUser(cache, &tg.User{ID: 1, AccessHash: 10, Username: "alice", FirstName: "Alice"})
	CacheUser(cache, &tg.User{ID: 1, AccessHash: 99, Min: true, FirstName: "Alice M."})
	entry, ok := cache.Get(uid.KindUser, 1)
	require.True(t, ok)
	assert.EqualValues(t, 10, entry.AccessHash, "a min user keeps the cached entry untouched")
	assert.Equal(t, "Alice", entry.Title)

	CacheEntities(cache, gotdpeer.NewEntities(map[int64]*tg.User{2: {ID: 2, AccessHash: 20, Min: true, Username: "bob"}}, nil, nil))
	entry, ok = cache.Get(uid.KindUser, 2)
	require.True(t, ok, "with nothing cached, a min user leaves its identity")
	assert.Zero(t, entry.AccessHash)
	assert.Equal(t, "bob", entry.Username)
}

func TestCacheChannelMinNeverContributesAccessHash(t *testing.T) {
	cache := peers.New()
	CacheChannel(cache, &tg.Channel{ID: 1, AccessHash: 10, Title: "Full", Username: "full"})
	CacheChannel(cache, &tg.Channel{ID: 1, AccessHash: 99, Min: true, Title: "Forwarded"})
	entry, _ := cache.Get(uid.KindChannel, 1)
	assert.EqualValues(t, 10, entry.AccessHash, "a min channel never overwrites a full entry")
	assert.Equal(t, "Full", entry.Title)

	CacheEntities(cache, gotdpeer.NewEntities(nil, nil, map[int64]*tg.Channel{2: {ID: 2, AccessHash: 20, Min: true, Username: "source"}}))
	entry, ok := cache.Get(uid.KindChannel, 2)
	require.True(t, ok)
	assert.Zero(t, entry.AccessHash, "a min channel alone is an identity without a hash")
	assert.Equal(t, "source", entry.Username)
}
