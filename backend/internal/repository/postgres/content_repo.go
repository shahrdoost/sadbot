package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"sadbot/internal/domain"
)

type ContentRepo struct{ db *pgxpool.Pool }

func NewContentRepo(db *pgxpool.Pool) *ContentRepo { return &ContentRepo{db: db} }

// LastMessageID returns the highest recorded source message id for a chat,
// or 0 when nothing was collected yet (caller falls back to backfill limit).
func (r *ContentRepo) LastMessageID(ctx context.Context, botID uuid.UUID, chatID int64) (int64, error) {
	var id pgtype.Int8
	err := r.db.QueryRow(ctx, `
		SELECT max(message_id) FROM content_posts
		WHERE bot_id = $1 AND source_chat_id = $2`, botID, chatID).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id.Int64, nil
}

// InsertPost records a processed source message. Duplicate keys are ignored so
// the collector can be retried safely.
func (r *ContentRepo) InsertPost(ctx context.Context, p domain.ContentPost) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO content_posts
			(bot_id, source_chat_id, message_id, source_title, source_link,
			 raw_text, cleaned_text, status, error)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (bot_id, source_chat_id, message_id) DO NOTHING`,
		p.BotID, p.SourceChatID, p.MessageID, p.SourceTitle, p.SourceLink,
		p.RawText, p.CleanedText, p.Status, p.Error)
	return err
}

// List returns a page of the ledger newest-first, optionally filtered by
// status and a text query over title/raw/cleaned/error columns.
func (r *ContentRepo) List(ctx context.Context, botID uuid.UUID, page, size int, status, q string) ([]domain.ContentPost, int, error) {
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 50
	}
	where := []string{"bot_id = $1"}
	args := []any{botID}
	if status != "" {
		args = append(args, status)
		where = append(where, fmt.Sprintf("status = $%d", len(args)))
	}
	if s := strings.TrimSpace(q); s != "" {
		args = append(args, "%"+s+"%")
		where = append(where, fmt.Sprintf(
			"(source_title ILIKE $%d OR raw_text ILIKE $%d OR cleaned_text ILIKE $%d OR error ILIKE $%d)",
			len(args), len(args), len(args), len(args)))
	}
	cond := strings.Join(where, " AND ")

	var total int
	if err := r.db.QueryRow(ctx, "SELECT count(*) FROM content_posts WHERE "+cond, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, size, (page-1)*size)
	rows, err := r.db.Query(ctx, `
		SELECT id, bot_id, source_chat_id, message_id, source_title, source_link,
		       raw_text, cleaned_text, status, error, created_at
		FROM content_posts WHERE `+cond+`
		ORDER BY id DESC
		LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []domain.ContentPost{}
	for rows.Next() {
		var p domain.ContentPost
		if err := rows.Scan(&p.ID, &p.BotID, &p.SourceChatID, &p.MessageID,
			&p.SourceTitle, &p.SourceLink, &p.RawText, &p.CleanedText,
			&p.Status, &p.Error, &p.CreatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, p)
	}
	return out, total, rows.Err()
}

// Counts returns status totals plus posts published/attempted in the last
// 24h, for the content tab's stat boxes.
func (r *ContentRepo) Counts(ctx context.Context, botID uuid.UUID) (map[string]int64, int64, error) {
	rows, err := r.db.Query(ctx, `
		SELECT status, count(*) FROM content_posts
		WHERE bot_id = $1 GROUP BY status`, botID)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var status string
		var n int64
		if err := rows.Scan(&status, &n); err != nil {
			return nil, 0, err
		}
		out[status] = n
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	var last24h int64
	if err := r.db.QueryRow(ctx, `
		SELECT count(*) FROM content_posts
		WHERE bot_id = $1 AND created_at > now() - interval '24 hours'`, botID).Scan(&last24h); err != nil {
		return nil, 0, err
	}
	return out, last24h, nil
}
