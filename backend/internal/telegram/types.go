package telegram

import "encoding/json"

// User is a Telegram user.
type User struct {
	ID           int64  `json:"id"`
	IsBot        bool   `json:"is_bot"`
	FirstName    string `json:"first_name"`
	LastName     string `json:"last_name,omitempty"`
	Username     string `json:"username,omitempty"`
	LanguageCode string `json:"language_code,omitempty"`
}

// Chat is a Telegram chat.
type Chat struct {
	ID        int64  `json:"id"`
	Type      string `json:"type"`
	FirstName string `json:"first_name,omitempty"`
	LastName  string `json:"last_name,omitempty"`
	Username  string `json:"username,omitempty"`
}

// Message is a Telegram message (subset used by the engine).
type Message struct {
	MessageID      int64           `json:"message_id"`
	From           *User           `json:"from,omitempty"`
	Chat           Chat            `json:"chat"`
	Date           int64           `json:"date"`
	Text           string          `json:"text,omitempty"`
	Photo          []PhotoSize     `json:"photo,omitempty"`
	Sticker        json.RawMessage `json:"sticker,omitempty"`
	Voice          json.RawMessage `json:"voice,omitempty"`
	Video          json.RawMessage `json:"video,omitempty"`
	Audio          json.RawMessage `json:"audio,omitempty"`
	Document       json.RawMessage `json:"document,omitempty"`
	Animation      json.RawMessage `json:"animation,omitempty"`
	VideoNote      json.RawMessage `json:"video_note,omitempty"`
	ReplyToMessage *Message        `json:"reply_to_message,omitempty"`
}

type PhotoSize struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
}

// PhotoFileID returns the largest photo file_id (sizes arrive ascending), or "".
func (m *Message) PhotoFileID() string {
	if len(m.Photo) == 0 {
		return ""
	}
	return m.Photo[len(m.Photo)-1].FileID
}

// MediaType reports which media kind this message carries, or "" when it is
// plain text. Used to relay media in the dating chat and to label it for the
// panel transcript.
func (m *Message) MediaType() string {
	if len(m.Photo) > 0 {
		return "photo"
	}
	for _, kind := range []struct {
		name string
		raw  json.RawMessage
	}{
		{"sticker", m.Sticker},
		{"voice", m.Voice},
		{"video_note", m.VideoNote},
		{"video", m.Video},
		{"animation", m.Animation},
		{"audio", m.Audio},
		{"document", m.Document},
	} {
		if len(kind.raw) > 0 {
			return kind.name
		}
	}
	return ""
}

// MediaTypeLabel is the Persian label shown to the panel for a relayed media.
func (m *Message) MediaTypeLabel() string {
	switch m.MediaType() {
	case "photo":
		return "📸 عکس"
	case "sticker":
		return "🎭 استیکر"
	case "voice":
		return "🎙 پیام صوتی"
	case "video_note":
		return "🎬 ویدیوی کوتاه"
	case "video":
		return "🎞 ویدیو"
	case "animation":
		return "🎞 گیف"
	case "audio":
		return "🎵 فایل صوتی"
	case "document":
		return "📎 فایل"
	}
	return ""
}

// CallbackQuery is an inline button press.
type CallbackQuery struct {
	ID      string   `json:"id"`
	From    User     `json:"from"`
	Message *Message `json:"message,omitempty"`
	Data    string   `json:"data"`
}

// Update is a single Telegram update.
type Update struct {
	UpdateID      int64          `json:"update_id"`
	Message       *Message       `json:"message,omitempty"`
	CallbackQuery *CallbackQuery `json:"callback_query,omitempty"`
}

func (u *Update) EffectiveUser() *User {
	if u.CallbackQuery != nil {
		return &u.CallbackQuery.From
	}
	if u.Message != nil && u.Message.From != nil {
		return u.Message.From
	}
	return nil
}

func (u *Update) EffectiveChatID() int64 {
	if u.CallbackQuery != nil && u.CallbackQuery.Message != nil {
		return u.CallbackQuery.Message.Chat.ID
	}
	if u.Message != nil {
		return u.Message.Chat.ID
	}
	return 0
}

// --- outgoing payloads ---

type InlineKeyboardButton struct {
	Text         string `json:"text"`
	URL          string `json:"url,omitempty"`
	CallbackData string `json:"callback_data,omitempty"`
}

type InlineKeyboardMarkup struct {
	InlineKeyboard [][]InlineKeyboardButton `json:"inline_keyboard"`
}

type ReplyKeyboardButton struct {
	Text            string `json:"text"`
	RequestContact  bool   `json:"request_contact,omitempty"`
	RequestLocation bool   `json:"request_location,omitempty"`
	// Style colors the button on mobile clients (Bot API 7.11+):
	// "primary" (brand blue) or "success" (green).
	Style string `json:"style,omitempty"`
}

type ReplyKeyboardMarkup struct {
	Keyboard        [][]ReplyKeyboardButton `json:"keyboard"`
	ResizeKeyboard  bool                    `json:"resize_keyboard,omitempty"`
	OneTimeKeyboard bool                    `json:"one_time_keyboard,omitempty"`
	IsPersistent    bool                    `json:"is_persistent,omitempty"`
}

type RemoveKeyboard struct {
	RemoveKeyboard bool `json:"remove_keyboard"`
}

type SendMessagePayload struct {
	ChatID      any             `json:"chat_id"` // numeric id or "@username"
	Text        string          `json:"text"`
	ParseMode   string          `json:"parse_mode,omitempty"`
	ReplyMarkup json.RawMessage `json:"reply_markup,omitempty"`
	ReplyTo     int64           `json:"reply_to_message_id,omitempty"`
}

type SendPhotoPayload struct {
	ChatID      any             `json:"chat_id"` // numeric id or "@username"
	Photo       string          `json:"photo"`   // Telegram file_id or https URL
	Caption     string          `json:"caption,omitempty"`
	ReplyMarkup json.RawMessage `json:"reply_markup,omitempty"`
}

type EditMessageTextPayload struct {
	ChatID      int64           `json:"chat_id"`
	MessageID   int64           `json:"message_id"`
	Text        string          `json:"text"`
	ParseMode   string          `json:"parse_mode,omitempty"`
	ReplyMarkup json.RawMessage `json:"reply_markup,omitempty"`
}

// EditMessageCaptionPayload edits the caption of a media message
// (editMessageText is rejected for photos/videos).
type EditMessageCaptionPayload struct {
	ChatID      int64           `json:"chat_id"`
	MessageID   int64           `json:"message_id"`
	Caption     string          `json:"caption,omitempty"`
	ParseMode   string          `json:"parse_mode,omitempty"`
	ReplyMarkup json.RawMessage `json:"reply_markup,omitempty"`
}

type AnswerCallbackPayload struct {
	CallbackQueryID string `json:"callback_query_id"`
	Text            string `json:"text,omitempty"`
	ShowAlert       bool   `json:"show_alert,omitempty"`
}

type APIResponse struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result,omitempty"`
	ErrorCode   int             `json:"error_code,omitempty"`
	Description string          `json:"description,omitempty"`
	Parameters  *struct {
		RetryAfter int `json:"retry_after"`
	} `json:"parameters,omitempty"`
}

type GetMeResult struct {
	ID              int64  `json:"id"`
	IsBot           bool   `json:"is_bot"`
	FirstName       string `json:"first_name"`
	Username        string `json:"username,omitempty"`
	CanJoinGroups   bool   `json:"can_join_groups"`
	CanReadMessages bool   `json:"can_read_all_group_messages"`
}
