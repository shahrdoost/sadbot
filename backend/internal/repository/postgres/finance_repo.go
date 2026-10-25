package postgres

import (
	"context"

	"sadbot/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type FinanceRepo struct {
	db *pgxpool.Pool
}

func NewFinanceRepo(db *pgxpool.Pool) *FinanceRepo { return &FinanceRepo{db: db} }

func (r *FinanceRepo) BotStats(ctx context.Context, botID uuid.UUID) (*domain.BotStats, error) {
	s := &domain.BotStats{}
	err := r.db.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM bot_end_users WHERE bot_id = $1),
			(SELECT count(*) FROM bot_end_users WHERE bot_id = $1 AND first_seen_at > now() - interval '30 days'),
			(SELECT COALESCE(sum(amount_rials), 0) FROM payments WHERE bot_id = $1 AND status = 'verified' AND created_at > now() - interval '30 days'),
			(SELECT count(*) FROM subscriptions WHERE bot_id = $1 AND status = 'active' AND expires_at > now()),
			(SELECT count(*) FROM bot_user_events WHERE bot_id = $1 AND created_at > now() - interval '24 hours')`,
		botID,
	).Scan(&s.TotalUsers, &s.NewUsers30d, &s.Revenue30d, &s.ActiveSubs, &s.Events24h)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (r *FinanceRepo) Overview(ctx context.Context) (*domain.FinanceOverview, error) {
	o := &domain.FinanceOverview{}
	err := r.db.QueryRow(ctx, `
		SELECT
			COALESCE((SELECT sum(amount_rials) FROM payments WHERE status = 'verified'), 0),
			(SELECT count(*) FROM bot_end_users),
			(SELECT count(*) FROM subscriptions WHERE status = 'active' AND expires_at > now())`,
	).Scan(&o.TotalRevenueRials, &o.TotalUsers, &o.ActiveSubs)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.Query(ctx, `
		SELECT b.id, b.name,
			COALESCE((SELECT sum(p.amount_rials) FROM payments p WHERE p.bot_id = b.id AND p.status = 'verified'), 0),
			(SELECT count(*) FROM bot_end_users u WHERE u.bot_id = b.id),
			(SELECT count(*) FROM subscriptions s WHERE s.bot_id = b.id AND s.status = 'active' AND s.expires_at > now())
		FROM bots b ORDER BY b.created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	o.PerBot = []domain.BotFinanceRow{}
	for rows.Next() {
		var row domain.BotFinanceRow
		if err := rows.Scan(&row.BotID, &row.BotName, &row.Revenue, &row.Users, &row.Subs); err != nil {
			return nil, err
		}
		o.PerBot = append(o.PerBot, row)
	}
	return o, rows.Err()
}
