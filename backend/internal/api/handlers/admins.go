package handlers

import (
	"encoding/json"
	"net"
	"net/http"

	"sadbot/internal/api/middleware"
	"sadbot/internal/domain"
	"sadbot/internal/service"
	"sadbot/pkg/respond"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type AdminsHandler struct{ svc *service.AdminService }

func NewAdminsHandler(svc *service.AdminService) *AdminsHandler { return &AdminsHandler{svc: svc} }

func clientIP(r *http.Request) *string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return nil
	}
	return &host
}

func (h *AdminsHandler) List(w http.ResponseWriter, r *http.Request) {
	actor := middleware.AdminFrom(r.Context())
	admins, err := h.svc.ListAdmins(r.Context(), actor)
	if err != nil {
		respond.Error(w, http.StatusForbidden, err.Error())
		return
	}
	if admins == nil {
		admins = []domain.Admin{}
	}
	respond.JSON(w, http.StatusOK, map[string]any{"admins": admins})
}

func (h *AdminsHandler) Create(w http.ResponseWriter, r *http.Request) {
	actor := middleware.AdminFrom(r.Context())
	var in struct {
		Email       string `json:"email"`
		Password    string `json:"password"`
		DisplayName string `json:"display_name"`
		Role        string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		respond.Error(w, http.StatusBadRequest, "درخواست نامعتبر است")
		return
	}
	a, err := h.svc.CreateAdmin(r.Context(), actor, in.Email, in.Password, in.DisplayName, in.Role, clientIP(r))
	if err != nil {
		respond.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	respond.JSON(w, http.StatusCreated, a)
}

func (h *AdminsHandler) Update(w http.ResponseWriter, r *http.Request) {
	actor := middleware.AdminFrom(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "شناسه نامعتبر")
		return
	}
	var in struct {
		DisplayName *string `json:"display_name"`
		Role        *string `json:"role"`
		Status      *string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		respond.Error(w, http.StatusBadRequest, "درخواست نامعتبر است")
		return
	}
	a, err := h.svc.UpdateAdmin(r.Context(), actor, id, in.DisplayName, in.Role, in.Status, clientIP(r))
	if err != nil {
		respond.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	respond.JSON(w, http.StatusOK, a)
}

func (h *AdminsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	actor := middleware.AdminFrom(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "شناسه نامعتبر")
		return
	}
	if err := h.svc.DeleteAdmin(r.Context(), actor, id, clientIP(r)); err != nil {
		respond.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	respond.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *AdminsHandler) GetBotAccess(w http.ResponseWriter, r *http.Request) {
	actor := middleware.AdminFrom(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "شناسه نامعتبر")
		return
	}
	accs, err := h.svc.GetBotAccess(r.Context(), actor, id)
	if err != nil {
		respond.Error(w, http.StatusForbidden, err.Error())
		return
	}
	if accs == nil {
		accs = []domain.BotAccess{}
	}
	respond.JSON(w, http.StatusOK, map[string]any{"access": accs})
}

func (h *AdminsHandler) SetBotAccess(w http.ResponseWriter, r *http.Request) {
	actor := middleware.AdminFrom(r.Context())
	adminID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "شناسه نامعتبر")
		return
	}
	botID, err := uuid.Parse(chi.URLParam(r, "botID"))
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "شناسهٔ بات نامعتبر")
		return
	}
	var in struct {
		Permissions []string `json:"permissions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		respond.Error(w, http.StatusBadRequest, "درخواست نامعتبر است")
		return
	}
	acc, err := h.svc.SetBotAccess(r.Context(), actor, adminID, botID, in.Permissions, clientIP(r))
	if err != nil {
		respond.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	respond.JSON(w, http.StatusOK, acc)
}
