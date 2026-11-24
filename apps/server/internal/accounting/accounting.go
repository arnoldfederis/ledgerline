// Package accounting is the boundary with the upstream accounting system:
// ask for everything modified since a timestamp, walk it a page at a time.
package accounting

import (
	"cmp"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"ledgerline/apps/server/internal/money"
)

// A debit increases spend against a budget; a credit (refund, reversal)
// decreases it.
const (
	DirectionDebit  = "debit"
	DirectionCredit = "credit"
)

var ErrInvalidCursor = errors.New("accounting: invalid cursor")

// Record is one upstream transaction, already in exact cents.
type Record struct {
	ExternalID   string
	CategoryCode string
	CategoryName string
	Vendor       string
	Description  string
	Amount       money.Cents
	Currency     string
	Direction    string
	OccurredOn   time.Time
	UpdatedAt    time.Time
	Voided       bool
}

func (r Record) SignedAmount() money.Cents { return money.Signed(r.Amount, r.Direction) }

type Page struct {
	Records    []Record
	NextCursor string
	HasMore    bool
}

// Source is an accounting system that can be polled for changes.
type Source interface {
	// Name is stored on every transaction and sync run, so two sources never
	// share a cursor.
	Name() string

	// FetchChanged returns records changed at or after since, oldest first. An
	// empty cursor starts at the beginning of that window.
	FetchChanged(ctx context.Context, since time.Time, cursor string, limit int) (Page, error)
}

// Validate checks the invariants the database also enforces, so a malformed
// record fails with a useful message instead of a constraint violation.
func (r Record) Validate() error {
	switch {
	case strings.TrimSpace(r.ExternalID) == "":
		return fmt.Errorf("accounting: record is missing an id")
	case strings.TrimSpace(r.CategoryCode) == "":
		return fmt.Errorf("accounting: record %s is missing a category code", r.ExternalID)
	case r.Direction != DirectionDebit && r.Direction != DirectionCredit:
		return fmt.Errorf("accounting: record %s has unknown direction %q", r.ExternalID, r.Direction)
	case r.Amount < 0:
		return fmt.Errorf("accounting: record %s has negative amount %s (use direction %q instead)",
			r.ExternalID, r.Amount, DirectionCredit)
	case r.OccurredOn.IsZero():
		return fmt.Errorf("accounting: record %s is missing occurred_on", r.ExternalID)
	case r.UpdatedAt.IsZero():
		return fmt.Errorf("accounting: record %s is missing updated_at", r.ExternalID)
	}
	return nil
}

// SortRecords orders records oldest change first. The id tiebreaker matters:
// records sharing a timestamp are normal, and a keyset cursor would otherwise
// skip or repeat part of a tied group.
func SortRecords(records []Record) {
	slices.SortStableFunc(records, func(a, b Record) int {
		if c := a.UpdatedAt.Compare(b.UpdatedAt); c != 0 {
			return c
		}
		return cmp.Compare(a.ExternalID, b.ExternalID)
	})
}

// PageRecords applies the Source contract to an in-memory slice.
//
// The cursor is a keyset, not an offset: an offset silently skips records when
// the upstream data shifts between two page requests.
func PageRecords(records []Record, since time.Time, cursor string, limit int) (Page, error) {
	if limit < 1 {
		return Page{}, fmt.Errorf("accounting: limit must be at least 1, got %d", limit)
	}

	sorted := slices.Clone(records)
	SortRecords(sorted)

	window := make([]Record, 0, len(sorted))
	for _, r := range sorted {
		if since.IsZero() || !r.UpdatedAt.Before(since) {
			window = append(window, r)
		}
	}

	if cursor != "" {
		after, err := DecodeCursor(cursor)
		if err != nil {
			return Page{}, err
		}

		idx := slices.IndexFunc(window, func(r Record) bool {
			if c := r.UpdatedAt.Compare(after.UpdatedAt); c != 0 {
				return c > 0
			}
			return r.ExternalID > after.ExternalID
		})
		if idx < 0 {
			window = nil
		} else {
			window = window[idx:]
		}
	}

	page := Page{Records: window}
	if len(window) > limit {
		page.Records = window[:limit]
		page.HasMore = true
		page.NextCursor = EncodeCursor(page.Records[len(page.Records)-1])
	}
	return page, nil
}

type cursorKey struct {
	UpdatedAt  time.Time
	ExternalID string
}

// EncodeCursor is opaque on purpose: a client that takes the cursor apart ends
// up depending on the sort key.
func EncodeCursor(r Record) string {
	raw := r.UpdatedAt.UTC().Format(time.RFC3339Nano) + "|" + r.ExternalID
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func DecodeCursor(cursor string) (cursorKey, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return cursorKey{}, fmt.Errorf("%w: not base64url", ErrInvalidCursor)
	}

	stamp, id, found := strings.Cut(string(raw), "|")
	if !found {
		return cursorKey{}, fmt.Errorf("%w: missing separator", ErrInvalidCursor)
	}

	updatedAt, err := time.Parse(time.RFC3339Nano, stamp)
	if err != nil {
		return cursorKey{}, fmt.Errorf("%w: bad timestamp %q", ErrInvalidCursor, stamp)
	}
	return cursorKey{UpdatedAt: updatedAt, ExternalID: id}, nil
}
