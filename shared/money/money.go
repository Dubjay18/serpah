// Package money provides safe, integer-based monetary arithmetic for the
// Seraph platform. All amounts are stored and computed in the smallest
// indivisible unit of the currency (Kobo for NGN, cents for USD/GBP/EUR)
// — NEVER as floating-point numbers.
package money

import "fmt"

// ─── Currency type ────────────────────────────────────────────────────────────

// Currency is a validated ISO-4217 3-letter currency code.
type Currency string

const (
	NGN Currency = "NGN" // Nigerian Naira  (1 NGN = 100 Kobo)
	USD Currency = "USD" // US Dollar       (1 USD = 100 Cents)
	GBP Currency = "GBP" // British Pound   (1 GBP = 100 Pence)
	EUR Currency = "EUR" // Euro            (1 EUR = 100 Cents)
)

// minorUnits maps a currency to the number of minor units per major unit.
// For example, 1 NGN = 100 Kobo, so NGN → 100.
// Currencies with no minor units (e.g. JPY) would have 1.
var minorUnits = map[Currency]int64{
	NGN: 100,
	USD: 100,
	GBP: 100,
	EUR: 100,
}

// MinorUnits returns how many minor units make up one major unit of c.
// Returns 100 as a safe default for unknown currencies.
func MinorUnits(c Currency) int64 {
	if u, ok := minorUnits[c]; ok {
		return u
	}
	return 100
}

// SupportedCurrencies returns all currencies accepted by the platform.
func SupportedCurrencies() []Currency {
	return []Currency{NGN, USD, GBP, EUR}
}

// Validate returns an error if c is not a platform-supported currency.
func (c Currency) Validate() error {
	if _, ok := minorUnits[c]; !ok {
		return fmt.Errorf("unsupported currency %q: must be one of NGN, USD, GBP, EUR", c)
	}
	return nil
}

// String implements fmt.Stringer.
func (c Currency) String() string { return string(c) }

// ─── Money type ───────────────────────────────────────────────────────────────

// Money pairs an integer amount (in the currency's minor unit) with a
// validated currency code. Use New() or FromMajorUnits() to construct it.
//
// Examples:
//
//	New(500, NGN)  → ₦5.00
//	New(199, USD)  → $1.99
type Money struct {
	// Amount is the value expressed in the currency's smallest unit.
	// e.g. 100 for NGN means 100 Kobo = ₦1.00
	Amount   int64
	Currency Currency
}

// New constructs a Money value from a minor-unit amount and currency.
// Returns an error if the currency is unsupported or the amount is negative.
func New(amountMinorUnits int64, c Currency) (Money, error) {
	if err := c.Validate(); err != nil {
		return Money{}, err
	}
	if amountMinorUnits < 0 {
		return Money{}, fmt.Errorf("money: amount must be non-negative, got %d", amountMinorUnits)
	}
	return Money{Amount: amountMinorUnits, Currency: c}, nil
}

// MustNew constructs a Money value and panics on error. Intended for
// compile-time constants and test fixtures only.
func MustNew(amountMinorUnits int64, c Currency) Money {
	m, err := New(amountMinorUnits, c)
	if err != nil {
		panic(err)
	}
	return m
}

// FromMajorUnits constructs a Money value from a whole major-unit amount.
// e.g. FromMajorUnits(5, NGN) = 500 Kobo = ₦5.00
func FromMajorUnits(major int64, c Currency) (Money, error) {
	if err := c.Validate(); err != nil {
		return Money{}, err
	}
	return Money{Amount: major * MinorUnits(c), Currency: c}, nil
}

// ─── Arithmetic ───────────────────────────────────────────────────────────────

// Add returns m + other. Panics if currencies differ.
func (m Money) Add(other Money) Money {
	m.mustSameCurrency(other)
	return Money{Amount: m.Amount + other.Amount, Currency: m.Currency}
}

// Sub returns m - other. Panics if currencies differ.
func (m Money) Sub(other Money) Money {
	m.mustSameCurrency(other)
	return Money{Amount: m.Amount - other.Amount, Currency: m.Currency}
}

// IsNegative reports whether the amount is below zero.
func (m Money) IsNegative() bool { return m.Amount < 0 }

// IsZero reports whether the amount is exactly zero.
func (m Money) IsZero() bool { return m.Amount == 0 }

// Equals reports structural equality (same currency and same amount).
func (m Money) Equals(other Money) bool {
	return m.Currency == other.Currency && m.Amount == other.Amount
}

// mustSameCurrency panics if m and other have different currencies.
func (m Money) mustSameCurrency(other Money) {
	if m.Currency != other.Currency {
		panic(fmt.Sprintf("money: currency mismatch: %s vs %s", m.Currency, other.Currency))
	}
}

// ─── Formatting ───────────────────────────────────────────────────────────────

// String returns a human-readable representation such as "NGN 5.00" or "USD 1.99".
func (m Money) String() string {
	u := MinorUnits(m.Currency)
	major := m.Amount / u
	minor := m.Amount % u
	return fmt.Sprintf("%s %d.%02d", m.Currency, major, minor)
}

// ─── NGN convenience ──────────────────────────────────────────────────────────

// Kobo is the smallest unit of NGN (1 NGN = 100 Kobo).
// Kept for backward compatibility with existing code.
type Kobo = int64

// FromNaira converts a whole-Naira amount to Kobo.
func FromNaira(naira int64) Kobo { return naira * 100 }
