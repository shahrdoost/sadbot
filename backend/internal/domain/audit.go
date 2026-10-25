package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type AuditLog struct {
	ID           int64           `json:"id"`
	ActorAdminID *uuid.UUID      `json:"actor_admin_id,omitempty"`
	Action       string          `json:"action"`
	TargetType   string          `json:"target_type"`
	TargetID     string          `json:"target_id"`
	BotID        *uuid.UUID      `json:"bot_id,omitempty"`
	Details      json.RawMessage `json:"details"`
	IP           *string         `json:"ip,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
}
