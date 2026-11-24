-- name: UpsertTransaction :one
-- The idempotent heart of the sync job.
--
-- `(source, external_id)` is the natural key, so replaying a record is an
-- update rather than a duplicate. The DO UPDATE is guarded by
-- `source_updated_at`, so an out-of-order or already-applied record changes
-- nothing and PostgreSQL returns no row -- which is how the caller counts a
-- record as skipped.
--
-- `xmax = 0` is true only for a freshly inserted row, so one round trip says
-- whether this was a create or an update.
INSERT INTO
  app.transactions (
    source,
    external_id,
    category_id,
    description,
    vendor,
    amount_cents,
    currency,
    direction,
    occurred_on,
    source_updated_at,
    voided_at,
    last_synced_at
  )
VALUES
  (
    @source,
    @external_id,
    @category_id,
    @description,
    @vendor,
    @amount_cents,
    @currency,
    @direction,
    @occurred_on,
    @source_updated_at,
    @voided_at,
    now()
  )
ON CONFLICT (source, external_id) DO UPDATE
SET
  category_id = excluded.category_id,
  description = excluded.description,
  vendor = excluded.vendor,
  amount_cents = excluded.amount_cents,
  currency = excluded.currency,
  direction = excluded.direction,
  occurred_on = excluded.occurred_on,
  source_updated_at = excluded.source_updated_at,
  voided_at = excluded.voided_at,
  last_synced_at = now()
WHERE
  transactions.source_updated_at < excluded.source_updated_at
RETURNING
  id,
  (xmax = 0) AS inserted;

-- name: ListTransactions :many
-- Both filters are optional: NULL widens the query instead of needing a second
-- hand-written variant.
SELECT
  t.id,
  t.external_id,
  t.description,
  t.vendor,
  t.amount_cents,
  t.currency,
  t.direction,
  t.occurred_on,
  t.source_updated_at,
  c.code AS category_code,
  c.name AS category_name
FROM
  app.transactions t
  JOIN app.categories c ON c.id = t.category_id
WHERE
  t.voided_at IS NULL
  AND (
    sqlc.narg(period_month)::date IS NULL
    OR (
      t.occurred_on >= sqlc.narg(period_month)::date
      AND t.occurred_on < (sqlc.narg(period_month)::date + make_interval(months => 1))
    )
  )
  AND (
    sqlc.narg(category_code)::text IS NULL
    OR c.code = sqlc.narg(category_code)::text
  )
ORDER BY
  t.occurred_on DESC,
  t.id DESC
LIMIT
  @row_limit
OFFSET
  @row_offset;

-- name: CountTransactions :one
SELECT
  COUNT(*)::bigint AS total
FROM
  app.transactions t
  JOIN app.categories c ON c.id = t.category_id
WHERE
  t.voided_at IS NULL
  AND (
    sqlc.narg(period_month)::date IS NULL
    OR (
      t.occurred_on >= sqlc.narg(period_month)::date
      AND t.occurred_on < (sqlc.narg(period_month)::date + make_interval(months => 1))
    )
  )
  AND (
    sqlc.narg(category_code)::text IS NULL
    OR c.code = sqlc.narg(category_code)::text
  );
