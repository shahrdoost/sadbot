package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"sadbot/internal/api/middleware"
	"sadbot/internal/domain"
	"sadbot/internal/service"
	"sadbot/pkg/respond"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// ModerationHandler serves the per-bot admin lists for confessions and
// support tickets (approve/reject/reply/close).
type ModerationHandler struct {
	svc *service.ModerationService
}

func NewModerationHandler(svc *service.ModerationService) *ModerationHandler {
	return &ModerationHandler{svc: svc}
}

func modBotID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "botID"))
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "شناسهٔ بات نامعتبر")
		return uuid.Nil, false
	}
	return id, true
}

func pageParam(r *http.Request) int {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	return page
}

func (h *ModerationHandler) ListConfessions(w http.ResponseWriter, r *http.Request) {
	botID, ok := modBotID(w, r)
	if !ok {
		return
	}
	items, total, err := h.svc.ListConfessions(r.Context(), botID, r.URL.Query().Get("status"), pageParam(r))
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	if items == nil {
		items = []domain.Confession{}
	}
	respond.JSON(w, http.StatusOK, map[string]any{"confessions": items, "total": total})
}

func (h *ModerationHandler) DecideConfession(w http.ResponseWriter, r *http.Request, approve bool) {
	botID, ok := modBotID(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "شناسهٔ نامعتبر")
		return
	}
	actor := middleware.AdminFrom(r.Context())
	var fnErr error
	if approve {
		fnErr = h.svc.ApproveConfession(r.Context(), botID, id, actor.ID)
	} else {
		fnErr = h.svc.RejectConfession(r.Context(), botID, id, actor.ID)
	}
	if fnErr != nil {
		respond.Error(w, http.StatusBadRequest, fnErr.Error())
		return
	}
	respond.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *ModerationHandler) ApproveConfession(w http.ResponseWriter, r *http.Request) {
	h.DecideConfession(w, r, true)
}

func (h *ModerationHandler) RejectConfession(w http.ResponseWriter, r *http.Request) {
	h.DecideConfession(w, r, false)
}

func (h *ModerationHandler) ListTickets(w http.ResponseWriter, r *http.Request) {
	botID, ok := modBotID(w, r)
	if !ok {
		return
	}
	items, total, err := h.svc.ListTickets(r.Context(), botID, r.URL.Query().Get("status"), pageParam(r))
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	if items == nil {
		items = []domain.SupportTicket{}
	}
	respond.JSON(w, http.StatusOK, map[string]any{"tickets": items, "total": total})
}

func (h *ModerationHandler) ReplyTicket(w http.ResponseWriter, r *http.Request) {
	botID, ok := modBotID(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "شناسهٔ نامعتبر")
		return
	}
	var in struct {
		Reply string `json:"reply"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		respond.Error(w, http.StatusBadRequest, "درخواست نامعتبر")
		return
	}
	actor := middleware.AdminFrom(r.Context())
	if err := h.svc.ReplyTicket(r.Context(), botID, id, actor.ID, in.Reply); err != nil {
		respond.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	respond.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *ModerationHandler) CloseTicket(w http.ResponseWriter, r *http.Request) {
	botID, ok := modBotID(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "شناسهٔ نامعتبر")
		return
	}
	if err := h.svc.CloseTicket(r.Context(), botID, id); err != nil {
		respond.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	respond.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
