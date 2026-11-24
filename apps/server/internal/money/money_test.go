package money_test

import (
	"errors"
	"testing"

	"ledgerline/apps/server/internal/money"
)

func TestParse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want money.Cents
	}{
		{"whole dollars", "1234", 123400},
		{"two decimals", "1234.56", 123456},
		{"one decimal is tenths not hundredths", "1.5", 150},
		{"trailing dot", "12.", 1200},
		{"leading dot", ".07", 7},
		{"zero", "0", 0},
		{"zero with decimals", "0.00", 0},
		{"sub-dollar", "0.09", 9},
		{"negative", "-12.34", -1234},
		{"explicit plus", "+12.34", 1234},
		{"currency symbol", "$12.34", 1234},
		{"negative with symbol", "-$1,234.56", -123456},
		{"thousands separators", "1,234,567.89", 123456789},
		{"surrounding whitespace", "  42.00  ", 4200},
		{"0.1 + 0.2 in cents", "0.30", 30},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := money.Parse(tc.in)
			if err != nil {
				t.Fatalf("Parse(%q) returned error: %v", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("Parse(%q) = %d cents, want %d", tc.in, got, tc.want)
			}
		})
	}
}

func TestParseRejects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		in      string
		wantErr error
	}{
		{"empty", "", money.ErrEmpty},
		{"whitespace only", "   ", money.ErrEmpty},
		{"sign only", "-", money.ErrEmpty},
		{"dot only", ".", money.ErrEmpty},
		{"letters", "twelve", money.ErrSyntax},
		{"trailing garbage", "12.34USD", money.ErrSyntax},
		{"two decimal points", "1.2.3", money.ErrSyntax},
		{"three decimals", "1.005", money.ErrPrecision},
		{"many decimals", "0.123456", money.ErrPrecision},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := money.Parse(tc.in); !errors.Is(err, tc.wantErr) {
				t.Errorf("Parse(%q) error = %v, want %v", tc.in, err, tc.wantErr)
			}
		})
	}
}

func TestParseRoundTrip(t *testing.T) {
	t.Parallel()

	for _, in := range []string{"0.00", "0.01", "12.34", "-12.34", "1234567.89", "-0.99"} {
		got, err := money.Parse(in)
		if err != nil {
			t.Fatalf("Parse(%q): %v", in, err)
		}
		if got.String() != in {
			t.Errorf("Parse(%q).String() = %q, want %q", in, got.String(), in)
		}
	}
}

func TestString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   money.Cents
		want string
	}{
		{0, "0.00"},
		{7, "0.07"},
		{70, "0.70"},
		{123456, "1234.56"},
		{-1, "-0.01"},
		{-123456, "-1234.56"},
	}

	for _, tc := range tests {
		if got := tc.in.String(); got != tc.want {
			t.Errorf("Cents(%d).String() = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestDisplay(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   money.Cents
		want string
	}{
		{0, "$0.00"},
		{99, "$0.99"},
		{100000, "$1,000.00"},
		{123456789, "$1,234,567.89"},
		{-4450000, "-$44,500.00"},
	}

	for _, tc := range tests {
		if got := tc.in.Display(); got != tc.want {
			t.Errorf("Cents(%d).Display() = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestSigned(t *testing.T) {
	t.Parallel()

	if got := money.Signed(5000, "debit"); got != 5000 {
		t.Errorf("Signed(5000, debit) = %d, want 5000", got)
	}
	if got := money.Signed(5000, "credit"); got != -5000 {
		t.Errorf("Signed(5000, credit) = %d, want -5000", got)
	}
}

func TestUtilizationBasisPoints(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		actual money.Cents
		budget money.Cents
		want   int64
	}{
		{"exactly on budget", 250000, 250000, 10000},
		{"half spent", 125000, 250000, 5000},
		{"nothing spent", 0, 250000, 0},
		{"over budget", 319000, 275000, 11600},
		{"well over budget", 806000, 650000, 12400},
		{"net credit", -5000, 250000, -200},
		{"zero budget", 100000, 0, 0},
		{"zero budget and zero spend", 0, 0, 0},
		{"rounds to nearest basis point", 100000, 300000, 3333},
		{"rounds half up", 15, 100, 1500},
		{"rounds .5 away from zero", 1, 3, 3333},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := money.UtilizationBasisPoints(tc.actual, tc.budget); got != tc.want {
				t.Errorf("UtilizationBasisPoints(%d, %d) = %d, want %d",
					tc.actual, tc.budget, got, tc.want)
			}
		})
	}
}

func TestAllocate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		total money.Cents
		n     int
		want  []money.Cents
	}{
		{"even split", 1000, 4, []money.Cents{250, 250, 250, 250}},
		{"stray cent goes to the front", 1000, 3, []money.Cents{334, 333, 333}},
		{"two stray cents", 1001, 3, []money.Cents{334, 334, 333}},
		{"single part", 777, 1, []money.Cents{777}},
		{"negative total", -1000, 3, []money.Cents{-334, -333, -333}},
		{"zero total", 0, 3, []money.Cents{0, 0, 0}},
		{"more parts than cents", 2, 5, []money.Cents{1, 1, 0, 0, 0}},
		{"no parts", 100, 0, nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := money.Allocate(tc.total, tc.n)
			if len(got) != len(tc.want) {
				t.Fatalf("Allocate(%d, %d) returned %d parts, want %d",
					tc.total, tc.n, len(got), len(tc.want))
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("Allocate(%d, %d)[%d] = %d, want %d",
						tc.total, tc.n, i, got[i], tc.want[i])
				}
			}
			// Not a cent created or lost.
			if sum := money.Sum(got...); len(got) > 0 && sum != tc.total {
				t.Errorf("Allocate(%d, %d) sums to %d, want %d", tc.total, tc.n, sum, tc.total)
			}
		})
	}
}

func TestSum(t *testing.T) {
	t.Parallel()

	// Fails the moment these become float64 dollars.
	var total money.Cents
	for range 1000 {
		total = total.Add(10).Add(20)
	}
	if total != 30000 {
		t.Errorf("1000 x (0.10 + 0.20) = %s, want 300.00", total)
	}

	if got := money.Sum(100, -250, 75); got != -75 {
		t.Errorf("Sum(100, -250, 75) = %d, want -75", got)
	}
	if got := money.Sum(); got != 0 {
		t.Errorf("Sum() = %d, want 0", got)
	}
}
