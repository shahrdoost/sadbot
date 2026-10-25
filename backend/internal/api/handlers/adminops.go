package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"sadbot/internal/api/middleware"
	"sadbot/internal/service"
	"sadbot/pkg/respond"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// AdminOpsHandler serves the panel's per-user admin actions: coin adjustments,
// user reset, blocking, streak reset, direct messages, and the stuck-user scan.
type AdminOpsHandler struct {
	svc *service.AdminOpsService
}

func NewAdminOpsHandler(svc *service.AdminOpsService) *AdminOpsHandler {
	return &AdminOpsHandler{svc: svc}
}

// adminUserID parses the bot and user ids from the route.
func adminUserID(w http.ResponseWriter, r *http.Request) (uuid.UUID, int64, bool) {
	botID, err := uuid.Parse(chi.URLParam(r, "botID"))
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "شناسهٔ بات نامعتبر")
		return uuid.Nil, 0, false
	}
	userID, err := strconv.ParseInt(chi.URLParam(r, "userID"), 10, 64)
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "شناسهٔ کاربر نامعتبر")
		return uuid.Nil, 0, false
	}
	return botID, userID, true
}

// UserDetail returns the forensics view for one end user.
func (h *AdminOpsHandler) UserDetail(w http.ResponseWriter, r *http.Request) {
	botID, userID, ok := adminUserID(w, r)
	if !ok {
		return
	}
	d, err := h.svc.UserDetail(r.Context(), botID, userID)
	if err != nil {
		respond.Error(w, http.StatusNotFound, err.Error())
		return
	}
	respond.JSON(w, http.StatusOK, d)
}

// AdjustCoins credits or debits coins for one user.
func (h *AdminOpsHandler) AdjustCoins(w http.ResponseWriter, r *http.Request) {
	botID, userID, ok := adminUserID(w, r)
	if !ok {
		return
	}
	var in struct {
		Delta int `json:"delta"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		respond.Error(w, http.StatusBadRequest, "درخواست نامعتبر است")
		return
	}
	if in.Delta == 0 || in.Delta > 1_000_000 || in.Delta < -1_000_000 {
		respond.Error(w, http.StatusBadRequest, "مقدار سکه باید بین ۱ تا ۱۰۰۰۰۰۰ باشد")
		return
	}
	actor := middleware.AdminFrom(r.Context())
	coins, err := h.svc.AdjustCoins(r.Context(), actor.ID, botID, userID, in.Delta)
	if err != nil {
		respond.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	respond.JSON(w, http.StatusOK, map[string]any{"coins": coins})
}

// ResetUser wipes the user's dating content and conversation state.
func (h *AdminOpsHandler) ResetUser(w http.ResponseWriter, r *http.Request) {
	botID, userID, ok := adminUserID(w, r)
	if !ok {
		return
	}
	actor := middleware.AdminFrom(r.Context())
	if err := h.svc.ResetUser(r.Context(), actor.ID, botID, userID); err != nil {
		respond.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	respond.JSON(w, http.StatusOK, map[string]any{"ok": true})
}

// SetBlocked blocks or unblocks a user.
func (h *AdminOpsHandler) SetBlocked(w http.ResponseWriter, r *http.Request) {
	botID, userID, ok := adminUserID(w, r)
	if !ok {
		return
	}
	var in struct {
		Blocked bool   `json:"blocked"`
		Reason  string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		respond.Error(w, http.StatusBadRequest, "درخواست نامعتبر است")
		return
	}
	if len([]rune(strings.TrimSpace(in.Reason))) > 200 {
		respond.Error(w, http.StatusBadRequest, "دلیل مسدودسازی بیش از حد طولانی است")
		return
	}
	actor := middleware.AdminFrom(r.Context())
	if err := h.svc.SetBlocked(r.Context(), actor.ID, botID, userID, in.Blocked, in.Reason); err != nil {
		respond.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	respond.JSON(w, http.StatusOK, map[string]any{"ok": true, "blocked": in.Blocked})
}

// ResetStreak clears a user's daily streak counters.
func (h *AdminOpsHandler) ResetStreak(w http.ResponseWriter, r *http.Request) {
	botID, userID, ok := adminUserID(w, r)
	if !ok {
		return
	}
	actor := middleware.AdminFrom(r.Context())
	if err := h.svc.ResetStreak(r.Context(), actor.ID, botID, userID); err != nil {
		respond.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	respond.JSON(w, http.StatusOK, map[string]any{"ok": true})
}

// DirectMessage sends a panel-authored message to one user through the bot.
func (h *AdminOpsHandler) DirectMessage(w http.ResponseWriter, r *http.Request) {
	botID, userID, ok := adminUserID(w, r)
	if !ok {
		return
	}
	var in struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		respond.Error(w, http.StatusBadRequest, "درخواست نامعتبر است")
		return
	}
	if len([]rune(strings.TrimSpace(in.Text))) > 4000 {
		respond.Error(w, http.StatusBadRequest, "متن پیام بیش از حد طولانی است")
		return
	}
	actor := middleware.AdminFrom(r.Context())
	if err := h.svc.DirectMessage(r.Context(), actor.ID, botID, userID, in.Text); err != nil {
		respond.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	respond.JSON(w, http.StatusOK, map[string]any{"ok": true})
}

// StuckUsers lists users stuck in searching/chatting.
func (h *AdminOpsHandler) StuckUsers(w http.ResponseWriter, r *http.Request) {
	botID, ok := modBotID(w, r)
	if !ok {
		return
	}
	rows, err := h.svc.StuckUsers(r.Context(), botID)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	respond.JSON(w, http.StatusOK, map[string]any{"users": rows, "total": len(rows)})
}

// ResetStuck releases every stuck user and their waiting partners.
func (h *AdminOpsHandler) ResetStuck(w http.ResponseWriter, r *http.Request) {
	botID, ok := modBotID(w, r)
	if !ok {
		return
	}
	actor := middleware.AdminFrom(r.Context())
	n, err := h.svc.ResetStuck(r.Context(), actor.ID, botID)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	respond.JSON(w, http.StatusOK, map[string]any{"reset": n})
}

// ResetAllChats moves every searching/chatting user back to 'joined'. Requires
// an explicit confirmation token so a stray click cannot disturb every chat.
func (h *AdminOpsHandler) ResetAllChats(w http.ResponseWriter, r *http.Request) {
	botID, ok := modBotID(w, r)
	if !ok {
		return
	}
	var in struct {
		Confirm string `json:"confirm"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		respond.Error(w, http.StatusBadRequest, "درخواست نامعتبر است")
		return
	}
	if in.Confirm != "confirm" {
		respond.Error(w, http.StatusBadRequest, "برای ریست همه، عبارت confirm را بفرستید")
		return
	}
	actor := middleware.AdminFrom(r.Context())
	n, err := h.svc.ResetAllChats(r.Context(), actor.ID, botID)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	respond.JSON(w, http.StatusOK, map[string]any{"reset": n})
}

// --- chat surveillance ---

// ActiveChats lists the live dating conversations of a bot.
func (h *AdminOpsHandler) ActiveChats(w http.ResponseWriter, r *http.Request) {
	botID, ok := modBotID(w, r)
	if !ok {
		return
	}
	chats, err := h.svc.ActiveChats(r.Context(), botID)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	respond.JSON(w, http.StatusOK, map[string]any{"chats": chats, "total": len(chats)})
}

// ChatTranscript returns the recorded messages of one conversation, given the
// two internal user ids.
func (h *AdminOpsHandler) ChatTranscript(w http.ResponseWriter, r *http.Request) {
	botID, ok := modBotID(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	userA, errA := strconv.ParseInt(q.Get("a"), 10, 64)
	userB, errB := strconv.ParseInt(q.Get("b"), 10, 64)
	if errA != nil || errB != nil {
		respond.Error(w, http.StatusBadRequest, "ایدی کاربران نامعتبر است")
		return
	}
	rows, err := h.svc.ChatTranscript(r.Context(), botID, userA, userB)
	if err != nil {
		respond.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	respond.JSON(w, http.StatusOK, map[string]any{"messages": rows, "total": len(rows)})
}

// RecentChatMessages is the global feed of chat-scene messages.
func (h *AdminOpsHandler) RecentChatMessages(w http.ResponseWriter, r *http.Request) {
	botID, ok := modBotID(w, r)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	rows, err := h.svc.RecentChatMessages(r.Context(), botID, limit)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	respond.JSON(w, http.StatusOK, map[string]any{"messages": rows, "total": len(rows)})
}

// AIHarness returns the AI companion overview: enabled switch plus the users
// currently in the AI scene (with their message counts).
func (h *AdminOpsHandler) AIHarness(w http.ResponseWriter, r *http.Request) {
	botID, ok := modBotID(w, r)
	if !ok {
		return
	}
	harness, err := h.svc.AIHarness(r.Context(), botID)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	respond.JSON(w, http.StatusOK, harness)
}

// AIUserDetail returns one user's AI conversation history.
func (h *AdminOpsHandler) AIUserDetail(w http.ResponseWriter, r *http.Request) {
	botID, userID, ok := adminUserID(w, r)
	if !ok {
		return
	}
	d, err := h.svc.AIUserDetail(r.Context(), botID, userID)
	if err != nil {
		respond.Error(w, http.StatusNotFound, err.Error())
		return
	}
	respond.JSON(w, http.StatusOK, d)
}

// SetAIEnabled toggles the ai_enabled flag (panel writes; bot reads).
func (h *AdminOpsHandler) SetAIEnabled(w http.ResponseWriter, r *http.Request) {
	botID, ok := modBotID(w, r)
	if !ok {
		return
	}
	var in struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		respond.Error(w, http.StatusBadRequest, "درخواست نامعتبر است")
		return
	}
	actor := middleware.AdminFrom(r.Context())
	cfg, err := h.svc.SetAIFeature(r.Context(), actor, botID, "ai_enabled", in.Enabled)
	if err != nil {
		respond.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	respond.JSON(w, http.StatusOK, cfg)
}

// ScanTelegramBlocks probes inactive users for bot-blocks (403 on send).
func (h *AdminOpsHandler) ScanTelegramBlocks(w http.ResponseWriter, r *http.Request) {
	botID, ok := modBotID(w, r)
	if !ok {
		return
	}
	var in struct {
		Days int `json:"days"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in) // empty body → defaults
	actor := middleware.AdminFrom(r.Context())
	res, err := h.svc.ScanTelegramBlocks(r.Context(), actor, botID, in.Days)
	if err != nil {
		respond.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	respond.JSON(w, http.StatusOK, res)
}

// BlockPairs lists who-blocked-whom relationships.
func (h *AdminOpsHandler) BlockPairs(w http.ResponseWriter, r *http.Request) {
	botID, ok := modBotID(w, r)
	if !ok {
		return
	}
	rows, err := h.svc.BlockPairs(r.Context(), botID)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	respond.JSON(w, http.StatusOK, map[string]any{"blocks": rows, "total": len(rows)})
}
