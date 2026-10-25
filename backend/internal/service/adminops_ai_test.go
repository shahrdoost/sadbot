package service

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"sadbot/internal/domain"
	"sadbot/internal/repository/postgres"

	"github.com/google/uuid"
)

// TestAIHarnessAndToggle: the panel can list users in the AI scene and flip
// the ai_enabled prices flag without disturbing other price keys. Requires a
// real Postgres (same DATABASE_URL contract as the dating integration tests).
func TestAIHarnessAndToggle(t *testing.T) {
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
	configRepo := postgres.NewConfigRepo(pool)
	auditRepo := postgres.NewAuditRepo(pool)

	admin, err := adminRepo.Create(ctx, "ai-harness-"+uuid.NewString()+"@example.com", "x", "تست", domain.RoleAdmin)
	if err != nil {
		t.Fatalf("admin: %v", err)
	}
	bot, err := botRepo.Create(ctx, "ai-harness-bot", nil, []byte("x"), domain.TemplateDating, admin.ID)
	if err != nil {
		t.Fatalf("bot: %v", err)
	}
	t.Cleanup(func() { _ = botRepo.Delete(ctx, bot.ID) })

	// seed a config with a prices key the toggle must not clobber
	if _, err := configRepo.EnsureDefault(ctx, bot.ID, []byte("{}"), []byte("{}"), []byte("{}"),
		[]byte(`{"sub_price": 123456}`)); err != nil {
		t.Fatalf("config: %v", err)
	}

	// two users parked in the AI scene, one at home
	aiState := []byte(`{"scene":"ai","data":{"ai_hist":[{"role":"user","content":"سلام"},{"role":"assistant","content":"سلام!"}]}}`)
	homeState := []byte(`{"scene":"home","data":{}}`)
	var aiID int64
	for _, tc := range []struct {
		tid   int64
		state []byte
		name  string
	}{{331, aiState, "علی"}, {332, aiState, "رضا"}, {333, homeState, "مریم"}} {
		u, err := userRepo.Upsert(ctx, bot.ID, tc.tid, nil, strPtr(tc.name), nil)
		if err != nil {
			t.Fatalf("upsert: %v", err)
		}
		if err := userRepo.SetState(ctx, bot.ID, u.ID, tc.state); err != nil {
			t.Fatalf("set state: %v", err)
		}
		if tc.tid == 331 {
			aiID = u.ID
		}
	}

	svc := NewAdminOpsService(datingRepo, userRepo, auditRepo, configRepo, nil)
	h, err := svc.AIHarness(ctx, bot.ID)
	if err != nil {
		t.Fatalf("harness: %v", err)
	}
	if !h.Enabled {
		t.Fatal("ai_enabled should default to on")
	}
	if len(h.Conversations) != 2 {
		t.Fatalf("conversations = %d, want 2 (home user must not appear)", len(h.Conversations))
	}
	if h.Conversations[0].Messages != 2 {
		t.Fatalf("messages = %d, want 2", h.Conversations[0].Messages)
	}
	if h.Conversations[0].LastMessage == nil || *h.Conversations[0].LastMessage != "سلام!" {
		t.Fatalf("last message = %v, want سلام!", h.Conversations[0].LastMessage)
	}

	// per-user history detail
	det, err := svc.AIUserDetail(ctx, bot.ID, aiID)
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if hist, ok := det["history"].([]map[string]any); !ok || len(hist) != 2 {
		t.Fatalf("history = %#v, want 2 entries", det["history"])
	}

	// toggle off; other price keys survive
	actor := &domain.Admin{ID: admin.ID}
	cfg, err := svc.SetAIFeature(ctx, actor, bot.ID, "ai_enabled", false)
	if err != nil {
		t.Fatalf("toggle: %v", err)
	}
	var prices map[string]any
	if err := json.Unmarshal(cfg.Prices, &prices); err != nil {
		t.Fatalf("prices: %v", err)
	}
	if prices["ai_enabled"] != float64(0) {
		t.Fatalf("ai_enabled = %v, want 0", prices["ai_enabled"])
	}
	if prices["sub_price"] != float64(123456) {
		t.Fatalf("sub_price clobbered: %v", prices["sub_price"])
	}

	h, err = svc.AIHarness(ctx, bot.ID)
	if err != nil {
		t.Fatalf("harness after toggle: %v", err)
	}
	if h.Enabled {
		t.Fatal("ai_enabled should be off after toggle")
	}

	// reject unknown keys
	if _, err := svc.SetAIFeature(ctx, actor, bot.ID, "sub_price", false); err == nil {
		t.Fatal("arbitrary key write must be rejected")
	}
}
