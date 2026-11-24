-- +goose Up
CREATE SCHEMA IF NOT EXISTS app;

-- `code` is the identifier the upstream system uses, so a renamed category
-- never forks a row.
CREATE TABLE app.categories (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  code text NOT NULL,
  name text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT categories_code_key UNIQUE (code),
  CONSTRAINT categories_code_not_blank CHECK (length(btrim(code)) > 0)
);

-- One budget per category per calendar month. The CHECK keeps `period_month`
-- on the first of the month without trusting every writer to remember.
CREATE TABLE app.budgets (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  category_id bigint NOT NULL REFERENCES app.categories (id) ON DELETE CASCADE,
  period_month date NOT NULL,
  amount_cents bigint NOT NULL,
  currency char(3) NOT NULL DEFAULT 'USD',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT budgets_category_period_key UNIQUE (category_id, period_month),
  CONSTRAINT budgets_period_is_month_start CHECK (period_month = date_trunc('month', period_month)::date),
  CONSTRAINT budgets_amount_non_negative CHECK (amount_cents >= 0)
);

-- Money is bigint cents, never a float and never numeric-with-scale.
--
-- `amount_cents` is a positive magnitude and `direction` says which way it
-- moves, so a refund is a credit rather than a negative debit; actuals are
-- SUM(debit) - SUM(credit).
--
-- `(source, external_id)` is the key the sync job upserts on, and
-- `source_updated_at` is its change-data-capture cursor.
CREATE TABLE app.transactions (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source text NOT NULL,
  external_id text NOT NULL,
  category_id bigint NOT NULL REFERENCES app.categories (id) ON DELETE RESTRICT,
  description text NOT NULL DEFAULT '',
  vendor text NOT NULL DEFAULT '',
  amount_cents bigint NOT NULL,
  currency char(3) NOT NULL DEFAULT 'USD',
  direction text NOT NULL,
  occurred_on date NOT NULL,
  source_updated_at timestamptz NOT NULL,
  voided_at timestamptz,
  first_synced_at timestamptz NOT NULL DEFAULT now(),
  last_synced_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT transactions_source_external_key UNIQUE (source, external_id),
  CONSTRAINT transactions_amount_non_negative CHECK (amount_cents >= 0),
  CONSTRAINT transactions_direction_valid CHECK (direction IN ('debit', 'credit'))
);

-- Reporting reads one month of one category and never looks at voided rows.
CREATE INDEX transactions_category_occurred_idx ON app.transactions (category_id, occurred_on)
WHERE
  voided_at IS NULL;

-- "What is the newest change I have already seen?" on every sync.
CREATE INDEX transactions_source_updated_at_idx ON app.transactions (source, source_updated_at DESC);

-- One row per sync attempt: the audit trail, and the only place the cursor is
-- persisted. A run that crashes stays 'running' and is visible as such.
CREATE TABLE app.sync_runs (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source text NOT NULL,
  trigger text NOT NULL,
  status text NOT NULL,
  cursor_before timestamptz,
  cursor_after timestamptz,
  pages_fetched integer NOT NULL DEFAULT 0,
  records_fetched integer NOT NULL DEFAULT 0,
  records_created integer NOT NULL DEFAULT 0,
  records_updated integer NOT NULL DEFAULT 0,
  records_skipped integer NOT NULL DEFAULT 0,
  error_message text,
  started_at timestamptz NOT NULL DEFAULT now(),
  finished_at timestamptz,
  duration_ms integer,
  CONSTRAINT sync_runs_status_valid CHECK (status IN ('running', 'success', 'failed')),
  CONSTRAINT sync_runs_trigger_valid CHECK (trigger IN ('startup', 'scheduler', 'manual')),
  CONSTRAINT sync_runs_finished_has_duration CHECK ((finished_at IS NULL) = (duration_ms IS NULL))
);

-- Resuming a sync means finding the latest successful run for a source.
CREATE INDEX sync_runs_source_status_started_idx ON app.sync_runs (source, status, started_at DESC);

-- +goose Down
DROP TABLE IF EXISTS app.sync_runs;

DROP TABLE IF EXISTS app.transactions;

DROP TABLE IF EXISTS app.budgets;

DROP TABLE IF EXISTS app.categories;
