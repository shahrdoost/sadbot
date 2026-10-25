package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"sadbot/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DatingRepo struct {
	db *pgxpool.Pool
}

func NewDatingRepo(db *pgxpool.Pool) *DatingRepo { return &DatingRepo{db: db} }

const datingCols = `bot_id, user_id, telegram_id, name, gender, age, province, city, interests, status, partner_user_id, rating, total_ratings, rating_sum, last_seen_at, coins, channel_bonus_at, photo_file_id, avatar_bonus_at, is_ghost, ghost_persona, ai_free_used, last_notified_at, notify_sent_on, notify_count_on, notify_dismissed_at, streak_count, streak_best, streak_last_date, boosted_until, name_edited_at, profile_edited_at`

func scanDating(row pgx.Row) (*domain.DatingProfile, error) {
	p := &domain.DatingProfile{}
	err := row.Scan(&p.BotID, &p.UserID, &p.TelegramID, &p.Name, &p.Gender, &p.Age, &p.Province, &p.City, &p.Interests, &p.Status, &p.PartnerUserID, &p.Rating, &p.TotalRatings, &p.RatingSum, &p.LastSeenAt, &p.Coins, &p.ChannelBonusAt, &p.PhotoFileID, &p.AvatarBonusAt, &p.IsGhost, &p.GhostPersona, &p.AIFreeUsed, &p.LastNotifiedAt, &p.NotifySentOn, &p.NotifyCountOn, &p.NotifyDismissedAt, &p.StreakCount, &p.StreakBest, &p.StreakLastDate, &p.BoostedUntil, &p.NameEditedAt, &p.ProfileEditedAt)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (r *DatingRepo) EnsureProfile(ctx context.Context, botID uuid.UUID, userID, telegramID int64) (*domain.DatingProfile, error) {
	return scanDating(r.db.QueryRow(ctx, `
		INSERT INTO dating_profiles (bot_id, user_id, telegram_id)
		VALUES ($1, $2, $3)
		ON CONFLICT (bot_id, user_id) DO UPDATE SET last_seen_at = now()
		RETURNING `+datingCols,
		botID, userID, telegramID))
}

func (r *DatingRepo) GetProfile(ctx context.Context, botID uuid.UUID, userID int64) (*domain.DatingProfile, error) {
	p, err := scanDating(r.db.QueryRow(ctx, `SELECT `+datingCols+` FROM dating_profiles WHERE bot_id = $1 AND user_id = $2`, botID, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("dating profile not found")
	}
	return p, err
}

func (r *DatingRepo) GetProfileByTelegramID(ctx context.Context, botID uuid.UUID, telegramID int64) (*domain.DatingProfile, error) {
	p, err := scanDating(r.db.QueryRow(ctx, `SELECT `+datingCols+` FROM dating_profiles WHERE bot_id = $1 AND telegram_id = $2`, botID, telegramID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("dating profile not found")
	}
	return p, err
}

type ProfileUpdate struct {
	Name        *string
	Gender      *string
	Age         *int
	Province    *string
	City        *string
	Interests   []string // nil = untouched; empty slice = cleared
	PhotoFileID *string
}

func (r *DatingRepo) UpdateProfile(ctx context.Context, botID uuid.UUID, userID int64, u ProfileUpdate) (*domain.DatingProfile, error) {
	return scanDating(r.db.QueryRow(ctx, `
		UPDATE dating_profiles SET
			name = COALESCE($3, name),
			gender = COALESCE($4, gender),
			age = COALESCE($5, age),
			province = COALESCE($6, province),
			city = COALESCE($7, city),
			interests = CASE WHEN $8::text[] IS NULL THEN interests ELSE $8::text[] END,
			photo_file_id = COALESCE($9, photo_file_id),
			last_seen_at = now()
		WHERE bot_id = $1 AND user_id = $2
		RETURNING `+datingCols,
		botID, userID, u.Name, u.Gender, u.Age, u.Province, u.City, u.Interests, u.PhotoFileID))
}

// SetStatus atomically transitions status; CAS on fromStatus ("" = any).
func (r *DatingRepo) SetStatus(ctx context.Context, botID uuid.UUID, userID int64, from, to string) (bool, error) {
	q := `UPDATE dating_profiles SET status = $4 WHERE bot_id = $1 AND user_id = $2`
	args := []any{botID, userID, nil, to}
	if from != "" {
		args[2] = from
		q += ` AND status = $3`
	}
	tag, err := r.db.Exec(ctx, q, args...)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// SetPartner CAS-transitions status and partner together.
func (r *DatingRepo) BeginChatting(ctx context.Context, botID uuid.UUID, userID, partnerUserID int64) (bool, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE dating_profiles SET status = 'chatting', partner_user_id = $3, last_seen_at = now()
		WHERE bot_id = $1 AND user_id = $2 AND status = 'searching'`,
		botID, userID, partnerUserID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// EndChat clears partner back to joined. Returns true if the user was chatting.
func (r *DatingRepo) EndChat(ctx context.Context, botID uuid.UUID, userID int64) (bool, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE dating_profiles SET status = 'joined', partner_user_id = NULL, last_seen_at = now()
		WHERE bot_id = $1 AND user_id = $2 AND status = 'chatting'`,
		botID, userID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (r *DatingRepo) LeaveSearch(ctx context.Context, botID uuid.UUID, userID int64) error {
	_, err := r.db.Exec(ctx, `
		DELETE FROM dating_search_requests WHERE bot_id = $1 AND user_id = $2`, botID, userID)
	if err != nil {
		return err
	}
	_, err = r.SetStatus(ctx, botID, userID, domain.DatingSearching, domain.DatingJoined)
	return err
}

// --- search queue ---

func (r *DatingRepo) EnqueueSearch(ctx context.Context, botID uuid.UUID, userID int64, filters map[string]any) error {
	raw, err := json.Marshal(filters)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(ctx, `
		INSERT INTO dating_search_requests (bot_id, user_id, filters)
		VALUES ($1, $2, $3)
		ON CONFLICT (bot_id, user_id) DO UPDATE SET filters = EXCLUDED.filters, created_at = now()`,
		botID, userID, raw)
	return err
}

// SearchCost reads the coin cost recorded on the user's active search request (0 when none).
func (r *DatingRepo) SearchCost(ctx context.Context, botID uuid.UUID, userID int64) (int, error) {
	var cost int
	err := r.db.QueryRow(ctx, `
		SELECT COALESCE(filters->>'cost', '0')::int FROM dating_search_requests
		WHERE bot_id = $1 AND user_id = $2`, botID, userID).Scan(&cost)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	return cost, err
}

// LockSearchingRows locks all searching profiles of the bot for the matcher transaction.
func (r *DatingRepo) LockSearchingProfiles(ctx context.Context, tx pgx.Tx, botID uuid.UUID) ([]domain.DatingProfile, error) {
	rows, err := tx.Query(ctx, `
		SELECT `+datingCols+` FROM dating_profiles
		WHERE bot_id = $1 AND status = 'searching'
		FOR UPDATE SKIP LOCKED`, botID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.DatingProfile
	for rows.Next() {
		p, err := scanDating(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

func (r *DatingRepo) Begin(ctx context.Context) (pgx.Tx, error) { return r.db.Begin(ctx) }

// --- msg refs (reply map) ---

func (r *DatingRepo) BindMsgRef(ctx context.Context, botID uuid.UUID, viewerUserID, viewerMsgID, partnerUserID, partnerMsgID int64) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO dating_msg_refs (bot_id, viewer_user_id, viewer_msg_id, partner_user_id, partner_msg_id)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (bot_id, viewer_user_id, viewer_msg_id)
		DO UPDATE SET partner_user_id = EXCLUDED.partner_user_id, partner_msg_id = EXCLUDED.partner_msg_id`,
		botID, viewerUserID, viewerMsgID, partnerUserID, partnerMsgID)
	return err
}

func (r *DatingRepo) ResolveMsgRef(ctx context.Context, botID uuid.UUID, viewerUserID, viewerMsgID int64) (*domain.DatingMsgRef, error) {
	ref := &domain.DatingMsgRef{}
	err := r.db.QueryRow(ctx, `
		SELECT bot_id, viewer_user_id, viewer_msg_id, partner_user_id, partner_msg_id, created_at
		FROM dating_msg_refs WHERE bot_id = $1 AND viewer_user_id = $2 AND viewer_msg_id = $3`,
		botID, viewerUserID, viewerMsgID,
	).Scan(&ref.BotID, &ref.ViewerUserID, &ref.ViewerMsgID, &ref.PartnerUserID, &ref.PartnerMsgID, &ref.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return ref, nil
}

// --- pending ratings ---

func (r *DatingRepo) SetPendingRate(ctx context.Context, botID uuid.UUID, userID, partnerUserID int64) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO dating_pending_rates (bot_id, user_id, partner_user_id)
		VALUES ($1, $2, $3)
		ON CONFLICT (bot_id, user_id) DO UPDATE SET partner_user_id = EXCLUDED.partner_user_id, created_at = now()`,
		botID, userID, partnerUserID)
	return err
}

func (r *DatingRepo) GetPendingRate(ctx context.Context, botID uuid.UUID, userID int64) (*domain.DatingPendingRate, error) {
	pr := &domain.DatingPendingRate{}
	err := r.db.QueryRow(ctx, `
		SELECT bot_id, user_id, partner_user_id, created_at
		FROM dating_pending_rates WHERE bot_id = $1 AND user_id = $2`,
		botID, userID,
	).Scan(&pr.BotID, &pr.UserID, &pr.PartnerUserID, &pr.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return pr, nil
}

func (r *DatingRepo) ClearPendingRate(ctx context.Context, botID uuid.UUID, userID int64) error {
	_, err := r.db.Exec(ctx, `DELETE FROM dating_pending_rates WHERE bot_id = $1 AND user_id = $2`, botID, userID)
	return err
}

// ApplyRating updates the rolling average like kpkchat: newAvg = round1((sum + score) / (total + 1)).
func (r *DatingRepo) ApplyRating(ctx context.Context, botID uuid.UUID, targetUserID int64, score int) error {
	_, err := r.db.Exec(ctx, `
		UPDATE dating_profiles SET
			rating_sum = rating_sum + $3,
			total_ratings = total_ratings + 1,
			rating = round(((rating_sum + $3)::numeric / (total_ratings + 1))::numeric, 1)
		WHERE bot_id = $1 AND user_id = $2`,
		botID, targetUserID, score)
	return err
}

// --- blocks ---

func (r *DatingRepo) Block(ctx context.Context, botID uuid.UUID, blockerID, blockedID int64, ttl time.Duration) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO dating_block_pairs (bot_id, blocker_id, blocked_id, expires_at)
		VALUES ($1, $2, $3, now() + $4::interval)
		ON CONFLICT (bot_id, blocker_id, blocked_id) DO UPDATE SET expires_at = EXCLUDED.expires_at`,
		botID, blockerID, blockedID, ttl.String())
	return err
}

func (r *DatingRepo) IsBlocked(ctx context.Context, botID uuid.UUID, a, b int64) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM dating_block_pairs
			WHERE bot_id = $1 AND expires_at > now()
			AND ((blocker_id = $2 AND blocked_id = $3) OR (blocker_id = $3 AND blocked_id = $2))
		)`, botID, a, b).Scan(&exists)
	return exists, err
}

// --- matcher support ---

// DatingSearchRow is a searching user joined with their chat id (for the matcher).
type DatingSearchRow struct {
	UserID     int64
	TelegramID int64
	Gender     *string
	Province         *string
	City             *string
	Age              *int
	Interests        []string
	Filter           string
	ProvinceFilter   string
	CityFilter       string
	InterestsFilter  []string
	AgeMin           int
	AgeMax           int
	CreatedAt        time.Time
}

func (r *DatingRepo) Searchers(ctx context.Context, botID uuid.UUID) ([]DatingSearchRow, error) {
	rows, err := r.db.Query(ctx, `
		SELECT p.user_id, p.telegram_id, p.gender, p.province, p.city, p.age, p.interests,
		       COALESCE(sq.filters->>'gender', 'any'),
		       COALESCE(sq.filters->>'province', ''),
		       COALESCE(sq.filters->>'city', ''),
		       COALESCE(ARRAY(SELECT jsonb_array_elements_text(sq.filters->'interests')), '{}'),
		       COALESCE(sq.filters->>'age_min', '0')::int,
		       COALESCE(sq.filters->>'age_max', '0')::int,
		       sq.created_at
		FROM dating_profiles p
		JOIN dating_search_requests sq ON sq.bot_id = p.bot_id AND sq.user_id = p.user_id
		WHERE p.bot_id = $1 AND p.status = 'searching'`, botID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DatingSearchRow
	for rows.Next() {
		var s DatingSearchRow
		if err := rows.Scan(&s.UserID, &s.TelegramID, &s.Gender, &s.Province, &s.City, &s.Age, &s.Interests, &s.Filter, &s.ProvinceFilter, &s.CityFilter, &s.InterestsFilter, &s.AgeMin, &s.AgeMax, &s.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// SearchExpiry identifies a timed-out search request.
type SearchExpiry struct {
	UserID    int64
	CreatedAt time.Time
}

// ExpiredSearches returns requests past their per-request timeout
// (filters.timeout_sec; default 40s — matcher quick searches).
func (r *DatingRepo) ExpiredSearches(ctx context.Context, botID uuid.UUID) ([]SearchExpiry, error) {
	rows, err := r.db.Query(ctx, `
		SELECT user_id, created_at FROM dating_search_requests
		WHERE bot_id = $1 AND created_at < now() - (COALESCE(filters->>'timeout_sec', '40') || ' seconds')::interval`, botID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SearchExpiry
	for rows.Next() {
		var e SearchExpiry
		if err := rows.Scan(&e.UserID, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// --- wallet ---

// AddCoins credits coins (n may be negative for adjustments).
func (r *DatingRepo) AddCoins(ctx context.Context, botID uuid.UUID, userID int64, n int) (int, error) {
	var coins int
	err := r.db.QueryRow(ctx, `
		UPDATE dating_profiles SET coins = coins + $3
		WHERE bot_id = $1 AND user_id = $2
		RETURNING coins`,
		botID, userID, n).Scan(&coins)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("dating profile not found")
	}
	return coins, err
}

// SpendCoins atomically deducts n coins when the balance allows (CAS).
func (r *DatingRepo) SpendCoins(ctx context.Context, botID uuid.UUID, userID int64, n int) (bool, error) {
	if n <= 0 {
		return true, nil
	}
	tag, err := r.db.Exec(ctx, `
		UPDATE dating_profiles SET coins = coins - $3
		WHERE bot_id = $1 AND user_id = $2 AND coins >= $3`,
		botID, userID, n)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// SetChannelBonus records the one-time channel mission; returns false if already claimed.
func (r *DatingRepo) SetChannelBonus(ctx context.Context, botID uuid.UUID, userID int64) (bool, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE dating_profiles SET channel_bonus_at = now()
		WHERE bot_id = $1 AND user_id = $2 AND channel_bonus_at IS NULL`,
		botID, userID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// --- avatars ---

// SetPhoto stores the profile photo; returns true when the one-time avatar
// bonus has just been claimed (first photo ever).
func (r *DatingRepo) SetPhoto(ctx context.Context, botID uuid.UUID, userID int64, fileID string) (bool, error) {
	var first bool
	err := r.db.QueryRow(ctx, `
		UPDATE dating_profiles p
		SET photo_file_id = $3, avatar_bonus_at = COALESCE(p.avatar_bonus_at, now())
		FROM (SELECT avatar_bonus_at AS old FROM dating_profiles WHERE bot_id = $1 AND user_id = $2) o
		WHERE p.bot_id = $1 AND p.user_id = $2
		RETURNING o.old IS NULL`,
		botID, userID, fileID).Scan(&first)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, fmt.Errorf("dating profile not found")
	}
	return first, err
}

// --- browse likes ---

// InsertLike records a browse action; false when the pair already reacted.
func (r *DatingRepo) InsertLike(ctx context.Context, botID uuid.UUID, fromUserID, toUserID int64, kind string) (bool, error) {
	tag, err := r.db.Exec(ctx, `
		INSERT INTO dating_likes (bot_id, from_user_id, to_user_id, kind)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (bot_id, from_user_id, to_user_id) DO NOTHING`,
		botID, fromUserID, toUserID, kind)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// HasLiked reports whether from has liked/superliked to (passes don't count).
func (r *DatingRepo) HasLiked(ctx context.Context, botID uuid.UUID, fromUserID, toUserID int64) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM dating_likes
			WHERE bot_id = $1 AND from_user_id = $2 AND to_user_id = $3
			AND kind IN ('like','super')
		)`, botID, fromUserID, toUserID).Scan(&exists)
	return exists, err
}

// CountLikesSince counts like/super actions by the user since the cutoff (free-quota).
func (r *DatingRepo) CountLikesSince(ctx context.Context, botID uuid.UUID, fromUserID int64, since time.Time) (int, error) {
	var n int
	err := r.db.QueryRow(ctx, `
		SELECT count(*) FROM dating_likes
		WHERE bot_id = $1 AND from_user_id = $2 AND created_at >= $3
		AND kind IN ('like','super')`,
		botID, fromUserID, since).Scan(&n)
	return n, err
}

// CountSuperSince counts superlikes by the user since the cutoff (daily cap).
func (r *DatingRepo) CountSuperSince(ctx context.Context, botID uuid.UUID, fromUserID int64, since time.Time) (int, error) {
	var n int
	err := r.db.QueryRow(ctx, `
		SELECT count(*) FROM dating_likes
		WHERE bot_id = $1 AND from_user_id = $2 AND created_at >= $3 AND kind = 'super'`,
		botID, fromUserID, since).Scan(&n)
	return n, err
}

// BrowseCandidates returns one complete, opposite-gender, never-reacted profile
// (status joined; blocks respected both ways; most recently seen first).
// BrowseFilter narrows the Tinder-style deck (kpkchat «جستجوی کاربران»
// filtered browse). Zero values = no constraint.
type BrowseFilter struct {
	Gender         string // "" = no gender constraint (caller passes viewer-opposite)
	ProvinceFilter string
	City           string
	Interests      []string // match profiles sharing ≥1 of these interests
	AgeMin         int
	AgeMax         int
}

func (r *DatingRepo) BrowseCandidates(ctx context.Context, botID uuid.UUID, viewerID int64, wantGender string, limit int) ([]domain.DatingProfile, error) {
	return r.BrowseCandidatesFiltered(ctx, botID, viewerID, BrowseFilter{Gender: wantGender}, limit)
}

func (r *DatingRepo) BrowseCandidatesFiltered(ctx context.Context, botID uuid.UUID, viewerID int64, f BrowseFilter, limit int) ([]domain.DatingProfile, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+datingCols+` FROM dating_profiles p
		WHERE p.bot_id = $1 AND p.user_id <> $2 AND p.status = 'joined'
			AND p.name IS NOT NULL AND p.name <> '' AND p.gender IS NOT NULL
			AND ($3 = '' OR p.gender = $3)
			AND ($5 = '' OR p.province = $5)
			AND ($6 = '' OR p.city = $6)
			AND (cardinality($7::text[]) = 0 OR p.interests && $7::text[])
			AND ($8 = 0 OR p.age >= $8)
			AND ($9 = 0 OR p.age <= $9)
			AND NOT EXISTS (
				SELECT 1 FROM dating_likes l
				WHERE l.bot_id = $1 AND l.from_user_id = $2 AND l.to_user_id = p.user_id
			)
			AND NOT EXISTS (
				SELECT 1 FROM dating_block_pairs b
				WHERE b.bot_id = $1 AND b.expires_at > now()
				AND ((b.blocker_id = $2 AND b.blocked_id = p.user_id) OR (b.blocker_id = p.user_id AND b.blocked_id = $2))
			)
		ORDER BY (p.boosted_until > now()) DESC, p.last_seen_at DESC
		LIMIT $4`,
		botID, viewerID, f.Gender, limit, f.ProvinceFilter, f.City, f.Interests, f.AgeMin, f.AgeMax)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.DatingProfile
	for rows.Next() {
		p, err := scanDating(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// --- phase 3: AI counters, ghosts, notifications ---

// IncAIFree increments the free AI message counter and returns the new value.
func (r *DatingRepo) IncAIFree(ctx context.Context, botID uuid.UUID, userID int64) (int, error) {
	var n int
	err := r.db.QueryRow(ctx, `
		UPDATE dating_profiles SET ai_free_used = ai_free_used + 1
		WHERE bot_id = $1 AND user_id = $2
		RETURNING ai_free_used`,
		botID, userID).Scan(&n)
	return n, err
}

// SetGhost marks a profile as a bot-controlled ghost persona.
func (r *DatingRepo) SetGhost(ctx context.Context, botID uuid.UUID, userID int64, persona string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE dating_profiles SET is_ghost = true, ghost_persona = $3
		WHERE bot_id = $1 AND user_id = $2`,
		botID, userID, persona)
	return err
}

// CountGhosts counts ghost profiles of a bot.
func (r *DatingRepo) CountGhosts(ctx context.Context, botID uuid.UUID) (int, error) {
	var n int
	err := r.db.QueryRow(ctx, `SELECT count(*) FROM dating_profiles WHERE bot_id = $1 AND is_ghost`, botID).Scan(&n)
	return n, err
}

// FreeGhosts returns idle ghosts (joined) available for injection.
func (r *DatingRepo) FreeGhosts(ctx context.Context, botID uuid.UUID, limit int) ([]domain.DatingProfile, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+datingCols+` FROM dating_profiles
		WHERE bot_id = $1 AND is_ghost AND status = 'joined'
		ORDER BY last_seen_at NULLS FIRST
		LIMIT $2`, botID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.DatingProfile
	for rows.Next() {
		p, err := scanDating(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// CountRealActive counts non-ghost users active within the window (ghost intensity).
func (r *DatingRepo) CountRealActive(ctx context.Context, botID uuid.UUID, within time.Duration) (int, error) {
	var n int
	err := r.db.QueryRow(ctx, `
		SELECT count(*) FROM dating_profiles
		WHERE bot_id = $1 AND NOT is_ghost AND last_seen_at > now() - $2::interval`,
		botID, within.String()).Scan(&n)
	return n, err
}

// NotifyRow is a notification candidate.
type NotifyRow struct {
	UserID     int64
	TelegramID int64
}

// NotifyCandidates selects re-engagement targets honoring all hard caps:
// joined+complete+real, seen in the last 14d but idle >2h, not dismissed
// since their last visit, gap since last notify, daily cap.
func (r *DatingRepo) NotifyCandidates(ctx context.Context, botID uuid.UUID, dailyCap, gapHours, limit int) ([]NotifyRow, error) {
	rows, err := r.db.Query(ctx, `
		SELECT user_id, telegram_id FROM dating_profiles
		WHERE bot_id = $1 AND NOT is_ghost AND status = 'joined'
			AND name IS NOT NULL AND name <> '' AND gender IS NOT NULL
			AND last_seen_at > now() - interval '14 days'
			AND last_seen_at < now() - interval '2 hours'
			AND (notify_dismissed_at IS NULL OR notify_dismissed_at < last_seen_at)
			AND (last_notified_at IS NULL OR last_notified_at < now() - $2::interval)
			AND (notify_sent_on IS NULL OR notify_sent_on < current_date OR notify_count_on < $3)
		ORDER BY last_seen_at DESC
		LIMIT $4`,
		botID, fmt.Sprintf("%d hours", gapHours), dailyCap, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []NotifyRow
	for rows.Next() {
		var n NotifyRow
		if err := rows.Scan(&n.UserID, &n.TelegramID); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// MarkNotified bumps the per-user notification counters (daily cap aware).
func (r *DatingRepo) MarkNotified(ctx context.Context, botID uuid.UUID, userIDs []int64) error {
	if len(userIDs) == 0 {
		return nil
	}
	_, err := r.db.Exec(ctx, `
		UPDATE dating_profiles SET
			last_notified_at = now(),
			notify_count_on = CASE WHEN notify_sent_on IS NULL OR notify_sent_on < current_date THEN 1 ELSE notify_count_on + 1 END,
			notify_sent_on = current_date
		WHERE bot_id = $1 AND user_id = ANY($2)`,
		botID, userIDs)
	return err
}

// SetNotifyDismissed records the user's mute choice.
func (r *DatingRepo) SetNotifyDismissed(ctx context.Context, botID uuid.UUID, userID int64) error {
	_, err := r.db.Exec(ctx, `
		UPDATE dating_profiles SET notify_dismissed_at = now()
		WHERE bot_id = $1 AND user_id = $2`,
		botID, userID)
	return err
}

// DatingUserCard is a panel row: end user joined with their dating profile.
type DatingUserCard struct {
	UserID       int64     `json:"user_id"`
	TelegramID   int64     `json:"telegram_id"`
	Username     *string   `json:"username,omitempty"`
	FirstName    *string   `json:"first_name,omitempty"`
	LastSeenAt   time.Time `json:"last_seen_at"`
	Status       *string   `json:"status,omitempty"`
	Coins        *int      `json:"coins,omitempty"`
	Rating       *float64  `json:"rating,omitempty"`
	TotalRatings *int      `json:"total_ratings,omitempty"`
	IsGhost      bool      `json:"is_ghost"`
	GhostPersona *string   `json:"ghost_persona,omitempty"`
}

// UserCards lists bot end users joined with dating profiles (panel UsersTab).
// ghostOnly restricts the list to ghost (fake) profiles.
func (r *DatingRepo) UserCards(ctx context.Context, botID uuid.UUID, q string, page, size int, ghostOnly bool) ([]DatingUserCard, int, error) {
	if page < 1 {
		page = 1
	}
	if size <= 0 || size > 100 {
		size = 20
	}
	offset := (page - 1) * size
	like := "%" + q + "%"
	var total int
	if err := r.db.QueryRow(ctx, `
		SELECT count(*) FROM bot_end_users u
		LEFT JOIN dating_profiles p ON p.bot_id = u.bot_id AND p.user_id = u.id
		WHERE u.bot_id = $1 AND ($2 = '' OR u.telegram_id::text = $2 OR u.username ILIKE $3 OR u.first_name ILIKE $3)
		  AND ($4 = false OR COALESCE(p.is_ghost, false))`,
		botID, q, like, ghostOnly).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.Query(ctx, `
		SELECT u.id, u.telegram_id, u.username, u.first_name, u.last_seen_at,
		       p.status, p.coins, p.rating, p.total_ratings, COALESCE(p.is_ghost, false), p.ghost_persona
		FROM bot_end_users u
		LEFT JOIN dating_profiles p ON p.bot_id = u.bot_id AND p.user_id = u.id
		WHERE u.bot_id = $1 AND ($2 = '' OR u.telegram_id::text = $2 OR u.username ILIKE $3 OR u.first_name ILIKE $3)
		  AND ($4 = false OR COALESCE(p.is_ghost, false))
		ORDER BY u.last_seen_at DESC
		LIMIT $5 OFFSET $6`,
		botID, q, like, ghostOnly, size, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []DatingUserCard
	for rows.Next() {
		var c DatingUserCard
		if err := rows.Scan(&c.UserID, &c.TelegramID, &c.Username, &c.FirstName, &c.LastSeenAt,
			&c.Status, &c.Coins, &c.Rating, &c.TotalRatings, &c.IsGhost, &c.GhostPersona); err != nil {
			return nil, 0, err
		}
		out = append(out, c)
	}
	return out, total, rows.Err()
}

// BlockPairRow is one active block relationship enriched with display names.
type BlockPairRow struct {
	BlockerID   int64     `json:"blocker_id"`
	BlockerName string    `json:"blocker_name"`
	BlockedID   int64     `json:"blocked_id"`
	BlockedName string    `json:"blocked_name"`
	CreatedAt   time.Time `json:"created_at"`
}

// BlockPairs lists active (unexpired) block relationships of a bot with both
// parties' dating names — the panel's blocked-relations scanner.
func (r *DatingRepo) BlockPairs(ctx context.Context, botID uuid.UUID, limit int) ([]BlockPairRow, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := r.db.Query(ctx, `
		SELECT b.blocker_id, COALESCE(pb.name, '#' || b.blocker_id),
		       b.blocked_id, COALESCE(pu.name, '#' || b.blocked_id),
		       b.created_at
		FROM dating_block_pairs b
		LEFT JOIN dating_profiles pb ON pb.bot_id = b.bot_id AND pb.user_id = b.blocker_id
		LEFT JOIN dating_profiles pu ON pu.bot_id = b.bot_id AND pu.user_id = b.blocked_id
		WHERE b.bot_id = $1 AND b.expires_at > now()
		ORDER BY b.created_at DESC
		LIMIT $2`, botID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []BlockPairRow{}
	for rows.Next() {
		var b BlockPairRow
		if err := rows.Scan(&b.BlockerID, &b.BlockerName, &b.BlockedID, &b.BlockedName, &b.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// RecordStreak atomically advances the daily streak and returns the new count.
func (r *DatingRepo) RecordStreak(ctx context.Context, botID uuid.UUID, userID int64) (int, error) {
	var count int
	err := r.db.QueryRow(ctx, `
		UPDATE dating_profiles SET
			streak_count = CASE
				WHEN streak_last_date = current_date THEN streak_count
				WHEN streak_last_date = current_date - 1 THEN streak_count + 1
				ELSE 1 END,
			streak_best = GREATEST(streak_best, CASE
				WHEN streak_last_date = current_date THEN streak_count
				WHEN streak_last_date = current_date - 1 THEN streak_count + 1
				ELSE 1 END),
			streak_last_date = current_date
		WHERE bot_id = $1 AND user_id = $2
		RETURNING streak_count`,
		botID, userID).Scan(&count)
	return count, err
}

// SetBoost extends the profile boost window until the given time.
func (r *DatingRepo) SetBoost(ctx context.Context, botID uuid.UUID, userID int64, until time.Time) error {
	_, err := r.db.Exec(ctx, `
		UPDATE dating_profiles SET boosted_until = GREATEST(COALESCE(boosted_until, now()), $3)
		WHERE bot_id = $1 AND user_id = $2`,
		botID, userID, until)
	return err
}

// MarkNameEdited records a paid name edit timestamp.
func (r *DatingRepo) MarkNameEdited(ctx context.Context, botID uuid.UUID, userID int64) error {
	_, err := r.db.Exec(ctx, `
		UPDATE dating_profiles SET name_edited_at = now()
		WHERE bot_id = $1 AND user_id = $2`, botID, userID)
	return err
}

// MarkProfileEdited records a paid profile edit timestamp.
func (r *DatingRepo) MarkProfileEdited(ctx context.Context, botID uuid.UUID, userID int64) error {
	_, err := r.db.Exec(ctx, `
		UPDATE dating_profiles SET profile_edited_at = now()
		WHERE bot_id = $1 AND user_id = $2`, botID, userID)
	return err
}

// --- panel admin operations ---

// AdjustCoins credits or debits coins by delta, clamping at zero so the
// non-negative CHECK cannot be violated by a panel debit.
func (r *DatingRepo) AdjustCoins(ctx context.Context, botID uuid.UUID, userID int64, delta int) (int, error) {
	var coins int
	err := r.db.QueryRow(ctx, `
		UPDATE dating_profiles SET coins = GREATEST(coins + $3, 0)
		WHERE bot_id = $1 AND user_id = $2
		RETURNING coins`,
		botID, userID, delta).Scan(&coins)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("dating profile not found")
	}
	return coins, err
}

// ResetProfile wipes a user's dating content while keeping the profile row, so
// they resume without re-onboarding. Returns false when no profile exists.
func (r *DatingRepo) ResetProfile(ctx context.Context, botID uuid.UUID, userID int64) (bool, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx, `
		UPDATE dating_profiles SET
			coins = 0, rating = 0, total_ratings = 0, rating_sum = 0,
			streak_count = 0, streak_best = 0, streak_last_date = NULL,
			boosted_until = NULL, ai_free_used = 0,
			channel_bonus_at = NULL, avatar_bonus_at = NULL,
			photo_file_id = NULL, name_edited_at = NULL, profile_edited_at = NULL,
			last_notified_at = NULL, notify_sent_on = NULL, notify_count_on = 0,
			notify_dismissed_at = NULL,
			status = 'joined', partner_user_id = NULL,
			blocked_at = NULL, blocked_reason = NULL
		WHERE bot_id = $1 AND user_id = $2`,
		botID, userID)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}
	// likes in either direction, plus any half-finished chat bookkeeping
	if _, err := tx.Exec(ctx, `
		DELETE FROM dating_likes WHERE bot_id = $1 AND (from_user_id = $2 OR to_user_id = $2)`,
		botID, userID); err != nil {
		return false, err
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM dating_pending_rates WHERE bot_id = $1 AND (user_id = $2 OR partner_user_id = $2)`,
		botID, userID); err != nil {
		return false, err
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM dating_search_requests WHERE bot_id = $1 AND user_id = $2`,
		botID, userID); err != nil {
		return false, err
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM dating_msg_refs WHERE bot_id = $1 AND (viewer_user_id = $2 OR partner_user_id = $2)`,
		botID, userID); err != nil {
		return false, err
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM dating_block_pairs WHERE bot_id = $1 AND (blocker_id = $2 OR blocked_id = $2)`,
		botID, userID); err != nil {
		return false, err
	}
	// free the partner's slot if this user was mid-chat
	if _, err := tx.Exec(ctx, `
		UPDATE dating_profiles SET status = 'joined', partner_user_id = NULL
		WHERE bot_id = $1 AND partner_user_id = $2`, botID, userID); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}

// SetBlocked blocks or unblocks a user. Blocking also pulls them out of any
// chat or search so a partner is not left waiting on a silent participant.
func (r *DatingRepo) SetBlocked(ctx context.Context, botID uuid.UUID, userID int64, blocked bool, reason string) error {
	if blocked {
		tag, err := r.db.Exec(ctx, `
			UPDATE dating_profiles SET
				blocked_at = now(), blocked_reason = NULLIF($3, ''),
				status = 'joined', partner_user_id = NULL
			WHERE bot_id = $1 AND user_id = $2`,
			botID, userID, reason)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return fmt.Errorf("dating profile not found")
		}
		_, err = r.db.Exec(ctx, `
			UPDATE dating_profiles SET status = 'joined', partner_user_id = NULL
			WHERE bot_id = $1 AND partner_user_id = $2`, botID, userID)
		return err
	}
	tag, err := r.db.Exec(ctx, `
		UPDATE dating_profiles SET blocked_at = NULL, blocked_reason = NULL
		WHERE bot_id = $1 AND user_id = $2`, botID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("dating profile not found")
	}
	return nil
}

// IsAdminBlocked reports whether a user is currently blocked by an admin.
func (r *DatingRepo) IsAdminBlocked(ctx context.Context, botID uuid.UUID, userID int64) (bool, error) {
	var blocked bool
	err := r.db.QueryRow(ctx, `
		SELECT blocked_at IS NOT NULL FROM dating_profiles
		WHERE bot_id = $1 AND user_id = $2`, botID, userID).Scan(&blocked)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return blocked, err
}

// StuckRow is a profile that has been searching or chatting longer than the
// configured thresholds allow.
type StuckRow struct {
	UserID       int64     `json:"user_id"`
	TelegramID   int64     `json:"telegram_id"`
	Name         *string   `json:"name,omitempty"`
	Username     *string   `json:"username,omitempty"`
	Status       string    `json:"status"`
	PartnerID    *int64    `json:"partner_user_id,omitempty"`
	LastSeenAt   time.Time `json:"last_seen_at"`
	StuckMinutes int       `json:"stuck_minutes"`
}

// StuckUsers lists profiles stuck in searching/chatting past the thresholds.
func (r *DatingRepo) StuckUsers(ctx context.Context, botID uuid.UUID, searchingMin, chattingMin int) ([]StuckRow, error) {
	rows, err := r.db.Query(ctx, `
		SELECT p.user_id, p.telegram_id, p.name, u.username, p.status, p.partner_user_id,
		       p.last_seen_at,
		       (EXTRACT(EPOCH FROM (now() - p.last_seen_at)) / 60)::int
		FROM dating_profiles p
		JOIN bot_end_users u ON u.bot_id = p.bot_id AND u.id = p.user_id
		WHERE p.bot_id = $1 AND NOT p.is_ghost AND p.blocked_at IS NULL AND (
			(p.status = 'searching' AND p.last_seen_at < now() - ($2::int * interval '1 minute'))
			OR (p.status = 'chatting' AND p.last_seen_at < now() - ($3::int * interval '1 minute'))
		)
		ORDER BY p.last_seen_at`,
		botID, searchingMin, chattingMin)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StuckRow
	for rows.Next() {
		var s StuckRow
		if err := rows.Scan(&s.UserID, &s.TelegramID, &s.Name, &s.Username, &s.Status,
			&s.PartnerID, &s.LastSeenAt, &s.StuckMinutes); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ResetStuck releases every stuck user back to 'joined' and returns how many
// were changed (partners of stuck chatters are released too).
func (r *DatingRepo) ResetStuck(ctx context.Context, botID uuid.UUID, searchingMin, chattingMin int) (int, error) {
	var partners []int64
	rows, err := r.db.Query(ctx, `
		SELECT p.partner_user_id FROM dating_profiles p
		WHERE p.bot_id = $1 AND NOT p.is_ghost AND p.status = 'chatting'
		  AND p.partner_user_id IS NOT NULL
		  AND p.last_seen_at < now() - ($2::int * interval '1 minute')`,
		botID, chattingMin)
	if err != nil {
		return 0, err
	}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		partners = append(partners, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	total := 0
	tag, err := r.db.Exec(ctx, `
		UPDATE dating_profiles SET status = 'joined', partner_user_id = NULL
		WHERE bot_id = $1 AND NOT is_ghost AND (
			(status = 'searching' AND last_seen_at < now() - ($2::int * interval '1 minute'))
			OR (status = 'chatting' AND last_seen_at < now() - ($3::int * interval '1 minute'))
		)`,
		botID, searchingMin, chattingMin)
	if err != nil {
		return 0, err
	}
	total += int(tag.RowsAffected())

	// release partners left waiting on a stuck chatter
	if len(partners) > 0 {
		tag, err := r.db.Exec(ctx, `
			UPDATE dating_profiles SET status = 'joined', partner_user_id = NULL
			WHERE bot_id = $1 AND status = 'chatting' AND user_id = ANY($2)`,
			botID, partners)
		if err != nil {
			return total, err
		}
		total += int(tag.RowsAffected())
	}
	return total, nil
}

// ResetAllChats moves every searching/chatting profile back to 'joined'
// without touching profiles, coins, or history.
func (r *DatingRepo) ResetAllChats(ctx context.Context, botID uuid.UUID) (int, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE dating_profiles SET status = 'joined', partner_user_id = NULL
		WHERE bot_id = $1 AND NOT is_ghost AND status IN ('searching','chatting')`,
		botID)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

// ResetStreak clears a user's daily streak counters.
func (r *DatingRepo) ResetStreak(ctx context.Context, botID uuid.UUID, userID int64) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE dating_profiles SET streak_count = 0, streak_best = 0, streak_last_date = NULL
		WHERE bot_id = $1 AND user_id = $2`, botID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("dating profile not found")
	}
	return nil
}

// UserDetail is the panel forensics view for one end user.
type UserDetail struct {
	UserID           int64        `json:"user_id"`
	TelegramID       int64        `json:"telegram_id"`
	Username         *string      `json:"username,omitempty"`
	FirstName        *string      `json:"first_name,omitempty"`
	LastName         *string      `json:"last_name,omitempty"`
	IsBlocked        bool         `json:"is_blocked"`
	FirstSeenAt      time.Time    `json:"first_seen_at"`
	LastSeenAt       time.Time    `json:"last_seen_at"`
	HasProfile       bool         `json:"has_profile"`
	Name             *string      `json:"name,omitempty"`
	Gender           *string      `json:"gender,omitempty"`
	Age              *int         `json:"age,omitempty"`
	City             *string      `json:"city,omitempty"`
	Status           *string      `json:"status,omitempty"`
	PartnerUserID    *int64       `json:"partner_user_id,omitempty"`
	Coins            *int         `json:"coins,omitempty"`
	Rating           *float64     `json:"rating,omitempty"`
	TotalRatings     *int         `json:"total_ratings,omitempty"`
	StreakCount      *int         `json:"streak_count,omitempty"`
	StreakBest       *int         `json:"streak_best,omitempty"`
	BoostedUntil     *time.Time   `json:"boosted_until,omitempty"`
	ProfileBlockedAt *time.Time   `json:"profile_blocked_at,omitempty"`
	BlockedReason    *string      `json:"blocked_reason,omitempty"`
	LikesSent        int          `json:"likes_sent"`
	LikesReceived    int          `json:"likes_received"`
	Payments         []PaymentRow `json:"payments"`
	RecentEvents     []EventRow   `json:"recent_events"`
}

// PaymentRow is one payment in the user-detail view.
type PaymentRow struct {
	ID          string    `json:"id"`
	AmountRials int64     `json:"amount_rials"`
	Kind        string    `json:"kind"`
	Gateway     string    `json:"gateway"`
	Status      string    `json:"status"`
	Note        *string   `json:"note,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// EventRow is one recent activity entry in the user-detail view.
type EventRow struct {
	ID        int64     `json:"id"`
	Kind      string    `json:"kind"`
	Text      *string   `json:"text,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// UserDetail assembles the panel's per-user forensics view.
func (r *DatingRepo) UserDetail(ctx context.Context, botID uuid.UUID, userID int64) (*UserDetail, error) {
	d := &UserDetail{UserID: userID}
	err := r.db.QueryRow(ctx, `
		SELECT u.telegram_id, u.username, u.first_name, u.last_name, u.is_blocked,
		       u.first_seen_at, u.last_seen_at,
		       p.user_id IS NOT NULL,
		       p.name, p.gender, p.age, p.city, p.status, p.partner_user_id,
		       p.coins, p.rating, p.total_ratings, p.streak_count, p.streak_best,
		       p.boosted_until, p.blocked_at, p.blocked_reason
		FROM bot_end_users u
		LEFT JOIN dating_profiles p ON p.bot_id = u.bot_id AND p.user_id = u.id
		WHERE u.bot_id = $1 AND u.id = $2`,
		botID, userID).Scan(
		&d.TelegramID, &d.Username, &d.FirstName, &d.LastName, &d.IsBlocked,
		&d.FirstSeenAt, &d.LastSeenAt,
		&d.HasProfile,
		&d.Name, &d.Gender, &d.Age, &d.City, &d.Status, &d.PartnerUserID,
		&d.Coins, &d.Rating, &d.TotalRatings, &d.StreakCount, &d.StreakBest,
		&d.BoostedUntil, &d.ProfileBlockedAt, &d.BlockedReason)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("user not found")
	}
	if err != nil {
		return nil, err
	}

	if err := r.db.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE from_user_id = $2),
		       count(*) FILTER (WHERE to_user_id = $2)
		FROM dating_likes WHERE bot_id = $1 AND kind <> 'pass'`,
		botID, userID).Scan(&d.LikesSent, &d.LikesReceived); err != nil {
		return nil, err
	}

	rows, err := r.db.Query(ctx, `
		SELECT id::text, amount_rials, kind, gateway, status, note, created_at
		FROM payments WHERE bot_id = $1 AND user_id = $2
		ORDER BY created_at DESC LIMIT 10`, botID, userID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var p PaymentRow
		if err := rows.Scan(&p.ID, &p.AmountRials, &p.Kind, &p.Gateway, &p.Status, &p.Note, &p.CreatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		d.Payments = append(d.Payments, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	evRows, err := r.db.Query(ctx, `
		SELECT id, kind, text, created_at FROM bot_user_events
		WHERE bot_id = $1 AND user_id = $2
		ORDER BY created_at DESC LIMIT 30`, botID, userID)
	if err != nil {
		return nil, err
	}
	defer evRows.Close()
	for evRows.Next() {
		var e EventRow
		if err := evRows.Scan(&e.ID, &e.Kind, &e.Text, &e.CreatedAt); err != nil {
			return nil, err
		}
		d.RecentEvents = append(d.RecentEvents, e)
	}
	if d.Payments == nil {
		d.Payments = []PaymentRow{}
	}
	if d.RecentEvents == nil {
		d.RecentEvents = []EventRow{}
	}
	return d, evRows.Err()
}

// --- chat surveillance (chat viewer / recent messages) ---

// ActiveChat is a live dating conversation, deduplicated by only keeping the
// row whose user_id is lower than its partner's.
type ActiveChat struct {
	UserAID    int64     `json:"user_a_id"`
	TelegramA  int64     `json:"telegram_a_id"`
	NameA      *string   `json:"name_a,omitempty"`
	UsernameA  *string   `json:"username_a,omitempty"`
	UserBID    int64     `json:"user_b_id"`
	TelegramB  int64     `json:"telegram_b_id"`
	NameB      *string   `json:"name_b,omitempty"`
	UsernameB  *string   `json:"username_b,omitempty"`
	LastSeenAt time.Time `json:"last_seen_at"`
	Status     string    `json:"status"`
	IsGhostA   bool      `json:"is_ghost_a"`
	IsGhostB   bool      `json:"is_ghost_b"`
}

// ActiveChats lists chatting pairs for the panel.
func (r *DatingRepo) ActiveChats(ctx context.Context, botID uuid.UUID) ([]ActiveChat, error) {
	rows, err := r.db.Query(ctx, `
		SELECT pa.user_id, pa.telegram_id, na.name, ua.username,
		       pb.user_id, pb.telegram_id, nb.name, ub.username,
		       GREATEST(pa.last_seen_at, pb.last_seen_at), pa.status,
		       pa.is_ghost, pb.is_ghost
		FROM dating_profiles pa
		JOIN dating_profiles pb ON pb.bot_id = pa.bot_id AND pb.partner_user_id = pa.user_id
		JOIN bot_end_users ua ON ua.bot_id = pa.bot_id AND ua.id = pa.user_id
		JOIN bot_end_users ub ON ub.bot_id = pa.bot_id AND ub.id = pb.user_id
		LEFT JOIN dating_profiles na ON na.bot_id = pa.bot_id AND na.user_id = pa.user_id
		LEFT JOIN dating_profiles nb ON nb.bot_id = pa.bot_id AND nb.user_id = pb.user_id
		WHERE pa.bot_id = $1 AND pa.status = 'chatting'
		  AND pa.user_id < pb.user_id
		  AND pa.partner_user_id = pb.user_id
		  AND pb.partner_user_id = pa.user_id
		ORDER BY GREATEST(pa.last_seen_at, pb.last_seen_at) DESC`,
		botID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ActiveChat
	for rows.Next() {
		var c ActiveChat
		if err := rows.Scan(&c.UserAID, &c.TelegramA, &c.NameA, &c.UsernameA,
			&c.UserBID, &c.TelegramB, &c.NameB, &c.UsernameB,
			&c.LastSeenAt, &c.Status, &c.IsGhostA, &c.IsGhostB); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// TranscriptEntry is one message in a reconstructed chat transcript. Only
// inbound text is persisted (SendTo never logs), so the transcript is built
// from the user_events rows tagged with scene=chat written by the engine,
// split across both participants.
type TranscriptEntry struct {
	EventID    int64           `json:"event_id"`
	UserID     int64           `json:"user_id"`
	TelegramID int64           `json:"telegram_id"`
	Name       *string         `json:"name,omitempty"`
	IsUserA    bool            `json:"is_user_a"`
	Kind       string          `json:"kind"`
	Text       *string         `json:"text,omitempty"`
	Meta       json.RawMessage `json:"meta,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
}

// ChatTranscript returns the recorded messages of one conversation, ordered
// oldest first. userA must be the lower user_id of the pair.
func (r *DatingRepo) ChatTranscript(ctx context.Context, botID uuid.UUID, userA, userB int64) ([]TranscriptEntry, error) {
	rows, err := r.db.Query(ctx, `
		SELECT e.id, e.user_id, u.telegram_id, u.first_name, e.user_id = $2,
		       e.kind, e.text, COALESCE(e.meta, '{}'::jsonb), e.created_at
		FROM bot_user_events e
		JOIN bot_end_users u ON u.bot_id = e.bot_id AND u.id = e.user_id
		WHERE e.bot_id = $1
		  AND e.user_id IN ($2, $3)
		  AND e.kind IN ('inbound_message')
		  AND (e.meta->>'scene' = 'chat')
		ORDER BY e.created_at`,
		botID, userA, userB)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TranscriptEntry
	for rows.Next() {
		var t TranscriptEntry
		if err := rows.Scan(&t.EventID, &t.UserID, &t.TelegramID, &t.Name, &t.IsUserA,
			&t.Kind, &t.Text, &t.Meta, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ChatPreview is one recent chat-scene message for the global feed. Only
// user-typed text is persisted (the relay itself is never logged), so every
// row is a message one participant typed; the other side received it.
type ChatPreview struct {
	EventID       int64     `json:"event_id"`
	UserID        int64     `json:"user_id"`
	TelegramID    int64     `json:"telegram_id"`
	Name          *string   `json:"name,omitempty"`
	Username      *string   `json:"username,omitempty"`
	PartnerID     *int64    `json:"partner_user_id,omitempty"`
	PartnerName   *string   `json:"partner_name,omitempty"`
	PartnerUser   *string   `json:"partner_username,omitempty"`
	Text          string    `json:"text"`
	Kind          string    `json:"kind"`
	IsOutbound    bool      `json:"is_outbound"`
	CreatedAt     time.Time `json:"created_at"`
}

// RecentChatMessages is the global recent-message feed covering every chat
// scene message, newest first. limit is capped at 200.
func (r *DatingRepo) RecentChatMessages(ctx context.Context, botID uuid.UUID, limit int) ([]ChatPreview, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := r.db.Query(ctx, `
		SELECT e.id, e.user_id, u.telegram_id, dp.name, u.username,
		       dp.partner_user_id, pp.name, pu.username,
		       COALESCE(e.text, ''), e.kind,
		       e.kind = 'outbound_message',
		       e.created_at
		FROM bot_user_events e
		JOIN bot_end_users u ON u.bot_id = e.bot_id AND u.id = e.user_id
		LEFT JOIN dating_profiles dp ON dp.bot_id = e.bot_id AND dp.user_id = e.user_id
		LEFT JOIN dating_profiles pp ON pp.bot_id = e.bot_id AND pp.user_id = dp.partner_user_id
		LEFT JOIN bot_end_users pu ON pu.bot_id = e.bot_id AND pu.id = dp.partner_user_id
		WHERE e.bot_id = $1
		  AND (e.meta->>'scene' = 'chat')
		ORDER BY e.created_at DESC
		LIMIT $2`,
		botID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ChatPreview
	for rows.Next() {
		var m ChatPreview
		if err := rows.Scan(&m.EventID, &m.UserID, &m.TelegramID, &m.Name, &m.Username,
			&m.PartnerID, &m.PartnerName, &m.PartnerUser,
			&m.Text, &m.Kind, &m.IsOutbound, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// RecordOutboundForPartner logs a forwarded chat message on the partner's
// side so the panel feed shows both directions of the conversation.
func (r *DatingRepo) RecordOutboundForPartner(ctx context.Context, botID uuid.UUID, partnerUserID int64, text string) error {
	if len(text) > 1000 {
		text = text[:1000]
	}
	_, err := r.db.Exec(ctx, `
		INSERT INTO bot_user_events (bot_id, user_id, kind, text, meta)
		VALUES ($1, $2, 'outbound_message', $3, '{"scene":"chat"}'::jsonb)`,
		botID, partnerUserID, text)
	return err
}
