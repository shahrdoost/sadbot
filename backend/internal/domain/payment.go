package domain

import (
	"time"

	"github.com/google/uuid"
)

const (
	PaymentKindCoinPackage  = "coin_package"
	PaymentKindSubscription = "subscription"
	PaymentKindManual       = "manual"

	PaymentStatusPending  = "pending"
	PaymentStatusVerified = "verified"
	PaymentStatusFailed   = "failed"
	PaymentStatusRefunded = "refunded"

	SubStatusActive    = "active"
	SubStatusCancelled = "cancelled"
	SubStatusExpired   = "expired"
)

type Payment struct {
	ID          uuid.UUID  `json:"id"`
	BotID       uuid.UUID  `json:"bot_id"`
	UserID      *int64     `json:"user_id,omitempty"`
	Username    *string    `json:"username,omitempty"`
	FirstName   *string    `json:"first_name,omitempty"`
	AmountRials int64      `json:"amount_rials"`
	Kind        string     `json:"kind"`
	Gateway     string     `json:"gateway"`
	ExternalRef *string    `json:"external_ref,omitempty"`
	Status      string     `json:"status"`
	Note        *string    `json:"note,omitempty"`
	RecordedBy  *uuid.UUID `json:"recorded_by,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	VerifiedAt  *time.Time `json:"verified_at,omitempty"`
	Coins       int        `json:"coins"`
	PackageID   *string    `json:"package_id,omitempty"`
}

type Subscription struct {
	ID          uuid.UUID  `json:"id"`
	BotID       uuid.UUID  `json:"bot_id"`
	UserID      int64      `json:"user_id"`
	Plan        string     `json:"plan"`
	Status      string     `json:"status"`
	AmountRials int64      `json:"amount_rials"`
	PaymentID   *uuid.UUID `json:"payment_id,omitempty"`
	StartedAt   time.Time  `json:"started_at"`
	ExpiresAt   time.Time  `json:"expires_at"`
}

type BotStats struct {
	TotalUsers  int64 `json:"total_users"`
	NewUsers30d int64 `json:"new_users_30d"`
	Revenue30d  int64 `json:"revenue_30d_rials"`
	ActiveSubs  int64 `json:"active_subscriptions"`
	Events24h   int64 `json:"events_24h"`
}

type FinanceOverview struct {
	TotalRevenueRials int64           `json:"total_revenue_rials"`
	TotalUsers        int64           `json:"total_users"`
	ActiveSubs        int64           `json:"active_subscriptions"`
	PerBot            []BotFinanceRow `json:"per_bot"`
}

type BotFinanceRow struct {
	BotID   uuid.UUID `json:"bot_id"`
	BotName string    `json:"bot_name"`
	Revenue int64     `json:"revenue_rials"`
	Users   int64     `json:"users"`
	Subs    int64     `json:"active_subscriptions"`
}
