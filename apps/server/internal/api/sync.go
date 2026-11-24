package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"ledgerline/apps/server/internal/ledger"
	"ledgerline/apps/server/internal/syncer"
	"ledgerline/apps/server/internal/utils"
)

const (
	defaultSyncRunLimit = 20
	maxSyncRunLimit     = 100
	manualSyncTimeout   = 2 * time.Minute
)

type SyncHandler struct {
	syncer *syncer.Service
	ledger *ledger.Service
	log    *slog.Logger
}

// Trigger serves POST /api/v1/sync. The sync runs inline and the response is
// the finished Result, because the point of the button is to see what it did.
func (h *SyncHandler) Trigger(w http.ResponseWriter, r *http.Request) {
	// Detached from the request context: closing the tab must not abort a sync
	// that is already writing. The timeout bounds it instead.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), manualSyncTimeout)
	defer cancel()

	result, err := h.syncer.Run(ctx, syncer.TriggerManual)
	switch {
	case err == nil:
		utils.WriteJSON(w, http.StatusOK, result)
	case errors.Is(err, syncer.ErrAlreadyRunning), errors.Is(err, syncer.ErrLocked):
		conflict(w, "a sync is already in progress for this source")
	default:
		// The run is already recorded as failed; report that rather than a
		// bare 500 so the client can show what went wrong.
		h.log.Error("manual sync failed", "error", err, "run_id", result.RunID)
		utils.WriteJSON(w, http.StatusBadGateway, map[string]any{
			"error": errorDetail{
				Code:    codeUnavailable,
				Message: "the sync run failed: " + err.Error(),
			},
			"result": result,
		})
	}
}

// Runs serves GET /api/v1/sync/runs?limit=20, newest first.
func (h *SyncHandler) Runs(w http.ResponseWriter, r *http.Request) {
	limit := int32(defaultSyncRunLimit)
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > maxSyncRunLimit {
			badRequest(w, "limit must be an integer between 1 and "+strconv.Itoa(maxSyncRunLimit))
			return
		}
		limit = int32(parsed)
	}

	runs, err := h.ledger.SyncRuns(r.Context(), limit)
	if err != nil {
		internalError(w, h.log, r, err)
		return
	}
	utils.WriteJSON(w, http.StatusOK, map[string]any{"runs": runs})
}
