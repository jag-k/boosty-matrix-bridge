package boosty

import (
	"encoding/json"
	"time"
)

// AuthResponse is the response from POST /oauth/token.
type AuthResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

// AuthData holds the current authentication state.
type AuthData struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	DeviceID     string `json:"device_id"`
	ExpiresAt    time.Time
}

// UserInfo is the response from GET /v1/user/current.
type UserInfo struct {
	ID         int    `json:"id"`
	BlogURL    string `json:"blogUrl"`
	Name       string `json:"name"`
	HasAvatar  bool   `json:"hasAvatar"`
	AvatarURL  string `json:"avatarUrl"`
	IsVerified bool   `json:"isVerifiedStreamer"`
	Email      string `json:"email"`
}

// DialogListResponse is the response from GET /v1/dialog/.
type DialogListResponse struct {
	Extra DialogListExtra `json:"extra"`
	Data  []Dialog        `json:"data"`
}

// DialogListExtra contains pagination info.
type DialogListExtra struct {
	Offset json.RawMessage `json:"offset"`
	Total  int             `json:"total"`
}

// Dialog represents a single dialog (DM conversation).
type Dialog struct {
	ID             int      `json:"id"`
	UnreadMsgCount int      `json:"unreadMsgCount"`
	Chatmate       Chatmate `json:"chatmate"`
	LastMessage    *Message `json:"lastMessage,omitempty"`
}

// Chatmate is the other party in a dialog.
type Chatmate struct {
	URL        string `json:"url"`
	ID         int    `json:"id"`
	Name       string `json:"name"`
	HasAvatar  bool   `json:"hasAvatar"`
	AvatarURL  string `json:"avatarUrl"`
	IsOfficial bool   `json:"isOfficial"`
	IsBlogger  bool   `json:"isBlogger"`
}

// DialogDetailResponse is the response from GET /v1/dialog/{dialogId}.
type DialogDetailResponse struct {
	SignedQuery string          `json:"signedQuery"`
	Chatmate    Chatmate        `json:"chatmate"`
	Relation    DialogRelation  `json:"relation"`
	Messages    MessageListData `json:"messages"`
}

// DialogRelation describes the relationship between the users.
type DialogRelation struct {
	CanWrite   bool `json:"canWrite"`
	CanWriteMe bool `json:"canWriteMe"`
}

// MessageListData wraps a list of messages.
type MessageListData struct {
	Data []Message `json:"data"`
}

// Message is a single message in a dialog.
type Message struct {
	ID        int             `json:"id"`
	DialogID  int             `json:"dialogId,omitempty"`
	AuthorID  int             `json:"authorId"`
	IsAuthor  bool            `json:"isAuthor"`
	CreatedAt int64           `json:"createdAt"`
	IsRead    bool            `json:"isRead"`
	Data      []ContentBlock  `json:"data"`
	PaidText  json.RawMessage `json:"paidText,omitempty"`
}

// CreatedAtTime converts the Unix timestamp to time.Time.
func (m Message) CreatedAtTime() time.Time {
	return time.Unix(m.CreatedAt, 0)
}

// ContentBlock is a single piece of message content.
// Boosty types: "text", "link", "image", "file", "audio_file", "ok_video", "video", "smile"
type ContentBlock struct {
	Type        string `json:"type"`
	Content     string `json:"content,omitempty"`
	Modificator string `json:"modificator,omitempty"`

	// Common for media types
	ID  string `json:"id,omitempty"`
	URL string `json:"url,omitempty"`

	// Image fields
	Width  int `json:"width,omitempty"`
	Height int `json:"height,omitempty"`
	Size   int `json:"size,omitempty"`

	// File / Audio fields
	Title    string `json:"title,omitempty"`
	Complete bool   `json:"complete,omitempty"`

	// Audio fields
	Duration int    `json:"duration,omitempty"`
	Artist   string `json:"artist,omitempty"`
	Album    string `json:"album,omitempty"`
	Track    string `json:"track,omitempty"`

	// Video (ok_video) fields
	PlayerURLs     []PlayerURL `json:"playerUrls,omitempty"`
	DefaultPreview string      `json:"defaultPreview,omitempty"`
	Preview        string      `json:"preview,omitempty"`

	// Smile (sticker) fields
	SmallURL  string `json:"smallUrl,omitempty"`
	MediumURL string `json:"mediumUrl,omitempty"`
	LargeURL  string `json:"largeUrl,omitempty"`
	Name      string `json:"name,omitempty"`

	// Link fields
	Explicit bool `json:"explicit,omitempty"`
}

// PlayerURL is a video stream URL for a specific resolution.
type PlayerURL struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

// ContentBlockType constants.
const (
	ContentTypeText    = "text"
	ContentTypeLink    = "link"
	ContentTypeImage   = "image"
	ContentTypeFile    = "file"
	ContentTypeAudio   = "audio_file"
	ContentTypeOKVideo = "ok_video"
	ContentTypeVideo   = "video"
	ContentTypeSmile   = "smile"
)

// TextContent is the parsed JSON inside ContentBlock.Content for type "text".
// Format: ["text", "unstyled", [[format_type, offset, length], ...]]
type TextContent struct {
	Text     string
	Style    string
	Entities []TextEntity
}

// TextEntity represents a formatting entity within text.
type TextEntity struct {
	FormatType int
	Offset     int
	Length     int
}

// TextFormatType constants matching Boosty's format types.
const (
	TextFormatBold      = 0
	TextFormatItalic    = 2
	TextFormatUnderline = 4
)
