package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// BroadcastRun tracks one panel/telegram broadcast job.
type BroadcastRun struct {
	ID         uuid.UUID  `json:"id"`
	BotID      uuid.UUID  `json:"bot_id"`
	Text       string     `json:"text"`
	Photo      *string    `json:"photo,omitempty"`
	Footer     *string    `json:"footer,omitempty"`
	Audience   int        `json:"audience"`
	Sent       int        `json:"sent"`
	Failed     int        `json:"failed"`
	Status     string     `json:"status"` // running | done
	CreatedAt  time.Time  `json:"created_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

type BroadcastRepo struct{ db *pgxpool.Pool }

func NewBroadcastRepo(db *pgxpool.Pool) *BroadcastRepo { return &BroadcastRepo{db: db} }

func (r *BroadcastRepo) CreateRun(ctx context.Context, botID, adminID uuid.UUID, text string, photo, footer *string, audience int) (BroadcastRun, error) {
	var b BroadcastRun
	err := r.db.QueryRow(ctx, `
		INSERT INTO broadcast_runs (bot_id, admin_id, text, photo, footer, audience)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, bot_id, text, photo, footer, audience, sent, failed, status, created_at, finished_at`,
		botID, adminID, text, photo, footer, audience).
		Scan(&b.ID, &b.BotID, &b.Text, &b.Photo, &b.Footer, &b.Audience, &b.Sent, &b.Failed, &b.Status, &b.CreatedAt, &b.FinishedAt)
	return b, err
}

func (r *BroadcastRepo) UpdateProgress(ctx context.Context, id uuid.UUID, sent, failed int) error {
	_, err := r.db.Exec(ctx, `UPDATE broadcast_runs SET sent = $2, failed = $3 WHERE id = $1`, id, sent, failed)
	return err
}

func (r *BroadcastRepo) Finish(ctx context.Context, id uuid.UUID, sent, failed int) error {
	_, err := r.db.Exec(ctx, `
		UPDATE broadcast_runs SET sent = $2, failed = $3, status = 'done', finished_at = now()
		WHERE id = $1`, id, sent, failed)
	return err
}

func (r *BroadcastRepo) Get(ctx context.Context, botID, id uuid.UUID) (*BroadcastRun, error) {
	var b BroadcastRun
	err := r.db.QueryRow(ctx, `
		SELECT id, bot_id, text, photo, footer, audience, sent, failed, status, created_at, finished_at
		FROM broadcast_runs WHERE bot_id = $1 AND id = $2`, botID, id).
		Scan(&b.ID, &b.BotID, &b.Text, &b.Photo, &b.Footer, &b.Audience, &b.Sent, &b.Failed, &b.Status, &b.CreatedAt, &b.FinishedAt)
	if err != nil {
		return nil, err
	}
	return &b, nil
}

func (r *BroadcastRepo) ListByBot(ctx context.Context, botID uuid.UUID, limit int) ([]BroadcastRun, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := r.db.Query(ctx, `
		SELECT id, bot_id, text, photo, footer, audience, sent, failed, status, created_at, finished_at
		FROM broadcast_runs WHERE bot_id = $1 ORDER BY created_at DESC LIMIT $2`, botID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BroadcastRun
	for rows.Next() {
		var b BroadcastRun
		if err := rows.Scan(&b.ID, &b.BotID, &b.Text, &b.Photo, &b.Footer, &b.Audience, &b.Sent, &b.Failed, &b.Status, &b.CreatedAt, &b.FinishedAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// BroadcastAudienceCount counts reachable end users (real, not blocked).
func (r *BroadcastRepo) BroadcastAudienceCount(ctx context.Context, botID uuid.UUID) (int, error) {
	var n int
	err := r.db.QueryRow(ctx, `
		SELECT count(*) FROM bot_end_users
		WHERE bot_id = $1 AND telegram_id > 0 AND NOT is_blocked`, botID).Scan(&n)
	return n, err
}

// BroadcastPage returns a batch of (userID, telegramID) after a cursor.
func (r *BroadcastRepo) BroadcastPage(ctx context.Context, botID uuid.UUID, afterID int64, limit int) ([]UserChat, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, telegram_id FROM bot_end_users
		WHERE bot_id = $1 AND telegram_id > 0 AND NOT is_blocked AND id > $2
		ORDER BY id LIMIT $3`, botID, afterID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UserChat
	for rows.Next() {
		var u UserChat
		if err := rows.Scan(&u.UserID, &u.TelegramID); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// UserChat is a minimal chat target.
type UserChat struct {
	UserID     int64
	TelegramID int64
}
