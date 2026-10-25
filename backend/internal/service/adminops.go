package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"sadbot/internal/domain"
	"sadbot/internal/repository/postgres"
	"sadbot/internal/telegram"

	"github.com/google/uuid"
)

// AdminOpsService backs the panel's per-user admin actions (coins, reset,
// block, streak, direct message, stuck scan). It is panel-only: there are no
// Telegram admin commands, so this service never receives updates from a bot.
type AdminOpsService struct {
	dating       *postgres.DatingRepo
	users        *postgres.BotUserRepo
	audit        *postgres.AuditRepo
	configs      *postgres.ConfigRepo
	notify       func(ctx context.Context, botID uuid.UUID, telegramID int64, text string) error
	searchingMin int
	chattingMin  int
}

// NewAdminOpsService wires the panel admin operations. notify may be nil when
// the deployment cannot reach a running bot; sending then reports an error.
func NewAdminOpsService(
	dating *postgres.DatingRepo,
	users *postgres.BotUserRepo,
	audit *postgres.AuditRepo,
	configs *postgres.ConfigRepo,
	notify func(ctx context.Context, botID uuid.UUID, telegramID int64, text string) error,
) *AdminOpsService {
	return &AdminOpsService{
		dating: dating, users: users, audit: audit, configs: configs, notify: notify,
		searchingMin: 5, chattingMin: 60,
	}
}

// SetStuckThresholds overrides the defaults used by the stuck-user scan.
func (s *AdminOpsService) SetStuckThresholds(searchingMin, chattingMin int) {
	if searchingMin > 0 {
		s.searchingMin = searchingMin
	}
	if chattingMin > 0 {
		s.chattingMin = chattingMin
	}
}

func (s *AdminOpsService) log(ctx context.Context, actor uuid.UUID, action, botID string, userID int64, meta map[string]any) {
	if s.audit == nil {
		return
	}
	botUUID, err := uuid.Parse(botID)
	if err != nil {
		return
	}
	target := strconv.FormatInt(userID, 10)
	if meta == nil {
		meta = map[string]any{}
	}
	meta["user_id"] = userID
	_ = s.audit.Insert(ctx, &actor, action, "bot_user", target, &botUUID, meta, nil)
}

// UserDetail returns the panel's forensics view for one end user.
func (s *AdminOpsService) UserDetail(ctx context.Context, botID uuid.UUID, userID int64) (*postgres.UserDetail, error) {
	return s.dating.UserDetail(ctx, botID, userID)
}

// AdjustCoins credits (or debits) coins and reports the new balance.
func (s *AdminOpsService) AdjustCoins(ctx context.Context, actor, botID uuid.UUID, userID int64, delta int) (int, error) {
	if delta == 0 {
		return 0, errors.New("مقدار سکه صفر است")
	}
	coins, err := s.dating.AdjustCoins(ctx, botID, userID, delta)
	if err != nil {
		return 0, err
	}
	s.log(ctx, actor, "bot_user.adjust_coins", botID.String(), userID, map[string]any{"delta": delta, "coins": coins})
	return coins, nil
}

// ResetUser wipes the user's dating content and conversation state but keeps
// the profile row, so no re-onboarding is needed.
func (s *AdminOpsService) ResetUser(ctx context.Context, actor, botID uuid.UUID, userID int64) error {
	ok, err := s.dating.ResetProfile(ctx, botID, userID)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("پروفایل دوستیابی این کاربر یافت نشد")
	}
	if err := s.users.ResetState(ctx, botID, userID); err != nil {
		return err
	}
	s.log(ctx, actor, "bot_user.reset", botID.String(), userID, nil)
	return nil
}

// SetBlocked blocks or unblocks a user; blocking pulls them out of any chat.
func (s *AdminOpsService) SetBlocked(ctx context.Context, actor, botID uuid.UUID, userID int64, blocked bool, reason string) error {
	if err := s.dating.SetBlocked(ctx, botID, userID, blocked, strings.TrimSpace(reason)); err != nil {
		return err
	}
	s.log(ctx, actor, "bot_user.set_blocked", botID.String(), userID, map[string]any{"blocked": blocked})
	return nil
}

// ResetStreak clears a user's daily streak.
func (s *AdminOpsService) ResetStreak(ctx context.Context, actor, botID uuid.UUID, userID int64) error {
	if err := s.dating.ResetStreak(ctx, botID, userID); err != nil {
		return err
	}
	s.log(ctx, actor, "bot_user.reset_streak", botID.String(), userID, nil)
	return nil
}

// StuckUsers lists users stuck in searching/chatting past the thresholds.
func (s *AdminOpsService) StuckUsers(ctx context.Context, botID uuid.UUID) ([]postgres.StuckRow, error) {
	rows, err := s.dating.StuckUsers(ctx, botID, s.searchingMin, s.chattingMin)
	if err != nil {
		return nil, err
	}
	if rows == nil {
		rows = []postgres.StuckRow{}
	}
	return rows, nil
}

// ResetStuck releases every stuck user (and their waiting partners).
func (s *AdminOpsService) ResetStuck(ctx context.Context, actor, botID uuid.UUID) (int, error) {
	n, err := s.dating.ResetStuck(ctx, botID, s.searchingMin, s.chattingMin)
	if err != nil {
		return 0, err
	}
	s.log(ctx, actor, "bot_user.reset_stuck", botID.String(), 0, map[string]any{"count": n})
	return n, nil
}

// ResetAllChats moves every searching/chatting user back to 'joined'.
func (s *AdminOpsService) ResetAllChats(ctx context.Context, actor, botID uuid.UUID) (int, error) {
	n, err := s.dating.ResetAllChats(ctx, botID)
	if err != nil {
		return 0, err
	}
	s.log(ctx, actor, "bot_user.reset_all", botID.String(), 0, map[string]any{"count": n})
	return n, nil
}

// --- chat surveillance ---

// ActiveChats lists the live dating conversations of a bot.
func (s *AdminOpsService) ActiveChats(ctx context.Context, botID uuid.UUID) ([]postgres.ActiveChat, error) {
	rows, err := s.dating.ActiveChats(ctx, botID)
	if err != nil {
		return nil, err
	}
	if rows == nil {
		rows = []postgres.ActiveChat{}
	}
	return rows, nil
}

// ChatTranscript returns the recorded messages of one conversation.
// userA must be the lower user_id of the pair; callers normalize.
func (s *AdminOpsService) ChatTranscript(ctx context.Context, botID uuid.UUID, userA, userB int64) ([]postgres.TranscriptEntry, error) {
	if userA == userB {
		return nil, errors.New("ایدی دو کاربر باید متفاوت باشد")
	}
	if userA > userB {
		userA, userB = userB, userA
	}
	rows, err := s.dating.ChatTranscript(ctx, botID, userA, userB)
	if err != nil {
		return nil, err
	}
	if rows == nil {
		rows = []postgres.TranscriptEntry{}
	}
	return rows, nil
}

// RecentChatMessages returns the global feed of chat-scene messages.
func (s *AdminOpsService) RecentChatMessages(ctx context.Context, botID uuid.UUID, limit int) ([]postgres.ChatPreview, error) {
	rows, err := s.dating.RecentChatMessages(ctx, botID, limit)
	if err != nil {
		return nil, err
	}
	if rows == nil {
		rows = []postgres.ChatPreview{}
	}
	return rows, nil
}

// DirectMessage sends a panel-authored message to one user through the bot.
func (s *AdminOpsService) DirectMessage(ctx context.Context, actor, botID uuid.UUID, userID int64, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return errors.New("متن پیام خالی است")
	}
	if s.notify == nil {
		return errors.New("ارسال پیام در دسترس نیست")
	}
	u, err := s.users.GetByID(ctx, botID, userID)
	if err != nil || u == nil {
		return fmt.Errorf("کاربر یافت نشد")
	}
	if err := s.notify(ctx, botID, u.TelegramID, text); err != nil {
		return fmt.Errorf("ارسال ناموفق (بات در حال اجرا نیست؟): %w", err)
	}
	s.log(ctx, actor, "bot_user.direct_message", botID.String(), userID, map[string]any{"len": len(text)})
	return nil
}

// --- AI harness ---

// AIConversation is one user's AI companion session as shown in the panel.
type AIConversation struct {
	UserID     int64      `json:"user_id"`
	TelegramID int64      `json:"telegram_id"`
	Username   *string    `json:"username,omitempty"`
	FirstName  *string    `json:"first_name,omitempty"`
	LastSeenAt time.Time  `json:"last_seen_at"`
	IsBlocked  bool       `json:"is_blocked"`
	IsGhost    bool       `json:"is_ghost"`
	AIFreeUsed int        `json:"ai_free_used"`
	Messages   int        `json:"messages"`
	LastMessage *string   `json:"last_message,omitempty"`
	LastAt     *time.Time `json:"last_at,omitempty"`
}

func rawMap(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return nil
	}
	return m
}

// getBoolFlag reads a 0/1 numeric flag from the bot's stored prices
// (the runtime only merges prices into BotContext, so numeric bools like
// paywall_enabled / ai_enabled live there).
func (s *AdminOpsService) getBoolFlag(ctx context.Context, botID uuid.UUID, key string, def bool) bool {
	if s.configs == nil {
		return def
	}
	cfg, err := s.configs.Get(ctx, botID)
	if err != nil || cfg == nil || len(cfg.Prices) == 0 {
		return def
	}
	prices := rawMap(cfg.Prices)
	if prices == nil {
		return def
	}
	switch v := prices[key].(type) {
	case float64:
		return v != 0
	case int:
		return v != 0
	case bool:
		return v
	default:
		return def
	}
}

// aiHistory reads the bounded ai_hist array from a user's state data.
func aiHistory(state json.RawMessage) []map[string]any {
	root := rawMap(state)
	if root == nil {
		return nil
	}
	data, _ := root["data"].(map[string]any)
	if data == nil {
		return nil
	}
	arr, _ := data["ai_hist"].([]any)
	var hist []map[string]any
	for _, item := range arr {
		if m, ok := item.(map[string]any); ok {
			hist = append(hist, m)
		}
	}
	return hist
}

// AIHarness is the full panel view of the AI companion: the on/off switch and
// the users currently in the AI scene.
type AIHarness struct {
	Enabled       bool              `json:"enabled"`
	Conversations []AIConversation `json:"conversations"`
}

// AIHarness lists users in the AI scene with their message counts.
func (s *AdminOpsService) AIHarness(ctx context.Context, botID uuid.UUID) (*AIHarness, error) {
	users, err := s.users.ListByScene(ctx, botID, "ai", 50)
	if err != nil {
		return nil, err
	}
	out := &AIHarness{
		Enabled:       s.getBoolFlag(ctx, botID, "ai_enabled", true),
		Conversations: make([]AIConversation, 0, len(users)),
	}
	for _, u := range users {
		c := AIConversation{
			UserID: u.ID, TelegramID: u.TelegramID, Username: u.Username,
			FirstName: u.FirstName, LastSeenAt: u.LastSeenAt, IsBlocked: u.IsBlocked,
		}
		if p, err := s.dating.GetProfile(ctx, botID, u.ID); err == nil && p != nil {
			c.IsGhost = p.IsGhost
			c.AIFreeUsed = p.AIFreeUsed
		}
		if hist := aiHistory(u.State); len(hist) > 0 {
			c.Messages = len(hist)
			last := hist[len(hist)-1]
			if m, ok := last["content"].(string); ok && m != "" {
				c.LastMessage = &m
			}
			if tm, ok := last["at"].(float64); ok && tm > 0 {
				t := time.Unix(int64(tm), 0).UTC()
				c.LastAt = &t
			}
		}
		out.Conversations = append(out.Conversations, c)
	}
	return out, nil
}

// AIUserDetail returns one user's AI history plus profile counters.
func (s *AdminOpsService) AIUserDetail(ctx context.Context, botID uuid.UUID, userID int64) (map[string]any, error) {
	u, err := s.users.GetByID(ctx, botID, userID)
	if err != nil || u == nil {
		return nil, errors.New("کاربر یافت نشد")
	}
	res := map[string]any{
		"user_id":      u.ID,
		"telegram_id":  u.TelegramID,
		"username":     u.Username,
		"first_name":   u.FirstName,
		"last_seen_at": u.LastSeenAt,
		"history":      []map[string]any{},
	}
	if p, err := s.dating.GetProfile(ctx, botID, userID); err == nil && p != nil {
		res["ai_free_used"] = p.AIFreeUsed
		res["is_ghost"] = p.IsGhost
	}
	if hist := aiHistory(u.State); len(hist) > 0 {
		res["history"] = hist
	}
	return res, nil
}

// SetAIFeature toggles the ai_enabled numeric flag in prices (the runtime
// merges prices into BotContext; feature_flags are not exposed to templates).
// Key is validated so the endpoint cannot write arbitrary config keys.
func (s *AdminOpsService) SetAIFeature(ctx context.Context, actor *domain.Admin, botID uuid.UUID, key string, enabled bool) (*domain.BotConfig, error) {
	if s.configs == nil {
		return nil, errors.New("پیکربندی در دسترس نیست")
	}
	if key != "ai_enabled" {
		return nil, errors.New("کلید نامعتبر است")
	}
	cfg, err := s.configs.Get(ctx, botID)
	if err != nil || cfg == nil {
		return nil, errors.New("تنظیمات بات یافت نشد")
	}
	prices := map[string]any{}
	if m := rawMap(cfg.Prices); m != nil {
		prices = m
	}
	if enabled {
		prices[key] = 1
	} else {
		prices[key] = 0
	}
	raw, err := json.Marshal(prices)
	if err != nil {
		return nil, err
	}
	updated := &domain.BotConfig{
		BotID: botID, Texts: cfg.Texts, Menus: cfg.Menus,
		FeatureFlags: cfg.FeatureFlags, Prices: raw, Version: cfg.Version,
	}
	saved, err := s.configs.Upsert(ctx, updated, &actor.ID)
	if err != nil {
		var conflict domain.ErrVersionConflict
		if errors.As(err, &conflict) {
			return nil, errors.New("تنظیمات همزمان توسط کاربر دیگری ویرایش شده؛ صفحه را تازه کنید")
		}
		return nil, err
	}
	s.log(ctx, actor.ID, "bot.ai_feature", botID.String(), 0, map[string]any{"key": key, "enabled": enabled})
	return saved, nil
}

// --- telegram-block + blocked-relations scanners ---

// ScanResult summarises one telegram-block probe run.
type ScanResult struct {
	Scanned   int `json:"scanned"`
	Blocked   int `json:"blocked"`
	Reachable int `json:"reachable"`
	Failed    int `json:"failed"`
}

// defaultBlockScanText is the re-engagement ping used to probe inactivity.
const defaultBlockScanText = "چند نفر منتظرتن — برگردی! 💬"

// ScanTelegramBlocks probes users inactive for `days` days by sending them a
// re-engagement message through the running bot. A 403 (user blocked the bot)
// marks the user is_blocked, which already excludes them from broadcasts.
// kpkchat parity: GET/POST /scan/telegram-blocks.
func (s *AdminOpsService) ScanTelegramBlocks(ctx context.Context, actor *domain.Admin, botID uuid.UUID, days int) (*ScanResult, error) {
	if s.notify == nil {
		return nil, errors.New("ارسال پیام در دسترس نیست")
	}
	if days < 1 || days > 365 {
		days = 7
	}
	candidates, err := s.users.ListInactive(ctx, botID, days, 100)
	if err != nil {
		return nil, err
	}
	res := &ScanResult{Scanned: len(candidates)}
	for _, u := range candidates {
		if err := s.notify(ctx, botID, u.TelegramID, defaultBlockScanText); err != nil {
			var apiErr *telegram.APIError
			if errors.As(err, &apiErr) && apiErr.IsForbidden() {
				if merr := s.users.MarkBlockedByBot(ctx, botID, u.ID); merr == nil {
					res.Blocked++
				} else {
					res.Failed++
				}
			} else {
				res.Failed++
			}
		} else {
			res.Reachable++
		}
	}
	s.log(ctx, actor.ID, "bot.telegram_block_scan", botID.String(), 0, map[string]any{
		"scanned": res.Scanned, "blocked": res.Blocked, "reachable": res.Reachable, "failed": res.Failed, "days": days,
	})
	return res, nil
}

// BlockPairs lists active user-level block relationships (who blocked whom).
func (s *AdminOpsService) BlockPairs(ctx context.Context, botID uuid.UUID) ([]postgres.BlockPairRow, error) {
	rows, err := s.dating.BlockPairs(ctx, botID, 200)
	if err != nil {
		return nil, err
	}
	if rows == nil {
		rows = []postgres.BlockPairRow{}
	}
	return rows, nil
}
