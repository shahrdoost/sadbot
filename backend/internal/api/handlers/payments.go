package handlers

import (
	"encoding/json"
	"net/http"

	"sadbot/internal/payment"
	"sadbot/pkg/respond"

	"github.com/google/uuid"
)

// PaymentsHandler serves the public gateway callback (no auth).
type PaymentsHandler struct {
	svc *payment.Service
}

func NewPaymentsHandler(svc *payment.Service) *PaymentsHandler {
	return &PaymentsHandler{svc: svc}
}

// Callback is the gateway redirect target. The gateway's own verdict is
// re-checked server-side via Complete() — query params are never trusted.
func (h *PaymentsHandler) Callback(w http.ResponseWriter, r *http.Request) {
	pid := r.URL.Query().Get("pid")
	if pid == "" {
		// the gateway's redirect appends orderId, which is this payment's uuid
		pid = r.URL.Query().Get("orderId")
	}
	id, err := uuid.Parse(pid)
	if err != nil {
		http.Error(w, "bad pid", http.StatusBadRequest)
		return
	}
	if err := h.svc.Complete(r.Context(), id); err != nil {
		// user-facing soft failure: sweeper/manual check will finish the job
		respond.JSON(w, http.StatusOK, map[string]any{
			"ok":      false,
			"message": "پرداخت هنوز تایید نشده است؛ نتیجه تا چند دقیقه دیگر در تلگرام اعلام می‌شود.",
		})
		return
	}
	respond.JSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"message": "پرداخت با موفقیت تایید شد. می‌توانید به تلگرام برگردید.",
	})
}

// GatewayHook is the gateway's server-to-server push (verified/failed). The
// body is a wake-up, not a verdict: the service re-checks the gateway before
// settling or failing anything.
func (h *PaymentsHandler) GatewayHook(w http.ResponseWriter, r *http.Request) {
	var body struct {
		OrderID string `json:"orderId"`
		Status  string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	id, err := uuid.Parse(body.OrderID)
	if err != nil {
		http.Error(w, "bad orderId", http.StatusBadRequest)
		return
	}
	if err := h.svc.Hook(r.Context(), id, body.Status); err != nil {
		// the push is fire-and-forget on the gateway side; the sweeper and the
		// the gateway-hook and the sweeper still settle it later
		respond.JSON(w, http.StatusOK, map[string]any{"ok": false, "message": err.Error()})
		return
	}
	respond.JSON(w, http.StatusOK, map[string]any{"ok": true})
}
