package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"sadbot/internal/domain"
	"sadbot/internal/service"
	"sadbot/internal/testutil"

	"github.com/google/uuid"
)

func newAuthedStack(t *testing.T) (*service.AuthService, *testutil.AdminRepo, http.Handler, *domain.Admin) {
	t.Helper()
	repo := testutil.NewAdminRepo()
	admin := repo.Add(&domain.Admin{
		ID:           uuid.New(),
		Email:        "a@example.com",
		PasswordHash: testutil.Hash("password123"),
		DisplayName:  "تست",
		Role:         domain.RoleSuperAdmin,
		Status:       domain.AdminStatusActive,
	})
	auth := service.NewAuthService(repo, "mw-test-secret")

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		admin := AdminFrom(r.Context())
		email := ""
		if admin != nil {
			email = admin.Email
		}
		w.Write([]byte(email))
	})

	return auth, repo, NewAuthenticator(auth).Handle(next), admin
}

func doReq(t *testing.T, h http.Handler, authHeader string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func loginToken(t *testing.T, auth *service.AuthService, email, pwd string) string {
	t.Helper()
	tok, _, err := auth.Login(context.Background(), email, pwd)
	if err != nil {
		t.Fatalf("login for token: %v", err)
	}
	return tok
}

func TestAuthenticator_MissingHeader(t *testing.T) {
	auth, _, h, _ := newAuthedStack(t)
	_ = auth
	rec := doReq(t, h, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestAuthenticator_NotBearer(t *testing.T) {
	_, _, h, _ := newAuthedStack(t)
	rec := doReq(t, h, "Basic abc")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestAuthenticator_GarbageToken(t *testing.T) {
	_, _, h, _ := newAuthedStack(t)
	rec := doReq(t, h, "Bearer garbage")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestAuthenticator_ValidTokenInjectsAdmin(t *testing.T) {
	auth, _, h, admin := newAuthedStack(t)
	tok := loginToken(t, auth, admin.Email, "password123")

	rec := doReq(t, h, "Bearer "+tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != admin.Email {
		t.Fatalf("handler saw email %q, want %q", rec.Body.String(), admin.Email)
	}
}

func TestAuthenticator_TokenForDeletedAdmin(t *testing.T) {
	auth, repo, h, admin := newAuthedStack(t)
	tok := loginToken(t, auth, admin.Email, "password123")
	_ = repo.Delete(context.Background(), admin.ID)

	rec := doReq(t, h, "Bearer "+tok)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 for deleted admin", rec.Code)
	}
}

func TestAdminFrom_EmptyContext(t *testing.T) {
	if AdminFrom(context.Background()) != nil {
		t.Fatal("AdminFrom on empty context must be nil")
	}
}
