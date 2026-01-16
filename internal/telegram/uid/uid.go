package uid

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

type Kind string

const (
	KindUsername Kind = "username"
	KindUser     Kind = "user"
	KindChat     Kind = "chat"
	KindChannel  Kind = "channel"
)

type PeerRef struct {
	Kind     Kind
	ID       int64
	Username string

	TopicID   int
	HasTopic  bool
	AnchorID  int
	HasAnchor bool
}

// Four-letter usernames exist: Fragment sells them and t.me serves them.
var usernameRE = regexp.MustCompile(`^[A-Za-z0-9_]{4,32}$`)

func Parse(raw string) (PeerRef, error) {
	s := clean(raw)
	if s == "" {
		return PeerRef{}, fmt.Errorf("empty telegram uid")
	}

	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
		return parseURL(s)
	}

	if strings.HasPrefix(s, "-100") {
		return parseBotAPIChannel(s)
	}
	if strings.HasPrefix(s, "-") {
		id, err := parsePositive(s[1:])
		if err != nil {
			return PeerRef{}, acceptedErr(s)
		}
		return PeerRef{Kind: KindChat, ID: id}, nil
	}

	if strings.Contains(s, ":") {
		return parseKindValue(s)
	}

	if strings.HasPrefix(s, "@") {
		name := strings.TrimPrefix(s, "@")
		if !validUsername(name) {
			return PeerRef{}, acceptedErr(s)
		}
		return PeerRef{Kind: KindUsername, Username: name}, nil
	}

	if _, err := strconv.ParseInt(s, 10, 64); err == nil {
		return PeerRef{}, fmt.Errorf("ambiguous telegram uid %q: use user:<id>, chat:<id>, channel:<id>, -<id>, or -100<id>", s)
	}

	return PeerRef{}, acceptedErr(s)
}

func (r PeerRef) String() string {
	base := ""
	switch {
	case r.Username != "" && r.Kind == KindUser:
		base = "user:@" + r.Username
	case r.Username != "":
		base = "@" + r.Username
	case r.Kind == KindChannel:
		base = fmt.Sprintf("channel:%d", r.ID)
	case r.Kind == KindChat:
		base = fmt.Sprintf("chat:%d", r.ID)
	case r.Kind == KindUser:
		base = fmt.Sprintf("user:%d", r.ID)
	default:
		base = string(r.Kind)
	}
	if r.HasTopic {
		base += fmt.Sprintf(":%d", r.TopicID)
	}
	return base
}

func clean(raw string) string {
	s := strings.TrimSpace(raw)
	if strings.HasPrefix(s, "<") && strings.HasSuffix(s, ">") && len(s) > 1 {
		s = strings.TrimSpace(s[1 : len(s)-1])
	}
	s = strings.Trim(s, "`\"'")
	return s
}

func parseURL(raw string) (PeerRef, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return PeerRef{}, fmt.Errorf("parse telegram url: %w", err)
	}
	host := strings.ToLower(u.Host)
	if host != "t.me" && host != "www.t.me" && host != "telegram.me" && host != "www.telegram.me" {
		return PeerRef{}, acceptedErr(raw)
	}

	parts := strings.Split(strings.Trim(u.EscapedPath(), "/"), "/")
	for i, part := range parts {
		unescaped, err := url.PathUnescape(part)
		if err != nil {
			return PeerRef{}, fmt.Errorf("parse telegram url path: %w", err)
		}
		parts[i] = unescaped
	}
	if len(parts) == 0 || parts[0] == "" {
		return PeerRef{}, acceptedErr(raw)
	}

	switch {
	case parts[0] == "c":
		return parseInternalURL(raw, parts)
	case parts[0] == "s" && len(parts) > 1:
		// t.me/s/<nick> is the web preview of a public channel.
		return parsePublicURL(raw, parts[1:])
	}
	return parsePublicURL(raw, parts)
}

func parsePublicURL(raw string, parts []string) (PeerRef, error) {
	if len(parts) > 3 || !validUsername(parts[0]) {
		return PeerRef{}, acceptedErr(raw)
	}

	ref := PeerRef{Kind: KindUsername, Username: parts[0]}
	if len(parts) >= 2 {
		anchor, err := parseInt(parts[len(parts)-1])
		if err != nil {
			return PeerRef{}, acceptedErr(raw)
		}
		ref.AnchorID = anchor
		ref.HasAnchor = true
	}
	if len(parts) == 3 {
		topic, err := parseInt(parts[1])
		if err != nil {
			return PeerRef{}, acceptedErr(raw)
		}
		ref.TopicID = topic
		ref.HasTopic = true
	}
	return ref, nil
}

func parseInternalURL(raw string, parts []string) (PeerRef, error) {
	if len(parts) < 3 || len(parts) > 4 {
		return PeerRef{}, acceptedErr(raw)
	}
	id, err := parsePositive(parts[1])
	if err != nil {
		return PeerRef{}, acceptedErr(raw)
	}
	ref := PeerRef{Kind: KindChannel, ID: id}
	if len(parts) == 3 {
		anchor, err := parseInt(parts[2])
		if err != nil {
			return PeerRef{}, acceptedErr(raw)
		}
		ref.AnchorID = anchor
		ref.HasAnchor = true
		return ref, nil
	}

	topic, err := parseInt(parts[2])
	if err != nil {
		return PeerRef{}, acceptedErr(raw)
	}
	anchor, err := parseInt(parts[3])
	if err != nil {
		return PeerRef{}, acceptedErr(raw)
	}
	ref.TopicID = topic
	ref.HasTopic = true
	ref.AnchorID = anchor
	ref.HasAnchor = true
	return ref, nil
}

func parseBotAPIChannel(s string) (PeerRef, error) {
	value := strings.TrimPrefix(s, "-100")
	ref, err := parseChannelValue(value)
	if err != nil {
		return PeerRef{}, acceptedErr(s)
	}
	return ref, nil
}

func parseKindValue(s string) (PeerRef, error) {
	kind, value, ok := strings.Cut(s, ":")
	if !ok || value == "" {
		return PeerRef{}, acceptedErr(s)
	}

	switch Kind(kind) {
	case KindUser:
		if strings.HasPrefix(value, "@") {
			name := strings.TrimPrefix(value, "@")
			if !validUsername(name) {
				return PeerRef{}, acceptedErr(s)
			}
			return PeerRef{Kind: KindUser, Username: name}, nil
		}
		id, err := parsePositive(value)
		if err != nil {
			return PeerRef{}, acceptedErr(s)
		}
		return PeerRef{Kind: KindUser, ID: id}, nil
	case KindChat:
		id, err := parsePositive(value)
		if err != nil {
			return PeerRef{}, acceptedErr(s)
		}
		return PeerRef{Kind: KindChat, ID: id}, nil
	case KindChannel:
		ref, err := parseChannelValue(value)
		if err != nil {
			return PeerRef{}, acceptedErr(s)
		}
		return ref, nil
	default:
		return PeerRef{}, acceptedErr(s)
	}
}

func parseChannelValue(value string) (PeerRef, error) {
	idText, topicText, hasTopic := strings.Cut(value, ":")
	id, err := parsePositive(idText)
	if err != nil {
		return PeerRef{}, err
	}
	ref := PeerRef{Kind: KindChannel, ID: id}
	if hasTopic {
		topic, err := parseInt(topicText)
		if err != nil {
			return PeerRef{}, err
		}
		ref.TopicID = topic
		ref.HasTopic = true
	}
	return ref, nil
}

func parsePositive(s string) (int64, error) {
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil || id < 0 {
		return 0, fmt.Errorf("invalid positive integer %q", s)
	}
	return id, nil
}

func parseInt(s string) (int, error) {
	id, err := strconv.Atoi(s)
	if err != nil || id < 0 {
		return 0, fmt.Errorf("invalid integer %q", s)
	}
	return id, nil
}

func validUsername(s string) bool {
	return usernameRE.MatchString(s)
}

func acceptedErr(s string) error {
	return fmt.Errorf("unsupported telegram uid %q: accepted forms include @nick, user:<id>, user:@nick, chat:<id>, channel:<id>, -<id>, -100<id>, and t.me links", s)
}
