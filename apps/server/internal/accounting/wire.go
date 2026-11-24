package accounting

import (
	"fmt"
	"time"

	"ledgerline/apps/server/internal/money"
)

// WireTransaction is the JSON the accounting API speaks.
//
// Amount is a string because JSON numbers are IEEE-754 doubles. As a float64
// every amount would arrive already rounded, and nothing downstream could fix
// it. ToRecord parses it to exact cents.
type WireTransaction struct {
	ID           string    `json:"id"`
	CategoryCode string    `json:"category_code"`
	CategoryName string    `json:"category_name"`
	Vendor       string    `json:"vendor"`
	Description  string    `json:"description"`
	Amount       string    `json:"amount"`
	Currency     string    `json:"currency"`
	Direction    string    `json:"direction"`
	OccurredOn   string    `json:"occurred_on"`
	UpdatedAt    time.Time `json:"updated_at"`
	Voided       bool      `json:"voided"`
}

type WirePage struct {
	Transactions []WireTransaction `json:"transactions"`
	NextCursor   string            `json:"next_cursor"`
	HasMore      bool              `json:"has_more"`
	ServerTime   time.Time         `json:"server_time"`
}

// WireFile is the on-disk fixture the bundled mock source reads.
type WireFile struct {
	Source       string            `json:"source"`
	GeneratedAt  time.Time         `json:"generated_at"`
	Transactions []WireTransaction `json:"transactions"`
}

func (w WireTransaction) ToRecord() (Record, error) {
	amount, err := money.Parse(w.Amount)
	if err != nil {
		return Record{}, fmt.Errorf("accounting: record %s has unusable amount: %w", w.ID, err)
	}

	occurredOn, err := time.ParseInLocation(time.DateOnly, w.OccurredOn, time.UTC)
	if err != nil {
		return Record{}, fmt.Errorf("accounting: record %s has unusable occurred_on %q: %w",
			w.ID, w.OccurredOn, err)
	}

	currency := w.Currency
	if currency == "" {
		currency = "USD"
	}

	r := Record{
		ExternalID:   w.ID,
		CategoryCode: w.CategoryCode,
		CategoryName: w.CategoryName,
		Vendor:       w.Vendor,
		Description:  w.Description,
		Amount:       amount,
		Currency:     currency,
		Direction:    w.Direction,
		OccurredOn:   occurredOn,
		UpdatedAt:    w.UpdatedAt.UTC(),
		Voided:       w.Voided,
	}
	if err := r.Validate(); err != nil {
		return Record{}, err
	}
	return r, nil
}

// ToWire lets the bundled mock serve records back out, keeping one definition
// of the contract instead of a producer copy and a consumer copy.
func (r Record) ToWire() WireTransaction {
	return WireTransaction{
		ID:           r.ExternalID,
		CategoryCode: r.CategoryCode,
		CategoryName: r.CategoryName,
		Vendor:       r.Vendor,
		Description:  r.Description,
		Amount:       r.Amount.String(),
		Currency:     r.Currency,
		Direction:    r.Direction,
		OccurredOn:   r.OccurredOn.UTC().Format(time.DateOnly),
		UpdatedAt:    r.UpdatedAt.UTC(),
		Voided:       r.Voided,
	}
}

// RecordsFromWire fails on the first unusable record. A partial sync that
// silently dropped three malformed rows is worse than one that stops.
func RecordsFromWire(in []WireTransaction) ([]Record, error) {
	out := make([]Record, 0, len(in))
	for _, w := range in {
		r, err := w.ToRecord()
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}
