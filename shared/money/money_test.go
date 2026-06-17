package money_test

import (
	"testing"

	"github.com/Dubjay18/seraph/shared/money"
)

// ─── NGN Kobo backward-compat ─────────────────────────────────────────────────

func TestFromNaira(t *testing.T) {
	if got := money.FromNaira(10); got != 1000 {
		t.Errorf("FromNaira(10) = %d, want 1000", got)
	}
}

func TestKoboArithmetic(t *testing.T) {
	// Kobo is now int64 — arithmetic is done via Money or plain int64 ops.
	a := money.FromNaira(1000) // 100_000 kobo
	b := money.FromNaira(250)  //  25_000 kobo
	diff := a - b
	if diff != money.FromNaira(750) {
		t.Errorf("expected 75000 kobo (750 NGN), got %d", diff)
	}
}

// ─── Currency tests ───────────────────────────────────────────────────────────

func TestCurrencyValidate(t *testing.T) {
	t.Run("supported currencies pass", func(t *testing.T) {
		for _, c := range money.SupportedCurrencies() {
			if err := c.Validate(); err != nil {
				t.Errorf("expected %s to be valid, got: %v", c, err)
			}
		}
	})

	t.Run("unsupported currency fails", func(t *testing.T) {
		bad := []money.Currency{"ZZZ", "JPY", "", "ngn", "usd"}
		for _, c := range bad {
			if err := c.Validate(); err == nil {
				t.Errorf("expected %q to be invalid", c)
			}
		}
	})
}

func TestMinorUnits(t *testing.T) {
	cases := []struct {
		c    money.Currency
		want int64
	}{
		{money.NGN, 100},
		{money.USD, 100},
		{money.GBP, 100},
		{money.EUR, 100},
		{"UNKNOWN", 100}, // safe default
	}
	for _, tc := range cases {
		if got := money.MinorUnits(tc.c); got != tc.want {
			t.Errorf("MinorUnits(%s) = %d, want %d", tc.c, got, tc.want)
		}
	}
}

// ─── Money construction ───────────────────────────────────────────────────────

func TestNew(t *testing.T) {
	t.Run("valid NGN", func(t *testing.T) {
		m, err := money.New(500, money.NGN)
		if err != nil {
			t.Fatal(err)
		}
		if m.Amount != 500 || m.Currency != money.NGN {
			t.Fatalf("unexpected value: %+v", m)
		}
	})

	t.Run("negative amount rejected", func(t *testing.T) {
		if _, err := money.New(-1, money.NGN); err == nil {
			t.Fatal("expected error for negative amount")
		}
	})

	t.Run("unsupported currency rejected", func(t *testing.T) {
		if _, err := money.New(100, "ZZZ"); err == nil {
			t.Fatal("expected error for unsupported currency")
		}
	})

	t.Run("zero amount allowed", func(t *testing.T) {
		m, err := money.New(0, money.USD)
		if err != nil || !m.IsZero() {
			t.Fatalf("expected zero, got err=%v m=%v", err, m)
		}
	})
}

func TestFromMajorUnits(t *testing.T) {
	m, err := money.FromMajorUnits(5, money.NGN)
	if err != nil {
		t.Fatal(err)
	}
	if m.Amount != 500 {
		t.Errorf("FromMajorUnits(5, NGN) = %d kobo, want 500", m.Amount)
	}
}

// ─── Arithmetic ───────────────────────────────────────────────────────────────

func TestMoneyAdd(t *testing.T) {
	a := money.MustNew(300, money.NGN)
	b := money.MustNew(200, money.NGN)
	got := a.Add(b)
	if got.Amount != 500 || got.Currency != money.NGN {
		t.Errorf("Add: got %v, want NGN 500", got)
	}
}

func TestMoneySub(t *testing.T) {
	a := money.MustNew(300, money.USD)
	b := money.MustNew(100, money.USD)
	got := a.Sub(b)
	if got.Amount != 200 || got.Currency != money.USD {
		t.Errorf("Sub: got %v, want USD 200", got)
	}
}

func TestAddCurrencyMismatchPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on currency mismatch")
		}
	}()
	money.MustNew(100, money.NGN).Add(money.MustNew(100, money.USD))
}

func TestIsNegative(t *testing.T) {
	m := money.Money{Amount: -1, Currency: money.NGN}
	if !m.IsNegative() {
		t.Error("expected IsNegative to be true")
	}
	pos := money.MustNew(1, money.NGN)
	if pos.IsNegative() {
		t.Error("expected IsNegative to be false for positive")
	}
}

func TestEquals(t *testing.T) {
	a := money.MustNew(100, money.USD)
	b := money.MustNew(100, money.USD)
	c := money.MustNew(100, money.NGN)
	if !a.Equals(b) {
		t.Error("expected a == b")
	}
	if a.Equals(c) {
		t.Error("expected a != c (different currencies)")
	}
}

// ─── String formatting ────────────────────────────────────────────────────────

func TestMoneyString(t *testing.T) {
	cases := []struct {
		m    money.Money
		want string
	}{
		{money.MustNew(500, money.NGN), "NGN 5.00"},
		{money.MustNew(199, money.USD), "USD 1.99"},
		{money.MustNew(0, money.GBP), "GBP 0.00"},
		{money.MustNew(100000, money.EUR), "EUR 1000.00"},
	}
	for _, tc := range cases {
		if got := tc.m.String(); got != tc.want {
			t.Errorf("String() = %q, want %q", got, tc.want)
		}
	}
}
