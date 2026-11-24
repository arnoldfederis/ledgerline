//go:build ignore

// Regenerates transactions.json, the fixture ledger the mock accounting API
// serves. The output is committed so a reviewer can read the demo data without
// running anything; this program exists so it is reproducible and tunable.
//
//	go run ./internal/mocksource/data/generate.go
//
// It builds the fixture through the same accounting.WireTransaction type the
// sync job parses, so the two can never drift. Amounts are produced from
// integer cents and rendered with money.Cents.String(), never from a float.
//
// Keep budgets in step with
// internal/database/migrations/00002_seed_budgets.sql.
package main

import (
	"cmp"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"time"

	"ledgerline/apps/server/internal/accounting"
	"ledgerline/apps/server/internal/money"
)

type category struct {
	code    string
	name    string
	vendors []string
	// descriptionf takes the vendor name.
	descriptionf string
	// count is how many transactions this category gets per month.
	count int
}

var categories = []category{
	{"software", "Software & Subscriptions", []string{"Figma", "Linear", "Notion", "1Password", "GitHub", "Vercel"}, "%s subscription", 6},
	{"payroll", "Payroll & Contractors", []string{"Deel", "Gusto", "Remote.com"}, "%s payroll run", 3},
	{"cloud", "Cloud & Hosting", []string{"AWS", "Cloudflare", "Fly.io", "Neon"}, "%s usage", 4},
	{"marketing", "Marketing & Advertising", []string{"Google Ads", "Meta Ads", "LinkedIn Ads", "Webflow", "Mailchimp"}, "%s campaign spend", 5},
	{"travel", "Travel & Meals", []string{"United Airlines", "Marriott", "Uber", "DoorDash", "Delta", "Airbnb", "Lyft"}, "%s charge", 7},
	{"office", "Office & Supplies", []string{"WeWork", "Staples", "Apple Store", "Amazon Business"}, "%s purchase", 4},
	{"professional-services", "Professional Services", []string{"Orrick LLP", "Pilot.com", "Deloitte"}, "%s engagement fee", 3},
	{"banking", "Banking & Fees", []string{"Stripe", "Mercury", "Wise", "Brex", "Ramp"}, "%s processing fees", 5},
}

var months = []string{"2026-09", "2026-10", "2026-11"}

// budgets mirrors 00002_seed_budgets.sql exactly, in cents.
var budgets = map[string]map[string]money.Cents{
	"2026-09": {"software": 250000, "payroll": 1150000, "cloud": 180000, "marketing": 600000,
		"travel": 320000, "office": 90000, "professional-services": 450000, "banking": 45000},
	"2026-10": {"software": 250000, "payroll": 1150000, "cloud": 180000, "marketing": 650000,
		"travel": 320000, "office": 90000, "professional-services": 450000, "banking": 45000},
	"2026-11": {"software": 275000, "payroll": 1200000, "cloud": 195000, "marketing": 650000,
		"travel": 280000, "office": 90000, "professional-services": 500000, "banking": 45000},
}

// ratios is the share of budget actually spent. Over 1.0 produces an
// over-budget category, which is what the dashboard is built to surface.
// Expressed in basis points to keep the arithmetic in integers.
var ratios = map[string]map[string]int64{
	"2026-09": {"software": 8200, "payroll": 9700, "cloud": 9500, "marketing": 9300,
		"travel": 6400, "office": 4400, "professional-services": 7400, "banking": 9100},
	"2026-10": {"software": 9500, "payroll": 10100, "cloud": 8800, "marketing": 7900,
		"travel": 11100, "office": 6800, "professional-services": 10000, "banking": 8400},
	"2026-11": {"software": 11600, "payroll": 9800, "cloud": 10700, "marketing": 12400,
		"travel": 7200, "office": 5300, "professional-services": 9100, "banking": 10200},
}

type slot struct {
	month string
	code  string
	index int
}

// refunds are stored as credits, never as negative debits.
var refunds = map[string]money.Cents{
	"2026-10|marketing": 42500,
	"2026-11|travel":    18900,
	"2026-09|software":  7200,
}

// lateEdits have an updated_at much later than their transaction date, so an
// incremental sync has genuine late changes to pick up.
var lateEdits = map[slot]bool{
	{"2026-11", "cloud", 0}:     true,
	{"2026-11", "marketing", 2}: true,
	{"2026-10", "payroll", 1}:   true,
	{"2026-11", "software", 4}:  true,
}

// voids stay in the ledger and drop out of actuals.
var voids = map[slot]bool{
	{"2026-10", "office", 3}:  true,
	{"2026-11", "banking", 4}: true,
}

var lateEditDate = time.Date(2026, time.November, 22, 0, 0, 0, 0, time.UTC)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "generate:", err)
		os.Exit(1)
	}
}

func run() error {
	// Fixed seed: regenerating without changing the inputs produces
	// byte-identical output.
	rng := rand.New(rand.NewPCG(2026_11_23, 0x1ed9e12e))

	var out []accounting.WireTransaction
	sequence := 0

	add := func(c category, cents money.Cents, direction string, day int, month string, s slot) {
		sequence++
		vendor := c.vendors[rng.IntN(len(c.vendors))]
		year, mon := parseMonth(month)
		occurred := time.Date(year, mon, day, 0, 0, 0, 0, time.UTC)

		stamp := occurred
		if lateEdits[s] || voids[s] {
			stamp = lateEditDate
		}
		updated := time.Date(stamp.Year(), stamp.Month(), stamp.Day(),
			8+rng.IntN(12), minutes[rng.IntN(len(minutes))], 0, 0, time.UTC)

		description := fmt.Sprintf(c.descriptionf, vendor)
		if direction == accounting.DirectionCredit {
			description = "Refund -- " + vendor
		}

		out = append(out, accounting.WireTransaction{
			ID:           fmt.Sprintf("txn_%05d", sequence),
			CategoryCode: c.code,
			CategoryName: c.name,
			Vendor:       vendor,
			Description:  description,
			Amount:       cents.String(),
			Currency:     "USD",
			Direction:    direction,
			OccurredOn:   occurred.Format(time.DateOnly),
			UpdatedAt:    updated,
			Voided:       voids[s],
		})
	}

	for _, month := range months {
		daysInMonth := 30
		if month == "2026-10" {
			daysInMonth = 31
		}

		for _, c := range categories {
			budget := budgets[month][c.code]
			net := money.Cents(int64(budget) * ratios[month][c.code] / 10000)
			refund := refunds[month+"|"+c.code]

			days := pickDays(rng, 2, daysInMonth-1, c.count)
			for i, cents := range allocate(rng, net+refund, c.count) {
				add(c, cents, accounting.DirectionDebit, days[i], month, slot{month, c.code, i})
			}
			if refund > 0 {
				add(c, refund, accounting.DirectionCredit, min(days[len(days)-1]+1, daysInMonth), month, slot{})
			}
		}
	}

	// Oldest change first: the order a change-data-capture consumer expects.
	slices.SortStableFunc(out, func(a, b accounting.WireTransaction) int {
		if c := a.UpdatedAt.Compare(b.UpdatedAt); c != 0 {
			return c
		}
		return cmp.Compare(a.ID, b.ID)
	})

	file := accounting.WireFile{
		Source:       "mock_accounting",
		GeneratedAt:  time.Date(2026, time.November, 23, 0, 0, 0, 0, time.UTC),
		Transactions: out,
	}

	encoded, err := json.Marshal(file, jsontext.WithIndent("  "))
	if err != nil {
		return fmt.Errorf("encode fixture: %w", err)
	}

	path := filepath.Join("internal", "mocksource", "data", "transactions.json")
	if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}

	report(path, out)
	return nil
}

var minutes = []int{0, 7, 15, 22, 38, 45, 51}

// allocate splits a total into n jittered amounts that sum back to exactly the
// total; the indivisible remainder lands on the first part.
func allocate(rng *rand.Rand, total money.Cents, n int) []money.Cents {
	weights := make([]float64, n)
	var sum float64
	for i := range weights {
		weights[i] = 0.6 + rng.Float64()*0.8
		sum += weights[i]
	}

	parts := make([]money.Cents, n)
	var assigned money.Cents
	for i, w := range weights {
		parts[i] = money.Cents(float64(total) * w / sum)
		assigned += parts[i]
	}
	parts[0] += total - assigned
	return parts
}

// pickDays returns n distinct days in [low, high), sorted.
func pickDays(rng *rand.Rand, low, high, n int) []int {
	pool := make([]int, 0, high-low)
	for d := low; d < high; d++ {
		pool = append(pool, d)
	}
	rng.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })

	days := slices.Clone(pool[:n])
	slices.Sort(days)
	return days
}

func parseMonth(month string) (int, time.Month) {
	t, err := time.Parse("2006-01", month)
	if err != nil {
		panic(err)
	}
	return t.Year(), t.Month()
}

// report prints the resulting actuals so a change to the knobs above is
// visible without opening the JSON.
func report(path string, out []accounting.WireTransaction) {
	net := map[string]money.Cents{}
	for _, t := range out {
		if t.Voided {
			continue
		}
		amount, err := money.Parse(t.Amount)
		if err != nil {
			panic(err)
		}
		net[t.OccurredOn[:7]+"|"+t.CategoryCode] += money.Signed(amount, t.Direction)
	}

	fmt.Printf("wrote %d transactions to %s\n", len(out), path)
	for _, month := range months {
		var over []string
		for _, c := range categories {
			if net[month+"|"+c.code] > budgets[month][c.code] {
				over = append(over, c.code)
			}
		}
		if over == nil {
			over = []string{"none"}
		}
		fmt.Printf("  %s: over budget -> %v\n", month, over)
	}
}
