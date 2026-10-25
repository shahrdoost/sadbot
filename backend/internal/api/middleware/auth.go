package middleware

import (
	"context"
	"net/http"
	"strings"

	"sadbot/internal/domain"
	"sadbot/internal/service"
	"sadbot/pkg/respond"
)

type ctxKey int

const adminKey ctxKey = iota

func AdminFrom(ctx context.Context) *domain.Admin {
	a, _ := ctx.Value(adminKey).(*domain.Admin)
	return a
}

// Authenticator validates the JWT bearer token and injects the admin into context.
type Authenticator struct {
	auth *service.AuthService
}

func NewAuthenticator(auth *service.AuthService) *Authenticator {
	return &Authenticator{auth: auth}
}

func (a *Authenticator) Handle(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Get("Authorization")
		if h == "" || !strings.HasPrefix(h, "Bearer ") {
			respond.Error(w, http.StatusUnauthorized, "احراز هویت لازم است")
			return
		}
		claims, err := a.auth.ParseToken(strings.TrimPrefix(h, "Bearer "))
		if err != nil {
			respond.Error(w, http.StatusUnauthorized, "توکن نامعتبر است")
			return
		}
		admin, err := a.auth.ParseClaims(r.Context(), claims)
		if err != nil {
			respond.Error(w, http.StatusUnauthorized, "حساب یافت نشد")
			return
		}
		ctx := context.WithValue(r.Context(), adminKey, admin)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
