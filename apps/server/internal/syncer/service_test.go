package syncer_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"ledgerline/apps/server/internal/accounting"
	"ledgerline/apps/server/internal/config"
	"ledgerline/apps/server/internal/database"
	"ledgerline/apps/server/internal/ledger"
	"ledgerline/apps/server/internal/money"
	"ledgerline/apps/server/internal/syncer"
)

// These run against real PostgreSQL: ON CONFLICT DO UPDATE, the xmax trick,
// advisory locks and transactional page writes are database behaviour, and
// mocking them would test the mock.
//
// TEST_DATABASE_URL must point at a database you do not mind having truncated.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	// go test does not read .env; pick it up the way the server does.
	config.LoadEnvFiles("../../.env", "../../../../.env")

	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set; skipping database-backed sync tests")
	}

	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	pool, err := database.Open(ctx, url, log)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := database.Migrate(ctx, pool, log); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}

	// Seeded categories and budgets stay: the reports depend on them.
	if _, err := pool.Exec(ctx, `TRUNCATE app.transactions, app.sync_runs RESTART IDENTITY`); err != nil {
		t.Fatalf("reset test database: %v", err)
	}
	return pool
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func fixtureRecords(t *testing.T, n int) []accounting.Record {
	t.Helper()

	records := make([]accounting.Record, 0, n)
	for i := range n {
		records = append(records, accounting.Record{
			ExternalID:   fmt.Sprintf("txn_%04d", i),
			CategoryCode: "software",
			CategoryName: "Software & Subscriptions",
			Vendor:       "Figma",
			Description:  "Figma subscription",
			Amount:       money.Cents(10_000 + i),
			Currency:     "USD",
			Direction:    accounting.DirectionDebit,
			OccurredOn:   time.Date(2026, time.November, 1+i%27, 0, 0, 0, 0, time.UTC),
			UpdatedAt:    time.Date(2026, time.November, 1+i%27, 9, 0, 0, 0, time.UTC),
		})
	}
	return records
}

func newService(t *testing.T, pool *pgxpool.Pool, source accounting.Source) *syncer.Service {
	t.Helper()
	return syncer.NewService(pool, source, discardLogger(), syncer.Options{
		PageSize: 10, // small enough that every test spans several pages
		Overlap:  5 * time.Minute,
	})
}

func countTransactions(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()

	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*)::int FROM app.transactions`).Scan(&n); err != nil {
		t.Fatalf("count transactions: %v", err)
	}
	return n
}

// The headline property: a second run duplicates nothing and says so.
func TestSyncIsIdempotent(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	records := fixtureRecords(t, 35)
	source := accounting.NewStaticSource("mock_accounting", records)
	service := newService(t, pool, source)

	first, err := service.Run(ctx, syncer.TriggerManual)
	if err != nil {
		t.Fatalf("first sync: %v", err)
	}
	if first.Status != syncer.StatusSuccess {
		t.Fatalf("first sync status = %s, want success", first.Status)
	}
	if first.RecordsCreated != len(records) {
		t.Errorf("first sync created %d records, want %d", first.RecordsCreated, len(records))
	}
	if first.RecordsUpdated != 0 || first.RecordsSkipped != 0 {
		t.Errorf("first sync updated %d and skipped %d, want 0 and 0",
			first.RecordsUpdated, first.RecordsSkipped)
	}
	if first.PagesFetched < 3 {
		t.Errorf("first sync walked %d pages of 35 records at 10 per page, want at least 3",
			first.PagesFetched)
	}
	if got := countTransactions(t, pool); got != len(records) {
		t.Fatalf("database holds %d transactions after the first sync, want %d", got, len(records))
	}

	second, err := service.Run(ctx, syncer.TriggerManual)
	if err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if second.RecordsCreated != 0 {
		t.Errorf("second sync created %d records, want 0 -- the upsert is not idempotent",
			second.RecordsCreated)
	}
	if second.RecordsUpdated != 0 {
		t.Errorf("second sync updated %d records, want 0 -- the source_updated_at guard is not working",
			second.RecordsUpdated)
	}
	if second.RecordsSkipped == 0 {
		t.Error("second sync skipped nothing; the overlap window re-read no records at all")
	}
	if got := countTransactions(t, pool); got != len(records) {
		t.Errorf("database holds %d transactions after the second sync, want %d -- duplicates were created",
			got, len(records))
	}
}

// Only what changed upstream comes back, and it updates in place.
func TestSyncAppliesOnlyChangesSinceTheCursor(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	records := fixtureRecords(t, 20)
	source := accounting.NewStaticSource("mock_accounting", records)
	service := newService(t, pool, source)

	if _, err := service.Run(ctx, syncer.TriggerStartup); err != nil {
		t.Fatalf("initial sync: %v", err)
	}

	// Both stamped later than anything synced so far.
	later := time.Date(2026, time.December, 1, 12, 0, 0, 0, time.UTC)

	amended := records[3]
	amended.Amount = money.Cents(999_99)
	amended.Description = "Figma subscription (corrected)"
	amended.UpdatedAt = later
	source.Upsert(amended)

	added := records[0]
	added.ExternalID = "txn_new_0001"
	added.Amount = money.Cents(4_200)
	added.UpdatedAt = later.Add(time.Minute)
	source.Upsert(added)

	result, err := service.Run(ctx, syncer.TriggerScheduler)
	if err != nil {
		t.Fatalf("incremental sync: %v", err)
	}

	if result.RecordsCreated != 1 {
		t.Errorf("created %d records, want 1", result.RecordsCreated)
	}
	if result.RecordsUpdated != 1 {
		t.Errorf("updated %d records, want 1", result.RecordsUpdated)
	}
	if got := countTransactions(t, pool); got != len(records)+1 {
		t.Errorf("database holds %d transactions, want %d", got, len(records)+1)
	}

	var amountCents int64
	var description string
	if err := pool.QueryRow(ctx,
		`SELECT amount_cents, description FROM app.transactions WHERE external_id = $1`,
		amended.ExternalID).Scan(&amountCents, &description); err != nil {
		t.Fatalf("read amended transaction: %v", err)
	}
	if amountCents != 99999 {
		t.Errorf("amended amount = %d cents, want 99999", amountCents)
	}
	if description != "Figma subscription (corrected)" {
		t.Errorf("amended description = %q, want the corrected one", description)
	}
}

// Out-of-order delivery is normal with an overlapping window.
func TestSyncIgnoresStaleRecords(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	records := fixtureRecords(t, 5)
	source := accounting.NewStaticSource("mock_accounting", records)
	service := newService(t, pool, source)

	if _, err := service.Run(ctx, syncer.TriggerManual); err != nil {
		t.Fatalf("initial sync: %v", err)
	}

	// Newer version lands first.
	newer := records[2]
	newer.Amount = money.Cents(500_00)
	newer.UpdatedAt = time.Date(2026, time.December, 10, 0, 0, 0, 0, time.UTC)
	source.Upsert(newer)
	if _, err := service.Run(ctx, syncer.TriggerManual); err != nil {
		t.Fatalf("sync of newer version: %v", err)
	}

	// Now an older edit of the same record arrives.
	stale := newer
	stale.Amount = money.Cents(1_00)
	stale.UpdatedAt = time.Date(2026, time.November, 2, 0, 0, 0, 0, time.UTC)
	source.Upsert(stale)
	result, err := service.Run(ctx, syncer.TriggerManual)
	if err != nil {
		t.Fatalf("sync of stale version: %v", err)
	}
	if result.RecordsUpdated != 0 {
		t.Errorf("a stale record updated %d rows, want 0", result.RecordsUpdated)
	}

	var amountCents int64
	if err := pool.QueryRow(ctx,
		`SELECT amount_cents FROM app.transactions WHERE external_id = $1`,
		newer.ExternalID).Scan(&amountCents); err != nil {
		t.Fatalf("read transaction: %v", err)
	}
	if amountCents != 50000 {
		t.Errorf("amount = %d cents, want 50000 -- a stale record overwrote newer data", amountCents)
	}
}

// A failed run stays visible and leaves the cursor where it was.
func TestFailedSyncIsRecordedAndDoesNotAdvanceTheCursor(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	source := accounting.NewStaticSource("mock_accounting", fixtureRecords(t, 12))
	service := newService(t, pool, source)

	good, err := service.Run(ctx, syncer.TriggerManual)
	if err != nil {
		t.Fatalf("initial sync: %v", err)
	}
	if good.CursorAfter == nil {
		t.Fatal("a successful sync left no cursor")
	}

	source.FailWith = errors.New("upstream accounting API is down")
	bad, err := service.Run(ctx, syncer.TriggerScheduler)
	if err == nil {
		t.Fatal("sync against a failing source returned no error")
	}
	if bad.Status != syncer.StatusFailed {
		t.Errorf("run status = %s, want failed", bad.Status)
	}
	if bad.CursorAfter != nil {
		t.Error("a failed run advanced the cursor; the next run would skip records")
	}
	if bad.ErrorMessage == "" {
		t.Error("a failed run recorded no error message")
	}

	source.FailWith = nil
	recovered, err := service.Run(ctx, syncer.TriggerScheduler)
	if err != nil {
		t.Fatalf("recovery sync: %v", err)
	}
	if recovered.CursorBefore == nil || !recovered.CursorBefore.Equal(*good.CursorAfter) {
		t.Errorf("recovery resumed from %v, want the last successful cursor %v",
			recovered.CursorBefore, good.CursorAfter)
	}

	ledgerService := ledger.NewService(pool)
	runs, err := ledgerService.SyncRuns(ctx, 10)
	if err != nil {
		t.Fatalf("read sync runs: %v", err)
	}
	if len(runs) != 3 {
		t.Fatalf("audit trail holds %d runs, want 3", len(runs))
	}
	if runs[1].Status != syncer.StatusFailed {
		t.Errorf("the middle run is %s, want failed", runs[1].Status)
	}
	for _, r := range runs {
		if r.FinishedAt == nil || r.DurationMs == nil {
			t.Errorf("run %d was never closed out: finished_at or duration_ms is null", r.ID)
		}
	}
}

// A void soft-deletes and drops straight out of the actuals.
func TestVoidedRecordsLeaveTheActuals(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	month := time.Date(2026, time.November, 1, 0, 0, 0, 0, time.UTC)
	records := []accounting.Record{
		{
			ExternalID: "txn_keep", CategoryCode: "software", CategoryName: "Software & Subscriptions",
			Amount: money.Cents(100_00), Currency: "USD", Direction: accounting.DirectionDebit,
			OccurredOn: month, UpdatedAt: month.Add(time.Hour),
		},
		{
			ExternalID: "txn_void", CategoryCode: "software", CategoryName: "Software & Subscriptions",
			Amount: money.Cents(40_00), Currency: "USD", Direction: accounting.DirectionDebit,
			OccurredOn: month, UpdatedAt: month.Add(2 * time.Hour),
		},
		{
			ExternalID: "txn_refund", CategoryCode: "software", CategoryName: "Software & Subscriptions",
			Amount: money.Cents(10_00), Currency: "USD", Direction: accounting.DirectionCredit,
			OccurredOn: month, UpdatedAt: month.Add(3 * time.Hour),
		},
	}

	source := accounting.NewStaticSource("mock_accounting", records)
	service := newService(t, pool, source)
	if _, err := service.Run(ctx, syncer.TriggerManual); err != nil {
		t.Fatalf("initial sync: %v", err)
	}

	report, err := ledger.NewService(pool).BudgetVsActual(ctx, month)
	if err != nil {
		t.Fatalf("report before void: %v", err)
	}
	// 100.00 - 10.00 + 40.00
	if got := lineFor(t, report, "software").ActualCents; got != 13000 {
		t.Fatalf("actuals before the void = %d cents, want 13000", got)
	}

	voided := records[1]
	voided.Voided = true
	voided.UpdatedAt = month.AddDate(0, 1, 0)
	source.Upsert(voided)

	if _, err := service.Run(ctx, syncer.TriggerScheduler); err != nil {
		t.Fatalf("sync of the void: %v", err)
	}

	report, err = ledger.NewService(pool).BudgetVsActual(ctx, month)
	if err != nil {
		t.Fatalf("report after void: %v", err)
	}
	if got := lineFor(t, report, "software").ActualCents; got != 9000 {
		t.Errorf("actuals after the void = %d cents, want 9000", got)
	}

	if got := countTransactions(t, pool); got != 3 {
		t.Errorf("database holds %d transactions, want 3 -- the void deleted a row", got)
	}
}

// Pins the sign convention and the over-budget classification end to end.
func TestBudgetVsActualAfterSync(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	month := time.Date(2026, time.November, 1, 0, 0, 0, 0, time.UTC)

	// The seeded November budget for software is 2,750.00.
	records := []accounting.Record{
		{
			ExternalID: "txn_1", CategoryCode: "software", CategoryName: "Software & Subscriptions",
			Amount: money.Cents(3_000_00), Currency: "USD", Direction: accounting.DirectionDebit,
			OccurredOn: month, UpdatedAt: month.Add(time.Hour),
		},
		{
			ExternalID: "txn_2", CategoryCode: "software", CategoryName: "Software & Subscriptions",
			Amount: money.Cents(100_00), Currency: "USD", Direction: accounting.DirectionCredit,
			OccurredOn: month, UpdatedAt: month.Add(2 * time.Hour),
		},
	}

	service := newService(t, pool, accounting.NewStaticSource("mock_accounting", records))
	if _, err := service.Run(ctx, syncer.TriggerManual); err != nil {
		t.Fatalf("sync: %v", err)
	}

	report, err := ledger.NewService(pool).BudgetVsActual(ctx, month)
	if err != nil {
		t.Fatalf("report: %v", err)
	}

	line := lineFor(t, report, "software")
	if line.BudgetCents != 275000 {
		t.Fatalf("budget = %d cents, want 275000 (check the seed migration)", line.BudgetCents)
	}
	// 3,000.00 less a 100.00 refund.
	if line.ActualCents != 290000 {
		t.Errorf("actual = %d cents, want 290000", line.ActualCents)
	}
	if line.VarianceCents != -15000 {
		t.Errorf("variance = %d cents, want -15000", line.VarianceCents)
	}
	if line.Status != ledger.StatusOverBudget {
		t.Errorf("status = %s, want %s", line.Status, ledger.StatusOverBudget)
	}
	// 290000 / 275000 = 105.4545% -> 10545 bps.
	if line.UtilizationBps != 10545 {
		t.Errorf("utilization = %d bps, want 10545", line.UtilizationBps)
	}
	if line.TransactionCount != 2 {
		t.Errorf("transaction count = %d, want 2", line.TransactionCount)
	}
	if report.Totals.OverBudgetCount != 1 {
		t.Errorf("over-budget categories = %d, want 1", report.Totals.OverBudgetCount)
	}
}

// Two runs at once must not interleave.
func TestConcurrentRunsAreRefused(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	source := accounting.NewStaticSource("mock_accounting", fixtureRecords(t, 5))

	// One service holds the lock, paused inside its first fetch; a second,
	// standing in for another process, then tries.
	blocking := &blockingSource{
		inner:   source,
		release: make(chan struct{}),
		entered: make(chan struct{}),
	}
	holder := syncer.NewService(pool, blocking, discardLogger(), syncer.Options{PageSize: 10})
	contender := newService(t, pool, source)

	done := make(chan error, 1)
	go func() {
		_, err := holder.Run(ctx, syncer.TriggerScheduler)
		done <- err
	}()

	<-blocking.entered
	_, err := contender.Run(ctx, syncer.TriggerManual)
	if !errors.Is(err, syncer.ErrLocked) {
		t.Errorf("concurrent run returned %v, want ErrLocked", err)
	}
	close(blocking.release)

	if err := <-done; err != nil {
		t.Fatalf("the holding run failed: %v", err)
	}
}

// blockingSource pauses a sync inside its first fetch.
type blockingSource struct {
	inner   accounting.Source
	release chan struct{}
	entered chan struct{}
	once    bool
}

func (b *blockingSource) Name() string { return b.inner.Name() }

func (b *blockingSource) FetchChanged(ctx context.Context, since time.Time, cursor string, limit int) (accounting.Page, error) {
	if !b.once {
		b.once = true
		close(b.entered)
		<-b.release
	}
	return b.inner.FetchChanged(ctx, since, cursor, limit)
}

func lineFor(t *testing.T, report ledger.Report, code string) ledger.Line {
	t.Helper()
	for _, line := range report.Categories {
		if line.CategoryCode == code {
			return line
		}
	}
	t.Fatalf("report has no line for category %q", code)
	return ledger.Line{}
}

// The dashboard fills its month picker from Months() and lets the API choose
// the default month, so the first entry of one must be the other. If the two
// orderings ever diverge the picker shows a month the report is not for.
func TestLatestMonthIsTheFirstReportableMonth(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	service := newService(t, pool, accounting.NewStaticSource("mock_accounting", fixtureRecords(t, 6)))
	if _, err := service.Run(ctx, syncer.TriggerManual); err != nil {
		t.Fatalf("sync: %v", err)
	}

	ledgerService := ledger.NewService(pool)

	months, err := ledgerService.Months(ctx)
	if err != nil {
		t.Fatalf("list months: %v", err)
	}
	if len(months) == 0 {
		t.Fatal("no reportable months after a sync")
	}

	latest, err := ledgerService.LatestMonth(ctx)
	if err != nil {
		t.Fatalf("latest month: %v", err)
	}

	if got := ledger.FormatMonth(latest); got != months[0] {
		t.Errorf("latest month is %s but the picker would select %s", got, months[0])
	}
}
