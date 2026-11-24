# How Ledgerline was built, step by step

A guided tour of the implementation, in the order it was actually written. Each
step says what was built, why it was built that way, and which file to open.

If you only read one section, read [Step 5](#step-5-the-sync-engine) — the sync
engine is the part worth talking about in an interview.

---

## The shape of the thing, before any code

Four questions had to be answered first, because everything else follows from
them.

**1. How is money stored?** Integer cents, `bigint` in PostgreSQL and `int64`
in Go. Never a float, never a `numeric` with a scale someone can change.

**2. What is the natural key for a transaction?** `(source, external_id)`. Not
a UUID this system invents — the id the upstream accounting system already
uses. That single decision is what makes the sync idempotent; everything else
is mechanics.

**3. Where does "what did I already sync?" live?** In a `sync_runs` table, as
the high-water mark of the last run that *succeeded*. Not in a config file, not
in memory, not in a `last_synced_at` column that a half-finished run could
advance.

**4. Is a refund a negative debit?** No. Amounts are positive magnitudes plus a
`direction` of `debit` or `credit`, and actuals are `SUM(debit) - SUM(credit)`.
Accountants think this way, the database can enforce it, and nobody has to
remember that some amounts are secretly negative.

---

## Step 1 — The schema

📄 [`apps/server/internal/database/migrations/00001_init_ledger.sql`](../apps/server/internal/database/migrations/00001_init_ledger.sql)

Four tables: `categories`, `budgets`, `transactions`, `sync_runs`.

The interesting parts are the constraints, because each one removes a class of
bug that would otherwise have to be defended in Go:

```sql
CONSTRAINT transactions_source_external_key UNIQUE (source, external_id),
CONSTRAINT transactions_amount_non_negative CHECK (amount_cents >= 0),
CONSTRAINT transactions_direction_valid     CHECK (direction IN ('debit', 'credit')),
```

- The unique key is what `ON CONFLICT` later targets. Without it there is no
  idempotency, only hope.
- `budgets_period_is_month_start` forces `period_month` to be the first of the
  month, so no writer can quietly file a budget under the 15th.
- `sync_runs_finished_has_duration` asserts that `finished_at` and
  `duration_ms` are either both set or both null — a run cannot be half-closed.

Two indexes, both narrow on purpose:

```sql
CREATE INDEX transactions_category_occurred_idx
    ON app.transactions (category_id, occurred_on)
    WHERE voided_at IS NULL;          -- reporting never looks at voided rows

CREATE INDEX transactions_source_updated_at_idx
    ON app.transactions (source, source_updated_at DESC);  -- the CDC cursor scan
```

📄 [`00002_seed_budgets.sql`](../apps/server/internal/database/migrations/00002_seed_budgets.sql)
seeds the chart of accounts and three months of budgets as idempotent upserts,
so re-running against a populated database is a no-op rather than a duplicate
key error. **Transactions are never seeded** — the only way an actual gets into
the database is through the sync job, which means the sync path is exercised
from the very first boot.

---

## Step 2 — Queries, and letting sqlc check them

📄 [`apps/server/internal/database/queries/`](../apps/server/internal/database/queries) ·
📄 [`apps/server/sqlc.yaml`](../apps/server/sqlc.yaml)

The SQL is hand-written and sqlc generates typed Go from it, type-checking
every query against the migrations. Rename a column and the build breaks —
not production.

Two details worth copying:

**`sqlc.yaml` points `schema:` at the migrations directory**, not at a separate
`schema.sql`. One source of truth, no drift.

**Type overrides** hand the application plain `time.Time` and `*time.Time`
instead of `pgtype.Timestamptz` wrappers:

```yaml
overrides:
  - db_type: "timestamptz"
    go_type: "time.Time"
  - db_type: "timestamptz"
    nullable: true
    go_type: { type: "time.Time", pointer: true }
```

pgx scans both; only one of them is pleasant to pass around and to construct in
a test.

> **A gotcha, so you do not lose an hour to it.** sqlc's `@param` shorthand
> mis-rewrites a query that also contains `INTERVAL '1 month'`, producing
> `syntax error at or near "ROUP"`. Two changes fix it: use the long
> `sqlc.arg(period_month)` form, and write `make_interval(months => 1)`
> instead of the interval literal.

---

## Step 3 — The money package

📄 [`apps/server/internal/money/money.go`](../apps/server/internal/money/money.go) ·
📄 [`money_test.go`](../apps/server/internal/money/money_test.go)

`type Cents int64`, and a parser that turns `"1234.56"` into `123456` by
splitting on the decimal point and doing integer arithmetic on each half. No
`strconv.ParseFloat` anywhere.

The judgement call worth defending out loud:

```go
if len(frac) > 2 {
    return 0, fmt.Errorf("%w: %q", ErrPrecision, s)
}
```

`"1.005"` is an **error**, not a rounding. A background job that silently
rounds amounts is a background job that silently changes the books. If the
upstream system starts sending sub-cent values, somebody should have to decide
what that means — loudly, once — rather than discovering it in a reconciliation
three months later.

Also here:

- `UtilizationBasisPoints` — `actual/budget` as an integer, rounded half away
  from zero, so no percentage ever becomes a float. A zero budget returns `0`
  rather than dividing by zero.
- `Allocate` — splits a total into N parts that sum back to exactly the total,
  spreading the indivisible remainder one cent at a time. `$10.00` three ways
  is `3.34 / 3.33 / 3.33`, never `3.33 × 3` with a cent evaporating.

The tests are table-driven and include the classic trap:

```go
var total money.Cents
for range 1000 {
    total = total.Add(10).Add(20)   // 0.10 + 0.20, a thousand times
}
// exactly 30000. In float64 dollars this drifts.
```

---

## Step 4 — The upstream contract and a mock worth having

📄 [`apps/server/internal/accounting/`](../apps/server/internal/accounting) ·
📄 [`apps/server/internal/mocksource/`](../apps/server/internal/mocksource)

`accounting.Source` is the port:

```go
type Source interface {
    Name() string
    FetchChanged(ctx context.Context, since time.Time, cursor string, limit int) (Page, error)
}
```

Three implementations sit behind it: an HTTP client, an in-memory static source
(used by tests and by the mock), and — eventually — a real QuickBooks client
that would change nothing else in the codebase.

**The wire format is the point.** In
[`wire.go`](../apps/server/internal/accounting/wire.go), `Amount` is a
`string`:

```go
type WireTransaction struct {
    Amount string `json:"amount"`   // "1234.56", never a JSON number
    ...
}
```

Real accounting APIs send decimal strings precisely because JSON numbers are
IEEE-754 doubles. If this were a `float64`, every amount would arrive already
rounded and nothing downstream could fix it.

**The mock is served over real HTTP.** It lives in the same process behind
`/mock/accounting/transactions`, but the sync worker reaches it with an actual
HTTP request, actual pagination and actual JSON decoding. A mock that
short-circuits into an in-process call tests nothing about the integration.

**Pagination is keyset, not offset.** The cursor encodes
`(updated_at, external_id)` and is base64url-encoded so clients cannot take it
apart and start depending on the sort key:

```go
func EncodeCursor(r Record) string {
    raw := r.UpdatedAt.UTC().Format(time.RFC3339Nano) + "|" + r.ExternalID
    return base64.RawURLEncoding.EncodeToString([]byte(raw))
}
```

The `external_id` tiebreaker is not decoration. Records sharing a timestamp are
completely normal — a batch import stamps them identically — and a page
boundary landing inside a tied group would skip or repeat part of it with a
timestamp-only cursor. There is a test named exactly that.

The fixture is 114 transactions across three months and eight categories, with
refunds, two voided records, and a handful whose `updated_at` is deliberately
much later than their transaction date so an incremental sync has real late
edits to pick up. It is generated by 📄
[`data/generate.go`](../apps/server/internal/mocksource/data/generate.go) —
a `//go:build ignore` program behind `go generate ./...` — and the output is
committed, so a reviewer can read the data without running anything.

The generator builds the fixture through the same `accounting.WireTransaction`
type the sync job parses, so producer and consumer cannot drift, and it splits
each category's monthly total with the same remainder-distributing allocation
`money.Allocate` uses, so the rows add back up to the cent.

---

## Step 5 — The sync engine

📄 [`apps/server/internal/syncer/service.go`](../apps/server/internal/syncer/service.go)

One run, start to finish:

```
1.  take an in-process guard (cheap: catches this process racing itself)
2.  take a PostgreSQL advisory lock on a pinned connection
       └─ not acquired → ErrLocked → the API answers 409
3.  read the cursor: newest cursor_after among runs with status = 'success'
4.  since = cursor - SYNC_OVERLAP    (nil cursor → the whole history)
5.  open a sync_runs row with status = 'running'
6.  for each page from the source:
       ├─ BEGIN
       ├─ upsert each category, then each transaction
       └─ COMMIT, then promote this page's category ids into the run cache
7.  success → CompleteSyncRun, cursor_after = newest updated_at seen
    failure → FailSyncRun, cursor_after stays NULL
8.  release the lock
```

### The upsert

📄 [`queries/transactions.sql`](../apps/server/internal/database/queries/transactions.sql)

```sql
INSERT INTO app.transactions (...) VALUES (...)
ON CONFLICT (source, external_id) DO UPDATE
   SET ..., last_synced_at = now()
 WHERE transactions.source_updated_at < excluded.source_updated_at
RETURNING id, (xmax = 0) AS inserted;
```

Three things in one round trip:

| What | How |
| ---- | --- |
| No duplicates | `ON CONFLICT` on the natural key — a replay is an update |
| No stale writes | the `WHERE` guard drops records older than what is stored |
| Created vs updated | `xmax = 0` is true only for a freshly inserted row |

And the detail that makes the counters honest: when the guard rejects a record,
PostgreSQL returns **no row at all**, so the Go side sees `pgx.ErrNoRows` and
counts it as *skipped*:

```go
switch {
case errors.Is(err, pgx.ErrNoRows):
    skipped++          // already had this version — the overlap doing its job
case err != nil:
    return err
case row.Inserted:
    created++
default:
    updated++
}
```

That is why the second run in the dashboard reads **0 created, 0 updated, N
unchanged**. It is not a cosmetic message; it is the upsert reporting what the
database actually did.

### Why the cursor only moves on success

A failed run writes `status = 'failed'` and leaves `cursor_after` NULL.
`GetSyncCursor` only reads successful runs, so the next attempt re-reads the
window the failed one did not finish. Dying halfway cannot skip records — the
worst case is doing some work twice, and doing work twice is free here.

### Why the window overlaps

```go
since = cursorBefore.Add(-s.opts.Overlap)   // default 5 minutes
```

Clock skew between two systems, or a record committed a moment after its
`updated_at` was stamped, would otherwise fall outside the window *forever*.
Rewinding slightly re-reads a few records; the idempotent upsert makes that
cost nothing. The overlap is safe **because** the write is idempotent — the two
properties only work as a pair.

### Why one transaction per page

Wrapping the whole run in one transaction would hold it open for the run's
duration and lose everything on an interruption. Per-page transactions mean an
interrupted run leaves whole pages applied. The subtlety is the category cache:
ids created inside a page are held in a *pending* map and only promoted into
the run-level cache after that page commits, so a rolled-back page cannot leave
a phantom id to blow up a later foreign key.

### Why an advisory lock

A manual sync landing while the scheduler is mid-run would interleave two
writers. A session-level advisory lock on a **pinned** connection (a pool will
not promise you the same connection otherwise) makes the second attempt fail
fast with `ErrLocked`. It works across processes, so moving the worker into its
own deployment later is a config change rather than a redesign.

📄 [`scheduler.go`](../apps/server/internal/syncer/scheduler.go) is deliberately
dull: a ticker, and contention logged at info rather than treated as an error.
A tick that arrives mid-run is dropped, not queued — the next window simply
covers more ground.

---

## Step 6 — The reporting layer

📄 [`apps/server/internal/ledger/ledger.go`](../apps/server/internal/ledger/ledger.go) ·
📄 [`queries/reports.sql`](../apps/server/internal/database/queries/reports.sql)

The aggregation happens in SQL, where the data is:

```sql
SUM(CASE WHEN t.direction = 'debit' THEN t.amount_cents ELSE -t.amount_cents END)
```

The `LEFT JOIN`s are chosen so the report tells the whole truth: a category with
a budget and no spend still appears (under budget by the full amount), and a
category with spend and **no** budget also appears, flagged `unbudgeted`. Only
categories with neither are dropped. An unbudgeted overspend is precisely the
thing a finance lead wants surfaced, so it must not be filtered out by an inner
join.

Go then does the small arithmetic and the classification:

```go
VarianceCents:  int64(budget.Sub(actual)),   // positive = money left
UtilizationBps: money.UtilizationBasisPoints(actual, budget),
Status:         status(budget, actual, row.HasBudget),
```

The sign convention is stated once, here, and the dashboard never re-derives
it.

One small UX decision with a real payoff: `LatestMonth` makes the dashboard
open on the most recent month that *has data*, rather than on whatever today's
date happens to be. A demo that opens on an empty screen because the calendar
moved past the fixture is a demo that looks broken.

---

## Step 7 — HTTP

📄 [`apps/server/internal/api/`](../apps/server/internal/api)

Standard library routing. Go 1.22+ method-and-path patterns are enough for a
dozen endpoints, and `net/http` has no dependencies to audit.

[`router.go`](../apps/server/internal/api/router.go) takes an explicit `Deps`
struct rather than reaching for package-level singletons — that is the
difference between a router you can stand up in a test with a fake and one you
cannot. The mock provider is registered conditionally, so in a real deployment
the handler simply is not there.

[`middleware.go`](../apps/server/internal/api/middleware.go): panic recovery,
structured request logging (health probes at debug level, so five-second
process-compose probes do not drown the real traffic), and a CORS **allowlist**
rather than `*`.

[`response.go`](../apps/server/internal/api/response.go): one error envelope
for every failure, and `internalError` logs the real cause while telling the
client only that something broke. Leaking a database error to a browser is how
schemas end up in bug-bounty reports.

Two handler details worth noticing:

```go
// api/sync.go — a user closing the tab must not abort a sync mid-write.
ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), manualSyncTimeout)
```

```go
// api/health.go — liveness must NOT touch the database.
// A liveness probe that fails when a dependency is down gets the process
// restarted for someone else's outage. Readiness checks the database; liveness
// answers "is this process running?" and nothing more.
```

📄 [`cmd/server/main.go`](../apps/server/cmd/server/main.go) wires it together
and binds the listener *before* starting the scheduler — the startup sync calls
back into this process over HTTP, and a bound-but-not-yet-serving listener
still queues the connection, so the race cannot be lost. Shutdown is a signal
context: stop accepting, drain in-flight requests, wait for the worker.

📄 [`cmd/migrate/main.go`](../apps/server/cmd/migrate/main.go) exists because
`DB_AUTO_MIGRATE` is a laptop convenience. In a deployment, schema changes
should be an explicit step that can be run, reviewed and rolled back on its
own.

---

## Step 8 — The dashboard

📄 [`apps/web/src/`](../apps/web/src)

**[`lib/api.ts`](../apps/web/src/lib/api.ts)** — types mirroring the Go DTOs
exactly, and an `ApiError` carrying the API's own code so callers can branch on
it (`409` from the sync endpoint is a *wait*, not a failure, and the UI says so).

**[`lib/format.ts`](../apps/web/src/lib/format.ts)** — the frontend's half of
the money discipline. It never divides by 100 and rounds:

```ts
const absolute = Math.abs(Math.trunc(cents));
const dollars  = Math.trunc(absolute / 100);
const remainder = absolute % 100;
return `${sign}$${group(dollars)}.${String(remainder).padStart(2, "0")}`;
```

**[`lib/queries.ts`](../apps/web/src/lib/queries.ts)** — TanStack Query hooks
with the keys collected in one object, which is what makes
`invalidateQueries({ queryKey: keys.all })` after a sync reliable. The sync-run
list polls every 30 seconds, so runs started by the *background scheduler*
appear without anyone pressing anything.

**The UI is Tailwind v4 + shadcn/ui** — `base-rhea` on a neutral base with
Inter, the same shadcn configuration as the quickpropose project, carrying none
of its palette. The generated primitives live in `src/components/ui` and are
left alone; everything Ledgerline-specific sits beside them in
`src/components`. The theme adds three things and nothing else: a mono family
for figures, three semantic hues (`--gain`, `--loss`, `--caution`), and an
`.eyebrow` label style.

**The page is one column inside `max-w-6xl mx-auto`.** A toolbar sits directly
above the KPI strip — last-sync status on the left, month picker, *History* and
*Sync now* on the right — because the month and the sync state are exactly what
those four figures are a function of. *History* opens the audit trail in a
right-hand `Sheet`: the run log is a reference, not something to keep on
screen, and a side rail competing with the report for width made both worse.

**The KPI figures animate with [NumberFlow](https://number-flow.barvian.me/),
without giving up exactness.** NumberFlow takes an `Intl.NumberFormat` value,
and since ES2023 that accepts a decimal *string* which Intl formats exactly. So
the strip passes `centsToDecimalString(cents)` — `3235000` → `"32350.00"`,
built with integer arithmetic — rather than `cents / 100`:

```tsx
<NumberFlow value={centsToDecimalString(totals.budget_cents)}
            format={{ style: "currency", currency: "USD" }} />
```

Utilization goes the same way: `bpsToRatioString(10060)` is `"1.0060"`, fed to
a percent-styled formatter. The animation is the only thing the library adds;
no float ever sits between the ledger and the screen, and a test round-trips
the strings through `Intl` to prove it.

One catch worth knowing about: NumberFlow animates *from* the value it last
rendered, so a component that unmounts between updates never animates at all.
Switching months did exactly that — the query key changed, `report.data` went
`undefined`, and the strip was replaced by the skeleton and rebuilt from
scratch. `placeholderData: keepPreviousData` keeps the previous month on screen
while the next one loads, which removes the skeleton flash *and* gives the
figures something to count from on the first switch rather than the second.

**The month picker had two flickers of its own**, both worth writing down
because neither is visible in code review:

- Its loading placeholder was `h-8` against the real trigger's `h-7`, and had
  no chevron. Four pixels is enough to shift the whole toolbar the instant the
  month arrives.
- The picker waited on the *report* rather than the *months* list, so the
  placeholder stayed up far longer than it needed to. `activeMonth` now reads
  `months.data?.[0]` first, which lands with a much smaller request.
- The trigger had a hard-coded `w-40`, and the labels inside it are different
  lengths, so it also resized between months. It now takes its natural width
  and reserves the longest label (`September 2026`) with a `::before` — a
  pseudo-element rather than a hidden span, so the reservation stays out of
  both the accessibility tree and `textContent`. Idle, loading and loaded all
  measure 159x28.

While a month is in flight the trigger stays mounted, goes `disabled`, sets
`aria-busy`, and swaps its chevron for a spinner — `SelectTrigger` grew an
optional `icon` prop for that. Giving the spinner the chevron's `size-4` was
not enough to make the swap invisible, for two reasons worth remembering:

- A filled circle reads heavier than a thin chevron at the same box size, so
  the right edge of the control appeared to thicken.
- `animate-spin` is a transform, and a rotating square's bounding box grows —
  the icon measured 16.02px mid-spin against the chevron's flat 16.

Both go away by putting the spinner in a fixed 16px slot and spinning a 14px
glyph inside it: the slot pins the layout, the untransformed wrapper absorbs
the rotation, and the smaller glyph evens out the optical weight. Idle and
loading now measure identically down to the hundredth of a pixel.

The signal that drives it is not `isFetching` alone, which would also spin on a
background refetch of the month already on screen:

```ts
const monthChanging = report.isFetching && report.data?.month !== activeMonth;
```

`keepPreviousData` leaves the previous report in place, so comparing its month
against the selected one is what separates a real change from a refresh.

**The flicker that outlasted all of that was in a different control.** `Sync
now` becomes `Syncing` while a sync runs, and `Syncing` is 11px narrower. The
toolbar lays its controls out in a right-aligned row, so the button shrinking
pulled the whole group 11px to the right — month picker included. Chasing
sub-pixel differences in the picker could never have found it, because the
picker was innocent:

| | idle | syncing |
| --- | --- | --- |
| control group | 380.97 @ 765.03 | 369.61 @ 776.39 |
| month picker x | 765.03 | **776.39** |
| History x | 930.16 | **941.52** |

[`FixedWidth`](../apps/web/src/components/fixed-width.tsx) reserves the longest
label with a `::before` and lays the live label over it, so the control keeps
one width whatever it says. The month picker uses it for month names, the sync
button for `Sync now`, and the History button's failure dot is positioned
absolutely for the same reason. Every column above is now identical in both
states.

The lesson is about method rather than CSS: a label whose text changes is a
layout change, and in a flex row it moves everything beside it. Measuring the
*neighbours* found in one step what measuring the suspect could not.

That second fix leans on an invariant: the first entry of `Months()` must be
the same month `LatestMonth()` returns, or the picker would show a month the
report is not for. Both read `ListReportableMonths`, so it holds by
construction — and `TestLatestMonthIsTheFirstReportableMonth` keeps it that
way if someone edits one query's `ORDER BY`.

**[`budget-bars.tsx`](../apps/web/src/components/budget-bars.tsx) is a bullet
chart, hand-built in CSS.** One shared horizontal scale across all categories,
a wide band for the budget and a narrow bar on top for the actual, with a tick
where the budget sits. Grouped bars need twice the height to say the same
thing, and still leave the reader comparing two heights to work out whether a
category is over. Doing it in CSS also dropped the charting library: the bundle
went from 706 kB to 410 kB.

**[`category-ledger.tsx`](../apps/web/src/components/category-ledger.tsx)** is
a shadcn `Table` with the transaction drill-down expanding inline as a second
row, rather than a detail panel pushed below the table.

**[`report-skeleton.tsx`](../apps/web/src/components/report-skeleton.tsx)**
mirrors the loaded report element for element, with each row box pinned to the
height the real one measures — 128px stat cell, 62px bar row, 72px section
header, 37px table row. Those numbers came from measuring the live DOM rather
than guessing, so the page does not jump when the data lands. The stat cell is
128px and not 116px because NumberFlow reserves room above and below the digits
for the slide; measuring caught that, reading the font size would not have.

Three things done deliberately:

*Status is never carried by colour alone.* An over-budget row has a red inset
rule, **and** a signed negative variance, **and** a named status. The table
reads correctly printed in black and white or by a colour-blind reviewer, and
the Vitest test asserts those non-colour signals.

*The row control is a real `<button>`* — keyboard users get it for free and
screen readers are told what it does.

*Every figure is Geist Mono with `tabular-nums`.* Columns of money only line up
when the digits are the same width, and the mono face is the single strongest
signal that this is a financial tool rather than a generic dashboard.

---

## Step 9 — Tests

```bash
bun run test
```

**Pure unit tests** cover the money parser and formatter, the pagination and
cursor logic, and wire-format validation.

**The mock provider is tested through a real `httptest` server and the real
HTTP client**, which verifies both sides of the pagination contract at once:
every record comes back exactly once, across several pages.

**The sync engine is tested against real PostgreSQL**, and that is a deliberate
choice. The behaviour under test — `ON CONFLICT DO UPDATE`, the `xmax` trick,
advisory locks, transactional page writes — is behaviour *of the database*.
Mocking it would test the mock. 📄
[`syncer/service_test.go`](../apps/server/internal/syncer/service_test.go)
covers:

| Test | What it pins down |
| ---- | ----------------- |
| `TestSyncIsIdempotent` | run twice → no duplicates, 0 created, 0 updated |
| `TestSyncAppliesOnlyChangesSinceTheCursor` | only what changed, updated in place |
| `TestSyncIgnoresStaleRecords` | an old edit never overwrites newer data |
| `TestFailedSyncIsRecordedAndDoesNotAdvanceTheCursor` | failure is auditable and resumable |
| `TestVoidedRecordsLeaveTheActuals` | soft delete, row retained |
| `TestBudgetVsActualAfterSync` | sign convention and classification, end to end |
| `TestConcurrentRunsAreRefused` | the advisory lock holds |

They skip themselves when `TEST_DATABASE_URL` is unset, so `go test ./...`
still works on a machine with no database.

---

## Step 10 — One formatter per language

📄 [`tools/scripts/format-sql.mjs`](../tools/scripts/format-sql.mjs)

`oxfmt` handles the TypeScript and `gofmt` the Go, which left the SQL as the
only hand-aligned code in the repo — and hand-aligned SQL drifts the moment
two people touch it. `bun run format` now runs all three, and
`format:sql:check` fails a CI run the same way a linter would.

The formatter is `sql-formatter` driven by a ~40-line script: 2-space indent,
uppercase keywords, PostgreSQL dialect. Two details were needed to make it
usable here:

```js
paramTypes: { named: ["@"] },   // otherwise @source becomes "@ source"
```

Without that, sqlc stops recognising its own placeholders. The script also
collapses `sqlc.narg (x)` back to `sqlc.narg(x)`, which sqlc tolerates either
way but reads badly with the space.

The goose `-- +goose StatementBegin` / `StatementEnd` markers came out at the
same time. They are only needed for statements that contain their own
semicolons — functions, `DO` blocks — and none of these migrations do. Dropping
them removed a formatting edge case and made the files shorter.

---

## If someone asks about it in an interview

Short answers that hold up:

**"How do you know the sync is idempotent?"** The natural key is
`(source, external_id)` and the write is `ON CONFLICT DO UPDATE`. The second
run reports zero created and zero updated, and there is a test that runs it
twice and counts the rows.

**"What happens if the sync crashes halfway?"** The run is marked failed with
`cursor_after` NULL. The cursor only reads successful runs, so the next attempt
re-reads that window. Some work is repeated; nothing is skipped.

**"Why not store money as `numeric`?"** `numeric` would be defensible, but
cents in a `bigint` are exact, fast, trivially comparable, and impossible to
misconfigure with the wrong scale. The important half of the answer is the
other end: amounts are parsed from decimal *strings* so they never pass through
a float on the way in.

**"How do you tell an insert from an update in one statement?"** `RETURNING
(xmax = 0)`. It is zero only on a freshly inserted row.

**"Why basis points?"** So a percentage can cross the wire as an integer.
`12400` is 124.00%, with no float in the JSON and no rounding in the browser.

**"What would you change first?"** Batch the upserts — per-record round trips
are fine at a hundred records and wrong at a hundred thousand. Then auth and
per-organisation scoping, before anything else ships.
