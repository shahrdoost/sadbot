package payment

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"sadbot/internal/domain"
	"sadbot/internal/repository/postgres"

	"github.com/google/uuid"
)

type notifyRecorder struct{ calls []string }

func (n *notifyRecorder) NotifyUser(_ uuid.UUID, _ int64, text string) {
	n.calls = append(n.calls, text)
}

// TestSweeperExpiresOnce locks the fix for the production spam bug: the
// sweeper used MarkFailed(..., "expired"), which violates the payments CHECK
// constraint (pending/verified/failed/refunded). The UPDATE errored, the
// `_ =` swallowed it, the row stayed pending forever and the user received
// «مهلت پرداخت شما تمام شد» every minute.
func TestSweeperExpiresOnce(t *testing.T) {
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
	payRepo := postgres.NewPaymentRepo(pool)

	admin, err := adminRepo.Create(ctx, fmt.Sprintf("pay-exp-%d@example.com", time.Now().UnixNano()), "x", "test", domain.RoleAdmin)
	if err != nil {
		t.Fatalf("admin: %v", err)
	}
	bot, err := botRepo.Create(ctx, "test-pay-exp-bot", nil, []byte("x"), domain.TemplateDating, admin.ID)
	if err != nil {
		t.Fatalf("bot: %v", err)
	}
	t.Cleanup(func() { _ = botRepo.Delete(ctx, bot.ID) })

	user, err := userRepo.Upsert(ctx, bot.ID, 777000111, nil, strPtr("Payer"), nil)
	if err != nil {
		t.Fatalf("user: %v", err)
	}
	uid := user.ID
	p, err := payRepo.Create(ctx, &domain.Payment{
		BotID: bot.ID, UserID: &uid, AmountRials: 100000,
		Kind: KindCoinPackage, Gateway: "test", Status: "pending", Coins: 40,
	})
	if err != nil {
		t.Fatalf("payment: %v", err)
	}
	// backdate so the sweeper treats it as stale
	if _, err := pool.Exec(ctx, "UPDATE payments SET created_at = now() - interval '1 hour' WHERE id = $1", p.ID); err != nil {
		t.Fatalf("backdate: %v", err)
	}

	// root-cause guard: the old status value must be rejected by the CHECK
	if ok, err := payRepo.MarkFailed(ctx, p.ID, "expired"); err == nil || ok {
		t.Fatal(`MarkFailed("expired") must fail — status violates the payments CHECK constraint`)
	}

	recorder := &notifyRecorder{}
	svc := NewService(payRepo, nil, &Gateway{}, recorder, "")
	sw := &Sweeper{svc: svc}

	sw.autoVerify(ctx)
	var status string
	if err := pool.QueryRow(ctx, "SELECT status FROM payments WHERE id = $1", p.ID).Scan(&status); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if status != "failed" {
		t.Fatalf("stale pending must become failed, got %q", status)
	}
	if len(recorder.calls) != 1 || !strings.Contains(recorder.calls[0], "مهلت پرداخت") {
		t.Fatalf("expiry must notify exactly once, got %v", recorder.calls)
	}

	// a second sweep must be silent — this is what spammed the user
	sw.autoVerify(ctx)
	if len(recorder.calls) != 1 {
		t.Fatalf("second sweep must not re-notify, got %d calls", len(recorder.calls))
	}
}

func strPtr(s string) *string { return &s }
