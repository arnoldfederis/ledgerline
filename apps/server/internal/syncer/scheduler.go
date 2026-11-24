package syncer

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

// Scheduler runs the sync service on a fixed interval. A failed run is retried
// by the next tick rather than by bespoke retry logic, because the cursor only
// advances on success.
type Scheduler struct {
	service      *Service
	interval     time.Duration
	runOnStartup bool
	log          *slog.Logger
}

func NewScheduler(service *Service, interval time.Duration, runOnStartup bool, log *slog.Logger) *Scheduler {
	return &Scheduler{
		service:      service,
		interval:     interval,
		runOnStartup: runOnStartup,
		log:          log.With("component", "sync-scheduler"),
	}
}

// Run blocks until ctx is cancelled. A tick arriving mid-run is dropped, not
// queued; the next window simply covers more ground.
func (sc *Scheduler) Run(ctx context.Context) {
	sc.log.Info("sync scheduler started", "interval", sc.interval, "run_on_startup", sc.runOnStartup)

	if sc.runOnStartup {
		sc.tick(ctx, TriggerStartup)
	}

	ticker := time.NewTicker(sc.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			sc.log.Info("sync scheduler stopped")
			return
		case <-ticker.C:
			sc.tick(ctx, TriggerScheduler)
		}
	}
}

func (sc *Scheduler) tick(ctx context.Context, trigger string) {
	_, err := sc.service.Run(ctx, trigger)
	switch {
	case err == nil:
		return
	case errors.Is(err, ErrAlreadyRunning), errors.Is(err, ErrLocked):
		sc.log.Info("sync skipped", "reason", err)
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		sc.log.Info("sync interrupted by shutdown")
	default:
		// Already recorded in sync_runs and logged in detail by the service.
		sc.log.Warn("sync run ended in failure", "error", err)
	}
}
