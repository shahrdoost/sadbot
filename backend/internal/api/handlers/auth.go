package handlers

import (
	"encoding/json"
	"net/http"

	"sadbot/internal/api/middleware"
	"sadbot/internal/service"
	"sadbot/pkg/respond"
)

type AuthHandler struct{ auth *service.AuthService }

func NewAuthHandler(auth *service.AuthService) *AuthHandler { return &AuthHandler{auth: auth} }

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		respond.Error(w, http.StatusBadRequest, "درخواست نامعتبر است")
		return
	}
	token, admin, err := h.auth.Login(r.Context(), in.Email, in.Password)
	if err != nil {
		respond.Error(w, http.StatusUnauthorized, err.Error())
		return
	}
	respond.JSON(w, http.StatusOK, map[string]any{
		"access_token": token,
		"admin":        admin,
	})
}

func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	admin := middleware.AdminFrom(r.Context())
	respond.JSON(w, http.StatusOK, admin)
}
