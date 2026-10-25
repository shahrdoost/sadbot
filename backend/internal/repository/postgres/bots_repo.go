package postgres

import (
	"context"
	"fmt"
	"time"

	"sadbot/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ─── Favaani ───

type FavaaniRepo struct{ db *pgxpool.Pool }

func NewFavaaniRepo(db *pgxpool.Pool) *FavaaniRepo { return &FavaaniRepo{db: db} }

func (r *FavaaniRepo) EnsureUser(ctx context.Context, botID uuid.UUID, userID int64, username string) (*domain.FavaaniUser, error) {
	u := &domain.FavaaniUser{}
	err := r.db.QueryRow(ctx, `
		INSERT INTO favaani_users (bot_id, user_id, username) VALUES ($1, $2, $3)
		ON CONFLICT (bot_id, user_id) DO UPDATE SET username = COALESCE($3, favaani_users.username)
		RETURNING bot_id, user_id, username, day, score, streak, total_spent, referral_code, referred_by, created_at`,
		botID, userID, username).Scan(&u.BotID, &u.UserID, &u.Username, &u.Day, &u.Score, &u.Streak, &u.TotalSpent, &u.ReferralCode, &u.ReferredBy, &u.CreatedAt)
	return u, err
}

func (r *FavaaniRepo) GetUser(ctx context.Context, botID uuid.UUID, userID int64) (*domain.FavaaniUser, error) {
	u := &domain.FavaaniUser{}
	err := r.db.QueryRow(ctx, `
		SELECT bot_id, user_id, username, day, score, streak, total_spent, referral_code, referred_by, created_at
		FROM favaani_users WHERE bot_id=$1 AND user_id=$2`, botID, userID).
		Scan(&u.BotID, &u.UserID, &u.Username, &u.Day, &u.Score, &u.Streak, &u.TotalSpent, &u.ReferralCode, &u.ReferredBy, &u.CreatedAt)
	return u, err
}

func (r *FavaaniRepo) SubmitChallenge(ctx context.Context, botID uuid.UUID, userID int64, day int, amount int64, log string) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO favaani_challenges (bot_id, user_id, day, amount, spend_log) VALUES ($1, $2, $3, $4, $5)`,
		botID, userID, day, amount, log)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(ctx, `
		UPDATE favaani_users SET day = $3, score = score + 1, streak = streak + 1, total_spent = total_spent + $4
		WHERE bot_id=$1 AND user_id=$2`, botID, userID, day+1, amount)
	return err
}

func (r *FavaaniRepo) CountUsers(ctx context.Context, botID uuid.UUID) (int, error) {
	var n int
	err := r.db.QueryRow(ctx, `SELECT count(*) FROM favaani_users WHERE bot_id=$1`, botID).Scan(&n)
	return n, err
}

// ─── Shlinks ───

type ShlinksRepo struct{ db *pgxpool.Pool }

func NewShlinksRepo(db *pgxpool.Pool) *ShlinksRepo { return &ShlinksRepo{db: db} }

func (r *ShlinksRepo) EnsureUser(ctx context.Context, botID uuid.UUID, userID int64, username string) (*domain.ShlinkUser, error) {
	u := &domain.ShlinkUser{}
	err := r.db.QueryRow(ctx, `
		INSERT INTO shlinks_users (bot_id, user_id, username) VALUES ($1, $2, $3)
		ON CONFLICT (bot_id, user_id) DO UPDATE SET username = COALESCE($3, shlinks_users.username)
		RETURNING bot_id, user_id, username, balance, total_earned, total_withdrawn, created_at`,
		botID, userID, username).Scan(&u.BotID, &u.UserID, &u.Username, &u.Balance, &u.TotalEarned, &u.TotalWithdrawn, &u.CreatedAt)
	return u, err
}

func (r *ShlinksRepo) GetUser(ctx context.Context, botID uuid.UUID, userID int64) (*domain.ShlinkUser, error) {
	u := &domain.ShlinkUser{}
	err := r.db.QueryRow(ctx, `
		SELECT bot_id, user_id, username, balance, total_earned, total_withdrawn, created_at
		FROM shlinks_users WHERE bot_id=$1 AND user_id=$2`, botID, userID).
		Scan(&u.BotID, &u.UserID, &u.Username, &u.Balance, &u.TotalEarned, &u.TotalWithdrawn, &u.CreatedAt)
	return u, err
}

func (r *ShlinksRepo) CreateLink(ctx context.Context, botID uuid.UUID, userID int64, shortCode, targetURL string) (*domain.ShlinkLink, error) {
	l := &domain.ShlinkLink{}
	err := r.db.QueryRow(ctx, `
		INSERT INTO shlinks_links (bot_id, user_id, short_code, target_url) VALUES ($1, $2, $3, $4)
		RETURNING id, bot_id, user_id, short_code, target_url, total_clicks, valid_clicks, total_earned, created_at`,
		botID, userID, shortCode, targetURL).Scan(&l.ID, &l.BotID, &l.UserID, &l.ShortCode, &l.TargetURL, &l.TotalClicks, &l.ValidClicks, &l.TotalEarned, &l.CreatedAt)
	return l, err
}

func (r *ShlinksRepo) GetLinkByCode(ctx context.Context, botID uuid.UUID, shortCode string) (*domain.ShlinkLink, error) {
	l := &domain.ShlinkLink{}
	err := r.db.QueryRow(ctx, `
		SELECT id, bot_id, user_id, short_code, target_url, total_clicks, valid_clicks, total_earned, created_at
		FROM shlinks_links WHERE bot_id=$1 AND short_code=$2`, botID, shortCode).
		Scan(&l.ID, &l.BotID, &l.UserID, &l.ShortCode, &l.TargetURL, &l.TotalClicks, &l.ValidClicks, &l.TotalEarned, &l.CreatedAt)
	return l, err
}

func (r *ShlinksRepo) ListLinks(ctx context.Context, botID uuid.UUID, userID int64) ([]domain.ShlinkLink, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, bot_id, user_id, short_code, target_url, total_clicks, valid_clicks, total_earned, created_at
		FROM shlinks_links WHERE bot_id=$1 AND user_id=$2 ORDER BY created_at DESC`, botID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.ShlinkLink
	for rows.Next() {
		var l domain.ShlinkLink
		if err := rows.Scan(&l.ID, &l.BotID, &l.UserID, &l.ShortCode, &l.TargetURL, &l.TotalClicks, &l.ValidClicks, &l.TotalEarned, &l.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (r *ShlinksRepo) RecordClick(ctx context.Context, botID uuid.UUID, linkID uuid.UUID, ip string, valid bool, earnAmount float64) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO shlinks_clicks (bot_id, link_id, ip, valid) VALUES ($1, $2, $3, $4)`,
		botID, linkID, ip, valid)
	if err != nil {
		return err
	}
	if valid {
		_, err = r.db.Exec(ctx, `
			UPDATE shlinks_links SET total_clicks = total_clicks + 1, valid_clicks = valid_clicks + 1, total_earned = total_earned + $3
			WHERE id=$1 AND bot_id=$2`, linkID, botID, earnAmount)
		if err != nil {
			return err
		}
		_, err = r.db.Exec(ctx, `
			UPDATE shlinks_users SET balance = balance + $3, total_earned = total_earned + $3
			WHERE bot_id=$1 AND user_id = (SELECT user_id FROM shlinks_links WHERE id=$2)`, botID, linkID, earnAmount)
	} else {
		_, err = r.db.Exec(ctx, `
			UPDATE shlinks_links SET total_clicks = total_clicks + 1 WHERE id=$1 AND bot_id=$2`, linkID, botID)
	}
	return err
}

func (r *ShlinksRepo) RequestWithdrawal(ctx context.Context, botID uuid.UUID, userID int64, amount float64) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE shlinks_users SET balance = balance - $3, total_withdrawn = total_withdrawn + $3
		WHERE bot_id=$1 AND user_id=$2 AND balance >= $3`, botID, userID, amount)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("insufficient balance")
	}
	_, err = r.db.Exec(ctx, `
		INSERT INTO shlinks_withdrawals (bot_id, user_id, amount) VALUES ($1, $2, $3)`,
		botID, userID, amount)
	return err
}

// ─── Whoseeme ───

type WhoseemeRepo struct{ db *pgxpool.Pool }

func NewWhoseemeRepo(db *pgxpool.Pool) *WhoseemeRepo { return &WhoseemeRepo{db: db} }

func (r *WhoseemeRepo) EnsureUser(ctx context.Context, botID uuid.UUID, userID int64, username string) (*domain.WhoseemeUser, error) {
	u := &domain.WhoseemeUser{}
	err := r.db.QueryRow(ctx, `
		INSERT INTO whoseeme_users (bot_id, user_id, username) VALUES ($1, $2, $3)
		ON CONFLICT (bot_id, user_id) DO UPDATE SET username = COALESCE($3, whoseeme_users.username)
		RETURNING bot_id, user_id, username, coins, balance, created_at`,
		botID, userID, username).Scan(&u.BotID, &u.UserID, &u.Username, &u.Coins, &u.Balance, &u.CreatedAt)
	return u, err
}

func (r *WhoseemeRepo) GetUser(ctx context.Context, botID uuid.UUID, userID int64) (*domain.WhoseemeUser, error) {
	u := &domain.WhoseemeUser{}
	err := r.db.QueryRow(ctx, `
		SELECT bot_id, user_id, username, coins, balance, created_at
		FROM whoseeme_users WHERE bot_id=$1 AND user_id=$2`, botID, userID).
		Scan(&u.BotID, &u.UserID, &u.Username, &u.Coins, &u.Balance, &u.CreatedAt)
	return u, err
}

func (r *WhoseemeRepo) CreateLink(ctx context.Context, botID uuid.UUID, userID int64, token, baitText string) (*domain.WhoseemeLink, error) {
	l := &domain.WhoseemeLink{}
	err := r.db.QueryRow(ctx, `
		INSERT INTO whoseeme_links (bot_id, user_id, token, bait_text) VALUES ($1, $2, $3, $4)
		RETURNING id, bot_id, user_id, token, bait_text, bait_image, content, is_active, created_at`,
		botID, userID, token, baitText).Scan(&l.ID, &l.BotID, &l.UserID, &l.Token, &l.BaitText, &l.BaitImage, &l.Content, &l.IsActive, &l.CreatedAt)
	return l, err
}

func (r *WhoseemeRepo) GetLinkByToken(ctx context.Context, botID uuid.UUID, token string) (*domain.WhoseemeLink, error) {
	l := &domain.WhoseemeLink{}
	err := r.db.QueryRow(ctx, `
		SELECT id, bot_id, user_id, token, bait_text, bait_image, content, is_active, created_at
		FROM whoseeme_links WHERE bot_id=$1 AND token=$2`, botID, token).
		Scan(&l.ID, &l.BotID, &l.UserID, &l.Token, &l.BaitText, &l.BaitImage, &l.Content, &l.IsActive, &l.CreatedAt)
	return l, err
}

func (r *WhoseemeRepo) RecordClick(ctx context.Context, botID uuid.UUID, linkID uuid.UUID, viewerName, viewerUsername string) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO whoseeme_clicks (bot_id, link_id, viewer_name, viewer_username) VALUES ($1, $2, $3, $4)`,
		botID, linkID, viewerName, viewerUsername)
	return err
}

func (r *WhoseemeRepo) GetClicks(ctx context.Context, botID uuid.UUID, linkID uuid.UUID) (int, error) {
	var n int
	err := r.db.QueryRow(ctx, `
		SELECT count(*) FROM whoseeme_clicks WHERE bot_id=$1 AND link_id=$2`, botID, linkID).Scan(&n)
	return n, err
}

func (r *WhoseemeRepo) ListLinks(ctx context.Context, botID uuid.UUID, userID int64) ([]domain.WhoseemeLink, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, bot_id, user_id, token, bait_text, bait_image, content, is_active, created_at
		FROM whoseeme_links WHERE bot_id=$1 AND user_id=$2 ORDER BY created_at DESC`, botID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.WhoseemeLink
	for rows.Next() {
		var l domain.WhoseemeLink
		if err := rows.Scan(&l.ID, &l.BotID, &l.UserID, &l.Token, &l.BaitText, &l.BaitImage, &l.Content, &l.IsActive, &l.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// ─── Faal ───

type FaalRepo struct{ db *pgxpool.Pool }

func NewFaalRepo(db *pgxpool.Pool) *FaalRepo { return &FaalRepo{db: db} }

func (r *FaalRepo) EnsureUser(ctx context.Context, botID uuid.UUID, userID int64, username string) (*domain.FaalUser, error) {
	u := &domain.FaalUser{}
	err := r.db.QueryRow(ctx, `
		INSERT INTO faal_users (bot_id, user_id, username) VALUES ($1, $2, $3)
		ON CONFLICT (bot_id, user_id) DO UPDATE SET username = COALESCE($3, faal_users.username)
		RETURNING bot_id, user_id, username, fortune_count, last_fortune_at, created_at`,
		botID, userID, username).Scan(&u.BotID, &u.UserID, &u.Username, &u.FortuneCount, &u.LastFortuneAt, &u.CreatedAt)
	return u, err
}

func (r *FaalRepo) GetLastFortuneTime(ctx context.Context, botID uuid.UUID, userID int64) (*time.Time, error) {
	var t *time.Time
	err := r.db.QueryRow(ctx, `SELECT last_fortune_at FROM faal_users WHERE bot_id=$1 AND user_id=$2`, botID, userID).Scan(&t)
	return t, err
}

func (r *FaalRepo) SaveFortune(ctx context.Context, botID uuid.UUID, userID int64, fortuneText, fortuneType string) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO faal_fortunes (bot_id, user_id, fortune_text, fortune_type) VALUES ($1, $2, $3, $4)`,
		botID, userID, fortuneText, fortuneType)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(ctx, `
		UPDATE faal_users SET fortune_count = fortune_count + 1, last_fortune_at = now()
		WHERE bot_id=$1 AND user_id=$2`, botID, userID)
	return err
}

func (r *FaalRepo) CountUsers(ctx context.Context, botID uuid.UUID) (int, error) {
	var n int
	err := r.db.QueryRow(ctx, `SELECT count(*) FROM faal_users WHERE bot_id=$1`, botID).Scan(&n)
	return n, err
}

// ─── AnonyChat ───

type AnonyChatRepo struct{ db *pgxpool.Pool }

func NewAnonyChatRepo(db *pgxpool.Pool) *AnonyChatRepo { return &AnonyChatRepo{db: db} }

func (r *AnonyChatRepo) EnsureUser(ctx context.Context, botID uuid.UUID, userID int64, username string) (*domain.AnonyChatUser, error) {
	u := &domain.AnonyChatUser{}
	err := r.db.QueryRow(ctx, `
		INSERT INTO anonychat_users (bot_id, user_id, username) VALUES ($1, $2, $3)
		ON CONFLICT (bot_id, user_id) DO UPDATE SET username = COALESCE($3, anonychat_users.username)
		RETURNING bot_id, user_id, username, status, partner_id, created_at`,
		botID, userID, username).Scan(&u.BotID, &u.UserID, &u.Username, &u.Status, &u.PartnerID, &u.CreatedAt)
	return u, err
}

func (r *AnonyChatRepo) GetStatus(ctx context.Context, botID uuid.UUID, userID int64) (status string, partnerID *int64, err error) {
	err = r.db.QueryRow(ctx, `SELECT status, partner_id FROM anonychat_users WHERE bot_id=$1 AND user_id=$2`, botID, userID).Scan(&status, &partnerID)
	return
}

func (r *AnonyChatRepo) SetStatus(ctx context.Context, botID uuid.UUID, userID int64, status string, partnerID *int64) error {
	_, err := r.db.Exec(ctx, `
		UPDATE anonychat_users SET status=$3, partner_id=$4 WHERE bot_id=$1 AND user_id=$2`,
		botID, userID, status, partnerID)
	return err
}

// FindPartner returns a random user in 'searching' status (excluding self).
func (r *AnonyChatRepo) FindPartner(ctx context.Context, botID uuid.UUID, userID int64) (int64, error) {
	var partnerID int64
	err := r.db.QueryRow(ctx, `
		SELECT user_id FROM anonychat_users
		WHERE bot_id=$1 AND user_id != $2 AND status='searching'
		ORDER BY random() LIMIT 1`, botID, userID).Scan(&partnerID)
	return partnerID, err
}

// MatchUsers atomically sets both users to chatting with each other.
func (r *AnonyChatRepo) MatchUsers(ctx context.Context, botID uuid.UUID, userA, userB int64) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `UPDATE anonychat_users SET status='chatting', partner_id=$3 WHERE bot_id=$1 AND user_id=$2`, botID, userA, userB)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE anonychat_users SET status='chatting', partner_id=$3 WHERE bot_id=$1 AND user_id=$2`, botID, userB, userA)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *AnonyChatRepo) EndChat(ctx context.Context, botID uuid.UUID, userID int64) (partnerID *int64, err error) {
	err = r.db.QueryRow(ctx, `
		UPDATE anonychat_users SET status='idle', partner_id=NULL WHERE bot_id=$1 AND user_id=$2
		RETURNING partner_id`, botID, userID).Scan(&partnerID)
	return
}

// ─── TapEarn ───

type TapEarnRepo struct{ db *pgxpool.Pool }

func NewTapEarnRepo(db *pgxpool.Pool) *TapEarnRepo { return &TapEarnRepo{db: db} }

func (r *TapEarnRepo) EnsureUser(ctx context.Context, botID uuid.UUID, userID int64, username string) (*domain.TapEarnUser, error) {
	u := &domain.TapEarnUser{}
	err := r.db.QueryRow(ctx, `
		INSERT INTO tapearn_users (bot_id, user_id, username) VALUES ($1, $2, $3)
		ON CONFLICT (bot_id, user_id) DO UPDATE SET username = COALESCE($3, tapearn_users.username)
		RETURNING bot_id, user_id, username, money, energy, max_energy, last_energy_update, last_tap, tap_power, created_at`,
		botID, userID, username).Scan(&u.BotID, &u.UserID, &u.Username, &u.Money, &u.Energy, &u.MaxEnergy, &u.LastEnergyUpdate, &u.LastTap, &u.TapPower, &u.CreatedAt)
	return u, err
}

func (r *TapEarnRepo) GetUser(ctx context.Context, botID uuid.UUID, userID int64) (*domain.TapEarnUser, error) {
	u := &domain.TapEarnUser{}
	err := r.db.QueryRow(ctx, `
		SELECT bot_id, user_id, username, money, energy, max_energy, last_energy_update, last_tap, tap_power, created_at
		FROM tapearn_users WHERE bot_id=$1 AND user_id=$2`, botID, userID).
		Scan(&u.BotID, &u.UserID, &u.Username, &u.Money, &u.Energy, &u.MaxEnergy, &u.LastEnergyUpdate, &u.LastTap, &u.TapPower, &u.CreatedAt)
	return u, err
}

// RechargeEnergy adds energy over time (lazy update).
func (r *TapEarnRepo) RechargeEnergy(ctx context.Context, botID uuid.UUID, userID int64, rechargePerMin int) (int, error) {
	var energy int
	err := r.db.QueryRow(ctx, `
		UPDATE tapearn_users SET
			energy = LEAST(max_energy, energy + GREATEST(0, floor(extract(epoch from (now() - last_energy_update)) / 60)::int * $3)),
			last_energy_update = CASE WHEN energy + GREATEST(0, floor(extract(epoch from (now() - last_energy_update)) / 60)::int * $3) >= max_energy
				THEN now() ELSE last_energy_update END
		WHERE bot_id=$1 AND user_id=$2
		RETURNING energy`, botID, userID, rechargePerMin).Scan(&energy)
	return energy, err
}

// Tap processes a tap: spend 1 energy, gain tap_power money.
func (r *TapEarnRepo) Tap(ctx context.Context, botID uuid.UUID, userID int64) (earned int64, err error) {
	err = r.db.QueryRow(ctx, `
		UPDATE tapearn_users SET
			energy = energy - 1, money = money + tap_power, last_tap = now()
		WHERE bot_id=$1 AND user_id=$2 AND energy > 0
		RETURNING tap_power`, botID, userID).Scan(&earned)
	if err == pgx.ErrNoRows {
		return 0, fmt.Errorf("no energy")
	}
	return earned, err
}

func (r *TapEarnRepo) UpgradePower(ctx context.Context, botID uuid.UUID, userID int64, cost int64) (ok bool, err error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE tapearn_users SET money = money - $3, tap_power = tap_power + 1
		WHERE bot_id=$1 AND user_id=$2 AND money >= $3`, botID, userID, cost)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (r *TapEarnRepo) CountUsers(ctx context.Context, botID uuid.UUID) (int, error) {
	var n int
	err := r.db.QueryRow(ctx, `SELECT count(*) FROM tapearn_users WHERE bot_id=$1`, botID).Scan(&n)
	return n, err
}

var _ = time.Now
