# Ledgerline

A budget-vs-actual dashboard fed by an incremental accounting sync.
**Go**, **PostgreSQL**, **React**, **Tailwind** and **shadcn/ui**, in a
Turborepo monorepo.

Ledgerline models the part of a finance integration that is actually hard: a
background worker polls an accounting system for everything that changed since
its last successful run, folds those changes into PostgreSQL without ever
duplicating a row, and serves the variance numbers to a dashboard. Amounts are
integer cents from end to end — there is no float anywhere between the upstream
API and the browser.

![The Ledgerline dashboard in dark mode](./docs/dashboard-dark.png)

<details>
<summary>Light mode</summary>

![The Ledgerline dashboard in light mode](./docs/dashboard-light.png)

</details>

> A step-by-step tour of how this was built, and why each decision was made, is
> in [docs/IMPLEMENTATION-WALKTHROUGH.md](./docs/IMPLEMENTATION-WALKTHROUGH.md).

---

## What it does

- **Incremental sync (change data capture).** A Go worker asks the accounting
  source for records modified since the high-water mark of the last successful
  run, and walks the result with a keyset cursor. This is the same shape as
  QuickBooks Online's Change Data Capture endpoint.
- **Idempotent ingest.** Every record is written with `ON CONFLICT DO UPDATE`
  on the natural key `(source, external_id)`, guarded by `source_updated_at`.
  Run the sync twice and the second run reports _0 created, 0 updated, N
  unchanged_ — visible in the dashboard, asserted in the tests.
- **Exact money.** Budgets, transactions and variances are `bigint` cents in
  PostgreSQL and `int64` cents in Go. The upstream API sends decimal strings,
  which are parsed straight to cents; sub-cent precision is rejected rather
  than rounded. Utilization travels as integer basis points, so even the
  percentage on the wire is exact.
- **An audit trail.** Every run — scheduled, manual or at startup — writes a
  row to `sync_runs`: trigger, status, cursor before and after, records
  fetched/created/updated/skipped, duration, and the error if it failed.
- **A dashboard that says something.** A KPI strip whose figures animate with
  [NumberFlow](https://number-flow.barvian.me/), a bullet chart measuring each
  category's actual spend against its budget, a dense ledger table with
  over-budget rows ruled in red, an inline drill-down into the transactions
  behind any category, and the sync audit trail in a side sheet. Tailwind v4
  and shadcn/ui (`base-rhea`), light and dark.

---

## How it fits together

```
                  ┌───────────────────────────────────────────────┐
                  │  server process (apps/server)                 │
                  │                                               │
  ┌──────────┐    │  ┌───────────┐  poll /mock/…  ┌─────────────┐ │
  │ browser  │───▶│  │ HTTP API  │                │ mock        │ │
  │ (React)  │    │  │ /api/v1   │                │ accounting  │ │
  └──────────┘    │  └─────┬─────┘                │ API         │ │
                  │        │                      └──────┬──────┘ │
                  │        │                             │        │
                  │  ┌─────▼─────┐   every 60s    ┌──────▼──────┐ │
                  │  │  ledger   │                │ sync worker │ │
                  │  │  service  │                │ (CDC loop)  │ │
                  │  └─────┬─────┘                └──────┬──────┘ │
                  └────────┼─────────────────────────────┼────────┘
                           │                             │
                           ▼                             ▼
                  ┌──────────────────────────────────────────────┐
                  │ PostgreSQL: categories, budgets,             │
                  │ transactions, sync_runs                      │
                  └──────────────────────────────────────────────┘
```

The mock accounting API is served by the same process, behind `/mock`, but the
sync worker talks to it over **real HTTP** with real pagination. That keeps the
integration path — client, cursor handling, decoding, error handling —
genuinely exercised in development instead of short-circuited by an in-process
call. Pointing at a real provider is a change to one environment variable and
one adapter file.

---

## Repository layout

```
apps/
  server/                        Go API + background sync worker
    cmd/
      server/                    the service: HTTP + scheduler in one process
      migrate/                   standalone migration runner for deploys
    internal/
      accounting/                the upstream contract: Source port, HTTP and
                                 in-memory adapters, wire types, cursors
      api/                       HTTP transport only: routing, middleware,
                                 request parsing, error envelopes
      config/                    all environment parsing, in one place
      database/
        migrations/              goose migrations (embedded in the binary)
        queries/                 hand-written SQL, input to sqlc
        sqlc/                    generated, type-safe query layer
      ledger/                    budget-vs-actual domain logic
      money/                     Cents: parsing, formatting, allocation
      mocksource/                the stand-in accounting API
        data/                    the fixture ledger + its Go generator
      syncer/                    the CDC engine and its scheduler
      utils/                     small transport helpers
  web/                           React + TypeScript dashboard (Vite)
    src/
      components/                dashboard components
        ui/                      shadcn/ui primitives, generated
      lib/                       API client, query hooks, formatters
docs/                            screenshots and the development guide
tools/
  scripts/format-sql.mjs         the SQL formatter, run by `bun run format`
  nix/, traefik/, certs/, secrets/
```

The Go layout is the conventional one — `cmd/` for entry points, `internal/`
for everything else — organised by capability rather than by technical layer.
`api` knows about HTTP and nothing about SQL; `ledger` and `syncer` know about
the domain and nothing about HTTP; `accounting` defines a port so the upstream
system can be swapped without touching either.

---

## Running it

Full environment setup (Devbox, direnv, certificates, secrets) is in
[docs/DEVELOPMENT.md](./docs/DEVELOPMENT.md). Once that is done:

```bash
dev up
```

That brings up PostgreSQL, Traefik, the Go server and the Vite dev server
together. The dashboard is at <https://app.ledgerline.dev> (or
<http://localhost:5173>) and the API at <https://server.ledgerline.dev> (or
<http://localhost:8080>).

On first boot the server migrates the schema, seeds the chart of accounts and
three months of budgets, and immediately runs a full sync — so the dashboard
has data the moment it loads. After that the worker syncs every 60 seconds, and
the **Sync now** button triggers one by hand.

To run just the two apps without process-compose:

```bash
bun run dev -F @repo/server
bun run dev -F @repo/web
```

---

## API

| Method | Path                                                          | Description                                                                                         |
| ------ | ------------------------------------------------------------- | --------------------------------------------------------------------------------------------------- |
| `GET`  | `/api/v1/reports/budget-vs-actual?month=2026-11`              | Budget, actual, variance and utilization per category. Defaults to the most recent month with data. |
| `GET`  | `/api/v1/reports/months`                                      | Every month that has a budget or activity, newest first.                                            |
| `GET`  | `/api/v1/categories`                                          | The chart of accounts.                                                                              |
| `GET`  | `/api/v1/transactions?month=&category=&limit=&offset=`        | One page of the ledger.                                                                             |
| `POST` | `/api/v1/sync`                                                | Trigger a sync; responds with the finished run. `409` if one is already in flight.                  |
| `GET`  | `/api/v1/sync/runs?limit=20`                                  | The sync audit trail.                                                                               |
| `GET`  | `/health/live`, `/health/ready`                               | Liveness (process only) and readiness (checks PostgreSQL).                                          |
| `GET`  | `/mock/accounting/transactions?updated_since=&cursor=&limit=` | The stand-in accounting provider.                                                                   |

```jsonc
// GET /api/v1/reports/budget-vs-actual?month=2026-11
{
  "month": "2026-11",
  "period_start": "2026-11-01",
  "period_end": "2026-11-30",
  "currency": "USD",
  "totals": {
    "budget_cents": 3235000,
    "actual_cents": 3249757,
    "variance_cents": -14757, // budget - actual: negative means overspend
    "utilization_bps": 10046, // 100.46% of budget, as an integer
    "over_budget_count": 3,
  },
  "categories": [
    {
      "category_code": "marketing",
      "category_name": "Marketing & Advertising",
      "budget_cents": 650000,
      "actual_cents": 806000,
      "variance_cents": -156000,
      "utilization_bps": 12400,
      "status": "over_budget",
      "budgeted": true,
      "transaction_count": 5,
    },
  ],
}
```

---

## Configuration

Everything is read once at startup by `internal/config`; nothing else in the
program calls `os.Getenv`. `apps/server/.env` is written by the repo's
`env-sync` script and holds working defaults.

| Variable                      | Default                                                                                  | Purpose                                                              |
| ----------------------------- | ---------------------------------------------------------------------------------------- | -------------------------------------------------------------------- |
| `DATABASE_URL`                | `postgres://postgres:postgres@localhost:5432/ledgerline?sslmode=disable&search_path=app` | PostgreSQL connection                                                |
| `TEST_DATABASE_URL`           | _(unset)_                                                                                | Database the sync tests truncate; they skip without it               |
| `DB_AUTO_MIGRATE`             | `true`                                                                                   | Migrate on boot. Turn off in production and use `cmd/migrate`        |
| `SERVER_ADDR`                 | `:8080`                                                                                  | Listen address                                                       |
| `CORS_ALLOWED_ORIGINS`        | `https://app.ledgerline.dev,http://localhost:5173`                                       | Allowlist, not `*`                                                   |
| `ACCOUNTING_SOURCE_NAME`      | `mock_accounting`                                                                        | Stored on every transaction and run, so sources never share a cursor |
| `ACCOUNTING_SOURCE_URL`       | `http://localhost:8080/mock/accounting/transactions`                                     | The upstream endpoint                                                |
| `ACCOUNTING_SOURCE_PAGE_SIZE` | `50`                                                                                     | Records per upstream request                                         |
| `SYNC_ENABLED`                | `true`                                                                                   | Run the background worker                                            |
| `SYNC_ON_STARTUP`             | `true`                                                                                   | Sync once at boot                                                    |
| `SYNC_INTERVAL`               | `60s`                                                                                    | Time between syncs                                                   |
| `SYNC_OVERLAP`                | `5m`                                                                                     | How far the cursor rewinds each run                                  |

---

## Design decisions

### Money is integer cents, everywhere

`float64` cannot represent `0.1`. Add a few thousand float dollars and the
total drifts; a budget report that is a cent out is a report nobody trusts.
Ledgerline stores `bigint` cents, works in `int64` cents, and the only place
decimal text exists is the upstream boundary — where
[`money.Parse`](./apps/server/internal/money/money.go) converts `"1234.56"`
directly to `123456` without a float in between. An amount with sub-cent
precision is an **error**, not a rounding decision made inside a background job.

Ratios have the same problem, so utilization crosses the wire as integer basis
points: `12400` is 124.00%. Even the frontend formatter splits cents with
integer arithmetic rather than dividing by 100 and rounding.

That holds right through the animated figures. `Intl.NumberFormat` has accepted
a decimal _string_ since ES2023 and formats it exactly, so the KPI strip hands
NumberFlow `centsToDecimalString(3235000)` — `"32350.00"` — instead of
`3235000 / 100`. There is no float between the ledger and the pixel.

### Debits and credits, not signed amounts

`amount_cents` is always a positive magnitude; a `direction` column says
`debit` or `credit`. Actuals are `SUM(debit) - SUM(credit)`, so a refund
reduces spend without anyone having to remember that negative amounts mean
something special. The database enforces both halves: a `CHECK` keeps the
magnitude non-negative and another restricts the direction to the two values.

### The cursor lives in `sync_runs`, and only success advances it

`GetSyncCursor` reads the newest `cursor_after` from runs that reached
`success`. A run that crashes or fails leaves its `cursor_after` NULL, so the
next run re-reads the window it did not finish — it is impossible to skip
records by dying halfway.

Each run also rewinds the cursor by `SYNC_OVERLAP` before asking for changes.
Clock skew between two systems, or a record committed a moment after its
`updated_at` was stamped, would otherwise fall through the window permanently.
Re-reading those records costs nothing because the write is idempotent, which
is exactly why the overlap is safe to have.

### `xmax = 0` to tell an insert from an update

```sql
INSERT INTO app.transactions (...) VALUES (...)
ON CONFLICT (source, external_id) DO UPDATE
   SET ...
 WHERE transactions.source_updated_at < excluded.source_updated_at
RETURNING id, (xmax = 0) AS inserted;
```

Three things happen in one round trip:

- **No duplicates.** `(source, external_id)` is the natural key, so replaying a
  record updates it.
- **No stale writes.** The `WHERE` guard rejects a record that is older than
  what is already stored, which is normal with an overlapping window and
  out-of-order delivery. PostgreSQL then returns no row at all, and the caller
  counts that as _skipped_.
- **Honest counters.** `xmax` is zero only on a freshly inserted row, so the
  run can report created versus updated without a second query.

### Keyset pagination, not offsets

The upstream cursor encodes `(updated_at, external_id)` and is opaque
(base64url). An offset would silently skip records whenever the upstream data
shifts between two page requests; a keyset asks "what sorts after this exact
record", which stays correct. The `external_id` tiebreaker matters: records
sharing a timestamp are normal, and without it a page boundary landing inside a
tied group would skip or repeat part of it. There is a test for exactly that.

### One transaction per page

A sync applies each page in its own transaction rather than wrapping the whole
run in one. A long sync does not hold a transaction open for its duration, and
an interrupted run leaves whole pages applied rather than a half-written page.
Category ids created inside a page are only cached for reuse _after_ that page
commits, so a rolled-back page cannot leave a phantom id behind.

### Advisory lock around a run

A manual sync landing while the scheduler is mid-run would interleave two
writers. The service takes a PostgreSQL session-level advisory lock on a pinned
connection for the length of the run; a second attempt gets `ErrLocked` and the
API answers `409`. It works across processes, so splitting the worker out of
the server later is a configuration change, not a redesign.

### Soft deletes for voided records

A record voided upstream sets `voided_at` instead of disappearing. Reports
filter on `voided_at IS NULL`, and the partial index does too, so the row stays
available for the audit trail while dropping straight out of the actuals.

### sqlc over an ORM

The SQL in `internal/database/queries` is the real query — reviewable,
`EXPLAIN`-able, with the index it needs visible next to it. sqlc type-checks it
against the goose migrations at build time, so a renamed column is a compile
error rather than a 2 a.m. page. The generated code is committed, so a
checkout builds without running a code generator first.

### One process, two jobs

The API and the sync worker run in one binary. At this size a second
deployable buys nothing but another thing to operate, and the advisory lock
means pulling the worker out later costs one flag. Migrations are the
exception: `DB_AUTO_MIGRATE` is a convenience for a laptop, and `cmd/migrate`
exists so a deployment can make schema changes an explicit, reviewable step.

---

## Testing

```bash
bun run test
```

**Go.** Table-driven tests over the money parser and formatter (including the
sub-cent rejections and the classic `0.1 + 0.2` trap), the pagination and
cursor logic, and the wire-format validation. The mock provider is tested
through a real `httptest` server and the real HTTP client, so the pagination
contract is verified on both sides at once.

The sync engine is tested against **real PostgreSQL**, because the behaviour
under test — `ON CONFLICT DO UPDATE`, the `xmax` trick, advisory locks,
transactional page writes — is behaviour of the database. Mocking it would test
the mock. Those tests cover:

- running the same sync twice creates no duplicates and reports nothing changed
- only records newer than the cursor are applied, and they update in place
- a stale record never overwrites newer data
- a failed run is recorded and does not advance the cursor; the next run
  resumes from the last good one
- a voided record leaves the actuals but stays in the table
- the budget-vs-actual arithmetic, sign convention and over-budget
  classification end to end
- a second concurrent run is refused

They skip themselves if `TEST_DATABASE_URL` is unset, so `go test ./...` works
on a machine with no database.

**Web.** Vitest covers the money and percentage formatters, and a Testing
Library test on the ledger table asserts that an over-budget row is marked by
its data attribute, its signed variance and its named status — not by colour
alone.

---

## Tooling

One formatter per language, all reachable from the repo root:

```bash
bun run format            # gofmt + oxfmt + the SQL formatter
bun run format:sql:check  # fails if any .sql file is unformatted
bun run lint              # go vet + oxlint
bun run check-types       # go build + tsc
bun run test              # Go tests + Vitest
bun run screenshots       # re-capture the README images at 2x
```

The SQL formatter is [`tools/scripts/format-sql.mjs`](./tools/scripts/format-sql.mjs):
`sql-formatter` on PostgreSQL dialect, 2-space indent, uppercase keywords. It
keeps sqlc's `@param` placeholders intact (`paramTypes: { named: ["@"] }`),
which the formatter would otherwise break by inserting a space.

The Go toolchain, `sqlc` and `goose` all come from Devbox, so there is nothing
to install globally — see [docs/DEVELOPMENT.md](./docs/DEVELOPMENT.md).
