package mapper

import (
	"fmt"
	"strings"
	"time"

	gotdpeer "github.com/gotd/td/telegram/message/peer"
	"github.com/gotd/td/tg"

	"go.octolab.org/toolset/indexit/internal/telegram/model"
	"go.octolab.org/toolset/indexit/internal/telegram/peers"
	"go.octolab.org/toolset/indexit/internal/telegram/uid"
)

func CacheEntities(cache *peers.Cache, entities gotdpeer.Entities) {
	for _, user := range entities.Users() {
		CacheUser(cache, user)
	}
	for _, chat := range entities.Chats() {
		cache.Put(peers.Entry{
			Kind:  uid.KindChat,
			ID:    chat.ID,
			Title: chat.Title,
		})
	}
	for _, channel := range entities.Channels() {
		CacheChannel(cache, channel)
	}
}

func CacheUser(cache *peers.Cache, user *tg.User) {
	if cache == nil || user == nil {
		return
	}
	cache.Put(peers.Entry{
		Kind:       uid.KindUser,
		ID:         user.ID,
		AccessHash: user.AccessHash,
		Username:   user.Username,
		Title:      userDisplay(user),
	})
}

func CacheChannel(cache *peers.Cache, channel *tg.Channel) {
	if cache == nil || channel == nil {
		return
	}
	cache.Put(peers.Entry{
		Kind:       uid.KindChannel,
		ID:         channel.ID,
		AccessHash: channel.AccessHash,
		Username:   channel.Username,
		Title:      channel.Title,
	})
}

func Dialog(dialog tg.DialogClass, entities gotdpeer.Entities, last tg.NotEmptyMessage) (model.DialogRecord, bool) {
	d, ok := dialog.(*tg.Dialog)
	if !ok {
		return model.DialogRecord{}, false
	}

	rec := model.DialogRecord{
		Kind:        "dialog",
		UnreadCount: d.UnreadCount,
		Pinned:      d.Pinned,
	}
	if last != nil && last.GetDate() > 0 {
		rec.LastMessageAt = unix(last.GetDate())
	}

	switch p := d.Peer.(type) {
	case *tg.PeerUser:
		user, ok := entities.User(p.UserID)
		if !ok {
			rec.PeerType = string(uid.KindUser)
			rec.PeerID = p.UserID
			rec.UID = fmt.Sprintf("user:%d", p.UserID)
			return rec, true
		}
		rec.PeerType = string(uid.KindUser)
		rec.PeerID = user.ID
		rec.UID = fmt.Sprintf("user:%d", user.ID)
		rec.Username = user.Username
		rec.Title = userDisplay(user)
		rec.Verified = user.Verified
		rec.Scam = user.Scam
		rec.Fake = user.Fake
	case *tg.PeerChat:
		chat, ok := entities.Chat(p.ChatID)
		rec.PeerType = string(uid.KindChat)
		rec.PeerID = p.ChatID
		rec.UID = fmt.Sprintf("chat:%d", p.ChatID)
		if ok {
			rec.Title = chat.Title
		}
	case *tg.PeerChannel:
		channel, ok := entities.Channel(p.ChannelID)
		rec.PeerID = p.ChannelID
		rec.UID = fmt.Sprintf("channel:%d", p.ChannelID)
		rec.PeerType = string(uid.KindChannel)
		if ok {
			if channel.Megagroup {
				rec.PeerType = "supergroup"
			}
			rec.Username = channel.Username
			rec.Title = channel.Title
			rec.IsForum = channel.Forum
			rec.Verified = channel.Verified
			rec.Scam = channel.Scam
			rec.Fake = channel.Fake
		}
	default:
		return model.DialogRecord{}, false
	}

	return rec, true
}

func Message(dialogUID string, topicID int, msg tg.NotEmptyMessage, entities gotdpeer.Entities) model.MessageRecord {
	rec := model.MessageRecord{
		Kind:      "message",
		DialogUID: dialogUID,
		ID:        msg.GetID(),
		TopicID:   topicID,
		Date:      unix(msg.GetDate()),
		Text:      "",
	}

	if from, ok := msg.GetFromID(); ok {
		rec.From = Peer(from, entities)
	}
	if reply, ok := msg.GetReplyTo(); ok {
		rec.ReplyTo = Reply(reply)
	}
	if reactions, ok := msg.GetReactions(); ok {
		rec.Reactions = Reactions(reactions)
	}

	if full, ok := msg.(*tg.Message); ok {
		rec.Text = full.Message
		if edit, ok := full.GetEditDate(); ok && edit > 0 {
			rec.EditDate = unix(edit)
		}
		if media, ok := full.GetMedia(); ok {
			rec.Media = Media(media)
		}
		if views, ok := full.GetViews(); ok {
			rec.Views = views
		}
		if fwd, ok := full.GetFwdFrom(); ok {
			rec.ForwardedFrom = Forward(fwd, entities)
		}
	}

	return rec
}

func Peer(peer tg.PeerClass, entities gotdpeer.Entities) *model.PeerDescriptor {
	switch p := peer.(type) {
	case *tg.PeerUser:
		desc := &model.PeerDescriptor{Type: string(uid.KindUser), ID: p.UserID}
		if user, ok := entities.User(p.UserID); ok {
			desc.Username = user.Username
			desc.Display = userDisplay(user)
		}
		return desc
	case *tg.PeerChat:
		desc := &model.PeerDescriptor{Type: string(uid.KindChat), ID: p.ChatID}
		if chat, ok := entities.Chat(p.ChatID); ok {
			desc.Display = chat.Title
		}
		return desc
	case *tg.PeerChannel:
		desc := &model.PeerDescriptor{Type: string(uid.KindChannel), ID: p.ChannelID}
		if channel, ok := entities.Channel(p.ChannelID); ok {
			if channel.Megagroup {
				desc.Type = "supergroup"
			}
			desc.Username = channel.Username
			desc.Display = channel.Title
		}
		return desc
	default:
		return nil
	}
}

func Reply(reply tg.MessageReplyHeaderClass) *model.ReplyDescriptor {
	header, ok := reply.(*tg.MessageReplyHeader)
	if !ok {
		return nil
	}
	out := &model.ReplyDescriptor{}
	if id, ok := header.GetReplyToMsgID(); ok {
		out.MessageID = id
	}
	if top, ok := header.GetReplyToTopID(); ok {
		out.TopID = top
	}
	if out.MessageID == 0 && out.TopID == 0 {
		return nil
	}
	return out
}

func Forward(fwd tg.MessageFwdHeader, entities gotdpeer.Entities) *model.ForwardDescriptor {
	out := &model.ForwardDescriptor{}
	if name, ok := fwd.GetFromName(); ok {
		out.FromName = name
	}
	if from, ok := fwd.GetFromID(); ok {
		out.From = Peer(from, entities)
	}
	if fwd.Date > 0 {
		out.Date = unix(fwd.Date)
	}
	if post, ok := fwd.GetChannelPost(); ok {
		out.PostID = post
	}
	if out.FromName == "" && out.From == nil && out.Date == "" && out.PostID == 0 {
		return nil
	}
	return out
}

func Media(media tg.MessageMediaClass) *model.MediaDescriptor {
	switch m := media.(type) {
	case *tg.MessageMediaPhoto:
		return &model.MediaDescriptor{Type: "photo"}
	case *tg.MessageMediaDocument:
		out := &model.MediaDescriptor{Type: "document"}
		if m.Video {
			out.Type = "video"
		}
		if m.Round {
			out.Type = "video"
		}
		if m.Voice {
			out.Type = "voice"
		}
		if doc, ok := m.Document.(*tg.Document); ok {
			out.MIME = doc.MimeType
			out.Size = doc.Size
			for _, attr := range doc.Attributes {
				switch a := attr.(type) {
				case *tg.DocumentAttributeAudio:
					if a.Voice {
						out.Type = "voice"
					} else if out.Type == "document" {
						out.Type = "audio"
					}
					out.Duration = a.Duration
				case *tg.DocumentAttributeVideo:
					if out.Type == "document" {
						out.Type = "video"
					}
					out.Duration = int(a.Duration)
				case *tg.DocumentAttributeSticker:
					out.Type = "sticker"
				}
			}
		}
		return out
	case *tg.MessageMediaWebPage:
		return &model.MediaDescriptor{Type: "webpage"}
	case *tg.MessageMediaGeo, *tg.MessageMediaGeoLive, *tg.MessageMediaVenue:
		return &model.MediaDescriptor{Type: "geo"}
	case *tg.MessageMediaContact:
		return &model.MediaDescriptor{Type: "contact"}
	case *tg.MessageMediaPoll:
		return &model.MediaDescriptor{Type: "poll"}
	default:
		return nil
	}
}

func Reactions(reactions tg.MessageReactions) *model.ReactionSummary {
	total := 0
	for _, result := range reactions.Results {
		total += result.Count
	}
	if total == 0 {
		return nil
	}
	return &model.ReactionSummary{Total: total}
}

func userDisplay(user *tg.User) string {
	if user == nil {
		return ""
	}
	return strings.TrimSpace(strings.Join([]string{user.FirstName, user.LastName}, " "))
}

func unix(value int) string {
	return time.Unix(int64(value), 0).UTC().Format(time.RFC3339)
}

// Topic maps a forum topic. The reduced constructor (tg.ForumTopicDeleted) has
// nothing but an ID, so it is dropped: a deleted topic is not a topic.
func Topic(dialogUID string, topic tg.ForumTopicClass) *model.TopicRecord {
	full, ok := topic.(*tg.ForumTopic)
	if !ok {
		return nil
	}
	return &model.TopicRecord{
		Kind:       "topic",
		DialogUID:  dialogUID,
		TopicID:    full.ID,
		Title:      full.Title,
		Date:       unix(full.Date),
		TopMessage: full.TopMessage,
		Unread:     full.UnreadCount,
		Closed:     full.Closed,
		Hidden:     full.Hidden,
		Pinned:     full.Pinned,
		My:         full.My,
	}
}
