package postgres

import (
	"context"
	"fmt"
	"time"

	"sadbot/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type EventRepo struct {
	db *pgxpool.Pool
}

func NewEventRepo(db *pgxpool.Pool) *EventRepo { return &EventRepo{db: db} }

func (r *EventRepo) Insert(ctx context.Context, botID uuid.UUID, userID int64, kind string, text *string, meta []byte) error {
	if meta == nil {
		meta = []byte("{}")
	}
	if text != nil && len(*text) > 1000 {
		t := (*text)[:1000]
		text = &t
	}
	_, err := r.db.Exec(ctx, `
		INSERT INTO bot_user_events (bot_id, user_id, kind, text, meta)
		VALUES ($1, $2, $3, $4, $5)`,
		botID, userID, kind, text, meta)
	return err
}

type EventFilter struct {
	Kind string
	From *string
	To   *string
	Page int
	Size int
}

type EventPage struct {
	Events []domain.BotUserEvent `json:"events"`
	Total  int64                 `json:"total"`
}

func (r *EventRepo) List(ctx context.Context, botID uuid.UUID, f EventFilter) (*EventPage, error) {
	if f.Size <= 0 || f.Size > 200 {
		f.Size = 50
	}
	if f.Page <= 0 {
		f.Page = 1
	}
	offset := (f.Page - 1) * f.Size

	where := `WHERE e.bot_id = $1`
	args := []any{botID}
	if f.Kind != "" {
		args = append(args, f.Kind)
		where += fmt.Sprintf(` AND e.kind = $%d`, len(args))
	}
	if f.From != nil {
		args = append(args, *f.From)
		where += fmt.Sprintf(` AND e.created_at >= $%d`, len(args))
	}
	if f.To != nil {
		args = append(args, *f.To)
		where += fmt.Sprintf(` AND e.created_at <= $%d`, len(args))
	}

	var total int64
	if err := r.db.QueryRow(ctx, `SELECT count(*) FROM bot_user_events e `+where, args...).Scan(&total); err != nil {
		return nil, err
	}

	listArgs := append(append([]any{}, args...), f.Size, offset)
	rows, err := r.db.Query(ctx, `
		SELECT e.id, e.bot_id, e.user_id, u.username, u.first_name, e.kind, e.text, e.meta, e.created_at
		FROM bot_user_events e
		LEFT JOIN bot_end_users u ON u.id = e.user_id AND u.bot_id = e.bot_id `+where+`
		ORDER BY e.created_at DESC
		LIMIT $`+fmt.Sprint(len(listArgs)-1)+` OFFSET $`+fmt.Sprint(len(listArgs)), listArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	page := &EventPage{Total: total, Events: []domain.BotUserEvent{}}
	for rows.Next() {
		var e domain.BotUserEvent
		if err := rows.Scan(&e.ID, &e.BotID, &e.UserID, &e.Username, &e.FirstName, &e.Kind, &e.Text, &e.Meta, &e.CreatedAt); err != nil {
			return nil, err
		}
		page.Events = append(page.Events, e)
	}
	return page, rows.Err()
}

// Purge deletes message events older than messageRetention and other events older than otherRetention.
func (r *EventRepo) Purge(ctx context.Context, messageRetention, otherRetention time.Duration) (int64, error) {
	tag, err := r.db.Exec(ctx, `
		DELETE FROM bot_user_events
		WHERE (kind IN ('inbound_message','outbound_message') AND created_at < now() - $1::interval)
		   OR (kind NOT IN ('inbound_message','outbound_message') AND created_at < now() - $2::interval)`,
		messageRetention.String(), otherRetention.String())
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
