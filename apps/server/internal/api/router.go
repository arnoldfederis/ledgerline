package api

import (
	"net/http"
)

func NewRouter() http.Handler {
	router := http.NewServeMux()

	health := &HealthHandler{}
	router.HandleFunc("GET /health/live", health.HealthLive)
	router.HandleFunc("GET /health/ready", health.HealthReady)

	return router
}
