package telegram

import (
	"context"
	"errors"
	"fmt"
	"strings"

	gotdpeer "github.com/gotd/td/telegram/message/peer"
	"github.com/gotd/td/tg"

	"go.octolab.org/toolset/indexit/internal/telegram/mapper"
	"go.octolab.org/toolset/indexit/internal/telegram/peers"
	"go.octolab.org/toolset/indexit/internal/telegram/uid"
)

var ErrColdPeer = errors.New("telegram peer is not in local cache")

type ResolvedPeer struct {
	Input   tg.InputPeerClass
	UID     string
	TopicID int
}

func ResolvePeer(ctx context.Context, api API, cache *peers.Cache, ref uid.PeerRef, guard RateGuard) (ResolvedPeer, error) {
	if ref.Kind == uid.KindChat {
		return ResolvedPeer{
			Input:   &tg.InputPeerChat{ChatID: ref.ID},
			UID:     fmt.Sprintf("chat:%d", ref.ID),
			TopicID: ref.TopicID,
		}, nil
	}

	if ref.Kind == uid.KindInvite {
		return ResolvedPeer{}, fmt.Errorf("invite link %s names no dialog here: run 'indexit telegram fetch peer' to inspect it", ref.String())
	}

	if ref.Username != "" {
		return resolveUsername(ctx, api, cache, ref, guard)
	}

	if ref.Kind == uid.KindUser || ref.Kind == uid.KindChannel {
		if entry, ok := cache.Get(ref.Kind, ref.ID); ok {
			return ResolvedPeer{
				Input:   inputFromCache(entry),
				UID:     fmt.Sprintf("%s:%d", ref.Kind, ref.ID),
				TopicID: ref.TopicID,
			}, nil
		}
		return ResolvedPeer{}, fmt.Errorf("%w: %s", ErrColdPeer, ref.String())
	}

	return ResolvedPeer{}, fmt.Errorf("unsupported telegram peer kind %q", ref.Kind)
}

func resolveUsername(ctx context.Context, api API, cache *peers.Cache, ref uid.PeerRef, guard RateGuard) (ResolvedPeer, error) {
	name := strings.TrimPrefix(ref.Username, "@")
	var result *tg.ContactsResolvedPeer
	if err := guard.Do(ctx, func(ctx context.Context) error {
		var err error
		result, err = api.ContactsResolveUsername(ctx, &tg.ContactsResolveUsernameRequest{Username: name})
		return err
	}); err != nil {
		return ResolvedPeer{}, err
	}

	entities := gotdpeer.EntitiesFromResult(result)
	mapper.CacheEntities(cache, entities)
	input, err := entities.ExtractPeer(result.Peer)
	if err != nil {
		return ResolvedPeer{}, fmt.Errorf("extract resolved peer: %w", err)
	}
	peerUID, err := canonicalUID(result.Peer)
	if err != nil {
		return ResolvedPeer{}, err
	}
	if ref.Kind == uid.KindUser {
		if _, ok := result.Peer.(*tg.PeerUser); !ok {
			return ResolvedPeer{}, fmt.Errorf("username @%s resolved to %T, want user", name, result.Peer)
		}
	}
	return ResolvedPeer{Input: input, UID: peerUID, TopicID: ref.TopicID}, nil
}

func canonicalUID(peer tg.PeerClass) (string, error) {
	switch p := peer.(type) {
	case *tg.PeerUser:
		return fmt.Sprintf("user:%d", p.UserID), nil
	case *tg.PeerChat:
		return fmt.Sprintf("chat:%d", p.ChatID), nil
	case *tg.PeerChannel:
		return fmt.Sprintf("channel:%d", p.ChannelID), nil
	default:
		return "", fmt.Errorf("unsupported resolved peer type %T", peer)
	}
}

func inputFromCache(entry peers.Entry) tg.InputPeerClass {
	switch entry.Kind {
	case uid.KindUser:
		return &tg.InputPeerUser{UserID: entry.ID, AccessHash: entry.AccessHash}
	case uid.KindChannel:
		return &tg.InputPeerChannel{ChannelID: entry.ID, AccessHash: entry.AccessHash}
	case uid.KindChat:
		return &tg.InputPeerChat{ChatID: entry.ID}
	default:
		return &tg.InputPeerEmpty{}
	}
}
