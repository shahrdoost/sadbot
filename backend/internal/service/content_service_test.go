package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"sadbot/internal/testutil"
)

// newContentService builds a ContentService over the bot-service fakes.
// posts (postgres) is passed nil — ledger methods aren't under test here.
func newContentService(t *testing.T) (*ContentService, *botSvcFixture) {
	t.Helper()
	f := newTestBotService(t)
	f.tgAccounts = testutil.NewTelegramAccountRepo()
	tg := NewTelegramAccountService(f.tgAccounts, nil, f.audit, f.svc)
	return NewContentService(f.svc, tg, nil), f
}

func validContentPut() ContentPut {
	return ContentPut{
		Version: 1, AccountID: "acc1", Folder: "Crypto", Channel: "@dest",
		PublishVia:  "bot",
		PollMinutes: 2, BackfillLimit: 20, MinLength: 40,
		BrandKey: "b1", BrandNameEN: "brand.example",
		BrandNameFA: "برند", BrandAccent: "#FFA070",
		BrandGlowA: "rgba(1,1,1,0.1)", BrandGlowB: "rgba(2,2,2,0.1)", BrandGlowC: "rgba(3,3,3,0.1)",
		CleanPrompt: "پاک‌سازی کن", CardPrompt: "خلاصه کن",
	}
}

func TestPutContentSettings_GuardsEmptySourceOrDestination(t *testing.T) {
	c, f := newContentService(t)
	actor := superAdmin()
	bot := createContentBot(t, f, actor)

	cases := map[string]func(*ContentPut){
		"account": func(p *ContentPut) { p.AccountID = " " },
		"folder":  func(p *ContentPut) { p.Folder = "" },
		"channel": func(p *ContentPut) { p.Channel = "" },
	}
	for name, mut := range cases {
		in := validContentPut()
		mut(&in)
		if _, err := c.PutContentSettings(context.Background(), actor, bot.ID, in, nil); err == nil {
			t.Fatalf("%s empty must be rejected (collector fail-closed)", name)
		}
	}
}

func TestPutContentSettings_MergesPreservingUnknownKeysAndFlags(t *testing.T) {
	c, f := newContentService(t)
	actor := superAdmin()
	bot := createContentBot(t, f, actor) // seeds content_folder + brand_key + flag

	in := validContentPut()
	in.Version = 1
	saved, err := c.PutContentSettings(context.Background(), actor, bot.ID, in, nil)
	if err != nil {
		t.Fatalf("PutContentSettings: %v", err)
	}

	var prices map[string]any
	if err := json.Unmarshal(saved.Prices, &prices); err != nil {
		t.Fatal(err)
	}
	if prices["content_channel"] != "@dest" || prices["content_account_id"] != "acc1" {
		t.Fatalf("written keys missing: %v", prices)
	}
	if prices["custom_legacy"] != nil {
		t.Fatalf("unexpected: %v", prices)
	}
	// feature flags ride along untouched
	if string(saved.FeatureFlags) != `{"chat_inspect":false}` {
		t.Fatalf("feature flags must be carried verbatim, got %s", saved.FeatureFlags)
	}
	// texts: prompts replaced, other keys (unknown_message) preserved
	var texts map[string]any
	if err := json.Unmarshal(saved.Texts, &texts); err != nil {
		t.Fatal(err)
	}
	if texts["clean_prompt"] != "پاک‌سازی کن" || texts["card_prompt"] != "خلاصه کن" {
		t.Fatalf("prompts not saved: %v", texts)
	}
	if _, ok := texts["unknown_message"]; !ok {
		t.Fatalf("other texts keys must be preserved: %v", texts)
	}
}

func TestPutContentSettings_EmptyPromptFallsBackToDefault(t *testing.T) {
	c, f := newContentService(t)
	actor := superAdmin()
	bot := createContentBot(t, f, actor)

	in := validContentPut()
	in.CleanPrompt = "" // reset → collector falls back to built-in CleanSystem
	saved, err := c.PutContentSettings(context.Background(), actor, bot.ID, in, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(saved.Texts), "پاک کن") || strings.Contains(string(saved.Texts), "پاک‌سازی کن") {
		t.Fatalf("empty prompt must drop the override: %s", saved.Texts)
	}
}

func TestPutContentSettings_VersionConflictPropagates(t *testing.T) {
	c, f := newContentService(t)
	actor := superAdmin()
	bot := createContentBot(t, f, actor)

	f.configs.ConflictOnUpsert = true
	in := validContentPut()
	if _, err := c.PutContentSettings(context.Background(), actor, bot.ID, in, nil); err == nil || !strings.Contains(err.Error(), "همزمان") {
		t.Fatalf("expected version conflict error, got %v", err)
	}
}

func TestGetContentSettings_TypedViewAndDefaults(t *testing.T) {
	c, f := newContentService(t)
	actor := superAdmin()
	bot := createContentBot(t, f, actor) // seeds acc123/Crypto/@dest + clean_prompt

	out, err := c.GetContentSettings(context.Background(), actor, bot.ID)
	if err != nil {
		t.Fatalf("GetContentSettings: %v", err)
	}
	if out.AccountID != "acc123" || out.Folder != "Crypto" || out.Channel != "@dest" {
		t.Fatalf("typed view wrong: %+v", out)
	}
	if out.BrandKey != "src-brand" {
		t.Fatalf("brand not mapped: %q", out.BrandKey)
	}
	if out.PollMinutes != 2 || out.BackfillLimit != 20 || !out.RenderCardSummary {
		t.Fatalf("defaults not merged: %+v", out)
	}
	if out.CleanPrompt != "پاک کن" {
		t.Fatalf("override prompt must come from texts, got %q", out.CleanPrompt)
	}
	if out.DefaultCleanPrompt == "" || out.DefaultCardPrompt == "" {
		t.Fatal("template default prompts must be exposed for the reset button")
	}
	if out.DefaultCleanPrompt == out.CleanPrompt {
		t.Fatal("bot override and template default must differ")
	}
}

func TestGetContentSettings_RejectsNonContentBot(t *testing.T) {
	c, f := newContentService(t)
	actor := superAdmin()
	bot, err := f.svc.Create(context.Background(), actor, CreateBotInput{Name: "dating", Token: validBotToken() + "q"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.GetContentSettings(context.Background(), actor, bot.ID); err == nil {
		t.Fatal("dating bot must be rejected by the content panel service")
	}
}
func TestFolders_AcceptsSidecarAccountID(t *testing.T) {
	c, f := newContentService(t)
	actor := superAdmin()
	// sidecar id as production stores it: 32-hex, which Go's uuid.Parse ALSO
	// accepts — the account-list match must win over the uuid passthrough
	sidecar := "9d65f63e35f447ebb1e3d077a6e5184b"
	acc, err := f.tgAccounts.Create(context.Background(), actor.ID, "DA", 42, "+98900", sidecar)
	if err != nil {
		t.Fatal(err)
	}

	got, err := c.resolveAccountID(context.Background(), actor, sidecar)
	if err != nil {
		t.Fatalf("sidecar id must resolve: %v", err)
	}
	if got != acc.ID {
		t.Fatalf("resolved %s, want panel id %s (sidecar hex must not be treated as a panel uuid)", got, acc.ID)
	}

	// panel uuid passes through untouched
	if got, err := c.resolveAccountID(context.Background(), actor, acc.ID.String()); err != nil || got != acc.ID {
		t.Fatalf("panel uuid passthrough failed: %v %v", got, err)
	}

	// unknown non-uuid → not found
	if _, err := c.resolveAccountID(context.Background(), actor, "does-not-exist"); err == nil {
		t.Fatal("unknown sidecar id must be rejected")
	}
}

func TestPutContentSettings_PublishViaValidationAndSave(t *testing.T) {
	c, f := newContentService(t)
	actor := superAdmin()
	bot := createContentBot(t, f, actor)

	// invalid path → rejected
	in := validContentPut()
	in.PublishVia = "carrier-pigeon"
	if _, err := c.PutContentSettings(context.Background(), actor, bot.ID, in, nil); err == nil {
		t.Fatal("unknown publish_via must be rejected")
	}

	// sidecar path → stored in prices
	in = validContentPut()
	in.PublishVia = "sidecar"
	saved, err := c.PutContentSettings(context.Background(), actor, bot.ID, in, nil)
	if err != nil {
		t.Fatalf("sidecar save: %v", err)
	}
	var prices map[string]any
	if err := json.Unmarshal(saved.Prices, &prices); err != nil {
		t.Fatal(err)
	}
	if prices["content_publish_via"] != "sidecar" {
		t.Fatalf("content_publish_via = %v, want sidecar", prices["content_publish_via"])
	}

	// empty (older UI payload) → normalized to bot
	in = validContentPut()
	in.PublishVia = ""
	saved, err = c.PutContentSettings(context.Background(), actor, bot.ID, in, nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal(saved.Prices, &prices)
	if prices["content_publish_via"] != "bot" {
		t.Fatalf("empty publish_via must default to bot, got %v", prices["content_publish_via"])
	}
}

func TestPutContentSettings_TextOnlyRoundTrip(t *testing.T) {
	c, f := newContentService(t)
	actor := superAdmin()
	bot := createContentBot(t, f, actor)

	in := validContentPut()
	in.TextOnly = true
	saved, err := c.PutContentSettings(context.Background(), actor, bot.ID, in, nil)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	var prices map[string]any
	if err := json.Unmarshal(saved.Prices, &prices); err != nil {
		t.Fatal(err)
	}
	if prices["content_text_only"] != true {
		t.Fatalf("content_text_only = %v, want true", prices["content_text_only"])
	}

	out, err := c.GetContentSettings(context.Background(), actor, bot.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !out.TextOnly {
		t.Fatal("Get must return text_only=true back")
	}

	// flip back off
	in.TextOnly = false
	in.Version = out.Version
	saved, err = c.PutContentSettings(context.Background(), actor, bot.ID, in, nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal(saved.Prices, &prices)
	if prices["content_text_only"] != false {
		t.Fatalf("content_text_only = %v, want false", prices["content_text_only"])
	}
}
