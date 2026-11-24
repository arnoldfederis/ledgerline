// Package syncer pulls everything the accounting source has changed since the
// last successful run and folds it into PostgreSQL.
//
// Named syncer rather than sync so it does not shadow the standard library in
// a file that also needs a mutex.
package syncer

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"ledgerline/apps/server/internal/accounting"
	"ledgerline/apps/server/internal/database/sqlc"
)

const (
	TriggerStartup   = "startup"
	TriggerScheduler = "scheduler"
	TriggerManual    = "manual"
)

const (
	StatusRunning = "running"
	StatusSuccess = "success"
	StatusFailed  = "failed"
)

// maxPages bounds a run, so a source that always answers has_more cannot spin
// forever.
const maxPages = 1000

var (
	ErrAlreadyRunning = errors.New("syncer: a sync is already running in this process")
	ErrLocked         = errors.New("syncer: another process is syncing this source")
)

type Options struct {
	PageSize int
	Overlap  time.Duration
}

type Service struct {
	pool    *pgxpool.Pool
	queries *sqlc.Queries
	source  accounting.Source
	log     *slog.Logger
	opts    Options

	// running avoids a round trip for the common case; the advisory lock
	// handles the cross-process one.
	running atomic.Bool
}

func NewService(pool *pgxpool.Pool, source accounting.Source, log *slog.Logger, opts Options) *Service {
	if opts.PageSize < 1 {
		opts.PageSize = 50
	}

	return &Service{
		pool:    pool,
		queries: sqlc.New(pool),
		source:  source,
		log:     log.With("component", "syncer", "source", source.Name()),
		opts:    opts,
	}
}

// Result mirrors the row written to sync_runs.
type Result struct {
	RunID          int64         `json:"run_id"`
	Source         string        `json:"source"`
	Trigger        string        `json:"trigger"`
	Status         string        `json:"status"`
	CursorBefore   *time.Time    `json:"cursor_before"`
	CursorAfter    *time.Time    `json:"cursor_after"`
	PagesFetched   int           `json:"pages_fetched"`
	RecordsFetched int           `json:"records_fetched"`
	RecordsCreated int           `json:"records_created"`
	RecordsUpdated int           `json:"records_updated"`
	RecordsSkipped int           `json:"records_skipped"`
	StartedAt      time.Time     `json:"started_at"`
	FinishedAt     time.Time     `json:"finished_at"`
	Duration       time.Duration `json:"-"`
	DurationMS     int64         `json:"duration_ms"`
	ErrorMessage   string        `json:"error_message,omitzero"`
}

// Run performs one incremental sync and records it in sync_runs. The Result is
// filled in even when the error is non-nil, so a failed run still reports how
// far it got.
func (s *Service) Run(ctx context.Context, trigger string) (Result, error) {
	if !s.running.CompareAndSwap(false, true) {
		return Result{}, ErrAlreadyRunning
	}
	defer s.running.Store(false)

	// A session-scoped lock lives on its connection, so it has to be the same
	// connection from lock to unlock -- which a pool will not promise unless
	// you hold the connection.
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("syncer: acquire lock connection: %w", err)
	}
	defer conn.Release()

	lockQueries := sqlc.New(conn)
	lockID := advisoryLockID(s.source.Name())

	acquired, err := lockQueries.TryAcquireAdvisoryLock(ctx, lockID)
	if err != nil {
		return Result{}, fmt.Errorf("syncer: take advisory lock: %w", err)
	}
	if !acquired {
		return Result{}, ErrLocked
	}
	defer func() {
		// Fresh context: on shutdown ctx is already cancelled but the unlock
		// still has to go out.
		releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if _, err := lockQueries.ReleaseAdvisoryLock(releaseCtx, lockID); err != nil {
			s.log.Warn("could not release sync lock", "error", err)
		}
	}()

	return s.run(ctx, trigger)
}

func (s *Service) run(ctx context.Context, trigger string) (Result, error) {
	source := s.source.Name()

	cursorBefore, err := s.cursor(ctx, source)
	if err != nil {
		return Result{}, err
	}

	// A nil cursor means this source has never synced, so the window is
	// everything.
	var since time.Time
	if cursorBefore != nil {
		since = cursorBefore.Add(-s.opts.Overlap)
	}

	run, err := s.queries.StartSyncRun(ctx, sqlc.StartSyncRunParams{
		Source:       source,
		Trigger:      trigger,
		CursorBefore: cursorBefore,
	})
	if err != nil {
		return Result{}, fmt.Errorf("syncer: open sync run: %w", err)
	}

	result := Result{
		RunID:        run.ID,
		Source:       source,
		Trigger:      trigger,
		Status:       StatusRunning,
		CursorBefore: cursorBefore,
		StartedAt:    run.StartedAt,
	}

	s.log.Info("sync started",
		"run_id", run.ID,
		"trigger", trigger,
		"since", formatCursor(since),
		"cursor_before", formatCursorPtr(cursorBefore))

	cursorAfter, err := s.drain(ctx, since, &result)
	if err != nil {
		result.ErrorMessage = err.Error()
		return s.markFailed(ctx, result, err)
	}

	// Never move the cursor backwards: a run that saw nothing keeps the
	// previous high-water mark.
	if cursorAfter == nil {
		cursorAfter = cursorBefore
	}
	return s.markSucceeded(ctx, result, cursorAfter)
}

// drain walks every page of the change window, applying each page in its own
// transaction. An interrupted run then leaves whole pages applied rather than
// a half-written one.
func (s *Service) drain(ctx context.Context, since time.Time, result *Result) (*time.Time, error) {
	categories := make(map[string]int64)

	var cursor string
	var newest *time.Time

	for page := 0; page < maxPages; page++ {
		fetched, err := s.source.FetchChanged(ctx, since, cursor, s.opts.PageSize)
		if err != nil {
			return nil, fmt.Errorf("syncer: fetch page %d: %w", page+1, err)
		}

		result.PagesFetched++
		result.RecordsFetched += len(fetched.Records)

		if len(fetched.Records) > 0 {
			if err := s.applyPage(ctx, fetched.Records, categories, result); err != nil {
				return nil, err
			}
			last := fetched.Records[len(fetched.Records)-1].UpdatedAt
			if newest == nil || last.After(*newest) {
				newest = &last
			}
		}

		// An empty page that still claims more work is a broken source.
		if !fetched.HasMore || fetched.NextCursor == "" || len(fetched.Records) == 0 {
			return newest, nil
		}
		cursor = fetched.NextCursor
	}

	return nil, fmt.Errorf("syncer: stopped after %d pages: source never reported the end of the change window", maxPages)
}

func (s *Service) applyPage(ctx context.Context, records []accounting.Record, categories map[string]int64, result *Result) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("syncer: begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }() // no-op after Commit

	q := s.queries.WithTx(tx)
	pending := make(map[string]int64)
	created, updated, skipped := 0, 0, 0

	for _, rec := range records {
		if err := rec.Validate(); err != nil {
			return fmt.Errorf("syncer: %w", err)
		}

		categoryID, ok := categories[rec.CategoryCode]
		if !ok {
			categoryID, ok = pending[rec.CategoryCode]
		}
		if !ok {
			category, err := q.UpsertCategory(ctx, sqlc.UpsertCategoryParams{
				Code: rec.CategoryCode,
				Name: categoryLabel(rec),
			})
			if err != nil {
				return fmt.Errorf("syncer: upsert category %q: %w", rec.CategoryCode, err)
			}
			categoryID = category.ID
			pending[rec.CategoryCode] = categoryID
		}

		// A void upstream is a soft delete: the row stays for the audit trail
		// and drops out of reports via voided_at IS NULL.
		var voidedAt *time.Time
		if rec.Voided {
			stamp := rec.UpdatedAt
			voidedAt = &stamp
		}

		row, err := q.UpsertTransaction(ctx, sqlc.UpsertTransactionParams{
			Source:          s.source.Name(),
			ExternalID:      rec.ExternalID,
			CategoryID:      categoryID,
			Description:     rec.Description,
			Vendor:          rec.Vendor,
			AmountCents:     int64(rec.Amount),
			Currency:        rec.Currency,
			Direction:       rec.Direction,
			OccurredOn:      rec.OccurredOn,
			SourceUpdatedAt: rec.UpdatedAt,
			VoidedAt:        voidedAt,
		})
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			// The DO UPDATE guard rejected it: we already hold this version.
			skipped++
		case err != nil:
			return fmt.Errorf("syncer: upsert transaction %s: %w", rec.ExternalID, err)
		case row.Inserted:
			created++
		default:
			updated++
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("syncer: commit page: %w", err)
	}

	// Only now are these ids durable enough for the next page to reuse.
	for code, id := range pending {
		categories[code] = id
	}
	result.RecordsCreated += created
	result.RecordsUpdated += updated
	result.RecordsSkipped += skipped
	return nil
}

func (s *Service) markSucceeded(ctx context.Context, result Result, cursorAfter *time.Time) (Result, error) {
	row, err := s.queries.CompleteSyncRun(ctx, sqlc.CompleteSyncRunParams{
		ID:             result.RunID,
		CursorAfter:    cursorAfter,
		PagesFetched:   int32(result.PagesFetched),
		RecordsFetched: int32(result.RecordsFetched),
		RecordsCreated: int32(result.RecordsCreated),
		RecordsUpdated: int32(result.RecordsUpdated),
		RecordsSkipped: int32(result.RecordsSkipped),
	})
	if err != nil {
		return result, fmt.Errorf("syncer: close sync run %d: %w", result.RunID, err)
	}

	result = applyRow(result, row)
	s.log.Info("sync finished",
		"run_id", result.RunID,
		"fetched", result.RecordsFetched,
		"created", result.RecordsCreated,
		"updated", result.RecordsUpdated,
		"skipped", result.RecordsSkipped,
		"pages", result.PagesFetched,
		"cursor_after", formatCursorPtr(result.CursorAfter),
		"duration_ms", result.DurationMS)
	return result, nil
}

func (s *Service) markFailed(ctx context.Context, result Result, cause error) (Result, error) {
	message := cause.Error()

	// Record the failure even if the caller's context is cancelled: a run
	// stuck in 'running' forever is worse than one marked failed.
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()

	row, err := s.queries.FailSyncRun(writeCtx, sqlc.FailSyncRunParams{
		ID:             result.RunID,
		ErrorMessage:   &message,
		PagesFetched:   int32(result.PagesFetched),
		RecordsFetched: int32(result.RecordsFetched),
		RecordsCreated: int32(result.RecordsCreated),
		RecordsUpdated: int32(result.RecordsUpdated),
		RecordsSkipped: int32(result.RecordsSkipped),
	})
	if err != nil {
		s.log.Error("could not record sync failure", "run_id", result.RunID, "error", err)
		result.Status = StatusFailed
		return result, cause
	}

	result = applyRow(result, row)
	s.log.Error("sync failed",
		"run_id", result.RunID,
		"fetched", result.RecordsFetched,
		"created", result.RecordsCreated,
		"updated", result.RecordsUpdated,
		"skipped", result.RecordsSkipped,
		"error", message)
	return result, cause
}

// cursor reads the high-water mark of the last successful run. Failed and
// still-running attempts are ignored, so a crash replays its window.
func (s *Service) cursor(ctx context.Context, source string) (*time.Time, error) {
	cursor, err := s.queries.GetSyncCursor(ctx, source)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("syncer: read cursor: %w", err)
	}
	return cursor, nil
}

func applyRow(result Result, row sqlc.AppSyncRun) Result {
	result.Status = row.Status
	result.CursorBefore = utcPtr(row.CursorBefore)
	result.CursorAfter = utcPtr(row.CursorAfter)
	result.PagesFetched = int(row.PagesFetched)
	result.RecordsFetched = int(row.RecordsFetched)
	result.RecordsCreated = int(row.RecordsCreated)
	result.RecordsUpdated = int(row.RecordsUpdated)
	result.RecordsSkipped = int(row.RecordsSkipped)
	result.StartedAt = row.StartedAt.UTC()
	if row.FinishedAt != nil {
		result.FinishedAt = row.FinishedAt.UTC()
	}
	if row.DurationMs != nil {
		result.DurationMS = int64(*row.DurationMs)
		result.Duration = time.Duration(*row.DurationMs) * time.Millisecond
	}
	if row.ErrorMessage != nil {
		result.ErrorMessage = *row.ErrorMessage
	}
	return result
}

// advisoryLockID derives a stable lock id, so one source always contends for
// the same lock and two sources never do.
func advisoryLockID(source string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte("ledgerline:sync:" + source))
	return int64(h.Sum64())
}

// utcPtr normalises a nullable timestamp. PostgreSQL returns timestamptz in
// the session's zone, and an API should not report the server's local offset.
func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	utc := t.UTC()
	return &utc
}

func formatCursor(t time.Time) string {
	if t.IsZero() {
		return "beginning of history"
	}
	return t.UTC().Format(time.RFC3339)
}

func formatCursorPtr(t *time.Time) string {
	if t == nil {
		return "none"
	}
	return formatCursor(*t)
}

func categoryLabel(rec accounting.Record) string {
	if rec.CategoryName != "" {
		return rec.CategoryName
	}
	return rec.CategoryCode
}
