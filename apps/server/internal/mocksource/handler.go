package mocksource

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"ledgerline/apps/server/internal/accounting"
	"ledgerline/apps/server/internal/utils"
)

const (
	defaultPageSize = 50
	maxPageSize     = 200
)

type Handler struct {
	source *accounting.StaticSource
}

func NewHandler(source *accounting.StaticSource) *Handler {
	return &Handler{source: source}
}

// Transactions serves GET /mock/accounting/transactions with
// updated_since (RFC 3339), cursor and limit (1..200).
func (h *Handler) Transactions(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	var since time.Time
	if raw := q.Get("updated_since"); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			utils.WriteJSON(w, http.StatusBadRequest, map[string]string{
				"error": "updated_since must be an RFC 3339 timestamp, for example 2026-11-01T00:00:00Z",
			})
			return
		}
		since = parsed.UTC()
	}

	limit := defaultPageSize
	if raw := q.Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > maxPageSize {
			utils.WriteJSON(w, http.StatusBadRequest, map[string]string{
				"error": "limit must be an integer between 1 and " + strconv.Itoa(maxPageSize),
			})
			return
		}
		limit = parsed
	}

	page, err := h.source.FetchChanged(r.Context(), since, q.Get("cursor"), limit)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, accounting.ErrInvalidCursor) {
			status = http.StatusBadRequest
		}
		utils.WriteJSON(w, status, map[string]string{"error": err.Error()})
		return
	}

	wire := accounting.WirePage{
		Transactions: make([]accounting.WireTransaction, 0, len(page.Records)),
		NextCursor:   page.NextCursor,
		HasMore:      page.HasMore,
		ServerTime:   time.Now().UTC(),
	}
	for _, rec := range page.Records {
		wire.Transactions = append(wire.Transactions, rec.ToWire())
	}

	utils.WriteJSON(w, http.StatusOK, wire)
}
