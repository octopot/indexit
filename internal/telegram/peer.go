package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"

	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"

	"go.octolab.org/toolset/indexit/internal/telegram/mapper"
	"go.octolab.org/toolset/indexit/internal/telegram/model"
	"go.octolab.org/toolset/indexit/internal/telegram/peers"
	"go.octolab.org/toolset/indexit/internal/telegram/uid"
)

// Error codes of a peer record that could not be fetched in full.
const (
	PeerErrBadRef          = "bad_ref"
	PeerErrNotFound        = "not_found"
	PeerErrColdPeer        = "cold_peer"
	PeerErrInviteNotMember = "invite_not_member"
	PeerErrInviteExpired   = "invite_expired"
	PeerErrRPC             = "rpc"
	// PeerErrNotChannel marks a user or a bot: the record carries its
	// identity, yet a consumer that checks only error must not take a person
	// for a channel or a group.
	PeerErrNotChannel = "not_channel"
)

type PeerOptions struct {
	Refs []string
}

// PeerStats counts the records of one FetchPeers run.
type PeerStats struct {
	Fetched int // records without an error
	Failed  int // records with an error, bad refs included
	BadRefs int // records rejected before any request
}

// peerFailure is a per-ref failure: it becomes the record's error and never
// aborts the batch. cause keeps the underlying error to tell a cancelled run
// from a failed ref.
type peerFailure struct {
	code    string
	message string
	cause   error
}

// FetchPeers emits one peer record per ref, in the order of the refs. A ref
// that cannot be parsed or fetched yields a record with an error instead of
// failing the run; only a write error or the end of the context does that.
//
// Channels and supergroups are read with channels.getFullChannel, basic chats
// with messages.getFullChat; users and bots are described from the resolved
// entity alone. Invite links are previewed with messages.checkChatInvite and
// are never joined.
func FetchPeers(ctx context.Context, api API, cache *peers.Cache, out Writer, opt PeerOptions, guard RateGuard) (PeerStats, error) {
	var stats PeerStats
	for _, raw := range opt.Refs {
		if err := ctx.Err(); err != nil {
			return stats, err
		}
		rec, failure := fetchPeer(ctx, api, cache, raw, guard)
		if failure == nil && (rec.PeerType == mapper.PeerTypeUser || rec.PeerType == mapper.PeerTypeBot) {
			failure = &peerFailure{
				code:    PeerErrNotChannel,
				message: fmt.Sprintf("%s is a %s, not a channel or a group", rec.UID, rec.PeerType),
			}
		}
		if failure != nil && failure.cause != nil && ctx.Err() != nil {
			return stats, ctx.Err()
		}
		rec.Kind = "peer"
		rec.Ref = raw
		if failure != nil {
			rec.Error = &model.PeerError{Code: failure.code, Message: failure.message}
			stats.Failed++
			if failure.code == PeerErrBadRef {
				stats.BadRefs++
			}
			slog.Default().Warn("peer: failed", "ref", raw, "code", failure.code, "error", failure.message)
		} else {
			stats.Fetched++
			slog.Default().Info("peer: fetched", "ref", raw, "uid", rec.UID, "type", rec.PeerType)
		}
		if err := out.Write(&rec); err != nil {
			return stats, err
		}
	}
	return stats, nil
}

// ParsePeerRef extends uid.Parse with the forms that name a peer but no
// message: a t.me link without a scheme and a bare t.me/c/<id> link.
func ParsePeerRef(raw string) (uid.PeerRef, error) {
	s := strings.TrimSpace(raw)
	lower := strings.ToLower(s)
	for _, host := range []string{"t.me/", "www.t.me/", "telegram.me/", "www.telegram.me/"} {
		if strings.HasPrefix(lower, host) {
			s = "https://" + s
			break
		}
	}
	ref, err := uid.Parse(s)
	if err == nil {
		return ref, nil
	}
	if id, ok := bareInternalLink(s); ok {
		return uid.PeerRef{Kind: uid.KindChannel, ID: id}, nil
	}
	return uid.PeerRef{}, err
}

func bareInternalLink(s string) (int64, bool) {
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return 0, false
	}
	switch strings.ToLower(u.Host) {
	case "t.me", "www.t.me", "telegram.me", "www.telegram.me":
	default:
		return 0, false
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 2 || parts[0] != "c" {
		return 0, false
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func fetchPeer(ctx context.Context, api API, cache *peers.Cache, raw string, guard RateGuard) (model.PeerRecord, *peerFailure) {
	ref, err := ParsePeerRef(raw)
	if err != nil {
		return model.PeerRecord{}, &peerFailure{code: PeerErrBadRef, message: err.Error()}
	}

	switch {
	case ref.Kind == uid.KindInvite:
		return fetchInvite(ctx, api, cache, ref.Invite, guard)
	case ref.Username != "":
		return fetchUsername(ctx, api, cache, ref, guard)
	case ref.Kind == uid.KindChannel:
		entry, ok := cache.Get(uid.KindChannel, ref.ID)
		if !ok {
			return model.PeerRecord{}, coldPeer(ref)
		}
		return fetchFullChannel(ctx, api, cache, &tg.InputChannel{ChannelID: entry.ID, AccessHash: entry.AccessHash}, guard)
	case ref.Kind == uid.KindChat:
		return fetchFullChat(ctx, api, cache, ref.ID, guard)
	case ref.Kind == uid.KindUser:
		entry, ok := cache.Get(uid.KindUser, ref.ID)
		if !ok {
			return model.PeerRecord{}, coldPeer(ref)
		}
		// The cache knows no bot flag and users get no full request: a
		// numeric user is described from the cache alone.
		return model.PeerRecord{
			UID:      fmt.Sprintf("%s:%d", uid.KindUser, entry.ID),
			PeerType: mapper.PeerTypeUser,
			PeerID:   entry.ID,
			Title:    entry.Title,
			Username: entry.Username,
		}, nil
	default:
		return model.PeerRecord{}, &peerFailure{code: PeerErrBadRef, message: fmt.Sprintf("unsupported telegram peer %s", ref.String())}
	}
}

func fetchUsername(ctx context.Context, api API, cache *peers.Cache, ref uid.PeerRef, guard RateGuard) (model.PeerRecord, *peerFailure) {
	var result *tg.ContactsResolvedPeer
	err := guard.Do(ctx, func(ctx context.Context) error {
		var err error
		result, err = api.ContactsResolveUsername(ctx, &tg.ContactsResolveUsernameRequest{Username: ref.Username})
		return err
	})
	if tgerr.Is(err, "USERNAME_NOT_OCCUPIED", "USERNAME_INVALID") {
		return model.PeerRecord{}, &peerFailure{
			code:    PeerErrNotFound,
			message: fmt.Sprintf("username @%s is not taken: %v", ref.Username, err),
			cause:   err,
		}
	}
	if err != nil {
		return model.PeerRecord{}, rpcFailure(err)
	}

	entities := entitiesFromLists(result.Users, result.Chats)
	mapper.CacheEntities(cache, entities)
	if _, isUser := result.Peer.(*tg.PeerUser); ref.Kind == uid.KindUser && !isUser {
		return model.PeerRecord{}, &peerFailure{
			code:    PeerErrNotFound,
			message: fmt.Sprintf("username @%s does not belong to a user", ref.Username),
		}
	}

	switch p := result.Peer.(type) {
	case *tg.PeerUser:
		user, ok := entities.User(p.UserID)
		if !ok {
			return model.PeerRecord{}, &peerFailure{code: PeerErrRPC, message: fmt.Sprintf("user %d is missing from the response", p.UserID)}
		}
		return mapper.PeerUser(user), nil
	case *tg.PeerChannel:
		channel, ok := entities.Channel(p.ChannelID)
		if !ok {
			return model.PeerRecord{}, &peerFailure{code: PeerErrRPC, message: fmt.Sprintf("channel %d is missing from the response", p.ChannelID)}
		}
		return fetchFullChannel(ctx, api, cache, &tg.InputChannel{ChannelID: channel.ID, AccessHash: channel.AccessHash}, guard)
	case *tg.PeerChat:
		return fetchFullChat(ctx, api, cache, p.ChatID, guard)
	default:
		return model.PeerRecord{}, &peerFailure{code: PeerErrRPC, message: fmt.Sprintf("unsupported resolved peer type %T", result.Peer)}
	}
}

// fetchFullChannel reads a channel or a supergroup in full. The channel itself
// and its linked chat come in the chats of the same response.
func fetchFullChannel(ctx context.Context, api API, cache *peers.Cache, input *tg.InputChannel, guard RateGuard) (model.PeerRecord, *peerFailure) {
	base := model.PeerRecord{
		UID:    fmt.Sprintf("%s:%d", uid.KindChannel, input.ChannelID),
		PeerID: input.ChannelID,
	}
	if entry, ok := cache.Get(uid.KindChannel, input.ChannelID); ok {
		base.Title = entry.Title
		base.Username = entry.Username
	}
	var result *tg.MessagesChatFull
	err := guard.Do(ctx, func(ctx context.Context) error {
		var err error
		result, err = api.ChannelsGetFullChannel(ctx, input)
		return err
	})
	if err != nil {
		return base, rpcFailure(err)
	}

	entities := entitiesFromLists(result.Users, result.Chats)
	mapper.CacheEntities(cache, entities)
	rec := base
	if mapped, ok := chatByID(result.Chats, input.ChannelID, true); ok {
		rec = mapped
	}
	full, _ := result.FullChat.(*tg.ChannelFull)
	mapper.ApplyChannelFull(&rec, full, entities)
	return rec, nil
}

// fetchFullChat reads a basic chat in full. No access hash is involved: a
// basic chat is addressed by its ID alone.
func fetchFullChat(ctx context.Context, api API, cache *peers.Cache, chatID int64, guard RateGuard) (model.PeerRecord, *peerFailure) {
	base := model.PeerRecord{
		UID:      fmt.Sprintf("%s:%d", uid.KindChat, chatID),
		PeerType: mapper.PeerTypeChat,
		PeerID:   chatID,
	}
	var result *tg.MessagesChatFull
	err := guard.Do(ctx, func(ctx context.Context) error {
		var err error
		result, err = api.MessagesGetFullChat(ctx, chatID)
		return err
	})
	if err != nil {
		return base, rpcFailure(err)
	}

	entities := entitiesFromLists(result.Users, result.Chats)
	mapper.CacheEntities(cache, entities)
	rec := base
	if mapped, ok := chatByID(result.Chats, chatID, false); ok {
		rec = mapped
	}
	full, _ := result.FullChat.(*tg.ChatFull)
	mapper.ApplyChatFull(&rec, full)
	return rec, nil
}

// fetchInvite previews an invite link. A chat the account is in, or may peek
// into, is then read in full like any other; a chat it is not in yields only
// the preview, without an ID.
func fetchInvite(ctx context.Context, api API, cache *peers.Cache, hash string, guard RateGuard) (model.PeerRecord, *peerFailure) {
	var result tg.ChatInviteClass
	err := guard.Do(ctx, func(ctx context.Context) error {
		var err error
		result, err = api.MessagesCheckChatInvite(ctx, hash)
		return err
	})
	if tgerr.Is(err, "INVITE_HASH_EXPIRED", "INVITE_HASH_INVALID") {
		return model.PeerRecord{Invite: &model.InviteDescriptor{Hash: hash, Member: false}}, &peerFailure{
			code:    PeerErrInviteExpired,
			message: fmt.Sprintf("invite link is expired or revoked: %v", err),
			cause:   err,
		}
	}
	if err != nil {
		return model.PeerRecord{Invite: &model.InviteDescriptor{Hash: hash, Member: false}}, rpcFailure(err)
	}

	switch inv := result.(type) {
	case *tg.ChatInviteAlready:
		rec, failure := fetchInviteChat(ctx, api, cache, inv.Chat, guard)
		rec.Invite = &model.InviteDescriptor{Hash: hash, Member: true}
		return rec, failure
	case *tg.ChatInvitePeek:
		rec, failure := fetchInviteChat(ctx, api, cache, inv.Chat, guard)
		rec.Invite = &model.InviteDescriptor{Hash: hash, Member: false, Peek: true, Expires: mapper.InviteExpires(inv.Expires)}
		return rec, failure
	case *tg.ChatInvite:
		mapper.CacheEntities(cache, entitiesFromLists(inv.Participants, nil))
		rec := mapper.PeerInvite(inv)
		rec.Invite = &model.InviteDescriptor{Hash: hash, Member: false, RequestNeeded: inv.RequestNeeded}
		return rec, &peerFailure{
			code:    PeerErrInviteNotMember,
			message: "the account is not a member of this chat and the preview carries no chat ID; indexit never joins chats",
		}
	default:
		return model.PeerRecord{Invite: &model.InviteDescriptor{Hash: hash, Member: false}}, &peerFailure{
			code:    PeerErrRPC,
			message: fmt.Sprintf("unsupported invite type %T", result),
		}
	}
}

func fetchInviteChat(ctx context.Context, api API, cache *peers.Cache, chat tg.ChatClass, guard RateGuard) (model.PeerRecord, *peerFailure) {
	mapper.CacheEntities(cache, entitiesFromLists(nil, []tg.ChatClass{chat}))
	base, _ := mapper.PeerChat(chat)
	var (
		rec     model.PeerRecord
		failure *peerFailure
	)
	switch c := chat.(type) {
	case *tg.Channel:
		rec, failure = fetchFullChannel(ctx, api, cache, &tg.InputChannel{ChannelID: c.ID, AccessHash: c.AccessHash}, guard)
	case *tg.Chat:
		rec, failure = fetchFullChat(ctx, api, cache, c.ID, guard)
	default:
		return base, &peerFailure{code: PeerErrRPC, message: fmt.Sprintf("invite leads to an inaccessible chat (%T)", chat)}
	}
	if failure != nil {
		// Keep what the invite told about the chat.
		return base, failure
	}
	return rec, nil
}

// chatByID maps the chat with the given ID out of a response's chats.
func chatByID(chats []tg.ChatClass, id int64, channel bool) (model.PeerRecord, bool) {
	for _, chat := range chats {
		var chatID int64
		var isChannel bool
		switch c := chat.(type) {
		case *tg.Channel:
			chatID, isChannel = c.ID, true
		case *tg.ChannelForbidden:
			chatID, isChannel = c.ID, true
		case *tg.Chat:
			chatID = c.ID
		case *tg.ChatForbidden:
			chatID = c.ID
		default:
			continue
		}
		if chatID == id && isChannel == channel {
			return mapper.PeerChat(chat)
		}
	}
	return model.PeerRecord{}, false
}

func coldPeer(ref uid.PeerRef) *peerFailure {
	err := ColdPeerHint(fmt.Errorf("%w: %s", ErrColdPeer, ref.String()), ref.String())
	return &peerFailure{code: PeerErrColdPeer, message: err.Error()}
}

func rpcFailure(err error) *peerFailure {
	return &peerFailure{code: PeerErrRPC, message: err.Error(), cause: err}
}
