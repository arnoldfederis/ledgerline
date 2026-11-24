// Package mocksource stands in for a real accounting API.
//
// It serves an embedded fixture over HTTP with the same updated_since +
// opaque-cursor contract a real provider uses, so the sync job exercises
// genuine HTTP, pagination and incremental filtering in development.
package mocksource

import (
	"embed"
	"encoding/json/v2"
	"fmt"
	"time"

	"ledgerline/apps/server/internal/accounting"
)

// Three months across eight categories, including refunds, voided records and
// rows whose updated_at is later than their transaction date so incremental
// syncs have something to pick up. Regenerate with `go generate ./...`.
//
//go:generate go run ./data/generate.go
//go:embed data/transactions.json
var dataFS embed.FS

const fixturePath = "data/transactions.json"

// Load reads the embedded fixture into an in-memory source.
func Load(name string) (*accounting.StaticSource, error) {
	raw, err := dataFS.ReadFile(fixturePath)
	if err != nil {
		return nil, fmt.Errorf("mocksource: read fixture: %w", err)
	}

	var file accounting.WireFile
	if err := json.Unmarshal(raw, &file); err != nil {
		return nil, fmt.Errorf("mocksource: decode fixture: %w", err)
	}

	records, err := accounting.RecordsFromWire(file.Transactions)
	if err != nil {
		return nil, fmt.Errorf("mocksource: %w", err)
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("mocksource: fixture %s contains no transactions", fixturePath)
	}

	return accounting.NewStaticSource(name, records), nil
}

// LatestChange reports the newest updated_at in the fixture.
func LatestChange(src *accounting.StaticSource) time.Time {
	var latest time.Time
	for _, r := range src.Records() {
		if r.UpdatedAt.After(latest) {
			latest = r.UpdatedAt
		}
	}
	return latest
}
