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
	PreviewURL   string `json:"preview_url,omitempty"`
	PreviewError string `json:"preview_error,omitempty"`
	Type         string `json:"type"`
	MIME         string `json:"mime,omitempty"`
	Size         int64  `json:"size,omitempty"`
	Duration     int    `json:"duration,omitempty"`
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
	GroupedID     int64              `json:"grouped_id,omitempty"`
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
	FirstName         string      `json:"first_name,omitempty"`
	LastName          string      `json:"last_name,omitempty"`
	Phone             string      `json:"phone,omitempty"`
	LangCode          string      `json:"lang_code,omitempty"`
	About             string      `json:"about,omitempty"`
	ParticipantsCount int         `json:"participants_count,omitempty"`
	LinkedChatUID     string      `json:"linked_chat_uid,omitempty"`
	LinkedChat        *LinkedChat `json:"linked_chat,omitempty"`
	// PersonalChannelUID is the channel a user shows on the profile; the
	// UID stays when the channel itself is missing from the response.
	PersonalChannelUID    string        `json:"personal_channel_uid,omitempty"`
	PersonalChannel       *LinkedChat   `json:"personal_channel,omitempty"`
	Birthday              string        `json:"birthday,omitempty"`
	Business              *BusinessCard `json:"business,omitempty"`
	Bot                   *BotCard      `json:"bot,omitempty"`
	StarsRating           *StarsRating  `json:"stars_rating,omitempty"`
	SendPaidMessagesStars *int64        `json:"send_paid_messages_stars,omitempty"`
	Verified              bool          `json:"verified,omitempty"`
	Premium               bool          `json:"premium,omitempty"`
	Scam                  bool          `json:"scam,omitempty"`
	Fake                  bool          `json:"fake,omitempty"`
	Support               bool          `json:"support,omitempty"`
	Deleted               bool          `json:"deleted,omitempty"`
	Restricted            string        `json:"restricted,omitempty"`
	Date                  string        `json:"date,omitempty"`
	AccountStatus
	Invite *InviteDescriptor `json:"invite,omitempty"`
	Error  *PeerError        `json:"error,omitempty"`
}

// LinkedChat is the discussion group of a channel, the channel of a
// discussion group, or the personal channel of a user.
type LinkedChat struct {
	UID      string `json:"uid"`
	Title    string `json:"title,omitempty"`
	Username string `json:"username,omitempty"`
}

// BusinessCard is what a business account shows on its profile.
type BusinessCard struct {
	Location  *BusinessLocation  `json:"location,omitempty"`
	WorkHours *BusinessWorkHours `json:"work_hours,omitempty"`
	Intro     *BusinessIntro     `json:"intro,omitempty"`
}

// BusinessLocation is an address, a point on the map, or both. A zero
// coordinate is a real one, so known coordinates survive JSON.
type BusinessLocation struct {
	Address   string   `json:"address,omitempty"`
	Latitude  *float64 `json:"latitude,omitempty"`
	Longitude *float64 `json:"longitude,omitempty"`
}

// BusinessWorkHours lists the open intervals of a week in minutes counted
// from Monday 00:00 in the time zone of the business.
type BusinessWorkHours struct {
	TimezoneID string             `json:"timezone_id,omitempty"`
	WeeklyOpen []BusinessOpenSpan `json:"weekly_open,omitempty"`
}

type BusinessOpenSpan struct {
	StartMinute int `json:"start_minute"`
	EndMinute   int `json:"end_minute"`
}

type BusinessIntro struct {
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
}

// BotCard describes a bot. Its capabilities are known whenever the card is
// there, so false values must survive JSON. ActiveUsers is omitted when
// Telegram does not report it.
type BotCard struct {
	Description             string       `json:"description,omitempty"`
	Commands                []BotCommand `json:"commands,omitempty"`
	PrivacyPolicyURL        string       `json:"privacy_policy_url,omitempty"`
	ActiveUsers             int          `json:"active_users,omitempty"`
	InlinePlaceholder       string       `json:"inline_placeholder,omitempty"`
	CanJoinGroups           *bool        `json:"can_join_groups,omitempty"`
	CanReadAllGroupMessages *bool        `json:"can_read_all_group_messages,omitempty"`
	HasMainApp              *bool        `json:"has_main_app,omitempty"`
}

type BotCommand struct {
	Command     string `json:"command"`
	Description string `json:"description,omitempty"`
}

// StarsRating is the profile rating earned with Telegram Stars, as returned.
type StarsRating struct {
	Level int   `json:"level"`
	Stars int64 `json:"stars"`
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
