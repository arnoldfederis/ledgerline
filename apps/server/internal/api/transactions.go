package api

import (
	"log/slog"
	"net/http"
	"strconv"

	"ledgerline/apps/server/internal/ledger"
	"ledgerline/apps/server/internal/utils"
)

const (
	defaultTransactionLimit = 50
	maxTransactionLimit     = 200
)

type TransactionHandler struct {
	ledger *ledger.Service
	log    *slog.Logger
}

// List serves GET /api/v1/transactions with optional month, category, limit
// and offset. The limit is capped server-side.
func (h *TransactionHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filter := ledger.TransactionFilter{Limit: defaultTransactionLimit}

	if raw := q.Get("month"); raw != "" {
		month, err := ledger.ParseMonth(raw)
		if err != nil {
			badRequest(w, err.Error())
			return
		}
		filter.Month = &month
	}

	if raw := q.Get("category"); raw != "" {
		filter.CategoryCode = &raw
	}

	if raw := q.Get("limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > maxTransactionLimit {
			badRequest(w, "limit must be an integer between 1 and "+strconv.Itoa(maxTransactionLimit))
			return
		}
		filter.Limit = int32(limit)
	}

	if raw := q.Get("offset"); raw != "" {
		offset, err := strconv.Atoi(raw)
		if err != nil || offset < 0 {
			badRequest(w, "offset must be a non-negative integer")
			return
		}
		filter.Offset = int32(offset)
	}

	page, err := h.ledger.Transactions(r.Context(), filter)
	if err != nil {
		internalError(w, h.log, r, err)
		return
	}
	utils.WriteJSON(w, http.StatusOK, page)
}
