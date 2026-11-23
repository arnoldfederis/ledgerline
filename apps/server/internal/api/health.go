package api

import (
	"ledgerline/apps/server/internal/utils"
	"net/http"
	"time"
)

type HealthHandler struct{}

func (h *HealthHandler) HealthLive(w http.ResponseWriter, _ *http.Request) {
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z07:00")
	utils.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok", "timestamp": now})
}

func (h *HealthHandler) HealthReady(w http.ResponseWriter, _ *http.Request) {
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z07:00")
	utils.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok", "timestamp": now})
}
