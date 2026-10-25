package domain

import (
	"time"

	"github.com/google/uuid"
)

const (
	DatingJoined    = "joined"
	DatingSearching = "searching"
	DatingChatting  = "chatting"
)

// DatingProfile mirrors kpkchat's CoreUser fields relevant to the chat core.
type DatingProfile struct {
	BotID             uuid.UUID  `json:"bot_id"`
	UserID            int64      `json:"user_id"` // bot_end_users.id
	TelegramID        int64      `json:"telegram_id"`
	Name              *string    `json:"name,omitempty"`
	Gender            *string    `json:"gender,omitempty"`
	Age               *int       `json:"age,omitempty"`
	Province          *string    `json:"province,omitempty"`
	City              *string    `json:"city,omitempty"`
	Interests         []string   `json:"interests"`
	Status            string     `json:"status"`
	PartnerUserID     *int64     `json:"partner_user_id,omitempty"`
	Rating            float64    `json:"rating"`
	TotalRatings      int        `json:"total_ratings"`
	RatingSum         int        `json:"rating_sum"`
	LastSeenAt        time.Time  `json:"last_seen_at"`
	Coins             int        `json:"coins"`
	ChannelBonusAt    *time.Time `json:"channel_bonus_at,omitempty"`
	PhotoFileID       *string    `json:"photo_file_id,omitempty"`
	AvatarBonusAt     *time.Time `json:"avatar_bonus_at,omitempty"`
	IsGhost           bool       `json:"is_ghost"`
	GhostPersona      *string    `json:"ghost_persona,omitempty"`
	AIFreeUsed        int        `json:"ai_free_used"`
	LastNotifiedAt    *time.Time `json:"last_notified_at,omitempty"`
	NotifySentOn      *time.Time `json:"notify_sent_on,omitempty"`
	NotifyCountOn     int        `json:"notify_count_on"`
	NotifyDismissedAt *time.Time `json:"notify_dismissed_at,omitempty"`
	StreakCount       int        `json:"streak_count"`
	StreakBest        int        `json:"streak_best"`
	StreakLastDate    *time.Time `json:"streak_last_date,omitempty"`
	BoostedUntil      *time.Time `json:"boosted_until,omitempty"`
	NameEditedAt      *time.Time `json:"name_edited_at,omitempty"`
	ProfileEditedAt   *time.Time `json:"profile_edited_at,omitempty"`
}

// DatingLike is a browse action (like/super/pass) from one user to another.
type DatingLike struct {
	BotID      uuid.UUID `json:"bot_id"`
	FromUserID int64     `json:"from_user_id"`
	ToUserID   int64     `json:"to_user_id"`
	Kind       string    `json:"kind"`
	CreatedAt  time.Time `json:"created_at"`
}

type DatingSearchRequest struct {
	ID        int64     `json:"id"`
	BotID     uuid.UUID `json:"bot_id"`
	UserID    int64     `json:"user_id"`
	Filters   []byte    `json:"filters"`
	CreatedAt time.Time `json:"created_at"`
}

type DatingMsgRef struct {
	BotID         uuid.UUID `json:"bot_id"`
	ViewerUserID  int64     `json:"viewer_user_id"`
	ViewerMsgID   int64     `json:"viewer_msg_id"`
	PartnerUserID int64     `json:"partner_user_id"`
	PartnerMsgID  int64     `json:"partner_msg_id"`
	CreatedAt     time.Time `json:"created_at"`
}

type DatingPendingRate struct {
	BotID         uuid.UUID `json:"bot_id"`
	UserID        int64     `json:"user_id"`
	PartnerUserID int64     `json:"partner_user_id"`
	CreatedAt     time.Time `json:"created_at"`
}

type DatingBlockPair struct {
	BotID     uuid.UUID `json:"bot_id"`
	BlockerID int64     `json:"blocker_id"`
	BlockedID int64     `json:"blocked_id"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}
