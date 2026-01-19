package mapper

import (
	"fmt"
	"strings"

	gotdpeer "github.com/gotd/td/telegram/message/peer"
	"github.com/gotd/td/tg"

	"go.octolab.org/toolset/indexit/internal/telegram/model"
	"go.octolab.org/toolset/indexit/internal/telegram/uid"
)

// Peer types of a PeerRecord. A gigagroup is a supergroup too: it keeps the
// supergroup identity, only posting is restricted to admins.
const (
	PeerTypeChannel    = "channel"
	PeerTypeSupergroup = "supergroup"
	PeerTypeChat       = "chat"
	PeerTypeUser       = "user"
	PeerTypeBot        = "bot"
)

// PeerChat maps a chat-like entity — a basic chat, a channel, or their
// forbidden stubs — to the identity part of a peer card.
func PeerChat(chat tg.ChatClass) (model.PeerRecord, bool) {
	switch c := chat.(type) {
	case *tg.Channel:
		return PeerChannel(c), true
	case *tg.ChannelForbidden:
		rec := peerRecord(uid.KindChannel, c.ID)
		rec.PeerType = channelType(c.Megagroup, false)
		rec.Title = c.Title
		return rec, true
	case *tg.Chat:
		rec := peerRecord(uid.KindChat, c.ID)
		rec.PeerType = PeerTypeChat
		rec.Title = c.Title
		rec.ParticipantsCount = c.ParticipantsCount
		rec.AccountStatus = accountStatus(c)
		if c.Date > 0 {
			rec.Date = unix(c.Date)
		}
		return rec, true
	case *tg.ChatForbidden:
		rec := peerRecord(uid.KindChat, c.ID)
		rec.PeerType = PeerTypeChat
		rec.Title = c.Title
		return rec, true
	default:
		return model.PeerRecord{}, false
	}
}

// PeerChannel maps a channel or a supergroup. Date is the channel's own date:
// when the account joined it, or when it was created for a non-member.
func PeerChannel(channel *tg.Channel) model.PeerRecord {
	rec := peerRecord(uid.KindChannel, channel.ID)
	rec.AccountStatus = accountStatus(channel)
	rec.PeerType = channelType(channel.Megagroup, channel.Gigagroup)
	rec.Title = channel.Title
	rec.Username, rec.Usernames = usernames(channel.Username, channel.Usernames)
	rec.Verified = channel.Verified
	rec.Scam = channel.Scam
	rec.Fake = channel.Fake
	rec.Restricted = restriction(channel.RestrictionReason)
	if channel.Date > 0 {
		rec.Date = unix(channel.Date)
	}
	return rec
}

// PeerUser maps a user or a bot.
func PeerUser(user *tg.User) model.PeerRecord {
	rec := peerRecord(uid.KindUser, user.ID)
	rec.PeerType = PeerTypeUser
	if user.Bot {
		rec.PeerType = PeerTypeBot
	}
	rec.Title = userDisplay(user)
	rec.Username, rec.Usernames = usernames(user.Username, user.Usernames)
	rec.Verified = user.Verified
	rec.Scam = user.Scam
	rec.Fake = user.Fake
	rec.Restricted = restriction(user.RestrictionReason)
	return rec
}

// ApplyChannelFull adds the full-channel details. The linked chat is looked up
// among the entities of the same response, which Telegram fills with it.
func ApplyChannelFull(rec *model.PeerRecord, full *tg.ChannelFull, entities gotdpeer.Entities) {
	if full == nil {
		return
	}
	rec.About = full.About
	if full.ParticipantsCount > 0 {
		rec.ParticipantsCount = full.ParticipantsCount
	}
	if full.LinkedChatID == 0 {
		return
	}
	rec.LinkedChatUID = fmt.Sprintf("%s:%d", uid.KindChannel, full.LinkedChatID)
	if linked, ok := entities.Channel(full.LinkedChatID); ok {
		username, _ := usernames(linked.Username, linked.Usernames)
		rec.LinkedChat = &model.LinkedChat{
			UID:      rec.LinkedChatUID,
			Title:    linked.Title,
			Username: username,
		}
	}
}

// ApplyChatFull adds the full-chat details of a basic chat. Its participant
// count is the length of the member list, when Telegram sends one.
func ApplyChatFull(rec *model.PeerRecord, full *tg.ChatFull) {
	if full == nil {
		return
	}
	rec.About = full.About
	if list, ok := full.Participants.(*tg.ChatParticipants); ok && len(list.Participants) > 0 {
		rec.ParticipantsCount = len(list.Participants)
	}
}

// PeerInvite maps the preview of an invite link to a chat the account is not
// in. Such a preview carries no chat ID, so the record has no UID.
func PeerInvite(invite *tg.ChatInvite) model.PeerRecord {
	rec := model.PeerRecord{Kind: "peer"}
	switch {
	case !invite.Channel:
		rec.PeerType = PeerTypeChat
	case invite.Broadcast:
		rec.PeerType = PeerTypeChannel
	default:
		rec.PeerType = PeerTypeSupergroup
	}
	rec.Title = invite.Title
	rec.About = invite.About
	rec.ParticipantsCount = invite.ParticipantsCount
	rec.Verified = invite.Verified
	rec.Scam = invite.Scam
	rec.Fake = invite.Fake
	return rec
}

// InviteExpires renders the expiry date of a peek invite.
func InviteExpires(date int) string {
	if date <= 0 {
		return ""
	}
	return unix(date)
}

func peerRecord(kind uid.Kind, id int64) model.PeerRecord {
	return model.PeerRecord{
		Kind:   "peer",
		UID:    fmt.Sprintf("%s:%d", kind, id),
		PeerID: id,
	}
}

func channelType(megagroup, gigagroup bool) string {
	if megagroup || gigagroup {
		return PeerTypeSupergroup
	}
	return PeerTypeChannel
}

// usernames picks the main username and the other active ones. Without a
// plain username, the main one is the active editable entry, else the first
// active entry. Inactive entries and duplicates are dropped.
func usernames(main string, list []tg.Username) (string, []string) {
	if main == "" {
		for _, u := range list {
			if u.Active && u.Editable {
				main = u.Username
				break
			}
		}
	}
	if main == "" {
		for _, u := range list {
			if u.Active {
				main = u.Username
				break
			}
		}
	}
	var rest []string
	seen := map[string]struct{}{strings.ToLower(main): {}}
	for _, u := range list {
		key := strings.ToLower(u.Username)
		if !u.Active || u.Username == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		rest = append(rest, u.Username)
	}
	return main, rest
}

// restriction joins the distinct texts of the restriction reasons.
func restriction(reasons []tg.RestrictionReason) string {
	var texts []string
	seen := map[string]struct{}{}
	for _, r := range reasons {
		text := strings.TrimSpace(r.Text)
		if text == "" {
			text = r.Reason
		}
		if text == "" {
			continue
		}
		if _, ok := seen[text]; ok {
			continue
		}
		seen[text] = struct{}{}
		texts = append(texts, text)
	}
	return strings.Join(texts, "; ")
}
