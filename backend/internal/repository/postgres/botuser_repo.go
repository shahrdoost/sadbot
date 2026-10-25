package postgres

import (
	"context"
	"errors"
	"fmt"

	"sadbot/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type BotUserRepo struct {
	db *pgxpool.Pool
}

func NewBotUserRepo(db *pgxpool.Pool) *BotUserRepo { return &BotUserRepo{db: db} }

const botUserCols = `id, bot_id, telegram_id, username, first_name, last_name, state, is_blocked, first_seen_at, last_seen_at`

func (r *BotUserRepo) Upsert(ctx context.Context, botID uuid.UUID, telegramID int64, username, firstName, lastName *string) (*domain.BotEndUser, error) {
	u := &domain.BotEndUser{}
	err := r.db.QueryRow(ctx, `
		INSERT INTO bot_end_users (bot_id, telegram_id, username, first_name, last_name)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (bot_id, telegram_id) DO UPDATE SET
			username = EXCLUDED.username,
			first_name = EXCLUDED.first_name,
			last_name = EXCLUDED.last_name,
			last_seen_at = now()
		RETURNING `+botUserCols,
		botID, telegramID, username, firstName, lastName,
	).Scan(&u.ID, &u.BotID, &u.TelegramID, &u.Username, &u.FirstName, &u.LastName, &u.State, &u.IsBlocked, &u.FirstSeenAt, &u.LastSeenAt)
	if err != nil {
		return nil, fmt.Errorf("upsert bot user: %w", err)
	}
	return u, nil
}

func (r *BotUserRepo) GetByTelegramID(ctx context.Context, botID uuid.UUID, telegramID int64) (*domain.BotEndUser, error) {
	u := &domain.BotEndUser{}
	err := r.db.QueryRow(ctx, `SELECT `+botUserCols+` FROM bot_end_users WHERE bot_id = $1 AND telegram_id = $2`, botID, telegramID).
		Scan(&u.ID, &u.BotID, &u.TelegramID, &u.Username, &u.FirstName, &u.LastName, &u.State, &u.IsBlocked, &u.FirstSeenAt, &u.LastSeenAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("user not found")
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (r *BotUserRepo) GetByID(ctx context.Context, botID uuid.UUID, id int64) (*domain.BotEndUser, error) {
	u := &domain.BotEndUser{}
	err := r.db.QueryRow(ctx, `SELECT `+botUserCols+` FROM bot_end_users WHERE bot_id = $1 AND id = $2`, botID, id).
		Scan(&u.ID, &u.BotID, &u.TelegramID, &u.Username, &u.FirstName, &u.LastName, &u.State, &u.IsBlocked, &u.FirstSeenAt, &u.LastSeenAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("user not found")
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}

type UserFilter struct {
	Query string
	From  *string // ISO date
	To    *string
	Page  int
	Size  int
}

type UserPage struct {
	Users []domain.BotEndUser `json:"users"`
	Total int64               `json:"total"`
}

func (r *BotUserRepo) List(ctx context.Context, botID uuid.UUID, f UserFilter) (*UserPage, error) {
	if f.Size <= 0 || f.Size > 100 {
		f.Size = 20
	}
	if f.Page <= 0 {
		f.Page = 1
	}
	offset := (f.Page - 1) * f.Size

	var total int64
	countQ := `SELECT count(*) FROM bot_end_users WHERE bot_id = $1`
	args := []any{botID}
	if f.Query != "" {
		args = append(args, "%"+f.Query+"%")
		countQ += fmt.Sprintf(` AND (username ILIKE $%d OR first_name ILIKE $%d OR telegram_id::text = $%d)`, len(args), len(args), len(args))
	}
	if f.From != nil {
		args = append(args, *f.From)
		countQ += fmt.Sprintf(` AND last_seen_at >= $%d`, len(args))
	}
	if f.To != nil {
		args = append(args, *f.To)
		countQ += fmt.Sprintf(` AND last_seen_at <= $%d`, len(args))
	}
	if err := r.db.QueryRow(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, err
	}

	listQ := `SELECT ` + botUserCols + ` FROM bot_end_users WHERE bot_id = $1`
	listArgs := []any{botID}
	if f.Query != "" {
		listArgs = append(listArgs, "%"+f.Query+"%")
		listQ += fmt.Sprintf(` AND (username ILIKE $%d OR first_name ILIKE $%d OR telegram_id::text = $%d)`, len(listArgs), len(listArgs), len(listArgs))
	}
	if f.From != nil {
		listArgs = append(listArgs, *f.From)
		listQ += fmt.Sprintf(` AND last_seen_at >= $%d`, len(listArgs))
	}
	if f.To != nil {
		listArgs = append(listArgs, *f.To)
		listQ += fmt.Sprintf(` AND last_seen_at <= $%d`, len(listArgs))
	}
	listArgs = append(listArgs, f.Size, offset)
	listQ += fmt.Sprintf(` ORDER BY last_seen_at DESC LIMIT $%d OFFSET $%d`, len(listArgs)-1, len(listArgs))

	rows, err := r.db.Query(ctx, listQ, listArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	page := &UserPage{Total: total, Users: []domain.BotEndUser{}}
	for rows.Next() {
		var u domain.BotEndUser
		if err := rows.Scan(&u.ID, &u.BotID, &u.TelegramID, &u.Username, &u.FirstName, &u.LastName, &u.State, &u.IsBlocked, &u.FirstSeenAt, &u.LastSeenAt); err != nil {
			return nil, err
		}
		page.Users = append(page.Users, u)
	}
	return page, rows.Err()
}

// ListByScene returns users whose state.scene matches, most recently seen
// first. Used by the panel's AI-harness view (scene "ai").
func (r *BotUserRepo) ListByScene(ctx context.Context, botID uuid.UUID, scene string, limit int) ([]domain.BotEndUser, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := r.db.Query(ctx, `
		SELECT id, bot_id, telegram_id, username, first_name, last_name, state, is_blocked, first_seen_at, last_seen_at
		FROM bot_end_users
		WHERE bot_id = $1 AND state->>'scene' = $2
		ORDER BY last_seen_at DESC
		LIMIT $3`, botID, scene, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.BotEndUser{}
	for rows.Next() {
		var u domain.BotEndUser
		if err := rows.Scan(&u.ID, &u.BotID, &u.TelegramID, &u.Username, &u.FirstName, &u.LastName, &u.State, &u.IsBlocked, &u.FirstSeenAt, &u.LastSeenAt); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// ListInactive returns users not seen for `days` days (still unblocked),
// oldest first — the probe set for the telegram-block scanner.
func (r *BotUserRepo) ListInactive(ctx context.Context, botID uuid.UUID, days, limit int) ([]domain.BotEndUser, error) {
	if days < 1 {
		days = 7
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := r.db.Query(ctx, `
		SELECT id, bot_id, telegram_id, username, first_name, last_name, state, is_blocked, first_seen_at, last_seen_at
		FROM bot_end_users
		WHERE bot_id = $1 AND NOT is_blocked AND last_seen_at < now() - make_interval(days => $2)
		ORDER BY last_seen_at ASC
		LIMIT $3`, botID, days, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.BotEndUser{}
	for rows.Next() {
		var u domain.BotEndUser
		if err := rows.Scan(&u.ID, &u.BotID, &u.TelegramID, &u.Username, &u.FirstName, &u.LastName, &u.State, &u.IsBlocked, &u.FirstSeenAt, &u.LastSeenAt); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// MarkBlockedByBot flags a user whose Telegram returned 403 (bot blocked).
// Broadcast audience queries already exclude is_blocked users.
func (r *BotUserRepo) MarkBlockedByBot(ctx context.Context, botID uuid.UUID, userID int64) error {
	_, err := r.db.Exec(ctx, `
		UPDATE bot_end_users SET is_blocked = true
		WHERE bot_id = $1 AND id = $2`, botID, userID)
	return err
}

func (r *BotUserRepo) SetState(ctx context.Context, botID uuid.UUID, userID int64, state []byte) error {
	tag, err := r.db.Exec(ctx, `UPDATE bot_end_users SET state = $3, last_seen_at = now() WHERE bot_id = $1 AND id = $2`, botID, userID, state)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("user not found")
	}
	return nil
}

// SetStateScene atomically updates only the scene field of the state JSONB.
func (r *BotUserRepo) SetStateScene(ctx context.Context, botID uuid.UUID, userID int64, scene string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE bot_end_users
		SET state = jsonb_set(coalesce(state, '{}'::jsonb), '{scene}', to_jsonb($3::text), true),
		    last_seen_at = now()
		WHERE bot_id = $1 AND id = $2`, botID, userID, scene)
	return err
}

func (r *BotUserRepo) CountAll(ctx context.Context) (int64, error) {
	var n int64
	err := r.db.QueryRow(ctx, `SELECT count(*) FROM bot_end_users`).Scan(&n)
	return n, err
}

// ResetState returns a user to the starting scene, discarding conversation
// progress (used by the panel's user reset).
func (r *BotUserRepo) ResetState(ctx context.Context, botID uuid.UUID, userID int64) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE bot_end_users SET state = '{"scene":"home","data":{}}'::jsonb, last_seen_at = now()
		WHERE bot_id = $1 AND id = $2`, botID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("user not found")
	}
	return nil
}
