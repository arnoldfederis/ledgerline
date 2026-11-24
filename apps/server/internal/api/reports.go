package api

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"ledgerline/apps/server/internal/ledger"
	"ledgerline/apps/server/internal/utils"
)

type ReportHandler struct {
	ledger *ledger.Service
	log    *slog.Logger
}

// BudgetVsActual serves GET /api/v1/reports/budget-vs-actual?month=2026-11.
func (h *ReportHandler) BudgetVsActual(w http.ResponseWriter, r *http.Request) {
	month, ok := h.resolveMonth(w, r)
	if !ok {
		return
	}

	report, err := h.ledger.BudgetVsActual(r.Context(), month)
	if err != nil {
		internalError(w, h.log, r, err)
		return
	}
	utils.WriteJSON(w, http.StatusOK, report)
}

// Months serves GET /api/v1/reports/months, newest first.
func (h *ReportHandler) Months(w http.ResponseWriter, r *http.Request) {
	months, err := h.ledger.Months(r.Context())
	if err != nil {
		internalError(w, h.log, r, err)
		return
	}
	utils.WriteJSON(w, http.StatusOK, map[string]any{"months": months})
}

// Categories serves GET /api/v1/categories.
func (h *ReportHandler) Categories(w http.ResponseWriter, r *http.Request) {
	categories, err := h.ledger.Categories(r.Context())
	if err != nil {
		internalError(w, h.log, r, err)
		return
	}
	utils.WriteJSON(w, http.StatusOK, map[string]any{"categories": categories})
}

// resolveMonth falls back to the latest month with data, then to the current
// calendar month. It writes its own error response.
func (h *ReportHandler) resolveMonth(w http.ResponseWriter, r *http.Request) (time.Time, bool) {
	if raw := r.URL.Query().Get("month"); raw != "" {
		month, err := ledger.ParseMonth(raw)
		if err != nil {
			badRequest(w, err.Error())
			return time.Time{}, false
		}
		return month, true
	}

	month, err := h.ledger.LatestMonth(r.Context())
	switch {
	case err == nil:
		return month, true
	case errors.Is(err, ledger.ErrNoData):
		// Empty database: report zeroes for this month so the dashboard can
		// render its empty state.
		now := time.Now().UTC()
		return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC), true
	default:
		internalError(w, h.log, r, err)
		return time.Time{}, false
	}
}
