// Package ledger turns rows into budget, actual, variance and utilization per
// category. Every amount is money.Cents and utilization is in basis points, so
// nothing on the way to the browser can round.
package ledger

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"ledgerline/apps/server/internal/database/sqlc"
	"ledgerline/apps/server/internal/money"
)

const (
	StatusOnTrack    = "on_track"
	StatusAtRisk     = "at_risk"
	StatusOverBudget = "over_budget"
	// StatusUnbudgeted is spend against a category with no approved budget:
	// a variance you cannot compute, and the one worth flagging.
	StatusUnbudgeted = "unbudgeted"
)

// atRiskBasisPoints is the share of budget at which a category is called out.
const atRiskBasisPoints = 9000

// ErrNoData means there is no budget or transaction in any month yet.
var ErrNoData = errors.New("ledger: no budgets or transactions available")

type Line struct {
	CategoryCode     string `json:"category_code"`
	CategoryName     string `json:"category_name"`
	BudgetCents      int64  `json:"budget_cents"`
	ActualCents      int64  `json:"actual_cents"`
	VarianceCents    int64  `json:"variance_cents"`
	UtilizationBps   int64  `json:"utilization_bps"`
	Status           string `json:"status"`
	Budgeted         bool   `json:"budgeted"`
	TransactionCount int64  `json:"transaction_count"`
}

type Totals struct {
	BudgetCents      int64 `json:"budget_cents"`
	ActualCents      int64 `json:"actual_cents"`
	VarianceCents    int64 `json:"variance_cents"`
	UtilizationBps   int64 `json:"utilization_bps"`
	CategoryCount    int   `json:"category_count"`
	OverBudgetCount  int   `json:"over_budget_count"`
	TransactionCount int64 `json:"transaction_count"`
}

type Report struct {
	Month       string `json:"month"`
	PeriodStart string `json:"period_start"`
	PeriodEnd   string `json:"period_end"`
	Currency    string `json:"currency"`
	Totals      Totals `json:"totals"`
	Categories  []Line `json:"categories"`
}

type Service struct {
	queries *sqlc.Queries
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{queries: sqlc.New(pool)}
}

// ParseMonth accepts "2026-11" and returns the first instant of that month in
// UTC, which is how period_month is stored.
func ParseMonth(s string) (time.Time, error) {
	t, err := time.ParseInLocation("2006-01", s, time.UTC)
	if err != nil {
		return time.Time{}, fmt.Errorf("ledger: month must look like 2026-11, got %q", s)
	}
	return t, nil
}

func FormatMonth(t time.Time) string { return t.UTC().Format("2006-01") }

// BudgetVsActual builds the report for one month. Variance is budget - actual,
// so positive is money left and negative is an overspend; that convention is
// stated here once and never re-derived downstream.
func (s *Service) BudgetVsActual(ctx context.Context, month time.Time) (Report, error) {
	rows, err := s.queries.GetBudgetVsActual(ctx, month)
	if err != nil {
		return Report{}, fmt.Errorf("ledger: read budget vs actual: %w", err)
	}

	report := Report{
		Month:       FormatMonth(month),
		PeriodStart: month.UTC().Format(time.DateOnly),
		PeriodEnd:   month.UTC().AddDate(0, 1, -1).Format(time.DateOnly),
		Currency:    "USD",
		Categories:  make([]Line, 0, len(rows)),
	}

	var totalBudget, totalActual money.Cents
	for _, row := range rows {
		budget := money.Cents(row.BudgetCents)
		actual := money.Cents(row.ActualCents)

		line := Line{
			CategoryCode:     row.CategoryCode,
			CategoryName:     row.CategoryName,
			BudgetCents:      int64(budget),
			ActualCents:      int64(actual),
			VarianceCents:    int64(budget.Sub(actual)),
			UtilizationBps:   money.UtilizationBasisPoints(actual, budget),
			Status:           status(budget, actual, row.HasBudget),
			Budgeted:         row.HasBudget,
			TransactionCount: row.TransactionCount,
		}

		totalBudget = totalBudget.Add(budget)
		totalActual = totalActual.Add(actual)
		if line.Status == StatusOverBudget {
			report.Totals.OverBudgetCount++
		}
		report.Totals.TransactionCount += row.TransactionCount
		report.Categories = append(report.Categories, line)
	}

	report.Totals.BudgetCents = int64(totalBudget)
	report.Totals.ActualCents = int64(totalActual)
	report.Totals.VarianceCents = int64(totalBudget.Sub(totalActual))
	report.Totals.UtilizationBps = money.UtilizationBasisPoints(totalActual, totalBudget)
	report.Totals.CategoryCount = len(report.Categories)

	return report, nil
}

func status(budget, actual money.Cents, budgeted bool) string {
	switch {
	case !budgeted:
		return StatusUnbudgeted
	case actual > budget:
		return StatusOverBudget
	case money.UtilizationBasisPoints(actual, budget) >= atRiskBasisPoints:
		return StatusAtRisk
	default:
		return StatusOnTrack
	}
}

// Months lists every month the dashboard can show, newest first.
func (s *Service) Months(ctx context.Context) ([]string, error) {
	rows, err := s.queries.ListReportableMonths(ctx)
	if err != nil {
		return nil, fmt.Errorf("ledger: list months: %w", err)
	}

	months := make([]string, 0, len(rows))
	for _, m := range rows {
		months = append(months, FormatMonth(m))
	}
	return months, nil
}

// LatestMonth is the most recent month with data. The dashboard opens here so
// a fresh database shows a populated screen rather than today's empty one.
func (s *Service) LatestMonth(ctx context.Context) (time.Time, error) {
	rows, err := s.queries.ListReportableMonths(ctx)
	if err != nil {
		return time.Time{}, fmt.Errorf("ledger: find latest month: %w", err)
	}
	if len(rows) == 0 {
		return time.Time{}, ErrNoData
	}
	return rows[0].UTC(), nil
}

type Category struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

func (s *Service) Categories(ctx context.Context) ([]Category, error) {
	rows, err := s.queries.ListCategories(ctx)
	if err != nil {
		return nil, fmt.Errorf("ledger: list categories: %w", err)
	}

	out := make([]Category, 0, len(rows))
	for _, c := range rows {
		out = append(out, Category{Code: c.Code, Name: c.Name})
	}
	return out, nil
}

type Transaction struct {
	ID           int64  `json:"id"`
	ExternalID   string `json:"external_id"`
	CategoryCode string `json:"category_code"`
	CategoryName string `json:"category_name"`
	Vendor       string `json:"vendor"`
	Description  string `json:"description"`
	AmountCents  int64  `json:"amount_cents"`
	// SignedAmountCents is the amount as it affects actuals, so the dashboard
	// need not know the debit/credit convention.
	SignedAmountCents int64  `json:"signed_amount_cents"`
	Currency          string `json:"currency"`
	Direction         string `json:"direction"`
	OccurredOn        string `json:"occurred_on"`
	SourceUpdatedAt   string `json:"source_updated_at"`
}

// TransactionFilter narrows a listing; a nil field means "any".
type TransactionFilter struct {
	Month        *time.Time
	CategoryCode *string
	Limit        int32
	Offset       int32
}

type TransactionPage struct {
	Transactions []Transaction `json:"transactions"`
	Total        int64         `json:"total"`
	Limit        int32         `json:"limit"`
	Offset       int32         `json:"offset"`
}

func (s *Service) Transactions(ctx context.Context, f TransactionFilter) (TransactionPage, error) {
	rows, err := s.queries.ListTransactions(ctx, sqlc.ListTransactionsParams{
		PeriodMonth:  f.Month,
		CategoryCode: f.CategoryCode,
		RowLimit:     f.Limit,
		RowOffset:    f.Offset,
	})
	if err != nil {
		return TransactionPage{}, fmt.Errorf("ledger: list transactions: %w", err)
	}

	total, err := s.queries.CountTransactions(ctx, sqlc.CountTransactionsParams{
		PeriodMonth:  f.Month,
		CategoryCode: f.CategoryCode,
	})
	if err != nil {
		return TransactionPage{}, fmt.Errorf("ledger: count transactions: %w", err)
	}

	page := TransactionPage{
		Transactions: make([]Transaction, 0, len(rows)),
		Total:        total,
		Limit:        f.Limit,
		Offset:       f.Offset,
	}
	for _, row := range rows {
		amount := money.Cents(row.AmountCents)
		page.Transactions = append(page.Transactions, Transaction{
			ID:                row.ID,
			ExternalID:        row.ExternalID,
			CategoryCode:      row.CategoryCode,
			CategoryName:      row.CategoryName,
			Vendor:            row.Vendor,
			Description:       row.Description,
			AmountCents:       int64(amount),
			SignedAmountCents: int64(money.Signed(amount, row.Direction)),
			Currency:          row.Currency,
			Direction:         row.Direction,
			OccurredOn:        row.OccurredOn.UTC().Format(time.DateOnly),
			SourceUpdatedAt:   row.SourceUpdatedAt.UTC().Format(time.RFC3339),
		})
	}
	return page, nil
}

type SyncRun struct {
	ID             int64      `json:"id"`
	Source         string     `json:"source"`
	Trigger        string     `json:"trigger"`
	Status         string     `json:"status"`
	CursorBefore   *time.Time `json:"cursor_before"`
	CursorAfter    *time.Time `json:"cursor_after"`
	PagesFetched   int32      `json:"pages_fetched"`
	RecordsFetched int32      `json:"records_fetched"`
	RecordsCreated int32      `json:"records_created"`
	RecordsUpdated int32      `json:"records_updated"`
	RecordsSkipped int32      `json:"records_skipped"`
	ErrorMessage   *string    `json:"error_message"`
	StartedAt      time.Time  `json:"started_at"`
	FinishedAt     *time.Time `json:"finished_at"`
	DurationMs     *int32     `json:"duration_ms"`
}

// SyncRuns returns the most recent sync attempts, newest first.
func (s *Service) SyncRuns(ctx context.Context, limit int32) ([]SyncRun, error) {
	rows, err := s.queries.ListSyncRuns(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("ledger: list sync runs: %w", err)
	}

	out := make([]SyncRun, 0, len(rows))
	for _, r := range rows {
		out = append(out, SyncRun{
			ID:             r.ID,
			Source:         r.Source,
			Trigger:        r.Trigger,
			Status:         r.Status,
			CursorBefore:   utcPtr(r.CursorBefore),
			CursorAfter:    utcPtr(r.CursorAfter),
			PagesFetched:   r.PagesFetched,
			RecordsFetched: r.RecordsFetched,
			RecordsCreated: r.RecordsCreated,
			RecordsUpdated: r.RecordsUpdated,
			RecordsSkipped: r.RecordsSkipped,
			ErrorMessage:   r.ErrorMessage,
			StartedAt:      r.StartedAt.UTC(),
			FinishedAt:     utcPtr(r.FinishedAt),
			DurationMs:     r.DurationMs,
		})
	}
	return out, nil
}

func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	utc := t.UTC()
	return &utc
}
