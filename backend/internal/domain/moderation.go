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

// SupportTicket is a user message routed to the bot admins. Message is the
// opening message; the rest of the conversation lives in Messages.
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
	// Messages is the thread oldest-first, opening message excluded (it stays
	// in Message). Empty for tickets that never got a reply.
	Messages []TicketMessage `json:"messages"`
}

// TicketMessage is one turn of a support ticket. Sender is "user" or "admin".
type TicketMessage struct {
	ID        int64     `json:"id"`
	Sender    string    `json:"sender"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}
