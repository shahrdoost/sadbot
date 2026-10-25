package domain

import (
	"encoding/json"
	"time"
)

// UserState is the engine's per-user navigation state (single JSONB document).
type UserState struct {
	Scene string          `json:"scene"`
	Step  string          `json:"step,omitempty"`
	Data  json.RawMessage `json:"data,omitempty"`
}

type BotEndUser struct {
	ID          int64           `json:"id"`
	BotID       string          `json:"bot_id"`
	TelegramID  int64           `json:"telegram_id"`
	Username    *string         `json:"username,omitempty"`
	FirstName   *string         `json:"first_name,omitempty"`
	LastName    *string         `json:"last_name,omitempty"`
	State       json.RawMessage `json:"state"`
	IsBlocked   bool            `json:"is_blocked"`
	FirstSeenAt time.Time       `json:"first_seen_at"`
	LastSeenAt  time.Time       `json:"last_seen_at"`
}

const (
	EventInboundMessage  = "inbound_message"
	EventOutboundMessage = "outbound_message"
	EventCallback        = "callback"
	EventCommand         = "command"
	EventUserStart       = "user_start"
	EventStateChange     = "state_change"
)

type BotUserEvent struct {
	ID        int64           `json:"id"`
	BotID     string          `json:"bot_id"`
	UserID    int64           `json:"user_id"`
	Username  *string         `json:"username,omitempty"`
	FirstName *string         `json:"first_name,omitempty"`
	Kind      string          `json:"kind"`
	Text      *string         `json:"text,omitempty"`
	Meta      json.RawMessage `json:"meta"`
	CreatedAt time.Time       `json:"created_at"`
}
