package dating

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"sadbot/internal/domain"
	"sadbot/internal/repository/postgres"
)

// TestDiscountRepoLifecycle: create → get → consume → exhausted.
func TestDiscountRepoLifecycle(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := postgres.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	defer pool.Close()

	adminRepo := postgres.NewAdminRepo(pool)
	admin, err := adminRepo.Create(ctx, fmt.Sprintf("test-%d@example.com", time.Now().UnixNano()), "x", "test", domain.RoleAdmin)
	if err != nil {
		t.Fatalf("admin: %v", err)
	}
	botRepo := postgres.NewBotRepo(pool)
	bot, err := botRepo.Create(ctx, "test-discount-bot", nil, []byte("x"), domain.TemplateDating, admin.ID)
	if err != nil {
		t.Fatalf("bot: %v", err)
	}
	t.Cleanup(func() { _ = botRepo.Delete(ctx, bot.ID) })

	repo := postgres.NewDiscountRepo(pool)

	d, err := repo.Create(ctx, bot.ID, "SAVE20", "percent", 20, 1, nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if d.Code != "SAVE20" || d.Kind != "percent" || d.Value != 20 {
		t.Fatalf("created code wrong: %+v", d)
	}

	got, err := repo.GetByCode(ctx, bot.ID, "save20") // case-insensitive
	if err != nil || got == nil {
		t.Fatalf("get: %+v err=%v", got, err)
	}

	ok, err := repo.IncrementUse(ctx, bot.ID, "SAVE20")
	if err != nil || !ok {
		t.Fatalf("first use should succeed: ok=%v err=%v", ok, err)
	}
	ok, _ = repo.IncrementUse(ctx, bot.ID, "SAVE20")
	if ok {
		t.Fatal("max_uses=1 must block second use")
	}
	got, _ = repo.GetByCode(ctx, bot.ID, "SAVE20")
	if got != nil {
		t.Fatalf("exhausted code must not be returned: %+v", got)
	}

	// expired code is invisible
	past := time.Now().Add(-time.Hour)
	if _, err := repo.Create(ctx, bot.ID, "OLD", "flat_toman", 5000, 0, &past); err != nil {
		t.Fatalf("create expired: %v", err)
	}
	got, _ = repo.GetByCode(ctx, bot.ID, "OLD")
	if got != nil {
		t.Fatalf("expired code must not be returned: %+v", got)
	}

	items, err := repo.ListByBot(ctx, bot.ID)
	if err != nil || len(items) != 2 {
		t.Fatalf("list: %d err=%v", len(items), err)
	}
	if err := repo.Delete(ctx, bot.ID, d.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
}

// TestDiscountBotFlow: user opens the wallet discount prompt, enters a valid
// percent code, and it is stored for the next purchase; invalid codes are
// rejected without state change.
func TestDiscountBotFlow(t *testing.T) {
	e := newBrowseEnv(t)
	ctx := context.Background()
	pool, err := postgres.Connect(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	defer pool.Close()
	repo := postgres.NewDiscountRepo(pool)
	if _, err := repo.Create(ctx, e.botID, "WELCOME10", "percent", 10, 0, nil); err != nil {
		t.Fatalf("create code: %v", err)
	}
	e.eng.SetData("discountRepo", repo)

	e.onboard(t, e.sendA, "امیر", "25", "آقام")
	e.feed(t, e.sendA, "⏭ رد شد")

	e.feedCB(t, e.sendA, "act:wallet:discount")
	if msg := e.sendA.lastTo(111); msg == nil || !contains(msg.Text, "کد تخفیف") {
		t.Fatalf("discount prompt expected: %+v", e.sendA.sent)
	}
	e.feed(t, e.sendA, "welcome10") // lowercase — repo uppercases
	if !sentContains(e.sendA, 111, "ثبت شد") {
		t.Fatalf("code accepted expected: %+v", e.sendA.sent)
	}
	if msg := e.sendA.lastTo(111); msg == nil || !contains(msg.Text, "کد تخفیف فعال") {
		t.Fatalf("wallet must show active code: %+v", e.sendA.sent)
	}
	pA, _ := e.store.ProfileByTelegram(ctx, e.botID, 111)
	state, _ := e.users.GetState(ctx, e.botID.String(), pA.UserID)
	if !contains(string(state), "WELCOME10") {
		t.Fatalf("state must store the code: %s", state)
	}

	// invalid code does not replace the stored one
	e.feedCB(t, e.sendA, "act:wallet:discount")
	e.feed(t, e.sendA, "NOPE")
	if msg := e.sendA.lastTo(111); msg == nil || !contains(msg.Text, "نامعتبر") {
		t.Fatalf("invalid code rejected expected: %+v", e.sendA.sent)
	}
	state, _ = e.users.GetState(ctx, e.botID.String(), pA.UserID)
	if !contains(string(state), "WELCOME10") {
		t.Fatalf("invalid code must not clear stored code: %s", state)
	}
}

// sentContains reports whether any recorded message to chatID holds sub.
func sentContains(f *fakeSender, chatID int64, sub string) bool {
	for _, m := range f.sent {
		if m.ChatID == chatID && contains(m.Text, sub) {
			return true
		}
	}
	return false
}
