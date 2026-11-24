// Command server runs the Ledgerline API and the background sync worker.
//
// One process: at this size a second deployable buys nothing but another thing
// to operate, and the advisory lock in the syncer means splitting it out later
// is a configuration change.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"ledgerline/apps/server/internal/accounting"
	"ledgerline/apps/server/internal/api"
	"ledgerline/apps/server/internal/config"
	"ledgerline/apps/server/internal/database"
	"ledgerline/apps/server/internal/ledger"
	"ledgerline/apps/server/internal/mocksource"
	"ledgerline/apps/server/internal/syncer"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if err := run(); err != nil {
		slog.Error("server exited with error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	log := newLogger(cfg)
	slog.SetDefault(log)
	log.Info("starting ledgerline", "version", version, "env", cfg.Env, "addr", cfg.Addr)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := database.Open(ctx, cfg.DatabaseURL, log)
	if err != nil {
		return err
	}
	defer pool.Close()

	if cfg.AutoMigrate {
		if err := database.Migrate(ctx, pool, log); err != nil {
			return err
		}
	}

	// The stand-in provider is served by this process and the sync job talks
	// to it over real HTTP, so the integration path stays exercised in
	// development instead of short-circuited by an in-process call.
	mockRecords, err := mocksource.Load(cfg.Source.Name)
	if err != nil {
		return err
	}
	log.Info("mock accounting source loaded",
		"records", len(mockRecords.Records()),
		"latest_change", mocksource.LatestChange(mockRecords).Format(time.RFC3339))

	source := accounting.NewHTTPSource(cfg.Source.Name, cfg.Source.URL, cfg.Source.Timeout)
	syncService := syncer.NewService(pool, source, log, syncer.Options{
		PageSize: cfg.Source.PageSize,
		Overlap:  cfg.Sync.Overlap,
	})
	ledgerService := ledger.NewService(pool)

	handler := api.NewRouter(api.Deps{
		Log:         log,
		Pool:        pool,
		Ledger:      ledgerService,
		Syncer:      syncService,
		MockSource:  mocksource.NewHandler(mockRecords),
		CORSOrigins: cfg.CORSOrigins,
		Version:     version,
		SourceName:  cfg.Source.Name,
	})

	srv := &http.Server{
		Addr:    cfg.Addr,
		Handler: handler,
		// Without these a slow client can hold connections open until the
		// process runs out of file descriptors.
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return ctx },
	}

	// Bind before starting the scheduler. The startup sync calls back into
	// this process over HTTP, and a bound-but-not-yet-serving listener still
	// queues the connection.
	listener, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		return err
	}

	var workers sync.WaitGroup
	if cfg.Sync.Enabled {
		scheduler := syncer.NewScheduler(syncService, cfg.Sync.Interval, cfg.Sync.RunOnStartup, log)
		workers.Add(1)
		go func() {
			defer workers.Done()
			scheduler.Run(ctx)
		}()
	} else {
		log.Warn("background sync is disabled", "hint", "set SYNC_ENABLED=true to run it on a timer")
	}

	serverErr := make(chan error, 1)
	go func() {
		log.Info("http server listening", "addr", listener.Addr().String())
		if err := srv.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
		close(serverErr)
	}()

	select {
	case err := <-serverErr:
		if err != nil {
			return err
		}
	case <-ctx.Done():
		log.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("graceful shutdown failed", "error", err)
	}
	workers.Wait()

	log.Info("ledgerline stopped")
	return nil
}

func newLogger(cfg config.Config) *slog.Logger {
	level := slog.LevelInfo
	if cfg.IsDevelopment() {
		level = slog.LevelDebug
	}
	opts := &slog.HandlerOptions{Level: level}

	if cfg.IsDevelopment() {
		return slog.New(slog.NewTextHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, opts))
}
