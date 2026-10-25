package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"sadbot/internal/domain"
	"sadbot/internal/service"
	"sadbot/internal/testutil"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func newRBACStack(t *testing.T) (*service.AdminService, *testutil.AdminRepo) {
	t.Helper()
	repo := testutil.NewAdminRepo()
	admins := service.NewAdminService(repo, &testutil.Audit{})
	return admins, repo
}

func serveWithAdmin(h http.Handler, admin *domain.Admin) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if admin != nil {
		ctx := context.WithValue(req.Context(), adminKey, admin)
		req = req.WithContext(ctx)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestRequireSuper_DeniesPlainAdmin(t *testing.T) {
	h := RequireSuper(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := serveWithAdmin(h, &domain.Admin{Role: domain.RoleAdmin})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestRequireSuper_AllowsSuper(t *testing.T) {
	h := RequireSuper(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := serveWithAdmin(h, &domain.Admin{Role: domain.RoleSuperAdmin})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestRequireSuper_DeniesNilAdmin(t *testing.T) {
	h := RequireSuper(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := serveWithAdmin(h, nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func botPermHandler(t *testing.T, admins *service.AdminService, botID string) http.Handler {
	t.Helper()
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := RequireBotPerm(admins, domain.PermBotConfig)(inner)

	// mount with :botID URL param via chi route context
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("botID", botID)
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
		h.ServeHTTP(w, r)
	})
}

func TestRequireBotPerm_InvalidUUID(t *testing.T) {
	admins, _ := newRBACStack(t)
	h := botPermHandler(t, admins, "not-a-uuid")
	rec := serveWithAdmin(h, &domain.Admin{Role: domain.RoleAdmin})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestRequireBotPerm_NoGrant(t *testing.T) {
	admins, _ := newRBACStack(t)
	botID := uuid.New()
	h := botPermHandler(t, admins, botID.String())
	rec := serveWithAdmin(h, &domain.Admin{ID: uuid.New(), Role: domain.RoleAdmin})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestRequireBotPerm_Granted(t *testing.T) {
	admins, repo := newRBACStack(t)
	botID := uuid.New()
	admin := &domain.Admin{ID: uuid.New(), Role: domain.RoleAdmin}
	_, _ = repo.UpsertBotAccess(context.Background(), admin.ID, botID, []string{domain.PermBotConfig}, nil)

	h := botPermHandler(t, admins, botID.String())
	rec := serveWithAdmin(h, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestRequireBotPerm_SuperBypasses(t *testing.T) {
	admins, _ := newRBACStack(t)
	botID := uuid.New()
	h := botPermHandler(t, admins, botID.String())
	rec := serveWithAdmin(h, &domain.Admin{ID: uuid.New(), Role: domain.RoleSuperAdmin})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}
