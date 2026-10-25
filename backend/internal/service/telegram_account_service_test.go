package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"sadbot/internal/domain"
	"sadbot/internal/engine"
	"sadbot/internal/engine/templates"
	"sadbot/internal/testutil"
	"sadbot/internal/tgsidecar"

	"github.com/google/uuid"
)

type fakeTGAccountRepo struct {
	rows map[uuid.UUID]*domain.TelegramAccount
}

func newFakeTGAccountRepo() *fakeTGAccountRepo {
	return &fakeTGAccountRepo{rows: map[uuid.UUID]*domain.TelegramAccount{}}
}

func (f *fakeTGAccountRepo) Create(ctx context.Context, adminID uuid.UUID, label string, tgUserID int64, phone, sidecarID string) (*domain.TelegramAccount, error) {
	a := &domain.TelegramAccount{ID: uuid.New(), AdminID: adminID, Label: label, TgUserID: tgUserID, Phone: phone, SidecarAccountID: sidecarID}
	f.rows[a.ID] = a
	cp := *a
	return &cp, nil
}

func (f *fakeTGAccountRepo) List(ctx context.Context, adminID *uuid.UUID) ([]domain.TelegramAccount, error) {
	out := []domain.TelegramAccount{}
	for _, a := range f.rows {
		if adminID != nil && a.AdminID != *adminID {
			continue
		}
		out = append(out, *a)
	}
	return out, nil
}

func (f *fakeTGAccountRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.TelegramAccount, error) {
	if a, ok := f.rows[id]; ok {
		cp := *a
		return &cp, nil
	}
	return nil, errNotFound
}

func (f *fakeTGAccountRepo) Delete(ctx context.Context, id uuid.UUID) error {
	delete(f.rows, id)
	return nil
}

var errNotFound = errStr("not found")

type errStr string

func (e errStr) Error() string { return string(e) }

func newTGFixture(t *testing.T) (*TelegramAccountService, *fakeTGAccountRepo, *httptest.Server) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /accounts", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"accounts": []any{}})
	})
	mux.HandleFunc("DELETE /accounts/", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})
	mux.HandleFunc("GET /accounts/acc-owner/chats", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"chats": []map[string]any{
			{"chat_id": "-1001", "title": "کانال تست", "type": "channel"},
		}})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	repo := newFakeTGAccountRepo()
	svc := NewTelegramAccountService(repo, tgsidecar.New(srv.URL, "k"), &testutil.Audit{}, nil)
	return svc, repo, srv
}

func tgOwner() *domain.Admin {
	return &domain.Admin{ID: uuid.New(), Email: "owner@example.com", Role: domain.RoleAdmin, Status: domain.AdminStatusActive}
}

func TestTGAccounts_ListScopedToOwner(t *testing.T) {
	svc, repo, _ := newTGFixture(t)
	owner := tgOwner()
	stranger := &domain.Admin{ID: uuid.New(), Role: domain.RoleAdmin, Status: domain.AdminStatusActive}

	_, _ = repo.Create(context.Background(), owner.ID, "owner-acc", 1, "+9890", "acc-owner")
	_, _ = repo.Create(context.Background(), stranger.ID, "stranger-acc", 2, "+9891", "acc-other")

	mine, err := svc.List(context.Background(), owner)
	if err != nil {
		t.Fatal(err)
	}
	if len(mine) != 1 || mine[0].Label != "owner-acc" {
		t.Fatalf("owner list = %+v", mine)
	}

	all, err := svc.List(context.Background(), &domain.Admin{Role: domain.RoleSuperAdmin})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("super list = %+v", all)
	}
}

func TestTGAccounts_ForeignAccountDenied(t *testing.T) {
	svc, repo, _ := newTGFixture(t)
	owner := tgOwner()
	stranger := &domain.Admin{ID: uuid.New(), Role: domain.RoleAdmin, Status: domain.AdminStatusActive}
	acc, _ := repo.Create(context.Background(), owner.ID, "a", 1, "+9890", "acc-owner")

	if _, err := svc.Chats(context.Background(), stranger, acc.ID); err == nil {
		t.Fatal("stranger must not read foreign chats")
	}
	if err := svc.Delete(context.Background(), stranger, acc.ID, nil); err == nil {
		t.Fatal("stranger must not delete foreign account")
	}

	chats, err := svc.Chats(context.Background(), owner, acc.ID)
	if err != nil {
		t.Fatalf("owner chats: %v", err)
	}
	if len(chats) != 1 || chats[0].Title != "کانال تست" {
		t.Fatalf("chats = %+v", chats)
	}
}

func TestTGAccounts_SidecarUnconfigured(t *testing.T) {
	repo := newFakeTGAccountRepo()
	svc := NewTelegramAccountService(repo, nil, &testutil.Audit{}, nil)
	if _, err := svc.StartLogin(context.Background(), "+9890"); err != ErrSidecarUnavailable {
		t.Fatalf("err = %v", err)
	}
}

func TestTGAccounts_CreateBotViaSidecar(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /accounts/acc-owner/botfather/create-bot", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{
			"token":    "123456:ABCDEFGHIJKLMNOPQRSTUVWXYZabcd",
			"username": "mytestbotbot",
		})
	})
	mux.HandleFunc("GET /accounts", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"accounts": []any{}})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	registry := engine.NewRegistry()
	registry.Register(&templates.WelcomeMenu{})
	botsRepo := testutil.NewBotRepo()
	configs := testutil.NewConfigRepo()
	admins := testutil.NewAdminRepo()
	audit := &testutil.Audit{}
	botSvc := NewBotService(botsRepo, configs, registry, &testutil.Validator{Username: "@mytestbotbot"},
		testutil.NewRuntime(), audit, admins, testEncKey())

	repo := newFakeTGAccountRepo()
	tgAudit := &testutil.Audit{}
	svc := NewTelegramAccountService(repo, tgsidecar.New(srv.URL, "k"), tgAudit, botSvc)

	owner := superAdmin()
	acc, _ := repo.Create(context.Background(), owner.ID, "acc", 1, "+9890", "acc-owner")

	bot, err := svc.CreateBot(context.Background(), owner, acc.ID, CreateBotViaAccountInput{
		Name: "بات تست", TemplateKey: domain.TemplateWelcomeMenu,
	}, nil)
	if err != nil {
		t.Fatalf("CreateBot: %v", err)
	}
	if bot.TelegramUsername == nil || *bot.TelegramUsername != "@mytestbotbot" {
		t.Fatalf("bot username = %v", bot.TelegramUsername)
	}
	found := false
	for _, a := range append(audit.Entries, tgAudit.Entries...) {
		if a == "telegram_account.create_bot" {
			found = true
		}
	}
	if !found {
		t.Fatalf("audit = %v + %v", audit.Entries, tgAudit.Entries)
	}
}

func TestTGAccounts_CreateBot_ForeignAccountDenied(t *testing.T) {
	svc, repo, _ := newTGFixture(t)
	owner := tgOwner()
	stranger := &domain.Admin{ID: uuid.New(), Role: domain.RoleAdmin, Status: domain.AdminStatusActive}
	acc, _ := repo.Create(context.Background(), owner.ID, "a", 1, "+9890", "acc-owner")

	if _, err := svc.CreateBot(context.Background(), stranger, acc.ID, CreateBotViaAccountInput{Name: "x"}, nil); err == nil {
		t.Fatal("stranger must not use a foreign account to create bots")
	}
}

func TestTGAccounts_SendMessage(t *testing.T) {
	var gotPath, gotText string
	mux := http.NewServeMux()
	mux.HandleFunc("GET /accounts", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"accounts": []any{}})
	})
	mux.HandleFunc("POST /accounts/acc-owner/chats/-1001/messages", func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		var body struct {
			Text string `json:"text"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotText = body.Text
		_ = json.NewEncoder(w).Encode(map[string]any{
			"message": map[string]any{"id": 3, "text": body.Text, "sender": map[string]any{"id": "42", "is_out": true}, "media": map[string]any{"kind": "none"}},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	repo := newFakeTGAccountRepo()
	audit := &testutil.Audit{}
	svc := NewTelegramAccountService(repo, tgsidecar.New(srv.URL, "k"), audit, nil)
	owner := tgOwner()
	stranger := &domain.Admin{ID: uuid.New(), Role: domain.RoleAdmin, Status: domain.AdminStatusActive}
	acc, _ := repo.Create(context.Background(), owner.ID, "a", 1, "+9890", "acc-owner")

	if _, err := svc.SendMessage(context.Background(), stranger, acc.ID, "-1001", "hi"); err == nil {
		t.Fatal("stranger must not send via a foreign account")
	}
	if gotPath != "" {
		t.Fatal("sidecar must not be called for a foreign account")
	}

	msg, err := svc.SendMessage(context.Background(), owner, acc.ID, "-1001", " سلام ")
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if msg.ID != 3 || gotText != " سلام " || gotPath == "" {
		t.Fatalf("msg=%+v text=%q path=%q", msg, gotText, gotPath)
	}
	if len(audit.Entries) != 1 || audit.Entries[0] != "telegram_account.send_message" {
		t.Fatalf("audit = %v", audit.Entries)
	}
}
