package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"sadbot/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PaymentRepo struct {
	db *pgxpool.Pool
}

func NewPaymentRepo(db *pgxpool.Pool) *PaymentRepo { return &PaymentRepo{db: db} }

const paymentCols = `id, bot_id, user_id, amount_rials, kind, gateway, external_ref, status, note, recorded_by, created_at, verified_at, coins, package_id`

func scanPayment(row pgx.Row) (*domain.Payment, error) {
	p := &domain.Payment{}
	err := row.Scan(&p.ID, &p.BotID, &p.UserID, &p.AmountRials, &p.Kind, &p.Gateway, &p.ExternalRef, &p.Status, &p.Note, &p.RecordedBy, &p.CreatedAt, &p.VerifiedAt, &p.Coins, &p.PackageID)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (r *PaymentRepo) Create(ctx context.Context, p *domain.Payment) (*domain.Payment, error) {
	verifiedAt := p.VerifiedAt
	if p.Status == domain.PaymentStatusVerified && verifiedAt == nil {
		now := time.Now()
		verifiedAt = &now
	}
	return scanPayment(r.db.QueryRow(ctx, `
		INSERT INTO payments (bot_id, user_id, amount_rials, kind, gateway, external_ref, status, note, recorded_by, verified_at, coins, package_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING `+paymentCols,
		p.BotID, p.UserID, p.AmountRials, p.Kind, p.Gateway, p.ExternalRef, p.Status, p.Note, p.RecordedBy, verifiedAt, p.Coins, p.PackageID))
}

func (r *PaymentRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Payment, error) {
	p, err := scanPayment(r.db.QueryRow(ctx, `SELECT `+paymentCols+` FROM payments WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("payment not found")
	}
	return p, err
}

func (r *PaymentRepo) ListByBot(ctx context.Context, botID uuid.UUID, page, size int) ([]domain.Payment, int64, error) {
	if size <= 0 || size > 100 {
		size = 20
	}
	if page <= 0 {
		page = 1
	}
	var total int64
	if err := r.db.QueryRow(ctx, `SELECT count(*) FROM payments WHERE bot_id = $1`, botID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.Query(ctx, `
		SELECT p.id, p.bot_id, p.user_id, u.username, u.first_name, p.amount_rials, p.kind, p.gateway, p.external_ref, p.status, p.note, p.recorded_by, p.created_at, p.verified_at, p.coins, p.package_id
		FROM payments p
		LEFT JOIN bot_end_users u ON u.id = p.user_id AND u.bot_id = p.bot_id
		WHERE p.bot_id = $1
		ORDER BY p.created_at DESC LIMIT $2 OFFSET $3`, botID, size, (page-1)*size)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []domain.Payment
	for rows.Next() {
		p := &domain.Payment{}
		if err := rows.Scan(&p.ID, &p.BotID, &p.UserID, &p.Username, &p.FirstName, &p.AmountRials, &p.Kind, &p.Gateway, &p.ExternalRef, &p.Status, &p.Note, &p.RecordedBy, &p.CreatedAt, &p.VerifiedAt, &p.Coins, &p.PackageID); err != nil {
			return nil, 0, err
		}
		out = append(out, *p)
	}
	return out, total, rows.Err()
}

// --- subscriptions ---

const subCols = `id, bot_id, user_id, plan, status, amount_rials, payment_id, started_at, expires_at`

func (r *PaymentRepo) CreateSubscription(ctx context.Context, s *domain.Subscription) (*domain.Subscription, error) {
	sub := &domain.Subscription{}
	err := r.db.QueryRow(ctx, `
		INSERT INTO subscriptions (bot_id, user_id, plan, status, amount_rials, payment_id, started_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING `+subCols,
		s.BotID, s.UserID, s.Plan, s.Status, s.AmountRials, s.PaymentID, s.StartedAt, s.ExpiresAt,
	).Scan(&sub.ID, &sub.BotID, &sub.UserID, &sub.Plan, &sub.Status, &sub.AmountRials, &sub.PaymentID, &sub.StartedAt, &sub.ExpiresAt)
	if err != nil {
		return nil, err
	}
	return sub, nil
}

func (r *PaymentRepo) HasActiveSubscription(ctx context.Context, botID uuid.UUID, userID int64) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM subscriptions WHERE bot_id = $1 AND user_id = $2 AND status = 'active' AND expires_at > now())`,
		botID, userID).Scan(&exists)
	return exists, err
}

// --- gateway payment support (sweeper / callback) ---

// ListPending returns pending payments younger than maxAge (for auto-verify).
func (r *PaymentRepo) ListPending(ctx context.Context, maxAge time.Duration) ([]domain.Payment, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+paymentCols+` FROM payments
		WHERE status = 'pending' AND created_at > now() - $1::interval
		ORDER BY created_at`, maxAge.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Payment
	for rows.Next() {
		p, err := scanPayment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// LatestPendingForUser returns the user's newest pending payment (manual verify).
func (r *PaymentRepo) LatestPendingForUser(ctx context.Context, botID uuid.UUID, userID int64) (*domain.Payment, error) {
	p, err := scanPayment(r.db.QueryRow(ctx, `
		SELECT `+paymentCols+` FROM payments
		WHERE bot_id = $1 AND user_id = $2 AND status = 'pending'
		ORDER BY created_at DESC LIMIT 1`, botID, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("no pending payment")
	}
	return p, err
}

// MarkVerified CAS-transitions pending→verified; returns false if already handled.
func (r *PaymentRepo) MarkVerified(ctx context.Context, id uuid.UUID) (bool, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE payments SET status = 'verified', verified_at = now()
		WHERE id = $1 AND status = 'pending'`, id)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// MarkFailed transitions pending→failed (gateway rejected / expiry). Returns
// false when the row was already handled (CAS, like MarkVerified).
func (r *PaymentRepo) MarkFailed(ctx context.Context, id uuid.UUID, status string) (bool, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE payments SET status = $2 WHERE id = $1 AND status = 'pending'`, id, status)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// ActivateSubscription upserts an active premium_monthly subscription linked to the payment.
func (r *PaymentRepo) ActivateSubscription(ctx context.Context, botID uuid.UUID, userID int64, paymentID uuid.UUID, amountRials int64, days int) (*domain.Subscription, error) {
	sub := &domain.Subscription{}
	err := r.db.QueryRow(ctx, `
		INSERT INTO subscriptions (bot_id, user_id, plan, status, amount_rials, payment_id, started_at, expires_at)
		VALUES ($1, $2, 'premium_monthly', 'active', $3, $4, now(), now() + ($5 || ' days')::interval)
		ON CONFLICT DO NOTHING
		RETURNING `+subCols,
		botID, userID, amountRials, paymentID, days,
	).Scan(&sub.ID, &sub.BotID, &sub.UserID, &sub.Plan, &sub.Status, &sub.AmountRials, &sub.PaymentID, &sub.StartedAt, &sub.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		// row exists: extend from current expiry (or now if lapsed)
		err = r.db.QueryRow(ctx, `
			UPDATE subscriptions SET
				status = 'active',
				started_at = now(),
				expires_at = GREATEST(expires_at, now()) + ($4 || ' days')::interval,
				amount_rials = $3,
				payment_id = $5
			WHERE bot_id = $1 AND user_id = $2
			RETURNING `+subCols,
			botID, userID, amountRials, days, paymentID,
		).Scan(&sub.ID, &sub.BotID, &sub.UserID, &sub.Plan, &sub.Status, &sub.AmountRials, &sub.PaymentID, &sub.StartedAt, &sub.ExpiresAt)
	}
	return sub, err
}

// ExpireSubscriptions marks active subscriptions past their expiry as expired.
func (r *PaymentRepo) ExpireSubscriptions(ctx context.Context) (int64, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE subscriptions SET status = 'expired'
		WHERE status = 'active' AND expires_at < now()`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// ActiveSubscriptionEnd returns the expiry of the user's active subscription (ok=false when none).
func (r *PaymentRepo) ActiveSubscriptionEnd(ctx context.Context, botID uuid.UUID, userID int64) (time.Time, bool, error) {
	var expires time.Time
	err := r.db.QueryRow(ctx, `
		SELECT expires_at FROM subscriptions
		WHERE bot_id = $1 AND user_id = $2 AND status = 'active' AND expires_at > now()
		ORDER BY expires_at DESC LIMIT 1`,
		botID, userID).Scan(&expires)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, false, nil
	}
	return expires, err == nil, err
}
