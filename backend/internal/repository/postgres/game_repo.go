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

// ─── BombLand ───

type BombLandRepo struct{ db *pgxpool.Pool }

func NewBombLandRepo(db *pgxpool.Pool) *BombLandRepo { return &BombLandRepo{db: db} }

const blCols = `bot_id, user_id, bombs, money, auto_lvl, last_auto, invited_by, created_at`

func scanBL(row pgx.Row) (*domain.BombLandPlayer, error) {
	p := &domain.BombLandPlayer{}
	err := row.Scan(&p.BotID, &p.UserID, &p.Bombs, &p.Money, &p.AutoLvl, &p.LastAuto, &p.InvitedBy, &p.CreatedAt)
	return p, err
}

func (r *BombLandRepo) EnsurePlayer(ctx context.Context, botID uuid.UUID, userID int64) (*domain.BombLandPlayer, error) {
	return scanBL(r.db.QueryRow(ctx, `
		INSERT INTO bombland_players (bot_id, user_id) VALUES ($1, $2)
		ON CONFLICT (bot_id, user_id) DO UPDATE SET last_auto = bombland_players.last_auto
		RETURNING `+blCols, botID, userID))
}

func (r *BombLandRepo) GetPlayer(ctx context.Context, botID uuid.UUID, userID int64) (*domain.BombLandPlayer, error) {
	p, err := scanBL(r.db.QueryRow(ctx, `SELECT `+blCols+` FROM bombland_players WHERE bot_id=$1 AND user_id=$2`, botID, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("bombland player not found")
	}
	return p, err
}

func (r *BombLandRepo) AddBombs(ctx context.Context, botID uuid.UUID, userID int64, n int) error {
	_, err := r.db.Exec(ctx, `UPDATE bombland_players SET bombs = bombs + $3 WHERE bot_id=$1 AND user_id=$2`, botID, userID, n)
	return err
}

func (r *BombLandRepo) SellAll(ctx context.Context, botID uuid.UUID, userID int64, pricePerBomb int) (bombsSold, moneyEarned int, err error) {
	err = r.db.QueryRow(ctx, `
		UPDATE bombland_players SET money = money + bombs * $3, bombs = 0
		WHERE bot_id=$1 AND user_id=$2 AND bombs > 0
		RETURNING bombs, bombs * $3`, botID, userID, pricePerBomb).Scan(&bombsSold, &moneyEarned)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, 0, nil
	}
	return
}

func (r *BombLandRepo) TryUpgrade(ctx context.Context, botID uuid.UUID, userID int64, cost int) (ok bool, err error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE bombland_players SET auto_lvl = auto_lvl + 1, money = money - $3
		WHERE bot_id=$1 AND user_id=$2 AND money >= $3`, botID, userID, cost)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// CatchUpAuto applies offline auto-production since last_auto.
func (r *BombLandRepo) CatchUpAuto(ctx context.Context, botID uuid.UUID, userID int64, ratePerMin int) (produced int, err error) {
	err = r.db.QueryRow(ctx, `
		UPDATE bombland_players SET
			bombs = bombs + GREATEST(0, floor(extract(epoch from (now() - last_auto)) / 60)::int * $3 * auto_lvl),
			last_auto = now()
		WHERE bot_id=$1 AND user_id=$2 AND auto_lvl > 0
		RETURNING GREATEST(0, floor(extract(epoch from (now() - last_auto)) / 60)::int * $3 * auto_lvl)`,
		botID, userID, ratePerMin).Scan(&produced)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	return
}

func (r *BombLandRepo) SetInvitedBy(ctx context.Context, botID uuid.UUID, userID, inviterID int64) (ok bool, err error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE bombland_players SET invited_by = $3
		WHERE bot_id=$1 AND user_id=$2 AND invited_by IS NULL`, botID, userID, inviterID)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}
	_, err = r.db.Exec(ctx, `
		INSERT INTO bombland_referrals (bot_id, inviter_id, invited_id) VALUES ($1, $2, $3)
		ON CONFLICT DO NOTHING`, botID, inviterID, userID)
	return true, err
}

func (r *BombLandRepo) ReferralCount(ctx context.Context, botID uuid.UUID, userID int64) (int, error) {
	var n int
	err := r.db.QueryRow(ctx, `SELECT count(*) FROM bombland_referrals WHERE bot_id=$1 AND inviter_id=$2`, botID, userID).Scan(&n)
	return n, err
}

func (r *BombLandRepo) TotalPlayers(ctx context.Context, botID uuid.UUID) (int, error) {
	var n int
	err := r.db.QueryRow(ctx, `SELECT count(*) FROM bombland_players WHERE bot_id=$1`, botID).Scan(&n)
	return n, err
}

// ─── Duel ───

type DuelRepo struct{ db *pgxpool.Pool }

func NewDuelRepo(db *pgxpool.Pool) *DuelRepo { return &DuelRepo{db: db} }

const duelCols = `bot_id, user_id, score, energy, last_energy_update, black_opal, candy, chocolate, soda, total_spent_toman, total_withdrawn_toman, is_bot, created_at`

func scanDuel(row pgx.Row) (*domain.DuelPlayer, error) {
	p := &domain.DuelPlayer{}
	err := row.Scan(&p.BotID, &p.UserID, &p.Score, &p.Energy, &p.LastEnergyUpdate, &p.BlackOpal, &p.Candy, &p.Chocolate, &p.Soda, &p.TotalSpentToman, &p.TotalWithdrawnToman, &p.IsBot, &p.CreatedAt)
	return p, err
}

func (r *DuelRepo) EnsurePlayer(ctx context.Context, botID uuid.UUID, userID int64) (*domain.DuelPlayer, error) {
	return scanDuel(r.db.QueryRow(ctx, `
		INSERT INTO duel_players (bot_id, user_id) VALUES ($1, $2)
		ON CONFLICT (bot_id, user_id) DO UPDATE SET last_energy_update = duel_players.last_energy_update
		RETURNING `+duelCols, botID, userID))
}

func (r *DuelRepo) GetPlayer(ctx context.Context, botID uuid.UUID, userID int64) (*domain.DuelPlayer, error) {
	p, err := scanDuel(r.db.QueryRow(ctx, `SELECT `+duelCols+` FROM duel_players WHERE bot_id=$1 AND user_id=$2`, botID, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("duel player not found")
	}
	return p, err
}

// RechargeEnergy lazily adds energy based on elapsed time.
func (r *DuelRepo) RechargeEnergy(ctx context.Context, botID uuid.UUID, userID int64, maxEnergy int, rechargeSec int) (current int, err error) {
	err = r.db.QueryRow(ctx, `
		UPDATE duel_players SET
			energy = LEAST($3, energy + GREATEST(0, floor(extract(epoch from (now() - last_energy_update)) / $4)::int)),
			last_energy_update = CASE
				WHEN energy + GREATEST(0, floor(extract(epoch from (now() - last_energy_update)) / $4)::int) >= $3 THEN now()
				ELSE last_energy_update + (GREATEST(0, floor(extract(epoch from (now() - last_energy_update)) / $4)::int) * $4 || ' seconds')::interval
			END
		WHERE bot_id=$1 AND user_id=$2
		RETURNING energy`, botID, userID, maxEnergy, rechargeSec).Scan(&current)
	return
}

func (r *DuelRepo) SpendEnergy(ctx context.Context, botID uuid.UUID, userID int64, n int) (ok bool, err error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE duel_players SET energy = energy - $3
		WHERE bot_id=$1 AND user_id=$2 AND energy >= $3`, botID, userID, n)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (r *DuelRepo) AddEnergy(ctx context.Context, botID uuid.UUID, userID int64, n int) error {
	_, err := r.db.Exec(ctx, `UPDATE duel_players SET energy = energy + $3 WHERE bot_id=$1 AND user_id=$2`, botID, userID, n)
	return err
}

func (r *DuelRepo) UpdateScore(ctx context.Context, botID uuid.UUID, userID int64, delta int) error {
	_, err := r.db.Exec(ctx, `UPDATE duel_players SET score = GREATEST(0, score + $3) WHERE bot_id=$1 AND user_id=$2`, botID, userID, delta)
	return err
}

func (r *DuelRepo) AddItems(ctx context.Context, botID uuid.UUID, userID int64, candy, chocolate, soda int) error {
	_, err := r.db.Exec(ctx, `
		UPDATE duel_players SET candy = candy + $3, chocolate = chocolate + $4, soda = soda + $5
		WHERE bot_id=$1 AND user_id=$2`, botID, userID, candy, chocolate, soda)
	return err
}

func (r *DuelRepo) ConsumeItems(ctx context.Context, botID uuid.UUID, userID int64, itemType string, n int) (ok bool, err error) {
	col := map[string]string{"candy": "candy", "chocolate": "chocolate", "soda": "soda"}[itemType]
	if col == "" {
		return false, fmt.Errorf("unknown item type %q", itemType)
	}
	tag, err := r.db.Exec(ctx, `
		UPDATE duel_players SET `+col+` = `+col+` - $3
		WHERE bot_id=$1 AND user_id=$2 AND `+col+` >= $3`, botID, userID, n)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (r *DuelRepo) AddOpals(ctx context.Context, botID uuid.UUID, userID int64, n int) error {
	_, err := r.db.Exec(ctx, `UPDATE duel_players SET black_opal = black_opal + $3 WHERE bot_id=$1 AND user_id=$2`, botID, userID, n)
	return err
}

func (r *DuelRepo) ConsumeOpals(ctx context.Context, botID uuid.UUID, userID int64, n int) (ok bool, err error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE duel_players SET black_opal = black_opal - $3
		WHERE bot_id=$1 AND user_id=$2 AND black_opal >= $3`, botID, userID, n)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (r *DuelRepo) RecordSpent(ctx context.Context, botID uuid.UUID, userID int64, toman int64) error {
	_, err := r.db.Exec(ctx, `UPDATE duel_players SET total_spent_toman = total_spent_toman + $3 WHERE bot_id=$1 AND user_id=$2`, botID, userID, toman)
	return err
}

// FindRival returns the player with the closest score (excluding self).
func (r *DuelRepo) FindRival(ctx context.Context, botID uuid.UUID, userID int64) (*domain.DuelPlayer, error) {
	p, err := scanDuel(r.db.QueryRow(ctx, `
		SELECT `+duelCols+` FROM duel_players
		WHERE bot_id=$1 AND user_id != $2 AND is_bot = true
		ORDER BY ABS(score - (SELECT score FROM duel_players WHERE bot_id=$1 AND user_id=$2))
		LIMIT 1`, botID, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("no rival found")
	}
	return p, err
}

// TopPlayers returns leaderboard ordered by score descending.
func (r *DuelRepo) TopPlayers(ctx context.Context, botID uuid.UUID, limit int) ([]domain.DuelPlayer, error) {
	rows, err := r.db.Query(ctx, `SELECT `+duelCols+` FROM duel_players WHERE bot_id=$1 ORDER BY score DESC LIMIT $2`, botID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.DuelPlayer
	for rows.Next() {
		p, err := scanDuel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

func (r *DuelRepo) TotalPlayers(ctx context.Context, botID uuid.UUID) (int, error) {
	var n int
	err := r.db.QueryRow(ctx, `SELECT count(*) FROM duel_players WHERE bot_id=$1`, botID).Scan(&n)
	return n, err
}

// EnsureBots seeds fake rival bots if fewer than minBots exist.
func (r *DuelRepo) EnsureBots(ctx context.Context, botID uuid.UUID, minBots int, names []string) error {
	var count int
	if err := r.db.QueryRow(ctx, `SELECT count(*) FROM duel_players WHERE bot_id=$1 AND is_bot=true`, botID).Scan(&count); err != nil {
		return err
	}
	for i := count; i < minBots; i++ {
		score := 800 + (i*37)%400
		_, err := r.db.Exec(ctx, `
			INSERT INTO duel_players (bot_id, user_id, score, is_bot)
			VALUES ($1, $2, $3, true)
			ON CONFLICT DO NOTHING`, botID, int64(1000000+i), score)
		if err != nil {
			return err
		}
	}
	return nil
}

// ─── Duel Pool ───

func (r *DuelRepo) GetPool(ctx context.Context, botID uuid.UUID) (*domain.DuelPool, error) {
	p := &domain.DuelPool{}
	err := r.db.QueryRow(ctx, `
		INSERT INTO duel_pools (bot_id) VALUES ($1)
		ON CONFLICT (bot_id) DO UPDATE SET bot_id = duel_pools.bot_id
		RETURNING bot_id, total_toman, total_distributed`, botID).Scan(&p.BotID, &p.TotalToman, &p.TotalDistributed)
	return p, err
}

func (r *DuelRepo) AddToPool(ctx context.Context, botID uuid.UUID, toman int64) error {
	_, err := r.db.Exec(ctx, `UPDATE duel_pools SET total_toman = total_toman + $2 WHERE bot_id=$1`, botID, toman)
	return err
}

func (r *DuelRepo) ReducePool(ctx context.Context, botID uuid.UUID, toman int64) (ok bool, err error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE duel_pools SET total_toman = total_toman - $2, total_distributed = total_distributed + $2
		WHERE bot_id=$1 AND total_toman >= $2`, botID, toman)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// ─── Duel Withdrawals ───

func (r *DuelRepo) CreateWithdrawal(ctx context.Context, w *domain.DuelWithdrawal) (*domain.DuelWithdrawal, error) {
	out := &domain.DuelWithdrawal{}
	err := r.db.QueryRow(ctx, `
		INSERT INTO duel_withdrawals (bot_id, user_id, amount_mg, amount_toman, kind)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, bot_id, user_id, amount_mg, amount_toman, kind, status, note, created_at, updated_at`,
		w.BotID, w.UserID, w.AmountMg, w.AmountToman, w.Kind,
	).Scan(&out.ID, &out.BotID, &out.UserID, &out.AmountMg, &out.AmountToman, &out.Kind, &out.Status, &out.Note, &out.CreatedAt, &out.UpdatedAt)
	return out, err
}

func (r *DuelRepo) ListWithdrawals(ctx context.Context, botID uuid.UUID, status string, limit int) ([]domain.DuelWithdrawal, error) {
	q := `SELECT id, bot_id, user_id, amount_mg, amount_toman, kind, status, note, created_at, updated_at
		FROM duel_withdrawals WHERE bot_id=$1`
	args := []any{botID}
	if status != "" {
		q += ` AND status=$2`
		args = append(args, status)
	}
	q += ` ORDER BY created_at DESC LIMIT $` + fmt.Sprint(len(args)+1)
	args = append(args, limit)
	rows, err := r.db.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.DuelWithdrawal
	for rows.Next() {
		var w domain.DuelWithdrawal
		if err := rows.Scan(&w.ID, &w.BotID, &w.UserID, &w.AmountMg, &w.AmountToman, &w.Kind, &w.Status, &w.Note, &w.CreatedAt, &w.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

func (r *DuelRepo) UpdateWithdrawalStatus(ctx context.Context, id uuid.UUID, status, note string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE duel_withdrawals SET status=$2, note=$3, updated_at=now() WHERE id=$1`, id, status, note)
	return err
}

// MarkWithdrawn records a completed withdrawal on the player.
func (r *DuelRepo) MarkWithdrawn(ctx context.Context, botID uuid.UUID, userID int64, toman int64) error {
	_, err := r.db.Exec(ctx, `
		UPDATE duel_players SET total_withdrawn_toman = total_withdrawn_toman + $3
		WHERE bot_id=$1 AND user_id=$2`, botID, userID, toman)
	return err
}

// EarnToman converts opals to spent/earned toman tracking (when user wins opals from pool).
func (r *DuelRepo) EarnToman(ctx context.Context, botID uuid.UUID, userID int64, toman int64) error {
	_, err := r.db.Exec(ctx, `
		UPDATE duel_players SET total_spent_toman = total_spent_toman + $3
		WHERE bot_id=$1 AND user_id=$2`, botID, userID, toman)
	return err
}

var _ = time.Now // keep time import
