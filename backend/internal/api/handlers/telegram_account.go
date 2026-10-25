package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"sadbot/internal/api/middleware"
	"sadbot/internal/domain"
	"sadbot/internal/service"
	"sadbot/internal/tgsidecar"
	"sadbot/pkg/respond"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func jsonDecode(r *http.Request, v any) error {
	return json.NewDecoder(r.Body).Decode(v)
}

type TelegramAccountsHandler struct {
	svc *service.TelegramAccountService
}

func NewTelegramAccountsHandler(svc *service.TelegramAccountService) *TelegramAccountsHandler {
	return &TelegramAccountsHandler{svc: svc}
}

func (h *TelegramAccountsHandler) writeErr(w http.ResponseWriter, err error) {
	writeTgErr(w, err)
}

// writeTgErr maps sidecar/service errors onto HTTP statuses (shared by the
// accounts and content handlers).
func writeTgErr(w http.ResponseWriter, err error) {
	var scErr *tgsidecar.Error
	if errors.As(err, &scErr) {
		switch {
		case scErr.Code == "PASSWORD_NEEDED":
			respond.Error(w, http.StatusUnauthorized, "PASSWORD_NEEDED")
		case scErr.Code == "WRONG_PASSWORD":
			respond.Error(w, http.StatusUnauthorized, "رمز عبور دو مرحله‌ای نادرست است")
		case scErr.Code == "INVALID_CODE":
			respond.Error(w, http.StatusBadRequest, "کد تایید نادرست است")
		case scErr.Code == "NO_FREE_USERNAME":
			respond.Error(w, http.StatusConflict, "آیدی آزادی از روی این نام پیدا نشد؛ نام دیگری یا «پایهٔ آیدی» دستی امتحان کنید")
		case scErr.Code == "BOTFATHER_TIMEOUT":
			respond.Error(w, http.StatusGatewayTimeout, "BotFather پاسخ نداد؛ چند دقیقه بعد دوباره تلاش کنید")
		case strings.HasPrefix(scErr.Code, "BOTFATHER_ERROR"):
			respond.Error(w, http.StatusConflict, strings.TrimPrefix(strings.TrimPrefix(scErr.Code, "BOTFATHER_ERROR:"), "BOTFATHER_ERROR"))
		case scErr.Code == "FLOOD_WAIT":
			respond.Error(w, http.StatusTooManyRequests, "تلگرام درخواست را محدود کرده؛ "+strconv.Itoa(scErr.Seconds)+" ثانیه بعد دوباره تلاش کنید")
		case scErr.Code == "EMPTY_TEXT":
			respond.Error(w, http.StatusBadRequest, "متن پیام خالی است")
		case scErr.Code == "TEXT_TOO_LONG":
			respond.Error(w, http.StatusBadRequest, "متن پیام طولانی است (حداکثر ۴۰۹۶ نویسه)")
		case scErr.Code == "WRITE_FORBIDDEN":
			respond.Error(w, http.StatusForbidden, "امکان ارسال پیام در این چت را ندارید")
		case scErr.Code == "CHANNEL_PRIVATE":
			respond.Error(w, http.StatusForbidden, "این کانال خصوصی است و دسترسی نوشتن ندارید")
		case scErr.Code == "USER_BLOCKED_BOT":
			respond.Error(w, http.StatusBadRequest, "طرف مقابل ربات شما را بلاک کرده است")
		case scErr.Code == "PEER_INVALID":
			respond.Error(w, http.StatusNotFound, "این چت در دسترس نیست")
		case scErr.Code == "RPC_ERROR":
			respond.Error(w, http.StatusBadRequest, "تلگرام این پیام را نپذیرفت")
		case scErr.Status == 404:
			respond.Error(w, http.StatusNotFound, "اکانت در سرویس تلگرام یافت نشد؛ دوباره وارد شوید")
		default:
			respond.Error(w, http.StatusBadGateway, "خطا در ارتباط با سرویس تلگرام")
		}
		return
	}
	if errors.Is(err, service.ErrSidecarUnavailable) {
		respond.Error(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	respond.Error(w, http.StatusBadRequest, err.Error())
}

func (h *TelegramAccountsHandler) StartLogin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Phone string `json:"phone"`
	}
	if err := jsonDecode(r, &in); err != nil || in.Phone == "" {
		respond.Error(w, http.StatusBadRequest, "شماره تلفن لازم است")
		return
	}
	pendingID, err := h.svc.StartLogin(r.Context(), in.Phone)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	respond.JSON(w, http.StatusOK, map[string]any{"ok": true, "pending_id": pendingID})
}

func (h *TelegramAccountsHandler) CompleteLogin(w http.ResponseWriter, r *http.Request) {
	actor := middleware.AdminFrom(r.Context())
	var in service.CompleteLoginInput
	if err := jsonDecode(r, &in); err != nil || in.Phone == "" || in.Code == "" || in.PendingID == "" {
		respond.Error(w, http.StatusBadRequest, "درخواست نامعتبر است")
		return
	}
	acc, err := h.svc.CompleteLogin(r.Context(), actor, in, clientIP(r))
	if err != nil {
		h.writeErr(w, err)
		return
	}
	respond.JSON(w, http.StatusCreated, acc)
}

func (h *TelegramAccountsHandler) List(w http.ResponseWriter, r *http.Request) {
	actor := middleware.AdminFrom(r.Context())
	accounts, err := h.svc.List(r.Context(), actor)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	respond.JSON(w, http.StatusOK, map[string]any{"accounts": accounts})
}

func (h *TelegramAccountsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	actor := middleware.AdminFrom(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "شناسه نامعتبر است")
		return
	}
	if err := h.svc.Delete(r.Context(), actor, id, clientIP(r)); err != nil {
		h.writeErr(w, err)
		return
	}
	respond.JSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// CreateBot drives BotFather from the selected account and creates the bot.
func (h *TelegramAccountsHandler) CreateBot(w http.ResponseWriter, r *http.Request) {
	actor := middleware.AdminFrom(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "شناسه نامعتبر است")
		return
	}
	var in service.CreateBotViaAccountInput
	if err := jsonDecode(r, &in); err != nil || in.Name == "" {
		respond.Error(w, http.StatusBadRequest, "نام بات لازم است")
		return
	}
	bot, err := h.svc.CreateBot(r.Context(), actor, id, in, clientIP(r))
	if err != nil {
		h.writeErr(w, err)
		return
	}
	respond.JSON(w, http.StatusCreated, bot)
}

func (h *TelegramAccountsHandler) Chats(w http.ResponseWriter, r *http.Request) {
	actor := middleware.AdminFrom(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "شناسه نامعتبر است")
		return
	}
	chats, err := h.svc.Chats(r.Context(), actor, id)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	respond.JSON(w, http.StatusOK, map[string]any{"chats": chats})
}

func (h *TelegramAccountsHandler) Messages(w http.ResponseWriter, r *http.Request) {
	actor := middleware.AdminFrom(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "شناسه نامعتبر است")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 50
	}
	offsetID, _ := strconv.Atoi(r.URL.Query().Get("offset_id"))
	msgs, oldest, err := h.svc.Messages(r.Context(), actor, id, chi.URLParam(r, "chatID"), limit, offsetID)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	respond.JSON(w, http.StatusOK, map[string]any{"messages": msgs, "oldest_id": oldest})
}

func (h *TelegramAccountsHandler) SendMessage(w http.ResponseWriter, r *http.Request) {
	actor := middleware.AdminFrom(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "شناسه نامعتبر است")
		return
	}
	var in struct {
		Text string `json:"text"`
	}
	if err := jsonDecode(r, &in); err != nil {
		respond.Error(w, http.StatusBadRequest, "درخواست نامعتبر است")
		return
	}
	msg, err := h.svc.SendMessage(r.Context(), actor, id, chi.URLParam(r, "chatID"), strings.TrimSpace(in.Text))
	if err != nil {
		h.writeErr(w, err)
		return
	}
	respond.JSON(w, http.StatusOK, map[string]any{"message": msg})
}

func (h *TelegramAccountsHandler) MessagePhoto(w http.ResponseWriter, r *http.Request) {
	if _, err := strconv.Atoi(chi.URLParam(r, "msgID")); err != nil {
		respond.Error(w, http.StatusBadRequest, "شناسه پیام نامعتبر است")
		return
	}
	h.streamPhoto(w, r, func(actor *domain.Admin, id uuid.UUID, chatID string) (io.ReadCloser, string, error) {
		return h.svc.Photo(r.Context(), actor, id, chatID, mustAtoi(chi.URLParam(r, "msgID")))
	})
}

func (h *TelegramAccountsHandler) ChatPhoto(w http.ResponseWriter, r *http.Request) {
	h.streamPhoto(w, r, func(actor *domain.Admin, id uuid.UUID, chatID string) (io.ReadCloser, string, error) {
		return h.svc.ChatPhoto(r.Context(), actor, id, chatID)
	})
}

func mustAtoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func (h *TelegramAccountsHandler) streamPhoto(w http.ResponseWriter, r *http.Request, open func(*domain.Admin, uuid.UUID, string) (io.ReadCloser, string, error)) {
	actor := middleware.AdminFrom(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "شناسه نامعتبر است")
		return
	}
	body, ctype, err := open(actor, id, chi.URLParam(r, "chatID"))
	if err != nil {
		h.writeErr(w, err)
		return
	}
	defer body.Close()
	if ctype == "" {
		ctype = "image/jpeg"
	}
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Cache-Control", "private, max-age=3600")
	if _, err := io.Copy(w, body); err != nil {
		return
	}
}
