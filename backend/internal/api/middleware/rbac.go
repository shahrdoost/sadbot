package middleware

import (
	"net/http"

	"sadbot/internal/service"
	"sadbot/pkg/respond"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// RequireSuper asserts the caller is a superadmin.
func RequireSuper(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		admin := AdminFrom(r.Context())
		if admin == nil || !admin.IsSuperAdmin() {
			respond.Error(w, http.StatusForbidden, "دسترسی superadmin لازم است")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireBotPerm asserts the caller may perform perm on the bot in the :botID URL param.
func RequireBotPerm(admins *service.AdminService, perm string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			admin := AdminFrom(r.Context())
			botID, err := uuid.Parse(chi.URLParam(r, "botID"))
			if err != nil {
				respond.Error(w, http.StatusBadRequest, "شناسهٔ بات نامعتبر است")
				return
			}
			ok, err := admins.Can(r.Context(), admin, botID, perm)
			if err != nil {
				respond.Error(w, http.StatusInternalServerError, "خطا در بررسی دسترسی")
				return
			}
			if !ok {
				respond.Error(w, http.StatusForbidden, "دسترسی ندارید")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
