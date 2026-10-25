package dating

import (
	"context"
	"fmt"
	"time"

	"sadbot/internal/domain"
	"sadbot/internal/repository/postgres"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Store provides dating-template persistence built on DatingRepo.
type Store struct {
	dating *postgres.DatingRepo
	users  *postgres.BotUserRepo
}

func NewStore(dating *postgres.DatingRepo, users *postgres.BotUserRepo) *Store {
	return &Store{dating: dating, users: users}
}

// SetScene updates the user's engine scene (used by matcher/sweeper/force-end).
func (s *Store) SetScene(ctx context.Context, botID string, userID int64, scene string) error {
	id, err := uuid.Parse(botID)
	if err != nil {
		return err
	}
	return s.users.SetStateScene(ctx, id, userID, scene)
}

func (s *Store) Repo() *postgres.DatingRepo { return s.dating }

// EnsureProfile creates the dating profile row if missing.
func (s *Store) EnsureProfile(ctx context.Context, botID uuid.UUID, userID, telegramID int64) (*domain.DatingProfile, error) {
	return s.dating.EnsureProfile(ctx, botID, userID, telegramID)
}

func (s *Store) Profile(ctx context.Context, botID uuid.UUID, userID int64) (*domain.DatingProfile, error) {
	return s.dating.GetProfile(ctx, botID, userID)
}

func (s *Store) ProfileByTelegram(ctx context.Context, botID uuid.UUID, telegramID int64) (*domain.DatingProfile, error) {
	return s.dating.GetProfileByTelegramID(ctx, botID, telegramID)
}

// IsProfileComplete reports whether onboarding finished (kpkchat fields).
func IsProfileComplete(p *domain.DatingProfile) bool {
	return p.Name != nil && *p.Name != "" &&
		p.Gender != nil && p.Age != nil &&
		p.Province != nil && p.City != nil
}

// StartSearch: CAS joined→searching + enqueue. cost is recorded in the
// request so cancel/timeout can refund exactly what was charged.
func (s *Store) StartSearch(ctx context.Context, botID uuid.UUID, userID int64, genderFilter string, cost int) error {
	return s.StartSearchWithFilters(ctx, botID, userID, map[string]any{"gender": genderFilter, "cost": cost})
}

// StartSearchWithFilters joins the queue with arbitrary filters
// (gender/city/age/timeout_sec for advanced search).
func (s *Store) StartSearchWithFilters(ctx context.Context, botID uuid.UUID, userID int64, filters map[string]any) error {
	ok, err := s.dating.SetStatus(ctx, botID, userID, domain.DatingJoined, domain.DatingSearching)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("user not in joined state")
	}
	return s.dating.EnqueueSearch(ctx, botID, userID, filters)
}

// CancelSearch leaves the queue and refunds the recorded search cost.
func (s *Store) CancelSearch(ctx context.Context, botID uuid.UUID, userID int64) error {
	cost, _ := s.dating.SearchCost(ctx, botID, userID)
	if err := s.dating.LeaveSearch(ctx, botID, userID); err != nil {
		return err
	}
	if cost > 0 {
		_, _ = s.dating.AddCoins(ctx, botID, userID, cost)
	}
	return nil
}

func (s *Store) IsSearching(ctx context.Context, botID uuid.UUID, userID int64) (bool, error) {
	p, err := s.dating.GetProfile(ctx, botID, userID)
	if err != nil {
		return false, err
	}
	return p.Status == domain.DatingSearching, nil
}

// BeginChat is the match completion: CAS both to chatting with partner.
func (s *Store) BeginChat(ctx context.Context, botID uuid.UUID, a, b int64) (bool, error) {
	okA, err := s.dating.BeginChatting(ctx, botID, a, b)
	if err != nil {
		return false, err
	}
	if !okA {
		return false, nil
	}
	okB, err := s.dating.BeginChatting(ctx, botID, b, a)
	if err != nil {
		// rollback A
		_, _ = s.dating.EndChat(ctx, botID, a)
		return false, err
	}
	if !okB {
		_, _ = s.dating.EndChat(ctx, botID, a)
		return false, nil
	}
	_ = s.dating.ClearPendingRate(ctx, botID, a)
	_ = s.dating.ClearPendingRate(ctx, botID, b)
	return true, nil
}

// EndChatForBoth force-ends a human chat and registers pending ratings.
func (s *Store) EndChatForBoth(ctx context.Context, botID uuid.UUID, a, b int64) error {
	if err := s.dating.SetPendingRate(ctx, botID, a, b); err != nil {
		return err
	}
	if err := s.dating.SetPendingRate(ctx, botID, b, a); err != nil {
		return err
	}
	if _, err := s.dating.EndChat(ctx, botID, a); err != nil {
		return err
	}
	_, err := s.dating.EndChat(ctx, botID, b)
	return err
}

func (s *Store) ApplyRating(ctx context.Context, botID uuid.UUID, fromUserID, targetUserID int64, score int) error {
	if err := s.dating.ApplyRating(ctx, botID, targetUserID, score); err != nil {
		return err
	}
	return s.dating.ClearPendingRate(ctx, botID, fromUserID)
}

func (s *Store) BlockAndEnd(ctx context.Context, botID uuid.UUID, blockerID, blockedID int64) error {
	if err := s.dating.Block(ctx, botID, blockerID, blockedID, 30*24*time.Hour); err != nil {
		return err
	}
	// blocker: no rating; clear any pending
	_ = s.dating.ClearPendingRate(ctx, botID, blockerID)
	// blocked partner: must rate
	_ = s.dating.SetPendingRate(ctx, botID, blockedID, blockerID)
	if _, err := s.dating.EndChat(ctx, botID, blockerID); err != nil {
		return err
	}
	_, err := s.dating.EndChat(ctx, botID, blockedID)
	return err
}

func (s *Store) IsBlocked(ctx context.Context, botID uuid.UUID, a, b int64) (bool, error) {
	return s.dating.IsBlocked(ctx, botID, a, b)
}

// --- reply map ---

func (s *Store) BindReply(ctx context.Context, botID uuid.UUID, viewerUserID, viewerMsgID, partnerUserID, partnerMsgID int64) error {
	return s.dating.BindMsgRef(ctx, botID, viewerUserID, viewerMsgID, partnerUserID, partnerMsgID)
}

func (s *Store) ResolveReply(ctx context.Context, botID uuid.UUID, viewerUserID, viewerMsgID int64) (int64, error) {
	ref, err := s.dating.ResolveMsgRef(ctx, botID, viewerUserID, viewerMsgID)
	if err != nil {
		return 0, err
	}
	if ref == nil {
		return 0, nil
	}
	return ref.PartnerMsgID, nil
}

// --- matcher support ---

// Searcher is a searching user joined with their telegram chat id.
type Searcher struct {
	UserID          int64
	TelegramID      int64
	Gender          string
	Province        string
	City            string
	Age             int
	Interests       []string
	Filter          string   // "any" | "male" | "female"
	ProvinceFilter  string   // advanced search: required province ("" = any)
	CityFilter      string   // advanced search: required city ("" = any)
	InterestsFilter []string // advanced search: match ≥1 of these interests (empty = any)
	AgeMin          int      // advanced search: min age (0 = any)
	AgeMax          int      // advanced search: max age (0 = any)
	CreatedAt       time.Time
}

// LoadSearchers returns all searching users of a bot with profile fields + chat ids.
func (s *Store) LoadSearchers(ctx context.Context, botID uuid.UUID) ([]Searcher, error) {
	rows, err := s.dating.Searchers(ctx, botID)
	if err != nil {
		return nil, err
	}
	out := make([]Searcher, 0, len(rows))
	for _, r := range rows {
		sr := Searcher{
			UserID: r.UserID, TelegramID: r.TelegramID,
			Interests: r.Interests, Filter: r.Filter,
			ProvinceFilter: r.ProvinceFilter, CityFilter: r.CityFilter,
			InterestsFilter: r.InterestsFilter,
			AgeMin: r.AgeMin, AgeMax: r.AgeMax,
			CreatedAt: r.CreatedAt,
		}
		if r.Gender != nil {
			sr.Gender = *r.Gender
		}
		if r.Province != nil {
			sr.Province = *r.Province
		}
		if r.City != nil {
			sr.City = *r.City
		}
		if r.Age != nil {
			sr.Age = *r.Age
		}
		out = append(out, sr)
	}
	return out, nil
}

// ClaimMatch atomically CASes both users into chatting (single tx).
func (s *Store) ClaimMatch(ctx context.Context, botID uuid.UUID, a, b int64) (bool, error) {
	tx, err := s.dating.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)

	okA, err := casChatting(ctx, tx, botID, a, b)
	if err != nil || !okA {
		return false, err
	}
	okB, err := casChatting(ctx, tx, botID, b, a)
	if err != nil {
		return false, err
	}
	if !okB {
		return false, nil
	}
	if _, err := tx.Exec(ctx, `DELETE FROM dating_search_requests WHERE bot_id = $1 AND user_id IN ($2,$3)`, botID, a, b); err != nil {
		return false, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM dating_pending_rates WHERE bot_id = $1 AND user_id IN ($2,$3)`, botID, a, b); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}

func casChatting(ctx context.Context, tx pgx.Tx, botID uuid.UUID, userID, partnerID int64) (bool, error) {
	tag, err := tx.Exec(ctx, `
		UPDATE dating_profiles SET status = 'chatting', partner_user_id = $3, last_seen_at = now()
		WHERE bot_id = $1 AND user_id = $2 AND status = 'searching'`,
		botID, userID, partnerID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// ConnectUsers links two users directly (browse mutual like / superlike /
// direct chat): both must be joinable (joined or searching). Single tx.
func (s *Store) ConnectUsers(ctx context.Context, botID uuid.UUID, a, b int64) (bool, error) {
	tx, err := s.dating.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)

	cas := func(userID, partnerID int64) (bool, error) {
		tag, err := tx.Exec(ctx, `
			UPDATE dating_profiles SET status = 'chatting', partner_user_id = $3, last_seen_at = now()
			WHERE bot_id = $1 AND user_id = $2 AND status IN ('joined','searching')`,
			botID, userID, partnerID)
		if err != nil {
			return false, err
		}
		return tag.RowsAffected() > 0, nil
	}
	okA, err := cas(a, b)
	if err != nil || !okA {
		return false, err
	}
	okB, err := cas(b, a)
	if err != nil || !okB {
		return false, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM dating_search_requests WHERE bot_id = $1 AND user_id IN ($2,$3)`, botID, a, b); err != nil {
		return false, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM dating_pending_rates WHERE bot_id = $1 AND user_id IN ($2,$3)`, botID, a, b); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}

// ExpiredSearches returns search requests past their per-request timeout.
func (s *Store) ExpiredSearches(ctx context.Context, botID uuid.UUID) ([]postgres.SearchExpiry, error) {
	return s.dating.ExpiredSearches(ctx, botID)
}
