package postgres

import (
	"context"
	"encoding/json"

	"sadbot/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AuditRepo struct {
	db *pgxpool.Pool
}

func NewAuditRepo(db *pgxpool.Pool) *AuditRepo { return &AuditRepo{db: db} }

func (r *AuditRepo) Insert(ctx context.Context, actorAdminID *uuid.UUID, action, targetType, targetID string, botID *uuid.UUID, details map[string]any, ip *string) error {
	if details == nil {
		details = map[string]any{}
	}
	raw, err := json.Marshal(details)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(ctx, `
		INSERT INTO audit_logs (actor_admin_id, action, target_type, target_id, bot_id, details, ip)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		actorAdminID, action, targetType, targetID, botID, raw, ip)
	return err
}

func (r *AuditRepo) List(ctx context.Context, page, size int) ([]domain.AuditLog, int64, error) {
	if size <= 0 || size > 200 {
		size = 50
	}
	if page <= 0 {
		page = 1
	}
	var total int64
	if err := r.db.QueryRow(ctx, `SELECT count(*) FROM audit_logs`).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.Query(ctx, `
		SELECT id, actor_admin_id, action, target_type, target_id, bot_id, details, ip::text, created_at
		FROM audit_logs ORDER BY created_at DESC LIMIT $1 OFFSET $2`, size, (page-1)*size)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []domain.AuditLog
	for rows.Next() {
		var a domain.AuditLog
		if err := rows.Scan(&a.ID, &a.ActorAdminID, &a.Action, &a.TargetType, &a.TargetID, &a.BotID, &a.Details, &a.IP, &a.CreatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, a)
	}
	return out, total, rows.Err()
}
