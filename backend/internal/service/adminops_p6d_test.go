package service

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"sadbot/internal/domain"
	"sadbot/internal/repository/postgres"
	"sadbot/internal/telegram"

	"github.com/google/uuid"
)

// TestTelegramBlockScanAndBlockPairs: the scanner probes inactive users,
// marks 403s as is_blocked (which excludes them from broadcasts), and the
// block-pairs view lists who blocked whom. Requires real Postgres.
func TestTelegramBlockScanAndBlockPairs(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := postgres.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	t.Cleanup(pool.Close)

	adminRepo := postgres.NewAdminRepo(pool)
	botRepo := postgres.NewBotRepo(pool)
	userRepo := postgres.NewBotUserRepo(pool)
	datingRepo := postgres.NewDatingRepo(pool)
	auditRepo := postgres.NewAuditRepo(pool)
	configRepo := postgres.NewConfigRepo(pool)

	admin, err := adminRepo.Create(ctx, "p6d-scan-"+uuid.NewString()+"@example.com", "x", "تست", domain.RoleAdmin)
	if err != nil {
		t.Fatalf("admin: %v", err)
	}
	bot, err := botRepo.Create(ctx, "p6d-scan-bot", nil, []byte("x"), domain.TemplateDating, admin.ID)
	if err != nil {
		t.Fatalf("bot: %v", err)
	}
	t.Cleanup(func() { _ = botRepo.Delete(ctx, bot.ID) })

	// two inactive users + one recently active (must not be probed)
	uReach, err := userRepo.Upsert(ctx, bot.ID, 551, nil, strPtr("رسان"), nil)
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	uBlocked, err := userRepo.Upsert(ctx, bot.ID, 552, nil, strPtr("بلاکر"), nil)
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if _, err := userRepo.Upsert(ctx, bot.ID, 553, nil, strPtr("فعال"), nil); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE bot_end_users SET last_seen_at = now() - interval '10 days'
		WHERE bot_id = $1 AND telegram_id IN (551, 552)`, bot.ID); err != nil {
		t.Fatalf("age rows: %v", err)
	}

	// dating profiles (with names) so block-pairs can show them
	store := datingRepo
	if _, err := store.EnsureProfile(ctx, bot.ID, uReach.ID, 551); err != nil {
		t.Fatalf("profile reach: %v", err)
	}
	if _, err := store.EnsureProfile(ctx, bot.ID, uBlocked.ID, 552); err != nil {
		t.Fatalf("profile blocked: %v", err)
	}
	if _, err := store.UpdateProfile(ctx, bot.ID, uReach.ID, postgres.ProfileUpdate{Name: strPtr("رسان")}); err != nil {
		t.Fatalf("name reach: %v", err)
	}
	if _, err := store.UpdateProfile(ctx, bot.ID, uBlocked.ID, postgres.ProfileUpdate{Name: strPtr("بلاکر")}); err != nil {
		t.Fatalf("name blocked: %v", err)
	}

	// notify fake: 551 succeeds, 552 answers 403 (user blocked the bot)
	var mu sync.Mutex
	var probed []int64
	notify := func(_ context.Context, _ uuid.UUID, telegramID int64, _ string) error {
		mu.Lock()
		probed = append(probed, telegramID)
		mu.Unlock()
		if telegramID == 552 {
			return &telegram.APIError{Code: 403, Description: "Forbidden: bot was blocked by the user"}
		}
		return nil
	}

	svc := NewAdminOpsService(datingRepo, userRepo, auditRepo, configRepo, notify)
	res, err := svc.ScanTelegramBlocks(ctx, &domain.Admin{ID: admin.ID}, bot.ID, 7)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if res.Scanned != 2 || res.Reachable != 1 || res.Blocked != 1 {
		t.Fatalf("scan result = %+v, want scanned=2 reachable=1 blocked=1", res)
	}
	// the recently-active user must not have been probed
	mu.Lock()
	seen := map[int64]bool{}
	for _, id := range probed {
		seen[id] = true
	}
	mu.Unlock()
	if seen[553] {
		t.Fatal("recently active user 553 must not be probed")
	}

	// 552 is now marked blocked → excluded from the broadcast audience
	u, err := userRepo.GetByTelegramID(ctx, bot.ID, 552)
	if err != nil || !u.IsBlocked {
		t.Fatalf("user 552 should be is_blocked=true (err=%v blocked=%v)", err, u.IsBlocked)
	}
	n, err := postgres.NewBroadcastRepo(pool).BroadcastAudienceCount(ctx, bot.ID)
	if err != nil {
		t.Fatalf("audience: %v", err)
	}
	if n != 2 { // 551 + 553; 552 excluded
		t.Fatalf("audience = %d, want 2 (blocked user excluded)", n)
	}

	// user-level block relationship
	if err := datingRepo.Block(ctx, bot.ID, uReach.ID, uBlocked.ID, time.Hour); err != nil {
		t.Fatalf("block: %v", err)
	}
	pairs, err := svc.BlockPairs(ctx, bot.ID)
	if err != nil {
		t.Fatalf("pairs: %v", err)
	}
	if len(pairs) != 1 {
		t.Fatalf("pairs = %d, want 1", len(pairs))
	}
	if pairs[0].BlockerName != "رسان" || pairs[0].BlockedName != "بلاکر" {
		t.Fatalf("pair names = %q / %q, want رسان / بلاکر", pairs[0].BlockerName, pairs[0].BlockedName)
	}
}

// TestBroadcastWithPhotoFooter: a broadcast run carries photo+footer through
// to the sender (Telegram accepts file_id or https URL in the photo field).
func TestBroadcastWithPhotoFooter(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := postgres.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	t.Cleanup(pool.Close)

	adminRepo := postgres.NewAdminRepo(pool)
	botRepo := postgres.NewBotRepo(pool)
	userRepo := postgres.NewBotUserRepo(pool)
	broadcastRepo := postgres.NewBroadcastRepo(pool)

	admin, err := adminRepo.Create(ctx, "p6d-bc-"+uuid.NewString()+"@example.com", "x", "تست", domain.RoleAdmin)
	if err != nil {
		t.Fatalf("admin: %v", err)
	}
	bot, err := botRepo.Create(ctx, "p6d-bc-bot", nil, []byte("x"), domain.TemplateDating, admin.ID)
	if err != nil {
		t.Fatalf("bot: %v", err)
	}
	t.Cleanup(func() { _ = botRepo.Delete(ctx, bot.ID) })
	if _, err := userRepo.Upsert(ctx, bot.ID, 661, nil, strPtr("گیرنده"), nil); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	var mu sync.Mutex
	var gotPhotos, gotFooters, gotTexts []string
	send := func(_ context.Context, _ uuid.UUID, _ int64, text, photo, footer string) error {
		mu.Lock()
		gotTexts = append(gotTexts, text)
		gotPhotos = append(gotPhotos, photo)
		gotFooters = append(gotFooters, footer)
		mu.Unlock()
		return nil
	}
	svc := NewBroadcastService(broadcastRepo, send)
	run, err := svc.Start(ctx, bot.ID, admin.ID, "متن اعلان", "https://example.com/pic.jpg", "پی‌نوشت")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if run.Photo == nil || *run.Photo != "https://example.com/pic.jpg" {
		t.Fatalf("run.Photo = %v", run.Photo)
	}

	// wait for the async runner to finish the single-recipient run
	var done *postgres.BroadcastRun
	for i := 0; i < 50; i++ {
		got, gerr := svc.Get(ctx, bot.ID, run.ID)
		if gerr == nil && got.Status == "done" {
			done = got
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if done == nil {
		t.Fatal("broadcast did not finish in time")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(gotPhotos) != 1 || gotPhotos[0] != "https://example.com/pic.jpg" {
		t.Fatalf("photos = %v", gotPhotos)
	}
	if len(gotFooters) != 1 || gotFooters[0] != "پی‌نوشت" {
		t.Fatalf("footers = %v", gotFooters)
	}
	if len(gotTexts) != 1 || gotTexts[0] != "متن اعلان" {
		t.Fatalf("texts = %v", gotTexts)
	}
}
