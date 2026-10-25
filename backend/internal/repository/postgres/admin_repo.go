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

type AdminRepo struct {
	db *pgxpool.Pool
}

func NewAdminRepo(db *pgxpool.Pool) *AdminRepo { return &AdminRepo{db: db} }

func (r *AdminRepo) Create(ctx context.Context, email, passwordHash, displayName, role string) (*domain.Admin, error) {
	a := &domain.Admin{}
	err := r.db.QueryRow(ctx, `
		INSERT INTO admins (email, password_hash, display_name, role)
		VALUES ($1, $2, $3, $4)
		RETURNING id, email, password_hash, display_name, role, status, created_at, updated_at`,
		email, passwordHash, displayName, role,
	).Scan(&a.ID, &a.Email, &a.PasswordHash, &a.DisplayName, &a.Role, &a.Status, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert admin: %w", err)
	}
	return a, nil
}

func (r *AdminRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Admin, error) {
	return r.scanOne(ctx, `SELECT id, email, password_hash, display_name, role, status, created_at, updated_at FROM admins WHERE id = $1`, id)
}

func (r *AdminRepo) GetByEmail(ctx context.Context, email string) (*domain.Admin, error) {
	return r.scanOne(ctx, `SELECT id, email, password_hash, display_name, role, status, created_at, updated_at FROM admins WHERE email = $1`, email)
}

func (r *AdminRepo) List(ctx context.Context) ([]domain.Admin, error) {
	rows, err := r.db.Query(ctx, `SELECT id, email, password_hash, display_name, role, status, created_at, updated_at FROM admins ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Admin
	for rows.Next() {
		var a domain.Admin
		if err := rows.Scan(&a.ID, &a.Email, &a.PasswordHash, &a.DisplayName, &a.Role, &a.Status, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *AdminRepo) Update(ctx context.Context, id uuid.UUID, displayName, role, status *string) (*domain.Admin, error) {
	a := &domain.Admin{}
	err := r.db.QueryRow(ctx, `
		UPDATE admins SET
			display_name = COALESCE($2, display_name),
			role = COALESCE($3, role),
			status = COALESCE($4, status),
			updated_at = now()
		WHERE id = $1
		RETURNING id, email, password_hash, display_name, role, status, created_at, updated_at`,
		id, displayName, role, status,
	).Scan(&a.ID, &a.Email, &a.PasswordHash, &a.DisplayName, &a.Role, &a.Status, &a.CreatedAt, &a.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("admin not found")
	}
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (r *AdminRepo) UpdatePassword(ctx context.Context, id uuid.UUID, passwordHash string) error {
	tag, err := r.db.Exec(ctx, `UPDATE admins SET password_hash = $2, updated_at = now() WHERE id = $1`, id, passwordHash)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("admin not found")
	}
	return nil
}

func (r *AdminRepo) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM admins WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("admin not found")
	}
	return nil
}

func (r *AdminRepo) Count(ctx context.Context) (int64, error) {
	var n int64
	err := r.db.QueryRow(ctx, `SELECT count(*) FROM admins`).Scan(&n)
	return n, err
}

// --- bot access grants ---

func (r *AdminRepo) GetBotAccess(ctx context.Context, adminID uuid.UUID, botID uuid.UUID) (*domain.BotAccess, error) {
	acc := &domain.BotAccess{}
	err := r.db.QueryRow(ctx, `
		SELECT id, admin_id, bot_id, permissions, granted_by, created_at
		FROM admin_bot_access WHERE admin_id = $1 AND bot_id = $2`, adminID, botID,
	).Scan(&acc.ID, &acc.AdminID, &acc.BotID, &acc.Permissions, &acc.GrantedBy, &acc.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return acc, nil
}

func (r *AdminRepo) ListBotAccessByAdmin(ctx context.Context, adminID uuid.UUID) ([]domain.BotAccess, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, admin_id, bot_id, permissions, granted_by, created_at
		FROM admin_bot_access WHERE admin_id = $1 ORDER BY created_at`, adminID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.BotAccess
	for rows.Next() {
		var a domain.BotAccess
		if err := rows.Scan(&a.ID, &a.AdminID, &a.BotID, &a.Permissions, &a.GrantedBy, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *AdminRepo) UpsertBotAccess(ctx context.Context, adminID, botID uuid.UUID, perms []string, grantedBy *uuid.UUID) (*domain.BotAccess, error) {
	acc := &domain.BotAccess{}
	err := r.db.QueryRow(ctx, `
		INSERT INTO admin_bot_access (admin_id, bot_id, permissions, granted_by)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (admin_id, bot_id)
		DO UPDATE SET permissions = EXCLUDED.permissions, granted_by = EXCLUDED.granted_by
		RETURNING id, admin_id, bot_id, permissions, granted_by, created_at`,
		adminID, botID, perms, grantedBy,
	).Scan(&acc.ID, &acc.AdminID, &acc.BotID, &acc.Permissions, &acc.GrantedBy, &acc.CreatedAt)
	if err != nil {
		return nil, err
	}
	return acc, nil
}

func (r *AdminRepo) HasBotPermission(ctx context.Context, adminID, botID uuid.UUID, perm string) (bool, error) {
	var perms []string
	err := r.db.QueryRow(ctx, `
		SELECT permissions FROM admin_bot_access WHERE admin_id = $1 AND bot_id = $2`, adminID, botID,
	).Scan(&perms)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	for _, p := range perms {
		if p == perm {
			return true, nil
		}
	}
	return false, nil
}

func (r *AdminRepo) scanOne(ctx context.Context, query string, arg any) (*domain.Admin, error) {
	a := &domain.Admin{}
	err := r.db.QueryRow(ctx, query, arg).
		Scan(&a.ID, &a.Email, &a.PasswordHash, &a.DisplayName, &a.Role, &a.Status, &a.CreatedAt, &a.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("admin not found")
	}
	if err != nil {
		return nil, err
	}
	return a, nil
}
