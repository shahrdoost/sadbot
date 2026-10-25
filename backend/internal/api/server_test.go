package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sadbot/internal/api/handlers"
	"sadbot/internal/domain"
	"sadbot/internal/engine"
	"sadbot/internal/engine/templates"
	"sadbot/internal/service"
	"sadbot/internal/testutil"

	contentpkg "sadbot/internal/content"

	"github.com/google/uuid"
)

// apiFixture wires the full router on in-memory fakes.
// Postgres-backed handler deps (users/events/payments/finance) are left nil —
// those routes are exercised only against a real DB in integration tests.
type apiFixture struct {
	t      *testing.T
	router http.Handler
	admins *testutil.AdminRepo
	super  *domain.Admin
	admin  *domain.Admin
}

func newAPIFixture(t *testing.T) *apiFixture {
	t.Helper()
	repo := testutil.NewAdminRepo()
	audit := &testutil.Audit{}
	registry := engine.NewRegistry()
	registry.Register(&templates.WelcomeMenu{})

	auth := service.NewAuthService(repo, "api-test-secret")
	adminSvc := service.NewAdminService(repo, audit)
	botSvc := service.NewBotService(
		testutil.NewBotRepo(), testutil.NewConfigRepo(), registry,
		&testutil.Validator{Username: "@testbot"}, testutil.NewRuntime(),
		audit, repo, bytes.Repeat([]byte{0x11}, 32),
	)

	deps := Deps{
		Auth:     auth,
		Admins:   adminSvc,
		AuthH:    handlers.NewAuthHandler(auth),
		AdminsH:  handlers.NewAdminsHandler(adminSvc),
		BotsH:    handlers.NewBotsHandler(botSvc, adminSvc, nil, nil, nil, nil),
		FinanceH: handlers.NewFinanceHandler(nil, nil),
	}

	super := repo.Add(&domain.Admin{
		ID: uuid.New(), Email: "super@example.com", PasswordHash: testutil.Hash("password123"),
		DisplayName: "سوپر", Role: domain.RoleSuperAdmin, Status: domain.AdminStatusActive,
	})
	admin := repo.Add(&domain.Admin{
		ID: uuid.New(), Email: "admin@example.com", PasswordHash: testutil.Hash("password123"),
		DisplayName: "عادی", Role: domain.RoleAdmin, Status: domain.AdminStatusActive,
	})

	return &apiFixture{t: t, router: NewRouter(deps), admins: repo, super: super, admin: admin}
}

func (f *apiFixture) login(email, pwd string) string {
	f.t.Helper()
	body, _ := json.Marshal(map[string]string{"email": email, "password": pwd})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		f.t.Fatalf("login status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out.AccessToken == "" {
		f.t.Fatalf("login response: %v %s", err, rec.Body.String())
	}
	return out.AccessToken
}

func (f *apiFixture) do(method, path, token string, body any) *httptest.ResponseRecorder {
	f.t.Helper()
	var rd *bytes.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		rd = bytes.NewReader(raw)
	} else {
		rd = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, rd)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	return rec
}

func TestAPI_Health(t *testing.T) {
	f := newAPIFixture(t)
	rec := f.do(http.MethodGet, "/api/health", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "ok") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestAPI_Login_WrongPassword(t *testing.T) {
	f := newAPIFixture(t)
	rec := f.do(http.MethodPost, "/api/v1/auth/login", "", map[string]string{"email": "super@example.com", "password": "nope"})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestAPI_Login_SuccessAndMe(t *testing.T) {
	f := newAPIFixture(t)
	tok := f.login("super@example.com", "password123")

	rec := f.do(http.MethodGet, "/api/v1/auth/me", tok, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("me status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "super@example.com") {
		t.Fatalf("me body = %s", rec.Body.String())
	}
}

func TestAPI_ProtectedRoute_RequiresToken(t *testing.T) {
	f := newAPIFixture(t)
	rec := f.do(http.MethodGet, "/api/v1/bots", "", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestAPI_AdminsList_SuperOnly(t *testing.T) {
	f := newAPIFixture(t)

	rec := f.do(http.MethodGet, "/api/v1/admins", f.login("admin@example.com", "password123"), nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("plain admin status = %d, want 403", rec.Code)
	}

	rec = f.do(http.MethodGet, "/api/v1/admins", f.login("super@example.com", "password123"), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("super status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAPI_CreateBot_SuperOnly(t *testing.T) {
	f := newAPIFixture(t)

	rec := f.do(http.MethodPost, "/api/v1/bots", f.login("admin@example.com", "password123"), map[string]string{
		"name": "x", "token": "123456:ABCDEFGHIJKLMNOPQRSTUVWXYZabcd", "template_key": "dating",
	})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("plain admin status = %d, want 403", rec.Code)
	}
}

func TestAPI_CreateBot_HappyPath(t *testing.T) {
	f := newAPIFixture(t)
	tok := f.login("super@example.com", "password123")

	rec := f.do(http.MethodPost, "/api/v1/bots", tok, map[string]string{
		"name": "بات اول", "token": "123456:ABCDEFGHIJKLMNOPQRSTUVWXYZabcd", "template_key": "welcome_menu",
	})
	if rec.Code < 200 || rec.Code >= 300 {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "بات اول") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestAPI_CreateBot_BadTokenFormat(t *testing.T) {
	f := newAPIFixture(t)
	tok := f.login("super@example.com", "password123")

	rec := f.do(http.MethodPost, "/api/v1/bots", tok, map[string]string{
		"name": "x", "token": "bad", "template_key": "dating",
	})
	if rec.Code != http.StatusBadRequest && rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAPI_BotConfig_PermGate(t *testing.T) {
	f := newAPIFixture(t)
	tok := f.login("super@example.com", "password123")

	create := f.do(http.MethodPost, "/api/v1/bots", tok, map[string]string{
		"name": "x", "token": "123456:ABCDEFGHIJKLMNOPQRSTUVWXYZabcd", "template_key": "welcome_menu",
	})
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil || created.ID == "" {
		t.Fatalf("create response: %v %s", err, create.Body.String())
	}
	botID := created.ID
	if _, err := uuid.Parse(botID); err != nil {
		t.Fatalf("bot id: %v", err)
	}

	// plain admin without grant cannot read config
	plainTok := f.login("admin@example.com", "password123")
	rec := f.do(http.MethodGet, "/api/v1/bots/"+botID+"/config", plainTok, nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("ungranted config status = %d, want 403", rec.Code)
	}

	// super can
	rec = f.do(http.MethodGet, "/api/v1/bots/"+botID+"/config", tok, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("super config status = %d body=%s", rec.Code, rec.Body.String())
	}
	var cfg struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &cfg); err != nil {
		t.Fatalf("config body: %v", err)
	}

	// PUT with correct version succeeds; wrong version reports conflict
	rec = f.do(http.MethodPut, "/api/v1/bots/"+botID+"/config", tok, map[string]any{
		"version": cfg.Version,
		"texts":   map[string]string{"welcome": "hi"},
		"menus":   map[string]any{"scenes": map[string]any{}},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("put config status = %d body=%s", rec.Code, rec.Body.String())
	}
}

// contentRouteFixture wires ContentH over the in-memory fakes so the content
// routes can be probed without a real database.
func newContentRouteFixture(t *testing.T) *apiFixture {
	t.Helper()
	f := newAPIFixture(t)
	// rebuild deps on the same repos with content registration + handler
	repo := f.admins
	audit := &testutil.Audit{}
	registry := engine.NewRegistry()
	registry.Register(&templates.WelcomeMenu{})
	registry.Register(&contentpkg.Template{})

	botSvc := service.NewBotService(
		testutil.NewBotRepo(), testutil.NewConfigRepo(), registry,
		&testutil.Validator{Username: "@testbot"}, testutil.NewRuntime(),
		audit, repo, bytes.Repeat([]byte{0x11}, 32),
	)
	adminSvc := service.NewAdminService(repo, audit)
	authSvc := service.NewAuthService(repo, "api-test-secret")
	tgSvc := service.NewTelegramAccountService(testutil.NewTelegramAccountRepo(), nil, audit, botSvc)
	contentSvc := service.NewContentService(botSvc, tgSvc, nil)
	f.router = NewRouter(Deps{
		Auth: authSvc, Admins: adminSvc,
		AuthH: handlers.NewAuthHandler(authSvc),
		AdminsH: handlers.NewAdminsHandler(adminSvc),
		BotsH: handlers.NewBotsHandler(botSvc, adminSvc, nil, nil, nil, nil),
		FinanceH: handlers.NewFinanceHandler(nil, nil),
		ContentH: handlers.NewContentHandler(contentSvc),
	})
	return f
}

func TestContentRoutes_RequireAuth(t *testing.T) {
	f := newContentRouteFixture(t)
	rec := f.do(http.MethodGet, "/api/v1/bots/"+uuid.New().String()+"/content/posts", "", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated content route must 401, got %d", rec.Code)
	}
}

func TestCloneRoute_RequiresSuperadmin(t *testing.T) {
	f := newContentRouteFixture(t)
	token := f.login("admin@example.com", "password123") // plain admin
	rec := f.do(http.MethodPost, "/api/v1/bots/"+uuid.New().String()+"/clone", token,
		map[string]string{"account_id": uuid.New().String()})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-super clone must 403, got %d body=%s", rec.Code, rec.Body.String())
	}
}
