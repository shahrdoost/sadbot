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

// ContentHandler serves the "content" template panel: typed settings over the
// bot's config, live folder/channel pickers from the MTProto sidecar, the
// collection ledger, and clone.
type ContentHandler struct {
	svc *service.ContentService
}

func NewContentHandler(svc *service.ContentService) *ContentHandler {
	return &ContentHandler{svc: svc}
}

func contentBotIDParam(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "botID"))
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "شناسهٔ بات نامعتبر")
		return uuid.Nil, false
	}
	return id, true
}

// GetSettings: GET /bots/{botID}/content/settings
func (h *ContentHandler) GetSettings(w http.ResponseWriter, r *http.Request) {
	id, ok := contentBotIDParam(w, r)
	if !ok {
		return
	}
	out, err := h.svc.GetContentSettings(r.Context(), middleware.AdminFrom(r.Context()), id)
	if err != nil {
		respond.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	respond.JSON(w, http.StatusOK, out)
}

// PutSettings: PUT /bots/{botID}/content/settings
func (h *ContentHandler) PutSettings(w http.ResponseWriter, r *http.Request) {
	id, ok := contentBotIDParam(w, r)
	if !ok {
		return
	}
	var in service.ContentPut
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		respond.Error(w, http.StatusBadRequest, "درخواست نامعتبر است")
		return
	}
	cfg, err := h.svc.PutContentSettings(r.Context(), middleware.AdminFrom(r.Context()), id, in, clientIP(r))
	if err != nil {
		respond.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	respond.JSON(w, http.StatusOK, cfg)
}

// Folders: GET /bots/{botID}/content/folders?account=<panel account uuid>
func (h *ContentHandler) Folders(w http.ResponseWriter, r *http.Request) {
	if _, ok := contentBotIDParam(w, r); !ok {
		return
	}
	folders, err := h.svc.Folders(r.Context(), middleware.AdminFrom(r.Context()), r.URL.Query().Get("account"))
	if err != nil {
		writeTgErr(w, err)
		return
	}
	respond.JSON(w, http.StatusOK, map[string]any{"folders": folders})
}

// Channels: GET /bots/{botID}/content/chats?account=<panel account uuid>
func (h *ContentHandler) Channels(w http.ResponseWriter, r *http.Request) {
	if _, ok := contentBotIDParam(w, r); !ok {
		return
	}
	chats, err := h.svc.Channels(r.Context(), middleware.AdminFrom(r.Context()), r.URL.Query().Get("account"))
	if err != nil {
		writeTgErr(w, err)
		return
	}
	respond.JSON(w, http.StatusOK, map[string]any{"chats": chats})
}

// Posts: GET /bots/{botID}/content/posts?page=&size=&status=&q=
func (h *ContentHandler) Posts(w http.ResponseWriter, r *http.Request) {
	id, ok := contentBotIDParam(w, r)
	if !ok {
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	size, _ := strconv.Atoi(r.URL.Query().Get("size"))
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	if status != "" {
		switch status {
		case "published", "no_content", "failed", "render_failed":
		default:
			respond.Error(w, http.StatusBadRequest, "وضعیت نامعتبر است")
			return
		}
	}
	out, err := h.svc.Posts(r.Context(), middleware.AdminFrom(r.Context()), id, page, size, status, r.URL.Query().Get("q"))
	if err != nil {
		respond.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	respond.JSON(w, http.StatusOK, out)
}

// Clone: POST /bots/{botID}/clone {account_id, username_hint?, name?}
func (h *ContentHandler) Clone(w http.ResponseWriter, r *http.Request) {
	id, ok := contentBotIDParam(w, r)
	if !ok {
		return
	}
	var in struct {
		AccountID    string `json:"account_id"`
		Name         string `json:"name"`
		UsernameHint string `json:"username_hint"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		respond.Error(w, http.StatusBadRequest, "درخواست نامعتبر است")
		return
	}
	res, err := h.svc.CloneBot(r.Context(), middleware.AdminFrom(r.Context()), id,
		in.AccountID, in.Name, in.UsernameHint, clientIP(r))
	if err != nil {
		writeTgErr(w, err)
		return
	}
	respond.JSON(w, http.StatusCreated, res)
}