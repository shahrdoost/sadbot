package service

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	contentpkg "sadbot/internal/content"
	"sadbot/internal/domain"
	"sadbot/internal/engine"
	"sadbot/internal/engine/templates"
	"sadbot/internal/testutil"
	pgcrypto "sadbot/pkg/crypto"

	"github.com/google/uuid"
)

const testEncKeyHex = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func testEncKey() []byte {
	k, _ := hex.DecodeString(testEncKeyHex)
	return k
}

type botSvcFixture struct {
	svc        *BotService
	bots       *testutil.BotRepo
	configs    *testutil.ConfigRepo
	validator  *testutil.Validator
	runtime    *testutil.Runtime
	audit      *testutil.Audit
	admins     *testutil.AdminRepo
	tgAccounts *testutil.TelegramAccountRepo // set by content tests
}

func newTestBotService(t *testing.T) *botSvcFixture {
	t.Helper()
	registry := engine.NewRegistry()
	registry.Register(&templates.WelcomeMenu{})
	registry.Register(&contentpkg.Template{})

	f := &botSvcFixture{
		bots:      testutil.NewBotRepo(),
		configs:   testutil.NewConfigRepo(),
		validator: &testutil.Validator{Username: "@testbot"},
		runtime:   testutil.NewRuntime(),
		audit:     &testutil.Audit{},
		admins:    testutil.NewAdminRepo(),
	}
	f.svc = NewBotService(f.bots, f.configs, registry, f.validator, f.runtime, f.audit, f.admins, testEncKey())
	return f
}

func superAdmin() *domain.Admin {
	return &domain.Admin{ID: uuid.New(), Email: "s@example.com", Role: domain.RoleSuperAdmin, Status: domain.AdminStatusActive}
}

func plainAdmin() *domain.Admin {
	return &domain.Admin{ID: uuid.New(), Email: "a@example.com", Role: domain.RoleAdmin, Status: domain.AdminStatusActive}
}

func validBotToken() string { return "123456:ABCDEFGHIJKLMNOPQRSTUVWXYZabcd" }

func TestCreateBot_RequiresSuperAdmin(t *testing.T) {
	f := newTestBotService(t)
	_, err := f.svc.Create(context.Background(), plainAdmin(), CreateBotInput{Name: "x", Token: validBotToken()}, nil)
	if err == nil || !strings.Contains(err.Error(), "superadmin") {
		t.Fatalf("expected superadmin-only error, got %v", err)
	}
}

func TestCreateBot_TokenFormatValidation(t *testing.T) {
	f := newTestBotService(t)
	_, err := f.svc.Create(context.Background(), superAdmin(), CreateBotInput{Name: "x", Token: "not-a-token"}, nil)
	if err == nil || !strings.Contains(err.Error(), "توکن") {
		t.Fatalf("expected token format error, got %v", err)
	}
	if f.validator.Calls != 0 {
		t.Fatal("validator must not be called for format-invalid tokens")
	}
}

func TestCreateBot_UnknownTemplate(t *testing.T) {
	f := newTestBotService(t)
	_, err := f.svc.Create(context.Background(), superAdmin(), CreateBotInput{Name: "x", Token: validBotToken(), TemplateKey: "nope"}, nil)
	if err == nil || !strings.Contains(err.Error(), "تمپلیت") {
		t.Fatalf("expected template error, got %v", err)
	}
}

func TestCreateBot_TelegramRejectsToken(t *testing.T) {
	f := newTestBotService(t)
	f.validator.Err = errors.New("401 Unauthorized")
	_, err := f.svc.Create(context.Background(), superAdmin(), CreateBotInput{Name: "x", Token: validBotToken()}, nil)
	if err == nil || !strings.Contains(err.Error(), "تلگرام") {
		t.Fatalf("expected telegram rejection error, got %v", err)
	}
}

func TestCreateBot_HappyPath_EncryptsAndSeedsConfig(t *testing.T) {
	f := newTestBotService(t)

	bot, err := f.svc.Create(context.Background(), superAdmin(), CreateBotInput{Name: "بات تست", Token: validBotToken(), TemplateKey: domain.TemplateWelcomeMenu}, nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if f.validator.LastTok != validBotToken() {
		t.Fatal("validator did not receive the raw token")
	}
	if bot.TelegramUsername == nil || *bot.TelegramUsername != "@testbot" {
		t.Fatalf("username = %v", bot.TelegramUsername)
	}

	stored, err := f.bots.GetByID(context.Background(), bot.ID)
	if err != nil {
		t.Fatal(err)
	}
	dec, err := pgcrypto.DecryptToken(testEncKey(), stored.TokenEncrypted)
	if err != nil || dec != validBotToken() {
		t.Fatalf("stored token not properly encrypted: %v %q", err, dec)
	}

	cfg, err := f.configs.Get(context.Background(), bot.ID)
	if err != nil {
		t.Fatalf("config not seeded: %v", err)
	}
	if !json.Valid(cfg.Texts) || !json.Valid(cfg.Menus) {
		t.Fatal("seeded config must be valid JSON")
	}

	if len(f.audit.Entries) == 0 || f.audit.Entries[0] != "bot.create" {
		t.Fatalf("audit = %v", f.audit.Entries)
	}
}

func TestUpdateBot_TokenChangeRestartsRuntime(t *testing.T) {
	f := newTestBotService(t)
	actor := superAdmin()
	bot, err := f.svc.Create(context.Background(), actor, CreateBotInput{Name: "x", Token: validBotToken()}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.runtime.Start(context.Background(), bot); err != nil {
		t.Fatal(err)
	}

	newTok := "987654:ZYXWVUTSRQPONMLKJIHGFEDCBAzyxwvu"
	_, err = f.svc.Update(context.Background(), actor, bot.ID, UpdateBotInput{Token: &newTok}, nil)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if f.runtime.IsRunning(bot.ID) {
		t.Fatal("runtime must be stopped after token change")
	}
	if f.validator.LastTok != newTok {
		t.Fatal("validator must receive new token")
	}
}

func TestUpdateConfig_VersionConflict(t *testing.T) {
	f := newTestBotService(t)
	actor := superAdmin()
	bot, err := f.svc.Create(context.Background(), actor, CreateBotInput{Name: "x", Token: validBotToken()}, nil)
	if err != nil {
		t.Fatal(err)
	}
	f.configs.ConflictOnUpsert = true

	_, err = f.svc.UpdateConfig(context.Background(), actor, bot.ID, UpdateConfigInput{
		Version: 3, Texts: json.RawMessage(`{"a":1}`), Menus: json.RawMessage(`{"scenes":{}}`),
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "همزمان") {
		t.Fatalf("expected optimistic-lock error, got %v", err)
	}
}

func TestUpdateConfig_RejectsInvalidJSON(t *testing.T) {
	f := newTestBotService(t)
	actor := superAdmin()
	bot, err := f.svc.Create(context.Background(), actor, CreateBotInput{Name: "x", Token: validBotToken()}, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.svc.UpdateConfig(context.Background(), actor, bot.ID, UpdateConfigInput{
		Version: 1, Texts: json.RawMessage(`{broken`), Menus: json.RawMessage(`{}`),
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "JSON") {
		t.Fatalf("expected JSON error, got %v", err)
	}
}

func TestUpdateConfig_NoPermission(t *testing.T) {
	f := newTestBotService(t)
	actor := superAdmin()
	bot, err := f.svc.Create(context.Background(), actor, CreateBotInput{Name: "x", Token: validBotToken()}, nil)
	if err != nil {
		t.Fatal(err)
	}
	stranger := plainAdmin()
	_, _ = f.admins.UpsertBotAccess(context.Background(), stranger.ID, bot.ID, []string{domain.PermBotRead}, nil)

	_, err = f.svc.UpdateConfig(context.Background(), stranger, bot.ID, UpdateConfigInput{
		Version: 1, Texts: json.RawMessage(`{}`), Menus: json.RawMessage(`{}`),
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "دسترسی") {
		t.Fatalf("expected permission error, got %v", err)
	}
}

func TestCan_SuperAlwaysTrue_GrantedAdmin(t *testing.T) {
	f := newTestBotService(t)
	botID := uuid.New()

	if ok, err := f.svc.Can(context.Background(), superAdmin(), botID, domain.PermBotRead); err != nil || !ok {
		t.Fatalf("superadmin: ok=%v err=%v", ok, err)
	}

	granted := plainAdmin()
	_, _ = f.admins.UpsertBotAccess(context.Background(), granted.ID, botID, []string{domain.PermBotFinance}, nil)
	if ok, _ := f.svc.Can(context.Background(), granted, botID, domain.PermBotFinance); !ok {
		t.Fatal("granted perm must pass")
	}
	if ok, _ := f.svc.Can(context.Background(), granted, botID, domain.PermBotConfig); ok {
		t.Fatal("ungranted perm must fail")
	}

	stranger := plainAdmin()
	if ok, _ := f.svc.Can(context.Background(), stranger, botID, domain.PermBotRead); ok {
		t.Fatal("ungranted admin must fail")
	}
}

func TestStart_SetsErrorStatusOnRuntimeFailure(t *testing.T) {
	f := newTestBotService(t)
	actor := superAdmin()
	bot, err := f.svc.Create(context.Background(), actor, CreateBotInput{Name: "x", Token: validBotToken()}, nil)
	if err != nil {
		t.Fatal(err)
	}
	f.runtime.StartErr = errors.New("401 Unauthorized")

	if err := f.svc.Start(context.Background(), actor, bot.ID, nil); err == nil {
		t.Fatal("expected start error")
	}
	stored, _ := f.bots.GetByID(context.Background(), bot.ID)
	if stored.Status != domain.BotStatusError || stored.LastError == nil {
		t.Fatalf("bot must be in error status, got %q %v", stored.Status, stored.LastError)
	}
}

func TestDelete_RequiresSuperAdmin(t *testing.T) {
	f := newTestBotService(t)
	if err := f.svc.Delete(context.Background(), plainAdmin(), uuid.New(), nil); err == nil {
		t.Fatal("non-super must not delete")
	}
}

// createContentBot registers a content bot with a source-shaped config: an
// account, a channel, an unrelated price key and a feature flag.
func createContentBot(t *testing.T, f *botSvcFixture, actor *domain.Admin) *domain.Bot {
	t.Helper()
	bot, err := f.svc.Create(context.Background(), actor, CreateBotInput{
		Name: "محتوا", Token: validBotToken(), TemplateKey: domain.TemplateContent,
	}, nil)
	if err != nil {
		t.Fatalf("Create content bot: %v", err)
	}
	_, err = f.configs.EnsureDefault(context.Background(), bot.ID,
		[]byte(`{"clean_prompt":"پاک کن","unknown_message":"این بات محتواست."}`), []byte(`{}`),
		[]byte(`{"chat_inspect":false}`),
		[]byte(`{"content_account_id":"acc123","content_channel":"@dest","content_folder":"Crypto","brand_key":"src-brand"}`))
	if err != nil {
		t.Fatalf("seed config: %v", err)
	}
	return bot
}

func TestCopyConfig_ContentClonedBotBlanksAccountAndChannel(t *testing.T) {
	f := newTestBotService(t)
	actor := superAdmin()
	src := createContentBot(t, f, actor)

	// clone target: created with template defaults (like BotService.Create)
	dst, err := f.svc.Create(context.Background(), actor, CreateBotInput{
		Name: "محتوا (کلون)", Token: validBotToken() + "x", TemplateKey: domain.TemplateContent,
	}, nil)
	if err != nil {
		t.Fatalf("Create dst: %v", err)
	}

	saved, err := f.svc.CopyConfig(context.Background(), actor, src.ID, dst.ID, nil)
	if err != nil {
		t.Fatalf("CopyConfig: %v", err)
	}

	var prices map[string]any
	if err := json.Unmarshal(saved.Prices, &prices); err != nil {
		t.Fatal(err)
	}
	if _, ok := prices["content_channel"]; ok {
		t.Fatal("cloned content_channel must be blanked (clone inherited source destination)")
	}
	if _, ok := prices["content_account_id"]; ok {
		t.Fatal("cloned content_account_id must be blanked (clone inheriting source account streams the source folder)")
	}
	if prices["content_folder"] != "Crypto" || prices["brand_key"] != "src-brand" {
		t.Fatalf("other price keys must be copied, got %v", prices)
	}
	if got := string(saved.Texts); !strings.Contains(got, "پاک کن") {
		t.Fatalf("texts must be copied, got %s", got)
	}
	if string(saved.FeatureFlags) != `{"chat_inspect":false}` {
		t.Fatalf("feature flags must be preserved verbatim, got %s", saved.FeatureFlags)
	}
}

func TestCopyConfig_ContentNonContentTemplateKeepsAllKeys(t *testing.T) {
	f := newTestBotService(t)
	actor := superAdmin()
	src, err := f.svc.Create(context.Background(), actor, CreateBotInput{Name: "dating", Token: validBotToken() + "y"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.configs.EnsureDefault(context.Background(), src.ID,
		[]byte(`{}`), []byte(`{}`), []byte(`{}`),
		[]byte(`{"sub_price":900}`))
	if err != nil {
		t.Fatal(err)
	}
	dst, err := f.svc.Create(context.Background(), actor, CreateBotInput{Name: "dating2", Token: validBotToken() + "z"}, nil)
	if err != nil {
		t.Fatal(err)
	}

	saved, err := f.svc.CopyConfig(context.Background(), actor, src.ID, dst.ID, nil)
	if err != nil {
		t.Fatalf("CopyConfig: %v", err)
	}
	var prices map[string]any
	if err := json.Unmarshal(saved.Prices, &prices); err != nil {
		t.Fatal(err)
	}
	if prices["sub_price"].(float64) != 900 {
		t.Fatalf("non-content prices must be copied as-is, got %v", prices)
	}
}

func TestCopyConfig_DeniedWithoutConfigPermission(t *testing.T) {
	f := newTestBotService(t)
	actor := superAdmin()
	src := createContentBot(t, f, actor)
	dst, err := f.svc.Create(context.Background(), actor, CreateBotInput{
		Name: "کلون", Token: validBotToken() + "w", TemplateKey: domain.TemplateContent,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	stranger := plainAdmin()
	// stranger gets read-only access — no config permission
	_, _ = f.admins.UpsertBotAccess(context.Background(), stranger.ID, src.ID, []string{domain.PermBotRead}, nil)
	if _, err := f.svc.CopyConfig(context.Background(), stranger, src.ID, dst.ID, nil); err == nil {
		t.Fatal("read-only admin must not copy config")
	}
}
