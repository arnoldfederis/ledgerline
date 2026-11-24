package mocksource_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ledgerline/apps/server/internal/accounting"
	"ledgerline/apps/server/internal/mocksource"
)

// A real socket, so these tests exercise the genuine path: request, encode,
// decode, cursor handling.
func newServer(t *testing.T) (*accounting.HTTPSource, *accounting.StaticSource) {
	t.Helper()

	records, err := mocksource.Load("mock_accounting")
	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /mock/accounting/transactions", mocksource.NewHandler(records).Transactions)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return accounting.NewHTTPSource("mock_accounting", srv.URL+"/mock/accounting/transactions", 5*time.Second), records
}

func TestFixtureLoads(t *testing.T) {
	t.Parallel()

	records, err := mocksource.Load("mock_accounting")
	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}

	all := records.Records()
	if len(all) < 100 {
		t.Fatalf("fixture holds %d records, expected a demo ledger of at least 100", len(all))
	}

	seen := map[string]bool{}
	for _, r := range all {
		if err := r.Validate(); err != nil {
			t.Errorf("fixture record is invalid: %v", err)
		}
		if seen[r.ExternalID] {
			t.Errorf("fixture contains duplicate external id %s", r.ExternalID)
		}
		seen[r.ExternalID] = true
	}
}

func TestHTTPSourceWalksEveryPage(t *testing.T) {
	t.Parallel()

	source, fixture := newServer(t)
	ctx := context.Background()

	seen := map[string]int{}
	cursor := ""
	pages := 0

	for {
		page, err := source.FetchChanged(ctx, time.Time{}, cursor, 25)
		if err != nil {
			t.Fatalf("fetch page %d: %v", pages+1, err)
		}
		pages++
		for _, r := range page.Records {
			seen[r.ExternalID]++
		}
		if !page.HasMore {
			break
		}
		cursor = page.NextCursor
		if pages > 50 {
			t.Fatal("pagination did not terminate")
		}
	}

	want := len(fixture.Records())
	if len(seen) != want {
		t.Errorf("fetched %d distinct records across %d pages, want %d", len(seen), pages, want)
	}
	for id, count := range seen {
		if count != 1 {
			t.Errorf("record %s came back %d times, want exactly 1", id, count)
		}
	}
	if pages < 2 {
		t.Errorf("fetched everything in %d page(s); the fixture should need several", pages)
	}
}

func TestHTTPSourceFiltersByUpdatedSince(t *testing.T) {
	t.Parallel()

	source, fixture := newServer(t)
	ctx := context.Background()

	all := fixture.Records()
	accounting.SortRecords(all)
	cut := all[len(all)/2].UpdatedAt

	page, err := source.FetchChanged(ctx, cut, "", 200)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}

	if len(page.Records) == 0 || len(page.Records) == len(all) {
		t.Fatalf("updated_since returned %d of %d records, expected a strict subset",
			len(page.Records), len(all))
	}
	for _, r := range page.Records {
		if r.UpdatedAt.Before(cut) {
			t.Errorf("record %s updated %s is older than the cut-off %s", r.ExternalID, r.UpdatedAt, cut)
		}
	}
}

func TestHTTPSourceRejectsBadParameters(t *testing.T) {
	t.Parallel()

	_, records := newServer(t)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /mock/accounting/transactions", mocksource.NewHandler(records).Transactions)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	tests := []struct {
		name  string
		query string
	}{
		{"unparsable updated_since", "?updated_since=yesterday"},
		{"limit is not a number", "?limit=lots"},
		{"limit too large", "?limit=10000"},
		{"limit of zero", "?limit=0"},
		{"corrupt cursor", "?cursor=not-a-valid-cursor"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := srv.Client().Get(srv.URL + "/mock/accounting/transactions" + tc.query)
			if err != nil {
				t.Fatalf("request: %v", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", resp.StatusCode)
			}
		})
	}
}
