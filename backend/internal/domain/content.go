package domain

import (
	"time"

	"github.com/google/uuid"
)

// Content post ledger statuses.
const (
	ContentStatusPublished     = "published"
	ContentStatusNoContent     = "no_content"
	ContentStatusFailed        = "failed"
	ContentStatusRenderFailed  = "render_failed"
)

// ContentPost is one collected source message for a content bot — the durable
// dedup key is (BotID, SourceChatID, MessageID).
type ContentPost struct {
	ID           int64     `json:"id"`
	BotID        uuid.UUID `json:"bot_id"`
	SourceChatID int64     `json:"source_chat_id"`
	MessageID    int64     `json:"message_id"`
	SourceTitle  *string   `json:"source_title,omitempty"`
	SourceLink   *string   `json:"source_link,omitempty"`
	RawText      *string   `json:"raw_text,omitempty"`
	CleanedText  *string   `json:"cleaned_text,omitempty"`
	Status       string    `json:"status"`
	Error        *string   `json:"error,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}
