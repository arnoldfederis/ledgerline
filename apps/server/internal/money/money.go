// Package money represents amounts as exact integer cents.
//
// No float64 touches an amount anywhere in Ledgerline. Decimal text exists
// only at the boundary with the accounting source, and Parse converts it
// straight to cents.
package money

import (
	"errors"
	"fmt"
	"strings"
)

// Cents is a signed amount in the minor unit: 1234 is $12.34.
type Cents int64

// PerUnit is the number of cents in one major unit.
const PerUnit = 100

var (
	ErrEmpty  = errors.New("money: empty amount")
	ErrSyntax = errors.New("money: malformed amount")
	// ErrPrecision is returned for sub-cent input. Rounding it here would mean
	// a sync job quietly changing the books.
	ErrPrecision = errors.New("money: amount has more than two decimal places")
	ErrRange     = errors.New("money: amount out of range")
)

const maxCents = Cents(1<<63 - 1)

// Parse converts "1234.56", "-12.3", "$1,234.56" or "0" into cents. It accepts
// an optional sign, a currency symbol, thousands separators and up to two
// decimal places.
func Parse(s string) (Cents, error) {
	t := strings.TrimSpace(s)
	if t == "" {
		return 0, ErrEmpty
	}

	negative := false
	switch t[0] {
	case '+':
		t = t[1:]
	case '-':
		negative = true
		t = t[1:]
	}
	t = strings.TrimPrefix(t, "$")
	t = strings.ReplaceAll(t, ",", "")
	if t == "" {
		return 0, ErrEmpty
	}

	whole, frac, _ := strings.Cut(t, ".")
	// Shape before precision, so "12.34USD" is malformed rather than too precise.
	if !allDigits(whole) || !allDigits(frac) {
		return 0, fmt.Errorf("%w: %q", ErrSyntax, s)
	}
	if whole == "" && frac == "" {
		return 0, ErrEmpty
	}
	if len(frac) > 2 {
		return 0, fmt.Errorf("%w: %q", ErrPrecision, s)
	}

	// "1.5" is fifty cents, not five.
	frac += strings.Repeat("0", 2-len(frac))

	cents, err := digitsToInt(whole)
	if err != nil {
		return 0, fmt.Errorf("%w: %q", err, s)
	}
	if cents > maxCents/PerUnit {
		return 0, fmt.Errorf("%w: %q", ErrRange, s)
	}
	cents *= PerUnit

	fracCents, err := digitsToInt(frac)
	if err != nil {
		return 0, fmt.Errorf("%w: %q", err, s)
	}
	if cents > maxCents-fracCents {
		return 0, fmt.Errorf("%w: %q", ErrRange, s)
	}
	cents += fracCents

	if negative {
		return -cents, nil
	}
	return cents, nil
}

func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// digitsToInt treats the empty string as zero, which is what makes ".50" and
// "12." parse.
func digitsToInt(s string) (Cents, error) {
	var n Cents
	for i := 0; i < len(s); i++ {
		d := s[i]
		if d < '0' || d > '9' {
			return 0, ErrSyntax
		}
		if n > (maxCents-Cents(d-'0'))/10 {
			return 0, ErrRange
		}
		n = n*10 + Cents(d-'0')
	}
	return n, nil
}

// String renders the wire format: two decimal places, no symbol or separators.
func (c Cents) String() string {
	sign := ""
	n := c
	if n < 0 {
		sign, n = "-", -n
	}
	return fmt.Sprintf("%s%d.%02d", sign, int64(n/PerUnit), int64(n%PerUnit))
}

// Display renders the amount for humans: "-$1,234.56".
func (c Cents) Display() string {
	sign := ""
	n := c
	if n < 0 {
		sign, n = "-", -n
	}
	return sign + "$" + group(int64(n/PerUnit)) + fmt.Sprintf(".%02d", int64(n%PerUnit))
}

func group(n int64) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}

	var b strings.Builder
	lead := len(s) % 3
	if lead > 0 {
		b.WriteString(s[:lead])
	}
	for i := lead; i < len(s); i += 3 {
		if b.Len() > 0 {
			b.WriteByte(',')
		}
		b.WriteString(s[i : i+3])
	}
	return b.String()
}

func (c Cents) Add(o Cents) Cents { return c + o }
func (c Cents) Sub(o Cents) Cents { return c - o }
func (c Cents) Neg() Cents        { return -c }

func (c Cents) Abs() Cents {
	if c < 0 {
		return -c
	}
	return c
}

func Sum(amounts ...Cents) Cents {
	var total Cents
	for _, a := range amounts {
		total += a
	}
	return total
}

// Signed applies an accounting direction to a magnitude. Transactions are
// stored as a positive amount plus a direction, so this is the only place the
// sign is decided.
func Signed(magnitude Cents, direction string) Cents {
	if direction == "credit" {
		return -magnitude
	}
	return magnitude
}

// UtilizationBasisPoints returns actual/budget in basis points: 10000 is on
// budget, 12750 is 127.50%. An unbudgeted category returns 0 rather than
// dividing by zero.
func UtilizationBasisPoints(actual, budget Cents) int64 {
	if budget == 0 {
		return 0
	}

	den := int64(budget.Abs())
	num := int64(actual) * 10000
	if num >= 0 {
		return (num + den/2) / den
	}
	return -((-num + den/2) / den)
}

// Allocate splits total into n parts that sum back to exactly total, spreading
// the remainder a cent at a time over the leading parts.
func Allocate(total Cents, n int) []Cents {
	if n <= 0 {
		return nil
	}

	base := total / Cents(n)
	remainder := total % Cents(n)
	step := Cents(1)
	if remainder < 0 {
		step, remainder = -1, -remainder
	}

	parts := make([]Cents, n)
	for i := range parts {
		parts[i] = base
		if Cents(i) < remainder {
			parts[i] += step
		}
	}
	return parts
}
