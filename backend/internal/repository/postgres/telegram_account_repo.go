package postgres

import (
	"context"
	"fmt"

	"sadbot/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type TelegramAccountRepo struct {
	db *pgxpool.Pool
}

func NewTelegramAccountRepo(db *pgxpool.Pool) *TelegramAccountRepo {
	return &TelegramAccountRepo{db: db}
}

const tgAccountCols = `id, admin_id, label, tg_user_id, phone, sidecar_account_id, created_at`

func scanTGAccount(row pgx.Row) (*domain.TelegramAccount, error) {
	var a domain.TelegramAccount
	err := row.Scan(&a.ID, &a.AdminID, &a.Label, &a.TgUserID, &a.Phone, &a.SidecarAccountID, &a.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *TelegramAccountRepo) Create(ctx context.Context, adminID uuid.UUID, label string, tgUserID int64, phone, sidecarID string) (*domain.TelegramAccount, error) {
	row := r.db.QueryRow(ctx,
		`INSERT INTO telegram_accounts (admin_id, label, tg_user_id, phone, sidecar_account_id)
		 VALUES ($1, $2, $3, $4, $5) RETURNING `+tgAccountCols,
		adminID, label, tgUserID, phone, sidecarID)
	a, err := scanTGAccount(row)
	if err != nil {
		return nil, fmt.Errorf("create telegram account: %w", err)
	}
	return a, nil
}

// List returns all accounts; pass adminID only for non-super callers.
func (r *TelegramAccountRepo) List(ctx context.Context, adminID *uuid.UUID) ([]domain.TelegramAccount, error) {
	var (
		rows pgx.Rows
		err  error
	)
	if adminID != nil {
		rows, err = r.db.Query(ctx,
			`SELECT `+tgAccountCols+` FROM telegram_accounts WHERE admin_id = $1 ORDER BY created_at DESC`, *adminID)
	} else {
		rows, err = r.db.Query(ctx,
			`SELECT `+tgAccountCols+` FROM telegram_accounts ORDER BY created_at DESC`)
	}
	if err != nil {
		return nil, fmt.Errorf("list telegram accounts: %w", err)
	}
	defer rows.Close()
	out := []domain.TelegramAccount{}
	for rows.Next() {
		a, err := scanTGAccount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

func (r *TelegramAccountRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.TelegramAccount, error) {
	row := r.db.QueryRow(ctx, `SELECT `+tgAccountCols+` FROM telegram_accounts WHERE id = $1`, id)
	a, err := scanTGAccount(row)
	if err != nil {
		return nil, fmt.Errorf("get telegram account: %w", err)
	}
	return a, nil
}

func (r *TelegramAccountRepo) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.Exec(ctx, `DELETE FROM telegram_accounts WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete telegram account: %w", err)
	}
	return nil
}
