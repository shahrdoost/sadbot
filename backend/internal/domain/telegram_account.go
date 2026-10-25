package domain

import (
	"time"

	"github.com/google/uuid"
)

// TelegramAccount is an MTProto user account managed via the tgsidecar service.
// The actual Telethon session file lives only in the sidecar volume.
type TelegramAccount struct {
	ID               uuid.UUID `json:"id"`
	AdminID          uuid.UUID `json:"admin_id"`
	Label            string    `json:"label"`
	TgUserID         int64     `json:"tg_user_id"`
	Phone            string    `json:"phone"`
	SidecarAccountID string    `json:"sidecar_account_id"`
	CreatedAt        time.Time `json:"created_at"`
	// DisplayName/Username come from the sidecar (not stored).
	DisplayName string `json:"display_name,omitempty"`
	Username    string `json:"username,omitempty"`
}
