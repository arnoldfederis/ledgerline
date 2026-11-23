package main

import (
	"errors"
	"fmt"
	"ledgerline/apps/server/internal/api"
	"log/slog"
	"net/http"
)

func main() {
	srv := &http.Server{
		Addr:    ":8080",
		Handler: api.NewRouter(),
	}

	fmt.Println("Server started on port 8080")
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("server error", "error", err)
	}
}
