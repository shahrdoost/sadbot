package dating

import (
	"context"
	"os"
	"testing"
	"time"

	"sadbot/internal/repository/postgres"
)

// TestStreakAndBoost: home activity starts a streak; boost spends coins and
// lifts the profile above the browse feed.
func TestStreakAndBoost(t *testing.T) {
	e := newBrowseEnv(t)
	ctx := context.Background()
	e.prices = map[string]any{
		"boost_1h_coins":       float64(2),
		"streak_rewards":       map[string]any{"1": float64(1)},
		"ghost_enabled":        float64(0),
		"browse_free_likes":    float64(10),
		"signup_bonus":         float64(5),
		"avatar_bonus":         float64(10),
		"search_stranger_male": float64(1),
	}

	e.onboard(t, e.sendA, "امیر", "25", "آقام")
	e.feed(t, e.sendA, "⏭ رد شد")
	// first home-scene interaction records the streak
	e.feed(t, e.sendA, "📖 راهنما")

	pA, _ := e.store.ProfileByTelegram(ctx, e.botID, 111)
	if pA.StreakCount != 1 {
		t.Fatalf("home interaction must start streak=1, got %d", pA.StreakCount)
	}
	if pA.StreakLastDate == nil {
		t.Fatal("streak_last_date must be set")
	}
	// milestone reward "1": +1 coin on top of signup 5
	if pA.Coins != 6 {
		t.Fatalf("streak milestone must add 1 coin, got %d", pA.Coins)
	}

	// same-day interaction does not double-count
	e.feed(t, e.sendA, "💰 سکه")
	pA, _ = e.store.ProfileByTelegram(ctx, e.botID, 111)
	if pA.StreakCount != 1 || pA.Coins != 6 {
		t.Fatalf("same-day must not re-award: streak=%d coins=%d", pA.StreakCount, pA.Coins)
	}

	// boost: 1h costs 2 coins
	e.feedCB(t, e.sendA, "act:boost:1")
	pA, _ = e.store.ProfileByTelegram(ctx, e.botID, 111)
	if pA.Coins != 4 {
		t.Fatalf("boost must cost 2 coins, got %d", pA.Coins)
	}
	if pA.BoostedUntil == nil || !pA.BoostedUntil.After(time.Now()) {
		t.Fatalf("boosted_until must be in the future: %+v", pA.BoostedUntil)
	}
	if msg := e.sendA.lastTo(111); msg == nil || !contains(msg.Text, "بوست فعال شد") {
		t.Fatalf("boost confirmation expected: %+v", e.sendA.sent)
	}

	// boost with insufficient coins fails
	e.feedCB(t, e.sendA, "act:boost:24") // costs 10 > 4
	pA, _ = e.store.ProfileByTelegram(ctx, e.botID, 111)
	if pA.Coins != 4 {
		t.Fatalf("failed boost must not spend: %d", pA.Coins)
	}
}

// TestPaidNameEdit: first name edit is free; a second within cooldown is
// blocked; after the cooldown (simulated by backdating) it costs coins.
func TestPaidNameEdit(t *testing.T) {
	e := newBrowseEnv(t)
	ctx := context.Background()
	e.prices = map[string]any{
		"edit_name_cost":          float64(100),
		"name_edit_cooldown_days": float64(14),
		"ghost_enabled":           float64(0),
		"signup_bonus":            float64(5),
	}

	e.onboard(t, e.sendA, "امیر", "25", "آقام")
	e.feed(t, e.sendA, "⏭ رد شد")

	// first edit is free
	e.feedCB(t, e.sendA, "act:edit:name")
	e.feed(t, e.sendA, "رضا")
	pA, _ := e.store.ProfileByTelegram(ctx, e.botID, 111)
	if pA.Name == nil || *pA.Name != "رضا" {
		t.Fatalf("name not updated: %+v", pA.Name)
	}
	if pA.NameEditedAt == nil {
		t.Fatal("name_edited_at must be stamped")
	}
	if pA.Coins != 5 {
		t.Fatalf("first edit must be free, coins=%d", pA.Coins)
	}

	// second edit within cooldown is blocked
	e.feedCB(t, e.sendA, "act:edit:name")
	if msg := e.sendA.lastTo(111); msg == nil || !contains(msg.Text, "یک‌بار مجازه") {
		t.Fatalf("cooldown expected: %+v", e.sendA.sent)
	}

	// backdate past cooldown; coins (5) < cost (100) → paid edit refused
	pool, err := postgres.Connect(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	defer pool.Close()
	old := time.Now().Add(-15 * 24 * time.Hour)
	if _, err := pool.Exec(ctx,
		`UPDATE dating_profiles SET name_edited_at = $3 WHERE bot_id = $1 AND user_id = $2`,
		e.botID, pA.UserID, old); err != nil {
		t.Fatalf("backdate: %v", err)
	}
	e.feedCB(t, e.sendA, "act:edit:name")
	if msg := e.sendA.lastTo(111); msg == nil || !contains(msg.Text, "سکه کافی نیست") {
		t.Fatalf("insufficient coins expected: %+v", e.sendA.sent)
	}
	pA, _ = e.store.ProfileByTelegram(ctx, e.botID, 111)
	if pA.Name == nil || *pA.Name != "رضا" {
		t.Fatalf("name must be unchanged after refused edit: %+v", pA.Name)
	}
	if pA.Coins != 5 {
		t.Fatalf("coins must be unchanged: %d", pA.Coins)
	}
}
