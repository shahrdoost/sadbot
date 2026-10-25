package dating

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"sadbot/internal/engine"
	"sadbot/internal/telegram"

	"github.com/google/uuid"
)

// Notifier sends a message to a chat on behalf of a bot (injected; testable).
type Notifier func(botID uuid.UUID, chatID int64, text string, kbd json.RawMessage) error

// Matcher runs the global 5s matching loop + search timeout sweep for all
// running dating bots (kpkchat match-queue + search-timer port).
type Matcher struct {
	store    *Store
	bots     BotLister
	notify   Notifier
	interval time.Duration
	pricesOf func(uuid.UUID) map[string]any
	textsOf  func(uuid.UUID) map[string]any
}

// BotLister yields (botID) pairs of running dating bots.
type BotLister interface {
	ListRunningByTemplate(ctx context.Context, templateKey string) ([]uuid.UUID, error)
}

func NewMatcher(store *Store, bots BotLister, notify Notifier) *Matcher {
	return &Matcher{
		store: store, bots: bots, notify: notify,
		interval: 5 * time.Second,
	}
}

// SetPricesFunc injects per-bot merged prices (ghost thresholds etc.).
func (m *Matcher) SetPricesFunc(fn func(uuid.UUID) map[string]any) { m.pricesOf = fn }

// SetTextsFunc injects per-bot merged texts (match notifications etc.).
func (m *Matcher) SetTextsFunc(fn func(uuid.UUID) map[string]any) { m.textsOf = fn }

// text returns an editable text with a hardcoded fallback.
func (m *Matcher) text(botID uuid.UUID, key, def string) string {
	if m.textsOf == nil {
		return def
	}
	if v, ok := m.textsOf(botID)[key].(string); ok && v != "" {
		return v
	}
	return def
}

func (m *Matcher) prices(botID uuid.UUID) map[string]any {
	if m.pricesOf == nil {
		return nil
	}
	return m.pricesOf(botID)
}

// Run loops until ctx is done.
func (m *Matcher) Run(ctx context.Context) {
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.Tick(ctx)
		}
	}
}

func (m *Matcher) Tick(ctx context.Context) {
	botIDs, err := m.bots.ListRunningByTemplate(ctx, "dating")
	if err != nil {
		log.Printf("[matcher] list bots: %v", err)
		return
	}
	for _, botID := range botIDs {
		m.sweepTimeouts(ctx, botID)
		m.matchOnce(ctx, botID)
		m.injectGhosts(ctx, botID)
	}
}

// matchOnce greedily pairs searchers (kpkchat pickBestPair, O(n²)).
func (m *Matcher) matchOnce(ctx context.Context, botID uuid.UUID) {
	searchers, err := m.store.LoadSearchers(ctx, botID)
	if err != nil || len(searchers) < 2 {
		return
	}
	used := make(map[int64]bool, len(searchers))
	for i := range searchers {
		a := searchers[i]
		if used[a.UserID] {
			continue
		}
		bestIdx := -1
		bestScore := -1
		for j := range searchers {
			if i == j {
				continue
			}
			b := searchers[j]
			if used[b.UserID] {
				continue
			}
			if blocked, _ := m.store.IsBlocked(ctx, botID, a.UserID, b.UserID); blocked {
				continue
			}
			if !genderCompatible(a, b) || !filtersCompatible(a, b) {
				continue
			}
			score := similarityScore(a, b)
			if score > bestScore {
				bestScore = score
				bestIdx = j
			}
		}
		if bestIdx < 0 {
			continue
		}
		b := searchers[bestIdx]
		ok, err := m.store.ClaimMatch(ctx, botID, a.UserID, b.UserID)
		if err != nil {
			log.Printf("[matcher] claim: %v", err)
			continue
		}
		if !ok {
			continue
		}
		used[a.UserID] = true
		used[b.UserID] = true
		// engine scene of both → chat
		_ = m.store.SetScene(ctx, botID.String(), a.UserID, "chat")
		_ = m.store.SetScene(ctx, botID.String(), b.UserID, "chat")
		m.notifyMatch(botID, a, b)
	}
}

// similarityScore — kpkchat calculateSimilarityScore weights.
func similarityScore(a, b Searcher) int {
	score := 0
	// gender (both advanced = 10; one-sided = 8; default = 5 — simplified to symmetric)
	if a.Gender != "" && b.Gender != "" && a.Gender != b.Gender {
		score += 10
	} else {
		score += 5
	}
	if a.City != "" && a.City == b.City {
		score += 8
	} else if a.Province != "" && a.Province == b.Province {
		score += 4
	}
	if a.Age > 0 && b.Age > 0 {
		diff := a.Age - b.Age
		if diff < 0 {
			diff = -diff
		}
		switch {
		case diff <= 3:
			score += 5
		case diff <= 5:
			score += 2
		}
	}
	shared := 0
	set := map[string]bool{}
	for _, i := range a.Interests {
		set[i] = true
	}
	for _, i := range b.Interests {
		if set[i] {
			shared++
		}
	}
	score += shared * 3
	return score
}

// genderCompatible: requester filters respected both ways (kpkchat matcher fix).
func genderCompatible(a, b Searcher) bool {
	if a.Filter != "" && a.Filter != "any" && b.Gender != "" && a.Filter != b.Gender {
		return false
	}
	if b.Filter != "" && b.Filter != "any" && a.Gender != "" && b.Filter != a.Gender {
		return false
	}
	return true
}

// filtersCompatible applies advanced-search province/city/interests/age constraints both ways.
func filtersCompatible(a, b Searcher) bool {
	if a.ProvinceFilter != "" && b.Province != a.ProvinceFilter {
		return false
	}
	if b.ProvinceFilter != "" && a.Province != b.ProvinceFilter {
		return false
	}
	if a.CityFilter != "" && b.City != a.CityFilter {
		return false
	}
	if b.CityFilter != "" && a.City != b.CityFilter {
		return false
	}
	if !interestsOverlap(a.InterestsFilter, b.Interests) {
		return false
	}
	if !interestsOverlap(b.InterestsFilter, a.Interests) {
		return false
	}
	if a.AgeMin > 0 && b.Age > 0 && b.Age < a.AgeMin {
		return false
	}
	if a.AgeMax > 0 && b.Age > 0 && b.Age > a.AgeMax {
		return false
	}
	if b.AgeMin > 0 && a.Age > 0 && a.Age < b.AgeMin {
		return false
	}
	if b.AgeMax > 0 && a.Age > 0 && a.Age > b.AgeMax {
		return false
	}
	return true
}

// interestsOverlap: if filter is empty → always match; otherwise the
// candidate must share at least one interest with the filter (OR semantics,
// same as kpkchat).
func interestsOverlap(filter, candidate []string) bool {
	if len(filter) == 0 {
		return true
	}
	set := map[string]bool{}
	for _, c := range candidate {
		set[c] = true
	}
	for _, f := range filter {
		if set[f] {
			return true
		}
	}
	return false
}

func (m *Matcher) notifyMatch(botID uuid.UUID, a, b Searcher) {
	ctx := context.Background()
	text := m.text(botID, "match.search", "وصلت انجام شد!") + humanRules
	kbd := chatKeyboard()
	for _, target := range []Searcher{a, b} {
		if err := m.notify(botID, target.TelegramID, text, kbd); err != nil {
			log.Printf("[matcher] notify %d: %v", target.TelegramID, err)
		}
	}
	_ = ctx
}

// sweepTimeouts cancels searches past their per-request timeout (kpkchat auto-cancel).
func (m *Matcher) sweepTimeouts(ctx context.Context, botID uuid.UUID) {
	expired, err := m.store.ExpiredSearches(ctx, botID)
	if err != nil || len(expired) == 0 {
		return
	}
	for _, e := range expired {
		if err := m.store.CancelSearch(ctx, botID, e.UserID); err != nil {
			continue
		}
		_ = m.store.SetScene(ctx, botID.String(), e.UserID, "home")
		p, err := m.store.Profile(ctx, botID, e.UserID)
		if err != nil {
			continue
		}
		_ = m.notify(botID, p.TelegramID, "جستجو لغو شد.", mustKBStyled(homeKB))
	}
}

func chatKeyboard() json.RawMessage {
	return mustKBD([][]string{
		{"🛑 قطع مکالمه"},
		{"🚫 بلاک"},
	})
}

func mustKBD(rows [][]string) json.RawMessage {
	var kbd telegram.ReplyKeyboardMarkup
	kbd.ResizeKeyboard = true
	for _, row := range rows {
		var kRow []telegram.ReplyKeyboardButton
		for _, label := range row {
			kRow = append(kRow, telegram.ReplyKeyboardButton{Text: label})
		}
		if len(kRow) > 0 {
			kbd.Keyboard = append(kbd.Keyboard, kRow)
		}
	}
	raw, _ := json.Marshal(kbd)
	return raw
}

func mustKBStyled(rows [][]engine.KBButton) json.RawMessage {
	var kbd telegram.ReplyKeyboardMarkup
	kbd.ResizeKeyboard = true
	kbd.IsPersistent = true
	for _, row := range rows {
		var kRow []telegram.ReplyKeyboardButton
		for _, b := range row {
			if b.Text != "" {
				kRow = append(kRow, telegram.ReplyKeyboardButton{Text: b.Text, Style: b.Style})
			}
		}
		if len(kRow) > 0 {
			kbd.Keyboard = append(kbd.Keyboard, kRow)
		}
	}
	raw, _ := json.Marshal(kbd)
	return raw
}
