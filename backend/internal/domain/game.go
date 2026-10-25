package domain

import (
	"time"

	"github.com/google/uuid"
)

// BombLandPlayer is one player's idle-factory state.
type BombLandPlayer struct {
	BotID     uuid.UUID  `json:"bot_id"`
	UserID    int64      `json:"user_id"`
	Bombs     int        `json:"bombs"`
	Money     int        `json:"money"`
	AutoLvl   int        `json:"auto_lvl"`
	LastAuto  time.Time  `json:"last_auto"`
	InvitedBy *int64     `json:"invited_by,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// DuelPlayer is one player's rock-paper-scissors state.
type DuelPlayer struct {
	BotID             uuid.UUID `json:"bot_id"`
	UserID            int64     `json:"user_id"`
	Score             int       `json:"score"`
	Energy            int       `json:"energy"`
	LastEnergyUpdate  time.Time `json:"last_energy_update"`
	BlackOpal         int       `json:"black_opal"`
	Candy             int       `json:"candy"`
	Chocolate         int       `json:"chocolate"`
	Soda              int       `json:"soda"`
	TotalSpentToman   int64     `json:"total_spent_toman"`
	TotalWithdrawnToman int64   `json:"total_withdrawn_toman"`
	IsBot             bool      `json:"is_bot"`
	CreatedAt         time.Time `json:"created_at"`
}

// DuelWithdrawal is a prize-redemption request.
type DuelWithdrawal struct {
	ID          uuid.UUID `json:"id"`
	BotID       uuid.UUID `json:"bot_id"`
	UserID      int64     `json:"user_id"`
	AmountMg    float64   `json:"amount_mg"`
	AmountToman int64     `json:"amount_toman"`
	Kind        string    `json:"kind"` // cash | physical
	Status      string    `json:"status"` // pending | approved | rejected | paid
	Note        *string   `json:"note,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// DuelPool is the shared prize pool per bot.
type DuelPool struct {
	BotID            uuid.UUID `json:"bot_id"`
	TotalToman       int64     `json:"total_toman"`
	TotalDistributed int64     `json:"total_distributed"`
}

// ─── Favaani (money mindset) ───

type FavaaniUser struct {
	BotID      uuid.UUID  `json:"bot_id"`
	UserID     int64      `json:"user_id"`
	Username   *string    `json:"username,omitempty"`
	Day        int        `json:"day"`
	Score      int        `json:"score"`
	Streak     int        `json:"streak"`
	TotalSpent int64      `json:"total_spent"`
	ReferralCode *string  `json:"referral_code,omitempty"`
	ReferredBy *int64     `json:"referred_by,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

// ─── Shlinks (link shortener) ───

type ShlinkUser struct {
	BotID         uuid.UUID `json:"bot_id"`
	UserID        int64     `json:"user_id"`
	Username      *string   `json:"username,omitempty"`
	Balance       float64   `json:"balance"`
	TotalEarned   float64   `json:"total_earned"`
	TotalWithdrawn float64  `json:"total_withdrawn"`
	CreatedAt     time.Time `json:"created_at"`
}

type ShlinkLink struct {
	ID          uuid.UUID `json:"id"`
	BotID       uuid.UUID `json:"bot_id"`
	UserID      int64     `json:"user_id"`
	ShortCode   string    `json:"short_code"`
	TargetURL   string    `json:"target_url"`
	TotalClicks int       `json:"total_clicks"`
	ValidClicks int       `json:"valid_clicks"`
	TotalEarned float64   `json:"total_earned"`
	CreatedAt   time.Time `json:"created_at"`
}

// ─── Whoseeme (who saw my link) ───

type WhoseemeUser struct {
	BotID     uuid.UUID `json:"bot_id"`
	UserID    int64     `json:"user_id"`
	Username  *string   `json:"username,omitempty"`
	Coins     int       `json:"coins"`
	Balance   float64   `json:"balance"`
	CreatedAt time.Time `json:"created_at"`
}

type WhoseemeLink struct {
	ID        uuid.UUID  `json:"id"`
	BotID     uuid.UUID  `json:"bot_id"`
	UserID    int64      `json:"user_id"`
	Token     string     `json:"token"`
	BaitText  *string    `json:"bait_text,omitempty"`
	BaitImage *string    `json:"bait_image,omitempty"`
	Content   *string    `json:"content,omitempty"`
	IsActive  bool       `json:"is_active"`
	CreatedAt time.Time  `json:"created_at"`
}

// ─── Faal (fortune telling) ───

type FaalUser struct {
	BotID         uuid.UUID  `json:"bot_id"`
	UserID        int64      `json:"user_id"`
	Username      *string    `json:"username,omitempty"`
	FortuneCount  int        `json:"fortune_count"`
	LastFortuneAt *time.Time `json:"last_fortune_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

// ─── AnonyChat (anonymous chat) ───

type AnonyChatUser struct {
	BotID     uuid.UUID  `json:"bot_id"`
	UserID    int64      `json:"user_id"`
	Username  *string    `json:"username,omitempty"`
	Status    string     `json:"status"`
	PartnerID *int64     `json:"partner_id,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// ─── TapEarn (tap-to-earn) ───

type TapEarnUser struct {
	BotID            uuid.UUID `json:"bot_id"`
	UserID           int64     `json:"user_id"`
	Username         *string   `json:"username,omitempty"`
	Money            int64     `json:"money"`
	Energy           int       `json:"energy"`
	MaxEnergy        int       `json:"max_energy"`
	LastEnergyUpdate time.Time `json:"last_energy_update"`
	LastTap          *time.Time `json:"last_tap,omitempty"`
	TapPower         int       `json:"tap_power"`
	CreatedAt        time.Time `json:"created_at"`
}
