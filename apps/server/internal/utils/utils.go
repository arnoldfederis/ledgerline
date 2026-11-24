// Package utils holds the small helpers shared across the transport layer.
package utils

import (
	"encoding/json/v2"
	"log/slog"
	"net/http"
)

// WriteJSON serialises data as the whole response body. The status goes out
// first, so an encoding failure can only truncate the body -- hence the log.
func WriteJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)

	if err := json.MarshalWrite(w, data); err != nil {
		slog.Error("failed to encode response body", "error", err, "status", status)
	}
}

func WriteNoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}
