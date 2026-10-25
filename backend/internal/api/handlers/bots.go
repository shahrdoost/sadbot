package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"sadbot/internal/api/middleware"
	"sadbot/internal/domain"
	"sadbot/internal/repository/postgres"
	"sadbot/internal/service"
	"sadbot/pkg/respond"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type BotsHandler struct {
	svc      *service.BotService
	admins   *service.AdminService
	users    *postgres.BotUserRepo
	events   *postgres.EventRepo
	payments *postgres.PaymentRepo
	finance  *postgres.FinanceRepo
}

func NewBotsHandler(svc *service.BotService, admins *service.AdminService, users *postgres.BotUserRepo, events *postgres.EventRepo, payments *postgres.PaymentRepo, finance *postgres.FinanceRepo) *BotsHandler {
	return &BotsHandler{svc: svc, admins: admins, users: users, events: events, payments: payments, finance: finance}
}

func botIDParam(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "botID"))
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "شناسهٔ بات نامعتبر")
		return uuid.Nil, false
	}
	return id, true
}

func (h *BotsHandler) List(w http.ResponseWriter, r *http.Request) {
	actor := middleware.AdminFrom(r.Context())
	bots, err := h.svc.List(r.Context(), actor)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	if bots == nil {
		bots = []domain.Bot{}
	}
	// annotate running state
	type botRow struct {
		*domain.Bot
		Running bool `json:"running"`
	}
	rows := make([]botRow, 0, len(bots))
	for i := range bots {
		rows = append(rows, botRow{Bot: &bots[i], Running: h.svc.IsRunning(bots[i].ID)})
	}
	respond.JSON(w, http.StatusOK, map[string]any{"bots": rows})
}

func (h *BotsHandler) Create(w http.ResponseWriter, r *http.Request) {
	actor := middleware.AdminFrom(r.Context())
	var in service.CreateBotInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		respond.Error(w, http.StatusBadRequest, "درخواست نامعتبر است")
		return
	}
	bot, err := h.svc.Create(r.Context(), actor, in, clientIP(r))
	if err != nil {
		respond.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	respond.JSON(w, http.StatusCreated, bot)
}

func (h *BotsHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := botIDParam(w, r)
	if !ok {
		return
	}
	actor := middleware.AdminFrom(r.Context())
	bot, err := h.svc.Get(r.Context(), actor, id)
	if err != nil {
		respond.Error(w, http.StatusForbidden, err.Error())
		return
	}
	respond.JSON(w, http.StatusOK, map[string]any{
		"bot":     bot,
		"running": h.svc.IsRunning(id),
	})
}

func (h *BotsHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := botIDParam(w, r)
	if !ok {
		return
	}
	actor := middleware.AdminFrom(r.Context())
	var in service.UpdateBotInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		respond.Error(w, http.StatusBadRequest, "درخواست نامعتبر است")
		return
	}
	bot, err := h.svc.Update(r.Context(), actor, id, in, clientIP(r))
	if err != nil {
		respond.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	respond.JSON(w, http.StatusOK, bot)
}

func (h *BotsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := botIDParam(w, r)
	if !ok {
		return
	}
	actor := middleware.AdminFrom(r.Context())
	if err := h.svc.Delete(r.Context(), actor, id, clientIP(r)); err != nil {
		respond.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	respond.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *BotsHandler) Start(w http.ResponseWriter, r *http.Request) {
	id, ok := botIDParam(w, r)
	if !ok {
		return
	}
	actor := middleware.AdminFrom(r.Context())
	if err := h.svc.Start(r.Context(), actor, id, clientIP(r)); err != nil {
		respond.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	respond.JSON(w, http.StatusOK, map[string]string{"status": "running"})
}

func (h *BotsHandler) Stop(w http.ResponseWriter, r *http.Request) {
	id, ok := botIDParam(w, r)
	if !ok {
		return
	}
	actor := middleware.AdminFrom(r.Context())
	if err := h.svc.Stop(r.Context(), actor, id, clientIP(r)); err != nil {
		respond.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	respond.JSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

func (h *BotsHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	id, ok := botIDParam(w, r)
	if !ok {
		return
	}
	actor := middleware.AdminFrom(r.Context())
	cfg, err := h.svc.GetConfig(r.Context(), actor, id)
	if err != nil {
		respond.Error(w, http.StatusForbidden, err.Error())
		return
	}
	respond.JSON(w, http.StatusOK, cfg)
}

func (h *BotsHandler) PutConfig(w http.ResponseWriter, r *http.Request) {
	id, ok := botIDParam(w, r)
	if !ok {
		return
	}
	actor := middleware.AdminFrom(r.Context())
	var in service.UpdateConfigInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		respond.Error(w, http.StatusBadRequest, "درخواست نامعتبر است")
		return
	}
	cfg, err := h.svc.UpdateConfig(r.Context(), actor, id, in, clientIP(r))
	if err != nil {
		respond.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	respond.JSON(w, http.StatusOK, cfg)
}

// --- bot-scoped data ---

func (h *BotsHandler) Users(w http.ResponseWriter, r *http.Request) {
	id, ok := botIDParam(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	size, _ := strconv.Atoi(q.Get("size"))
	f := postgres.UserFilter{Query: q.Get("q"), Page: page, Size: size}
	if v := q.Get("from"); v != "" {
		f.From = &v
	}
	if v := q.Get("to"); v != "" {
		f.To = &v
	}
	pageRes, err := h.users.List(r.Context(), id, f)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	respond.JSON(w, http.StatusOK, pageRes)
}

func (h *BotsHandler) Events(w http.ResponseWriter, r *http.Request) {
	id, ok := botIDParam(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	size, _ := strconv.Atoi(q.Get("size"))
	f := postgres.EventFilter{Kind: q.Get("kind"), Page: page, Size: size}
	if v := q.Get("from"); v != "" {
		f.From = &v
	}
	if v := q.Get("to"); v != "" {
		f.To = &v
	}
	pageRes, err := h.events.List(r.Context(), id, f)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	respond.JSON(w, http.StatusOK, pageRes)
}

func (h *BotsHandler) Payments(w http.ResponseWriter, r *http.Request) {
	id, ok := botIDParam(w, r)
	if !ok {
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	size, _ := strconv.Atoi(r.URL.Query().Get("size"))
	list, total, err := h.payments.ListByBot(r.Context(), id, page, size)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []domain.Payment{}
	}
	respond.JSON(w, http.StatusOK, map[string]any{"payments": list, "total": total})
}

func (h *BotsHandler) CreatePayment(w http.ResponseWriter, r *http.Request) {
	id, ok := botIDParam(w, r)
	if !ok {
		return
	}
	actor := middleware.AdminFrom(r.Context())
	var in struct {
		UserID      *int64  `json:"user_id"`
		AmountRials int64   `json:"amount_rials"`
		Kind        string  `json:"kind"`
		Note        *string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		respond.Error(w, http.StatusBadRequest, "درخواست نامعتبر است")
		return
	}
	if in.AmountRials < 0 {
		respond.Error(w, http.StatusBadRequest, "مبلغ نامعتبر است")
		return
	}
	if in.Kind == "" {
		in.Kind = domain.PaymentKindManual
	}
	p := &domain.Payment{
		BotID: id, UserID: in.UserID, AmountRials: in.AmountRials,
		Kind: in.Kind, Gateway: "manual", Status: domain.PaymentStatusVerified,
		Note: in.Note, RecordedBy: &actor.ID,
	}
	saved, err := h.payments.Create(r.Context(), p)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	respond.JSON(w, http.StatusCreated, saved)
}

// GetSettings returns the numeric/bool price fields for the panel form.
func (h *BotsHandler) GetSettings(w http.ResponseWriter, r *http.Request) {
	id, ok := botIDParam(w, r)
	if !ok {
		return
	}
	actor := middleware.AdminFrom(r.Context())
	fields, err := h.svc.GetSettings(r.Context(), actor, id)
	if err != nil {
		respond.Error(w, http.StatusForbidden, err.Error())
		return
	}
	respond.JSON(w, http.StatusOK, map[string]any{
		"fields": fields,
		"groups": service.SettingGroups(),
	})
}

// UpdateSettings validates the submitted numeric fields and saves them,
// preserving every other price key.
func (h *BotsHandler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	id, ok := botIDParam(w, r)
	if !ok {
		return
	}
	var in struct {
		Version int            `json:"version"`
		Values  map[string]int `json:"values"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		respond.Error(w, http.StatusBadRequest, "درخواست نامعتبر است")
		return
	}
	actor := middleware.AdminFrom(r.Context())
	cfg, err := h.svc.UpdateSettings(r.Context(), actor, id, in.Version, in.Values, clientIP(r))
	if err != nil {
		respond.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	respond.JSON(w, http.StatusOK, cfg)
}

// GetChannels returns the membership channels for the panel editor.
func (h *BotsHandler) GetChannels(w http.ResponseWriter, r *http.Request) {
	id, ok := botIDParam(w, r)
	if !ok {
		return
	}
	actor := middleware.AdminFrom(r.Context())
	entries, err := h.svc.GetChannels(r.Context(), actor, id)
	if err != nil {
		respond.Error(w, http.StatusForbidden, err.Error())
		return
	}
	respond.JSON(w, http.StatusOK, map[string]any{"channels": entries})
}

// SetChannels replaces the membership channel list.
func (h *BotsHandler) SetChannels(w http.ResponseWriter, r *http.Request) {
	id, ok := botIDParam(w, r)
	if !ok {
		return
	}
	var in struct {
		Version  int                    `json:"version"`
		Channels []service.ChannelEntry `json:"channels"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		respond.Error(w, http.StatusBadRequest, "درخواست نامعتبر است")
		return
	}
	actor := middleware.AdminFrom(r.Context())
	cfg, err := h.svc.SetChannels(r.Context(), actor, id, in.Version, in.Channels, clientIP(r))
	if err != nil {
		respond.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	respond.JSON(w, http.StatusOK, cfg)
}

func (h *BotsHandler) Stats(w http.ResponseWriter, r *http.Request) {
	id, ok := botIDParam(w, r)
	if !ok {
		return
	}
	stats, err := h.finance.BotStats(r.Context(), id)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	respond.JSON(w, http.StatusOK, stats)
}
