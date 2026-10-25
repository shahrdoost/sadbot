package domain

import (
	"time"

	"github.com/google/uuid"
)

const (
	RoleSuperAdmin = "superadmin"
	RoleAdmin      = "admin"

	AdminStatusActive   = "active"
	AdminStatusDisabled = "disabled"
)

type Admin struct {
	ID           uuid.UUID `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	DisplayName  string    `json:"display_name"`
	Role         string    `json:"role"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (a *Admin) IsSuperAdmin() bool { return a.Role == RoleSuperAdmin }

type BotAccess struct {
	ID          uuid.UUID  `json:"id"`
	AdminID     uuid.UUID  `json:"admin_id"`
	BotID       uuid.UUID  `json:"bot_id"`
	Permissions []string   `json:"permissions"`
	GrantedBy   *uuid.UUID `json:"granted_by,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}
