package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DiscountCode is a per-bot promo code (percent off, flat Toman off, or
// instant coin bonus).
type DiscountCode struct {
	ID        uuid.UUID  `json:"id"`
	BotID     uuid.UUID  `json:"bot_id"`
	Code      string     `json:"code"`
	Kind      string     `json:"kind"` // percent | flat_toman | coin_bonus
	Value     int        `json:"value"`
	MaxUses   int        `json:"max_uses"` // 0 = unlimited
	UsedCount int        `json:"used_count"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

type DiscountRepo struct{ db *pgxpool.Pool }

func NewDiscountRepo(db *pgxpool.Pool) *DiscountRepo { return &DiscountRepo{db: db} }

func (r *DiscountRepo) Create(ctx context.Context, botID uuid.UUID, code, kind string, value, maxUses int, expiresAt *time.Time) (DiscountCode, error) {
	var d DiscountCode
	err := r.db.QueryRow(ctx, `
		INSERT INTO discount_codes (bot_id, code, kind, value, max_uses, expires_at)
		VALUES ($1, upper($2), $3, $4, $5, $6)
		RETURNING id, bot_id, code, kind, value, max_uses, used_count, expires_at, created_at`,
		botID, code, kind, value, maxUses, expiresAt).
		Scan(&d.ID, &d.BotID, &d.Code, &d.Kind, &d.Value, &d.MaxUses, &d.UsedCount, &d.ExpiresAt, &d.CreatedAt)
	return d, err
}

func (r *DiscountRepo) ListByBot(ctx context.Context, botID uuid.UUID) ([]DiscountCode, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, bot_id, code, kind, value, max_uses, used_count, expires_at, created_at
		FROM discount_codes WHERE bot_id = $1 ORDER BY created_at DESC`, botID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DiscountCode
	for rows.Next() {
		var d DiscountCode
		if err := rows.Scan(&d.ID, &d.BotID, &d.Code, &d.Kind, &d.Value, &d.MaxUses, &d.UsedCount, &d.ExpiresAt, &d.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (r *DiscountRepo) Delete(ctx context.Context, botID, id uuid.UUID) error {
	_, err := r.db.Exec(ctx, `DELETE FROM discount_codes WHERE bot_id = $1 AND id = $2`, botID, id)
	return err
}

// GetByCode returns an active (not expired, uses remaining) code or nil.
func (r *DiscountRepo) GetByCode(ctx context.Context, botID uuid.UUID, code string) (*DiscountCode, error) {
	var d DiscountCode
	err := r.db.QueryRow(ctx, `
		SELECT id, bot_id, code, kind, value, max_uses, used_count, expires_at, created_at
		FROM discount_codes
		WHERE bot_id = $1 AND code = upper($2)
		  AND (expires_at IS NULL OR expires_at > now())
		  AND (max_uses = 0 OR used_count < max_uses)`,
		botID, code).
		Scan(&d.ID, &d.BotID, &d.Code, &d.Kind, &d.Value, &d.MaxUses, &d.UsedCount, &d.ExpiresAt, &d.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// IncrementUse consumes one use (CAS on remaining uses).
func (r *DiscountRepo) IncrementUse(ctx context.Context, botID uuid.UUID, code string) (bool, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE discount_codes SET used_count = used_count + 1
		WHERE bot_id = $1 AND code = upper($2)
		  AND (expires_at IS NULL OR expires_at > now())
		  AND (max_uses = 0 OR used_count < max_uses)`,
		botID, code)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}
