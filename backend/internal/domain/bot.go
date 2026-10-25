package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

const (
	BotStatusStopped  = "stopped"
	BotStatusStarting = "starting"
	BotStatusRunning  = "running"
	BotStatusError    = "error"

	TemplateWelcomeMenu = "welcome_menu"
	TemplateDating      = "dating"
	TemplateContent     = "content"
	TemplateBombLand    = "bombland"
	TemplateDuel        = "duel"
	TemplateFavaani     = "favaani"
	TemplateShlinks     = "shlinks"
	TemplateWhoseeme    = "whoseeme"
	TemplateFaal        = "faal"
	TemplateAnonyChat   = "anonychat"
	TemplateTapEarn     = "tapearn"
)

type Bot struct {
	ID               uuid.UUID  `json:"id"`
	Name             string     `json:"name"`
	TelegramUsername *string    `json:"telegram_username,omitempty"`
	TokenEncrypted   []byte     `json:"-"`
	TemplateKey      string     `json:"template_key"`
	Status           string     `json:"status"`
	LastError        *string    `json:"last_error,omitempty"`
	TokenValidatedAt *time.Time `json:"token_validated_at,omitempty"`
	CreatedBy        uuid.UUID  `json:"created_by"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// BotConfig is the whole per-bot configuration document (single JSONB row).
type BotConfig struct {
	BotID        uuid.UUID       `json:"bot_id"`
	Texts        json.RawMessage `json:"texts"`
	Menus        json.RawMessage `json:"menus"`
	FeatureFlags json.RawMessage `json:"feature_flags"`
	Prices       json.RawMessage `json:"prices"`
	Version      int             `json:"version"`
	UpdatedBy    *uuid.UUID      `json:"updated_by,omitempty"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

// ErrVersionConflict signals an optimistic-lock failure on config update.
type ErrVersionConflict struct{}

func (ErrVersionConflict) Error() string { return "config version conflict" }
