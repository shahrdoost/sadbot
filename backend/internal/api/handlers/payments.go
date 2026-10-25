package handlers

import (
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
	id, err := uuid.Parse(pid)
	if err != nil {
		http.Error(w, "bad pid", http.StatusBadRequest)
		return
	}
	if err := h.svc.Complete(r.Context(), id); err != nil {
		// user-facing soft failure: sweeper/manual check will finish the job
		respond.JSON(w, http.StatusOK, map[string]any{
			"ok":      false,
			"message": "پرداخت هنوز تایید نشده است؛ چند دقیقه بعد دکمه «بررسی پرداخت» را بزنید.",
		})
		return
	}
	respond.JSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"message": "پرداخت با موفقیت تایید شد. می‌توانید به تلگرام برگردید.",
	})
}
