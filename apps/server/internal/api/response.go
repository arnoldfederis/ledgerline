package api

import (
	"log/slog"
	"net/http"

	"ledgerline/apps/server/internal/utils"
)

// errorEnvelope is the shape every failure takes, so clients can handle errors
// generically instead of per endpoint.
type errorEnvelope struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Clients branch on the code; the message is for humans and may change.
const (
	codeBadRequest  = "bad_request"
	codeNotFound    = "not_found"
	codeConflict    = "conflict"
	codeInternal    = "internal_error"
	codeUnavailable = "service_unavailable"
)

func writeError(w http.ResponseWriter, status int, code, message string) {
	utils.WriteJSON(w, status, errorEnvelope{Error: errorDetail{Code: code, Message: message}})
}

func badRequest(w http.ResponseWriter, message string) {
	writeError(w, http.StatusBadRequest, codeBadRequest, message)
}

func notFound(w http.ResponseWriter, message string) {
	writeError(w, http.StatusNotFound, codeNotFound, message)
}

func conflict(w http.ResponseWriter, message string) {
	writeError(w, http.StatusConflict, codeConflict, message)
}

// internalError logs the cause and tells the client only that something broke.
func internalError(w http.ResponseWriter, log *slog.Logger, r *http.Request, err error) {
	log.Error("request failed", "method", r.Method, "path", r.URL.Path, "error", err)
	writeError(w, http.StatusInternalServerError, codeInternal, "the server could not complete the request")
}
