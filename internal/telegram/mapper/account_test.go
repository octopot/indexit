package mapper

import (
	"encoding/json"
	"testing"

	gotdpeer "github.com/gotd/td/telegram/message/peer"
	"github.com/gotd/td/tg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAccountStatusInDialogsAndPeers(t *testing.T) {
	channelAdmin := &tg.Channel{ID: 1}
	channelAdmin.SetAdminRights(tg.ChatAdminRights{DeleteMessages: true})
	chatAdmin := &tg.Chat{ID: 1}
	chatAdmin.SetAdminRights(tg.ChatAdminRights{InviteUsers: true})
	// The presence of admin_rights identifies an admin, even with no flags.
	emptyRights := &tg.Channel{ID: 1}
	emptyRights.SetAdminRights(tg.ChatAdminRights{})
	migrated := &tg.Chat{ID: 1}
	migrated.SetMigratedTo(&tg.InputChannel{ChannelID: 2})
	migratedOwner := *migrated
	migratedOwner.Creator = true

	for _, tt := range []struct {
		name                   string
		chat                   tg.ChatClass
		member, admin, creator bool
		unknown                bool
	}{
		{name: "channel member", chat: &tg.Channel{ID: 1}, member: true},
		{name: "supergroup member", chat: &tg.Channel{ID: 1, Megagroup: true}, member: true},
		{name: "gigagroup member", chat: &tg.Channel{ID: 1, Gigagroup: true}, member: true},
		{name: "channel admin", chat: channelAdmin, member: true, admin: true},
		{name: "admin with empty rights", chat: emptyRights, member: true, admin: true},
		{name: "channel owner without admin_rights", chat: &tg.Channel{ID: 1, Creator: true}, member: true, admin: true, creator: true},
		{name: "channel left", chat: &tg.Channel{ID: 1, Left: true}},
		{name: "channel owner left", chat: &tg.Channel{ID: 1, Left: true, Creator: true}, admin: true, creator: true},
		{name: "min channel", chat: &tg.Channel{ID: 1, Min: true}, unknown: true},
		{name: "min flags cannot be trusted", chat: &tg.Channel{ID: 1, Min: true, Creator: true, Left: true}, unknown: true},
		{name: "forbidden channel", chat: &tg.ChannelForbidden{ID: 1}, unknown: true},
		{name: "channel direct messages", chat: &tg.Channel{ID: 1, Monoforum: true}, unknown: true},
		{name: "channel direct messages owner", chat: &tg.Channel{ID: 1, Monoforum: true, Creator: true}, unknown: true},
		{name: "chat member", chat: &tg.Chat{ID: 1}, member: true},
		{name: "chat admin", chat: chatAdmin, member: true, admin: true},
		{name: "chat owner without admin_rights", chat: &tg.Chat{ID: 1, Creator: true}, member: true, admin: true, creator: true},
		{name: "chat left", chat: &tg.Chat{ID: 1, Left: true}},
		{name: "chat deactivated", chat: &tg.Chat{ID: 1, Deactivated: true}},
		{name: "chat migrated", chat: migrated},
		{name: "chat migrated owner", chat: &migratedOwner, admin: true, creator: true},
		{name: "chat deactivated owner", chat: &tg.Chat{ID: 1, Deactivated: true, Creator: true}, admin: true, creator: true},
		{name: "forbidden chat", chat: &tg.ChatForbidden{ID: 1}, unknown: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var peer tg.PeerClass = &tg.PeerChannel{ChannelID: 1}
			switch tt.chat.(type) {
			case *tg.Chat, *tg.ChatForbidden:
				peer = &tg.PeerChat{ChatID: 1}
			}
			chats := tg.ChatClassArray{tt.chat}
			entities := gotdpeer.NewEntities(nil, chats.ChatToMap(), chats.ChannelToMap())
			dialog, ok := Dialog(&tg.Dialog{Peer: peer}, entities, nil)
			require.True(t, ok)
			card, ok := PeerChat(tt.chat)
			require.True(t, ok)
			assert.Equal(t, dialog.AccountStatus, card.AccountStatus)

			for _, record := range []any{dialog, card} {
				data, err := json.Marshal(record)
				require.NoError(t, err)
				assert.Regexp(t, `^\{"_kind":`, string(data))
				var fields map[string]any
				require.NoError(t, json.Unmarshal(data, &fields))
				for field, want := range map[string]bool{
					"is_member": tt.member, "is_admin": tt.admin, "is_creator": tt.creator,
				} {
					if tt.unknown {
						assert.NotContains(t, fields, field)
					} else {
						assert.Equal(t, want, fields[field], "%s must include known false values", field)
					}
				}
			}
		})
	}
}

func TestAccountStatusUnknownOrNotApplicable(t *testing.T) {
	for _, peer := range []tg.PeerClass{
		&tg.PeerChannel{ChannelID: 1}, &tg.PeerChat{ChatID: 1}, &tg.PeerUser{UserID: 1},
	} {
		rec, ok := Dialog(&tg.Dialog{Peer: peer}, gotdpeer.Entities{}, nil)
		require.True(t, ok)
		assert.Nil(t, rec.IsMember)
		assert.Nil(t, rec.IsAdmin)
		assert.Nil(t, rec.IsCreator)
	}
	for _, rec := range []any{
		PeerUser(&tg.User{ID: 1}), PeerUser(&tg.User{ID: 1, Bot: true}),
		PeerInvite(&tg.ChatInvite{Channel: true, Title: "Preview"}),
	} {
		data, err := json.Marshal(rec)
		require.NoError(t, err)
		assert.NotContains(t, string(data), "is_member")
		assert.NotContains(t, string(data), "is_admin")
		assert.NotContains(t, string(data), "is_creator")
	}
}
