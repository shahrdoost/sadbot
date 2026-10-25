package domain

import (
	"time"

	"github.com/google/uuid"
)

const (
	ConfessionPending  = "pending"
	ConfessionApproved = "approved"
	ConfessionRejected = "rejected"

	TicketOpen     = "open"
	TicketAnswered = "answered"
	TicketClosed   = "closed"
)

// Confession is an anonymous confession awaiting admin approval before the
// bot publishes it to the configured channel.
type Confession struct {
	ID             int64      `json:"id"`
	BotID          uuid.UUID  `json:"bot_id"`
	UserID         int64      `json:"user_id"`
	Username       *string    `json:"username,omitempty"`
	FirstName      *string    `json:"first_name,omitempty"`
	Text           string     `json:"text"`
	Status         string     `json:"status"`
	CreatedAt      time.Time  `json:"created_at"`
	DecidedAt      *time.Time `json:"decided_at,omitempty"`
	DecidedBy      *uuid.UUID `json:"decided_by,omitempty"`
	PublishedMsgID *int64     `json:"published_msg_id,omitempty"`
}

// SupportTicket is a user message routed to the bot admins.
type SupportTicket struct {
	ID         int64      `json:"id"`
	BotID      uuid.UUID  `json:"bot_id"`
	UserID     int64      `json:"user_id"`
	Username   *string    `json:"username,omitempty"`
	FirstName  *string    `json:"first_name,omitempty"`
	Message    string     `json:"message"`
	AdminReply *string    `json:"admin_reply,omitempty"`
	Status     string     `json:"status"`
	CreatedAt  time.Time  `json:"created_at"`
	RepliedAt  *time.Time `json:"replied_at,omitempty"`
}
