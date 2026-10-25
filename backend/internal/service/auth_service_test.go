package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"sadbot/internal/domain"
	"sadbot/internal/testutil"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

func newTestAuth(t *testing.T) (*AuthService, *testutil.AdminRepo) {
	t.Helper()
	repo := testutil.NewAdminRepo()
	return NewAuthService(repo, "unit-test-secret"), repo
}

func seedAdmin(t *testing.T, repo *testutil.AdminRepo, email, pwd, role, status string) *domain.Admin {
	t.Helper()
	return repo.Add(&domain.Admin{
		ID:           uuid.New(),
		Email:        email,
		PasswordHash: testutil.Hash(pwd),
		DisplayName:  "تست",
		Role:         role,
		Status:       status,
	})
}

func TestLogin_Success(t *testing.T) {
	svc, repo := newTestAuth(t)
	seedAdmin(t, repo, "a@example.com", "password123", domain.RoleSuperAdmin, domain.AdminStatusActive)

	token, admin, err := svc.Login(context.Background(), "a@example.com", "password123")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if token == "" {
		t.Fatal("empty token")
	}
	if admin.Email != "a@example.com" {
		t.Fatalf("admin = %v", admin)
	}

	claims, err := svc.ParseToken(token)
	if err != nil {
		t.Fatalf("ParseToken: %v", err)
	}
	if claims.AdminID != admin.ID || claims.Role != domain.RoleSuperAdmin {
		t.Fatalf("claims = %+v", claims)
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	svc, repo := newTestAuth(t)
	seedAdmin(t, repo, "a@example.com", "password123", domain.RoleAdmin, domain.AdminStatusActive)

	if _, _, err := svc.Login(context.Background(), "a@example.com", "wrong-pass"); err == nil {
		t.Fatal("wrong password must fail")
	}
}

func TestLogin_UnknownEmail(t *testing.T) {
	svc, _ := newTestAuth(t)
	if _, _, err := svc.Login(context.Background(), "nobody@example.com", "password123"); err == nil {
		t.Fatal("unknown email must fail")
	}
}

func TestLogin_DisabledAccount(t *testing.T) {
	svc, repo := newTestAuth(t)
	seedAdmin(t, repo, "a@example.com", "password123", domain.RoleAdmin, domain.AdminStatusDisabled)

	_, _, err := svc.Login(context.Background(), "a@example.com", "password123")
	if err == nil || !strings.Contains(err.Error(), "غیرفعال") {
		t.Fatalf("expected disabled error, got %v", err)
	}
}

func TestParseToken_WrongSecret(t *testing.T) {
	svcA, repo := newTestAuth(t)
	seedAdmin(t, repo, "a@example.com", "password123", domain.RoleAdmin, domain.AdminStatusActive)
	token, _, err := svcA.Login(context.Background(), "a@example.com", "password123")
	if err != nil {
		t.Fatal(err)
	}

	svcB := NewAuthService(repo, "a-completely-different-secret")
	if _, err := svcB.ParseToken(token); err == nil {
		t.Fatal("token signed with another secret must be rejected")
	}
}

func TestParseToken_Garbage(t *testing.T) {
	svc, _ := newTestAuth(t)
	if _, err := svc.ParseToken("not.a.jwt"); err == nil {
		t.Fatal("garbage token must be rejected")
	}
}

func TestParseToken_Expired(t *testing.T) {
	repo := testutil.NewAdminRepo()
	admin := seedAdmin(t, repo, "a@example.com", "password123", domain.RoleAdmin, domain.AdminStatusActive)
	svc := NewAuthService(repo, "unit-test-secret")

	claims := jwt.NewWithClaims(jwt.SigningMethodHS256, TokenClaims{
		AdminID: admin.ID,
		Role:    admin.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
		},
	})
	tok, err := claims.SignedString([]byte("unit-test-secret"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ParseToken(tok); err == nil {
		t.Fatal("expired token must be rejected")
	}
}

func TestParseClaims_LoadsFreshAdmin(t *testing.T) {
	svc, repo := newTestAuth(t)
	admin := seedAdmin(t, repo, "a@example.com", "password123", domain.RoleAdmin, domain.AdminStatusActive)
	claims := &TokenClaims{AdminID: admin.ID, Role: admin.Role}

	got, err := svc.ParseClaims(context.Background(), claims)
	if err != nil {
		t.Fatalf("ParseClaims: %v", err)
	}
	if got.ID != admin.ID {
		t.Fatalf("got admin %v", got)
	}
}

func TestHashPassword_MinLength(t *testing.T) {
	if _, err := HashPassword("short"); err == nil {
		t.Fatal("password < 8 chars must be rejected")
	}
	h, err := HashPassword("longenough")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if bcrypt.CompareHashAndPassword([]byte(h), []byte("longenough")) != nil {
		t.Fatal("hash does not verify against original password")
	}
}
