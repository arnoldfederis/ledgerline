-- name: GetSyncCursor :one
-- The high-water mark of the last run that actually succeeded. Failed and
-- still-running attempts are ignored, so a crash mid-run replays its window
-- rather than silently skipping records.
SELECT
  cursor_after
FROM
  app.sync_runs
WHERE
  source = @source
  AND status = 'success'
  AND cursor_after IS NOT NULL
ORDER BY
  cursor_after DESC
LIMIT
  1;

-- name: StartSyncRun :one
INSERT INTO
  app.sync_runs (source, trigger, status, cursor_before)
VALUES
  (@source, @trigger, 'running', @cursor_before)
RETURNING
  *;

-- name: CompleteSyncRun :one
UPDATE app.sync_runs
SET
  status = 'success',
  cursor_after = @cursor_after,
  pages_fetched = @pages_fetched,
  records_fetched = @records_fetched,
  records_created = @records_created,
  records_updated = @records_updated,
  records_skipped = @records_skipped,
  finished_at = now(),
  duration_ms = (
    EXTRACT(
      EPOCH
      FROM
        (now() - started_at)
    ) * 1000
  )::integer
WHERE
  id = @id
RETURNING
  *;

-- name: FailSyncRun :one
-- A failed run records what it managed to process and deliberately leaves
-- `cursor_after` NULL, so the next run re-reads the window.
UPDATE app.sync_runs
SET
  status = 'failed',
  error_message = @error_message,
  pages_fetched = @pages_fetched,
  records_fetched = @records_fetched,
  records_created = @records_created,
  records_updated = @records_updated,
  records_skipped = @records_skipped,
  finished_at = now(),
  duration_ms = (
    EXTRACT(
      EPOCH
      FROM
        (now() - started_at)
    ) * 1000
  )::integer
WHERE
  id = @id
RETURNING
  *;

-- name: ListSyncRuns :many
SELECT
  *
FROM
  app.sync_runs
ORDER BY
  started_at DESC
LIMIT
  @row_limit;

-- name: TryAcquireAdvisoryLock :one
-- Session-scoped lock, held on one pinned connection for the length of a run,
-- so a manual trigger racing the scheduler cannot interleave with it.
SELECT
  pg_try_advisory_lock(@lock_id::bigint) AS acquired;

-- name: ReleaseAdvisoryLock :one
SELECT
  pg_advisory_unlock(@lock_id::bigint) AS released;
