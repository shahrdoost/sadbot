package handlers

import (
	"net/http"
	"strconv"

	"sadbot/internal/repository/postgres"
	"sadbot/pkg/respond"
)

type FinanceHandler struct {
	finance *postgres.FinanceRepo
	audit   *postgres.AuditRepo
}

func NewFinanceHandler(finance *postgres.FinanceRepo, audit *postgres.AuditRepo) *FinanceHandler {
	return &FinanceHandler{finance: finance, audit: audit}
}

func (h *FinanceHandler) Overview(w http.ResponseWriter, r *http.Request) {
	ov, err := h.finance.Overview(r.Context())
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	respond.JSON(w, http.StatusOK, ov)
}

func (h *FinanceHandler) AuditLogs(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	size, _ := strconv.Atoi(r.URL.Query().Get("size"))
	logs, total, err := h.audit.List(r.Context(), page, size)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	if logs == nil {
		logs = nil
	}
	respond.JSON(w, http.StatusOK, map[string]any{"logs": logs, "total": total})
}
