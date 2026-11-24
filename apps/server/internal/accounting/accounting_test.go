package accounting_test

import (
	"errors"
	"testing"
	"time"

	"ledgerline/apps/server/internal/accounting"
	"ledgerline/apps/server/internal/money"
)

func at(day int, hour int) time.Time {
	return time.Date(2026, time.November, day, hour, 0, 0, 0, time.UTC)
}

func record(id string, updated time.Time) accounting.Record {
	return accounting.Record{
		ExternalID:   id,
		CategoryCode: "software",
		CategoryName: "Software & Subscriptions",
		Amount:       1000,
		Currency:     "USD",
		Direction:    accounting.DirectionDebit,
		OccurredOn:   time.Date(2026, time.November, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt:    updated,
	}
}

func TestPageRecordsWalksEveryRecordExactlyOnce(t *testing.T) {
	t.Parallel()

	var all []accounting.Record
	for i := 1; i <= 25; i++ {
		all = append(all, record(idFor(i), at(1+i%5, i)))
	}

	seen := map[string]int{}
	cursor := ""
	pages := 0

	for {
		page, err := accounting.PageRecords(all, time.Time{}, cursor, 7)
		if err != nil {
			t.Fatalf("PageRecords: %v", err)
		}
		pages++
		for _, r := range page.Records {
			seen[r.ExternalID]++
		}
		if !page.HasMore {
			break
		}
		cursor = page.NextCursor
		if pages > 10 {
			t.Fatal("pagination did not terminate")
		}
	}

	if len(seen) != len(all) {
		t.Errorf("walked %d distinct records, want %d", len(seen), len(all))
	}
	for id, count := range seen {
		if count != 1 {
			t.Errorf("record %s returned %d times, want exactly 1", id, count)
		}
	}
	if pages != 4 {
		t.Errorf("walked %d pages of 25 records at 7 per page, want 4", pages)
	}
}

// A timestamp-only cursor gets this wrong: it either skips the rest of the
// tied group or replays it forever.
func TestPageRecordsHandlesTiedTimestamps(t *testing.T) {
	t.Parallel()

	same := at(10, 9)
	all := []accounting.Record{
		record("txn_a", same),
		record("txn_b", same),
		record("txn_c", same),
		record("txn_d", same),
	}

	page1, err := accounting.PageRecords(all, time.Time{}, "", 2)
	if err != nil {
		t.Fatalf("page 1: %v", err)
	}
	if got := ids(page1.Records); got != "txn_a,txn_b" {
		t.Fatalf("page 1 = %s, want txn_a,txn_b", got)
	}
	if !page1.HasMore {
		t.Fatal("page 1 should report more records")
	}

	page2, err := accounting.PageRecords(all, time.Time{}, page1.NextCursor, 2)
	if err != nil {
		t.Fatalf("page 2: %v", err)
	}
	if got := ids(page2.Records); got != "txn_c,txn_d" {
		t.Fatalf("page 2 = %s, want txn_c,txn_d", got)
	}
	if page2.HasMore {
		t.Error("page 2 should be the last page")
	}
}

func TestPageRecordsFiltersBySince(t *testing.T) {
	t.Parallel()

	all := []accounting.Record{
		record("txn_old", at(1, 9)),
		record("txn_edge", at(5, 9)),
		record("txn_new", at(9, 9)),
	}

	// Inclusive: the overlap window must re-read the record sitting exactly on
	// the cursor.
	page, err := accounting.PageRecords(all, at(5, 9), "", 10)
	if err != nil {
		t.Fatalf("PageRecords: %v", err)
	}
	if got := ids(page.Records); got != "txn_edge,txn_new" {
		t.Errorf("records = %s, want txn_edge,txn_new", got)
	}
}

func TestPageRecordsRejectsBadInput(t *testing.T) {
	t.Parallel()

	if _, err := accounting.PageRecords(nil, time.Time{}, "", 0); err == nil {
		t.Error("limit of 0 should be rejected")
	}
	if _, err := accounting.PageRecords(nil, time.Time{}, "not-a-cursor", 10); !errors.Is(err, accounting.ErrInvalidCursor) {
		t.Error("a malformed cursor should return ErrInvalidCursor")
	}
}

func TestCursorRoundTrip(t *testing.T) {
	t.Parallel()

	in := record("txn_42", at(7, 13))
	key, err := accounting.DecodeCursor(accounting.EncodeCursor(in))
	if err != nil {
		t.Fatalf("DecodeCursor: %v", err)
	}
	if key.ExternalID != "txn_42" {
		t.Errorf("external id = %q, want txn_42", key.ExternalID)
	}
	if !key.UpdatedAt.Equal(in.UpdatedAt) {
		t.Errorf("updated at = %s, want %s", key.UpdatedAt, in.UpdatedAt)
	}
}

func TestWireToRecordParsesMoneyExactly(t *testing.T) {
	t.Parallel()

	wire := accounting.WireTransaction{
		ID:           "txn_00001",
		CategoryCode: "cloud",
		CategoryName: "Cloud & Hosting",
		Vendor:       "AWS",
		Description:  "AWS usage",
		Amount:       "1234.56",
		Currency:     "USD",
		Direction:    accounting.DirectionDebit,
		OccurredOn:   "2026-11-04",
		UpdatedAt:    at(4, 9),
	}

	got, err := wire.ToRecord()
	if err != nil {
		t.Fatalf("ToRecord: %v", err)
	}
	if got.Amount != money.Cents(123456) {
		t.Errorf("amount = %d cents, want 123456", got.Amount)
	}
	if got.OccurredOn.Format(time.DateOnly) != "2026-11-04" {
		t.Errorf("occurred on = %s, want 2026-11-04", got.OccurredOn)
	}

	if back := got.ToWire(); back.Amount != "1234.56" {
		t.Errorf("round-tripped amount = %q, want 1234.56", back.Amount)
	}
}

func TestWireToRecordRejectsBadRecords(t *testing.T) {
	t.Parallel()

	base := accounting.WireTransaction{
		ID:           "txn_00001",
		CategoryCode: "cloud",
		Amount:       "10.00",
		Direction:    accounting.DirectionDebit,
		OccurredOn:   "2026-11-04",
		UpdatedAt:    at(4, 9),
	}

	tests := []struct {
		name  string
		mutve func(*accounting.WireTransaction)
	}{
		{"missing id", func(w *accounting.WireTransaction) { w.ID = "" }},
		{"missing category", func(w *accounting.WireTransaction) { w.CategoryCode = "" }},
		{"unknown direction", func(w *accounting.WireTransaction) { w.Direction = "transfer" }},
		{"sub-cent amount", func(w *accounting.WireTransaction) { w.Amount = "10.005" }},
		{"amount is not a number", func(w *accounting.WireTransaction) { w.Amount = "ten dollars" }},
		{"bad date", func(w *accounting.WireTransaction) { w.OccurredOn = "04/11/2026" }},
		{"missing updated_at", func(w *accounting.WireTransaction) { w.UpdatedAt = time.Time{} }},
		{"negative amount", func(w *accounting.WireTransaction) { w.Amount = "-10.00" }},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := base
			tc.mutve(&w)
			if _, err := w.ToRecord(); err == nil {
				t.Errorf("ToRecord accepted a record with %s", tc.name)
			}
		})
	}
}

func TestRecordSignedAmount(t *testing.T) {
	t.Parallel()

	debit := record("txn_d", at(1, 9))
	if got := debit.SignedAmount(); got != 1000 {
		t.Errorf("debit signed amount = %d, want 1000", got)
	}

	credit := debit
	credit.Direction = accounting.DirectionCredit
	if got := credit.SignedAmount(); got != -1000 {
		t.Errorf("credit signed amount = %d, want -1000", got)
	}
}

func ids(records []accounting.Record) string {
	out := ""
	for i, r := range records {
		if i > 0 {
			out += ","
		}
		out += r.ExternalID
	}
	return out
}

func idFor(i int) string {
	return "txn_" + string(rune('a'+i%26)) + string(rune('0'+i/26))
}
