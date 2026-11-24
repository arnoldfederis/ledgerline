package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"ledgerline/apps/server/internal/utils"
)

type HealthHandler struct {
	pool    *pgxpool.Pool
	version string
	log     *slog.Logger
}

type healthResponse struct {
	Status    string `json:"status"`
	Version   string `json:"version"`
	Timestamp string `json:"timestamp"`
	Database  string `json:"database,omitzero"`
	Detail    string `json:"detail,omitzero"`
}

// HealthLive must not touch the database: a liveness probe that fails when a
// dependency is down gets the process restarted for someone else's outage.
func (h *HealthHandler) HealthLive(w http.ResponseWriter, _ *http.Request) {
	utils.WriteJSON(w, http.StatusOK, healthResponse{
		Status:    "ok",
		Version:   h.version,
		Timestamp: timestamp(),
	})
}

// HealthReady answers whether this instance can serve traffic, which here
// means whether it can reach PostgreSQL.
func (h *HealthHandler) HealthReady(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := h.pool.Ping(ctx); err != nil {
		h.log.Warn("readiness probe failed", "error", err)
		utils.WriteJSON(w, http.StatusServiceUnavailable, healthResponse{
			Status:    "unavailable",
			Version:   h.version,
			Timestamp: timestamp(),
			Database:  "unreachable",
			Detail:    "database is not reachable",
		})
		return
	}

	utils.WriteJSON(w, http.StatusOK, healthResponse{
		Status:    "ok",
		Version:   h.version,
		Timestamp: timestamp(),
		Database:  "reachable",
	})
}

func timestamp() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05.000Z07:00")
}
