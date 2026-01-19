package model

type DialogRecord struct {
	Kind          string `json:"_kind"`
	UID           string `json:"uid"`
	PeerType      string `json:"peer_type"`
	PeerID        int64  `json:"peer_id"`
	Username      string `json:"username,omitempty"`
	Title         string `json:"title,omitempty"`
	IsForum       bool   `json:"is_forum,omitempty"`
	UnreadCount   int    `json:"unread_count,omitempty"`
	LastMessageAt string `json:"last_message_at,omitempty"`
	Pinned        bool   `json:"pinned,omitempty"`
	Verified      bool   `json:"verified,omitempty"`
	Scam          bool   `json:"scam,omitempty"`
	Fake          bool   `json:"fake,omitempty"`
	AccountStatus
}

// AccountStatus describes the authenticated account in a channel or group.
// Nil means unknown or not applicable; known false values must survive JSON.
type AccountStatus struct {
	IsMember  *bool `json:"is_member,omitempty"`
	IsAdmin   *bool `json:"is_admin,omitempty"`
	IsCreator *bool `json:"is_creator,omitempty"`
}

type PeerDescriptor struct {
	Type     string `json:"type"`
	ID       int64  `json:"id"`
	Username string `json:"username,omitempty"`
	Display  string `json:"display,omitempty"`
}

type MediaDescriptor struct {
	Type     string `json:"type"`
	MIME     string `json:"mime,omitempty"`
	Size     int64  `json:"size,omitempty"`
	Duration int    `json:"duration,omitempty"`
}

type ReplyDescriptor struct {
	MessageID int `json:"message_id,omitempty"`
	TopID     int `json:"top_id,omitempty"`
}

type ForwardDescriptor struct {
	FromName string          `json:"from_name,omitempty"`
	From     *PeerDescriptor `json:"from,omitempty"`
	Date     string          `json:"date,omitempty"`
	PostID   int             `json:"post_id,omitempty"`
}

type ReactionSummary struct {
	Total int `json:"total"`
}

type MessageRecord struct {
	Kind          string             `json:"_kind"`
	DialogUID     string             `json:"dialog_uid"`
	ID            int                `json:"id"`
	TopicID       int                `json:"topic_id,omitempty"`
	Date          string             `json:"date"`
	EditDate      string             `json:"edit_date,omitempty"`
	From          *PeerDescriptor    `json:"from,omitempty"`
	Text          string             `json:"text"`
	Media         *MediaDescriptor   `json:"media,omitempty"`
	ReplyTo       *ReplyDescriptor   `json:"reply_to,omitempty"`
	ForwardedFrom *ForwardDescriptor `json:"forwarded_from,omitempty"`
	Views         int                `json:"views,omitempty"`
	Reactions     *ReactionSummary   `json:"reactions,omitempty"`
}

// TopicRecord is one forum topic of a dialog.
type TopicRecord struct {
	Kind       string `json:"_kind"`
	DialogUID  string `json:"dialog_uid"`
	TopicID    int    `json:"topic_id"`
	Title      string `json:"title"`
	Date       string `json:"date"`
	TopMessage int    `json:"top_message,omitempty"`
	Messages   int    `json:"messages,omitempty"`
	Unread     int    `json:"unread_count,omitempty"`
	Closed     bool   `json:"closed,omitempty"`
	Hidden     bool   `json:"hidden,omitempty"`
	Pinned     bool   `json:"pinned,omitempty"`
	My         bool   `json:"my,omitempty"`
}

// MediaRecord is one downloaded (or skipped) file. It is the manifest side of
// `fetch media`: the bytes go to disk, this says where they came from.
type MediaRecord struct {
	Kind      string `json:"_kind"`
	DialogUID string `json:"dialog_uid"`
	MessageID int    `json:"message_id"`
	TopicID   int    `json:"topic_id,omitempty"`
	GroupedID int64  `json:"grouped_id,omitempty"`
	Date      string `json:"date"`
	Type      string `json:"type"`
	MIME      string `json:"mime,omitempty"`
	Size      int64  `json:"size,omitempty"`
	Path      string `json:"path"`
	Skipped   bool   `json:"skipped,omitempty"`
	Error     string `json:"error,omitempty"`
}

// PeerRecord is the card of one peer as asked for by `fetch peer`: one record
// per ref, in the order of the refs. A ref that could not be fetched still
// yields a record, with Error set and as much identity as was learned.
type PeerRecord struct {
	Kind              string      `json:"_kind"`
	Ref               string      `json:"ref"`
	UID               string      `json:"uid,omitempty"`
	PeerType          string      `json:"peer_type,omitempty"`
	PeerID            int64       `json:"peer_id,omitempty"`
	Title             string      `json:"title,omitempty"`
	Username          string      `json:"username,omitempty"`
	Usernames         []string    `json:"usernames,omitempty"`
	About             string      `json:"about,omitempty"`
	ParticipantsCount int         `json:"participants_count,omitempty"`
	LinkedChatUID     string      `json:"linked_chat_uid,omitempty"`
	LinkedChat        *LinkedChat `json:"linked_chat,omitempty"`
	Verified          bool        `json:"verified,omitempty"`
	Scam              bool        `json:"scam,omitempty"`
	Fake              bool        `json:"fake,omitempty"`
	Restricted        string      `json:"restricted,omitempty"`
	Date              string      `json:"date,omitempty"`
	AccountStatus
	Invite *InviteDescriptor `json:"invite,omitempty"`
	Error  *PeerError        `json:"error,omitempty"`
}

// LinkedChat is the discussion group of a channel, or the channel of a
// discussion group.
type LinkedChat struct {
	UID      string `json:"uid"`
	Title    string `json:"title,omitempty"`
	Username string `json:"username,omitempty"`
}

// InviteDescriptor describes the invite link a peer was reached through.
// Member tells whether the account is in the chat; Peek, that it may preview
// the chat without joining it until Expires.
type InviteDescriptor struct {
	Hash          string `json:"hash"`
	Member        bool   `json:"member"`
	Peek          bool   `json:"peek,omitempty"`
	Expires       string `json:"expires,omitempty"`
	RequestNeeded bool   `json:"request_needed,omitempty"`
}

// PeerError explains why a ref yielded no full card.
type PeerError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
