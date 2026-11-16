package handlers

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"

	"sadbot/internal/payment"
	"sadbot/pkg/respond"
)

// OpsReportHandler receives payment outcomes from platforms outside sadbot
// (sadai bills a wallet on sadgpt.ir; sadbot sells coins in its bots) and
// relays them into one Telegram channel, so the operator reads every payment
// in one place.
//
// It is public by path but gated by a shared bearer token: the platforms run
// on separate hosts and cannot hold a panel JWT.
type OpsReportHandler struct {
	ops   *payment.OpsChannel
	token string
}

func NewOpsReportHandler(ops *payment.OpsChannel, token string) *OpsReportHandler {
	return &OpsReportHandler{ops: ops, token: token}
}

// maxReportBytes bounds the body: a legitimate report is a few hundred bytes,
// and an unbounded decoder is an easy memory sink on a public endpoint.
const maxReportBytes = 8 << 10

func (h *OpsReportHandler) Report(w http.ResponseWriter, r *http.Request) {
	// A disabled channel must not look like an auth failure to the sender:
	// it means "accepted, nothing to do here".
	if h == nil || h.ops == nil || h.token == "" {
		respond.JSON(w, http.StatusOK, map[string]any{"ok": true, "skipped": "ops channel disabled"})
		return
	}

	if !h.authorized(r) {
		respond.JSON(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}

	var rep payment.ExternalReport
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxReportBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&rep); err != nil {
		respond.JSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad body"})
		return
	}
	if strings.TrimSpace(rep.Platform) == "" {
		respond.JSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "platform is required"})
		return
	}
	if rep.Status == "" {
		respond.JSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "status is required"})
		return
	}
	if rep.Amount < 0 {
		respond.JSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "amount must not be negative"})
		return
	}

	h.ops.PaymentReported(r.Context(), rep)
	respond.JSON(w, http.StatusOK, map[string]any{"ok": true})
}

// authorized compares in constant time so a wrong token leaks nothing about
// the right one's prefix.
func (h *OpsReportHandler) authorized(r *http.Request) bool {
	got := r.Header.Get("Authorization")
	if after, ok := strings.CutPrefix(got, "Bearer "); ok {
		got = after
	} else if v := r.Header.Get("X-Ops-Token"); v != "" {
		// header-only clients that cannot set Authorization
		got = v
	} else {
		got = ""
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(h.token)) == 1
}
