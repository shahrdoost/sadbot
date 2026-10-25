package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"sadbot/internal/api/middleware"
	"sadbot/internal/dating"
	"sadbot/internal/repository/postgres"
	"sadbot/internal/service"
	"sadbot/pkg/respond"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// PanelHandler serves per-bot admin extras: dating user cards, broadcast,
// discount codes, and confession seeds.
type PanelHandler struct {
	dating     *postgres.DatingRepo
	broadcast  *service.BroadcastService
	discounts  *postgres.DiscountRepo
	users      *postgres.BotUserRepo
	moderation *postgres.ModerationRepo
}

func NewPanelHandler(dating *postgres.DatingRepo, broadcast *service.BroadcastService, discounts *postgres.DiscountRepo, users *postgres.BotUserRepo, moderation *postgres.ModerationRepo) *PanelHandler {
	return &PanelHandler{dating: dating, broadcast: broadcast, discounts: discounts, users: users, moderation: moderation}
}

func panelBotID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "botID"))
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "شناسهٔ بات نامعتبر")
		return uuid.Nil, false
	}
	return id, true
}

// DatingUsers lists bot end users joined with dating profile fields.
func (h *PanelHandler) DatingUsers(w http.ResponseWriter, r *http.Request) {
	botID, ok := panelBotID(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	size, _ := strconv.Atoi(q.Get("size"))
	cards, total, err := h.dating.UserCards(r.Context(), botID, q.Get("q"), page, size, q.Get("ghost") == "only")
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	if cards == nil {
		cards = []postgres.DatingUserCard{}
	}
	respond.JSON(w, http.StatusOK, map[string]any{"users": cards, "total": total})
}

// BroadcastPreview returns the audience count for the broadcast composer.
func (h *PanelHandler) BroadcastPreview(w http.ResponseWriter, r *http.Request) {
	botID, ok := panelBotID(w, r)
	if !ok {
		return
	}
	n, err := h.broadcast.Preview(r.Context(), botID)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	respond.JSON(w, http.StatusOK, map[string]any{"audience": n})
}

// StartBroadcast kicks off an async broadcast run.
func (h *PanelHandler) StartBroadcast(w http.ResponseWriter, r *http.Request) {
	botID, ok := panelBotID(w, r)
	if !ok {
		return
	}
	var in struct {
		Text   string  `json:"text"`
		Photo  *string `json:"photo"`
		Footer *string `json:"footer"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || strings.TrimSpace(in.Text) == "" {
		respond.Error(w, http.StatusBadRequest, "متن اعلان خالی است")
		return
	}
	if in.Photo != nil && strings.TrimSpace(*in.Photo) == "" {
		in.Photo = nil
	}
	if in.Footer != nil && strings.TrimSpace(*in.Footer) == "" {
		in.Footer = nil
	}
	actor := middleware.AdminFrom(r.Context())
	run, err := h.broadcast.Start(r.Context(), botID, actor.ID, strings.TrimSpace(in.Text), deref(in.Photo), deref(in.Footer))
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	respond.JSON(w, http.StatusAccepted, run)
}

// BroadcastRuns lists recent broadcast runs of a bot.
func (h *PanelHandler) BroadcastRuns(w http.ResponseWriter, r *http.Request) {
	botID, ok := panelBotID(w, r)
	if !ok {
		return
	}
	runs, err := h.broadcast.List(r.Context(), botID)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	if runs == nil {
		runs = []postgres.BroadcastRun{}
	}
	respond.JSON(w, http.StatusOK, map[string]any{"runs": runs})
}

// ListDiscounts lists discount codes of a bot.
func (h *PanelHandler) ListDiscounts(w http.ResponseWriter, r *http.Request) {
	botID, ok := panelBotID(w, r)
	if !ok {
		return
	}
	items, err := h.discounts.ListByBot(r.Context(), botID)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	if items == nil {
		items = []postgres.DiscountCode{}
	}
	respond.JSON(w, http.StatusOK, map[string]any{"discounts": items})
}

// CreateDiscount creates a discount code.
func (h *PanelHandler) CreateDiscount(w http.ResponseWriter, r *http.Request) {
	botID, ok := panelBotID(w, r)
	if !ok {
		return
	}
	var in struct {
		Code      string `json:"code"`
		Kind      string `json:"kind"`
		Value     int    `json:"value"`
		MaxUses   int    `json:"max_uses"`
		ExpiresAt string `json:"expires_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		respond.Error(w, http.StatusBadRequest, "درخواست نامعتبر است")
		return
	}
	in.Code = strings.TrimSpace(in.Code)
	if in.Code == "" || in.Value <= 0 {
		respond.Error(w, http.StatusBadRequest, "کد و مقدار الزامی است")
		return
	}
	switch in.Kind {
	case "percent", "flat_toman", "coin_bonus":
	default:
		respond.Error(w, http.StatusBadRequest, "نوع کد نامعتبر است")
		return
	}
	if in.Kind == "percent" && in.Value > 100 {
		respond.Error(w, http.StatusBadRequest, "درصد باید ۱ تا ۱۰۰ باشد")
		return
	}
	var exp *time.Time
	if in.ExpiresAt != "" {
		t, err := time.Parse(time.RFC3339, in.ExpiresAt)
		if err != nil {
			respond.Error(w, http.StatusBadRequest, "تاریخ انقضا نامعتبر است")
			return
		}
		exp = &t
	}
	d, err := h.discounts.Create(r.Context(), botID, in.Code, in.Kind, in.Value, in.MaxUses, exp)
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "ساخت کد شکست خورد (کد تکراری؟)")
		return
	}
	respond.JSON(w, http.StatusCreated, d)
}

// DeleteDiscount removes a discount code.
func (h *PanelHandler) DeleteDiscount(w http.ResponseWriter, r *http.Request) {
	botID, ok := panelBotID(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "شناسه نامعتبر")
		return
	}
	if err := h.discounts.Delete(r.Context(), botID, id); err != nil {
		respond.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	respond.JSON(w, http.StatusOK, map[string]any{"ok": true})
}

// SeedConfessions inserts the kpkchat cold-start confession texts as pending
// rows (published only after admin approval, unlike kpkchat's auto-post).
func (h *PanelHandler) SeedConfessions(w http.ResponseWriter, r *http.Request) {
	botID, ok := panelBotID(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	// synthetic system author so the FK holds without a real user
	seedUser, err := h.users.Upsert(ctx, botID, -30001, nil, strPtr("کاربر نمونه"), nil)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	inserted := 0
	for _, text := range dating.SeedConfessions {
		if _, err := h.moderation.InsertConfession(ctx, botID, seedUser.ID, text); err != nil {
			respond.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		inserted++
	}
	respond.JSON(w, http.StatusCreated, map[string]any{"inserted": inserted})
}

func strPtr(s string) *string { return &s }

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
