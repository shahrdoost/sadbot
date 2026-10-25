package dating

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"sadbot/internal/domain"
	"sadbot/internal/repository/postgres"

	"github.com/google/uuid"
)

// TestConfigPricesRoundTrip guards the jsonb write path end to end.
func TestConfigPricesRoundTrip(t *testing.T) {
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

	botRepo := postgres.NewBotRepo(pool)
	adminRepo := postgres.NewAdminRepo(pool)
	admin, err := adminRepo.Create(ctx, "cfg-"+uuid.New().String()+"@example.com", "x", "test", domain.RoleAdmin)
	if err != nil {
		t.Fatalf("admin: %v", err)
	}
	bot, err := botRepo.Create(ctx, "test-config-bot", nil, []byte("x"), domain.TemplateDating, admin.ID)
	if err != nil {
		t.Fatalf("bot: %v", err)
	}
	t.Cleanup(func() { _ = botRepo.Delete(ctx, bot.ID) })

	repo := postgres.NewConfigRepo(pool)
	prices := []byte(`{"channels":["@a","@b"],"channel_bonus":10}`)
	texts := []byte(`{"welcome":"سلام"}`)
	menus := []byte(`{"scenes":{}}`)
	flags := []byte(`{}`)

	if _, err := repo.EnsureDefault(ctx, bot.ID, texts, menus, flags, prices); err != nil {
		t.Fatalf("ensure default: %v", err)
	}
	// save into a Go map the exact way runtime.mergeJSON does
	var merged map[string]any
	got, err := repo.Get(ctx, bot.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if err := json.Unmarshal(got.Prices, &merged); err != nil {
		t.Fatalf("prices must be a JSON object, not a JSON string: %v (raw=%s)", err, got.Prices)
	}
	channels, ok := merged["channels"].([]any)
	if !ok || len(channels) != 2 {
		t.Fatalf("channels did not survive the round trip: %v (raw=%s)", merged["channels"], got.Prices)
	}
	if bonus, ok := merged["channel_bonus"].(float64); !ok || bonus != 10 {
		t.Fatalf("channel_bonus did not survive: %v", merged["channel_bonus"])
	}

	// update path (Upsert) must hold the same guarantee
	updated := []byte(`{"channels":["@only"],"channel_bonus":25}`)
	if _, err := repo.Upsert(ctx, &domain.BotConfig{
		BotID: bot.ID, Texts: texts, Menus: menus, FeatureFlags: flags, Prices: updated, Version: got.Version,
	}, &admin.ID); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	after, err := repo.Get(ctx, bot.ID)
	if err != nil {
		t.Fatalf("get after upsert: %v", err)
	}
	var merged2 map[string]any
	if err := json.Unmarshal(after.Prices, &merged2); err != nil {
		t.Fatalf("upserted prices must be a JSON object: %v (raw=%s)", err, after.Prices)
	}
	if bonus, ok := merged2["channel_bonus"].(float64); !ok || bonus != 25 {
		t.Fatalf("upserted channel_bonus lost: %v (raw=%s)", merged2["channel_bonus"], after.Prices)
	}

	// and it must merge onto template defaults the way the runtime does
	var defaults map[string]any
	if err := json.Unmarshal((&Template{}).DefaultPrices(), &defaults); err != nil {
		t.Fatalf("template defaults: %v", err)
	}
	for k, v := range merged2 {
		defaults[k] = v
	}
	if bonus, ok := defaults["channel_bonus"].(float64); !ok || bonus != 25 {
		t.Fatalf("override lost during merge: %v", defaults["channel_bonus"])
	}
	if _, ok := defaults["superlike_cost"]; !ok {
		t.Fatal("template defaults must survive alongside overrides")
	}
	_ = time.Now
}
