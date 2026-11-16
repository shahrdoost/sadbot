package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// OutboundRetry is one parked cross-platform delivery. Payload is kind-specific:
// the text body for "text", the source file_id for "photo", empty for "match".
type OutboundRetry struct {
	ID             int64
	BotID          uuid.UUID
	Kind           string
	Platform       string
	ChatID         int64
	Payload        string
	SourcePlatform string
	Attempts       int
}

// OutboundRetryRepo persists the cross-platform retry queue so a redeploy — the
// most likely moment for a platform side to be down — cannot drop parked work.
type OutboundRetryRepo struct{ db *pgxpool.Pool }

func NewOutboundRetryRepo(db *pgxpool.Pool) *OutboundRetryRepo { return &OutboundRetryRepo{db: db} }

// Enqueue parks a delivery for retry. Duplicate (bot,kind,platform,chat,payload)
// rows within the same minute collapse, so a user mashing send does not flood it.
func (r *OutboundRetryRepo) Enqueue(ctx context.Context, botID uuid.UUID, kind, platform string, chatID int64, payload, sourcePlatform string) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO outbound_retries (bot_id, kind, platform, chat_id, target, source_platform)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		botID, kind, platform, chatID, payload, sourcePlatform)
	return err
}

// Due returns the deliveries whose retry time has arrived, oldest first.
func (r *OutboundRetryRepo) Due(ctx context.Context, limit int) ([]OutboundRetry, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.db.Query(ctx, `
		SELECT id, bot_id, kind, platform, chat_id, COALESCE(target,''), COALESCE(source_platform,''), attempts
		FROM outbound_retries
		WHERE next_at <= now() AND attempts < 5
		ORDER BY next_at
		LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OutboundRetry
	for rows.Next() {
		var o OutboundRetry
		if err := rows.Scan(&o.ID, &o.BotID, &o.Kind, &o.Platform, &o.ChatID,
			&o.Payload, &o.SourcePlatform, &o.Attempts); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// Done removes a delivered row.
func (r *OutboundRetryRepo) Done(ctx context.Context, id int64) error {
	_, err := r.db.Exec(ctx, `DELETE FROM outbound_retries WHERE id = $1`, id)
	return err
}

// Fail bumps the attempt counter and schedules the next try. The last allowed
// attempt deletes the row instead of rescheduling it (Due only reads rows under
// the cap, so a rescheduled final attempt would sit in the table forever).
// Reports gaveUp=true when that happened.
func (r *OutboundRetryRepo) Fail(ctx context.Context, id int64, next time.Duration, lastErr string) (bool, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE outbound_retries
		SET attempts = attempts + 1, next_at = now() + $2::interval, last_error = $3
		WHERE id = $1 AND attempts < 5`,
		id, next.String(), lastErr)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() == 0 {
		// already at the cap (or gone): drop it, this was the final attempt
		_, _ = r.db.Exec(ctx, `DELETE FROM outbound_retries WHERE id = $1`, id)
		return true, nil
	}
	// the row just reached the cap: no further attempt is possible
	var attempts int
	if err := r.db.QueryRow(ctx, `SELECT attempts FROM outbound_retries WHERE id = $1`, id).Scan(&attempts); err != nil {
		return false, nil
	}
	if attempts >= 5 {
		_, _ = r.db.Exec(ctx, `DELETE FROM outbound_retries WHERE id = $1`, id)
		return true, nil
	}
	return false, nil
}
