package service

import (
	"context"
	"fmt"

	"github.com/Dubjay18/seraph/shared/money"
)

// ─── Domain types ─────────────────────────────────────────────────────────────

// EntryType represents the direction of a ledger entry.
type EntryType string

const (
	EntryDebit  EntryType = "DEBIT"
	EntryCredit EntryType = "CREDIT"
)

// Entry represents one side of a double-entry transaction.
// Amount carries both the value (in minor units) and the currency together,
// preventing accidental cross-currency arithmetic.
type Entry struct {
	AccountID string
	Type      EntryType
	Amount    money.Money // e.g. money.MustNew(500_00, money.NGN) = ₦500.00
}

// PostRequest is the input to PostTransaction.
//
// Invariants:
//   - All entries must have a supported currency (enforced by money.Money).
//   - For each currency present in the entries, the sum of DEBIT amounts
//     MUST equal the sum of CREDIT amounts (balanced double-entry per currency).
//   - Cross-currency transactions (e.g. DEBIT NGN + CREDIT USD) are rejected
//     unless a balancing FX conversion entry is included.
type PostRequest struct {
	IdempotencyKey string
	Description    string
	Entries        []Entry
}

// ─── Service ──────────────────────────────────────────────────────────────────

// LedgerRepository is the data-access interface for the ledger service.
type LedgerRepository interface {
	// PostTransactionTx inserts a transaction and its entries in one DB transaction,
	// and atomically updates account_balances_cache for each affected (account, currency) pair.
	PostTransactionTx(ctx context.Context, req PostRequest) (txnID string, err error)
	// IsIdempotencyKeyUsed returns true if the key has already been processed.
	IsIdempotencyKeyUsed(ctx context.Context, key string) (bool, error)
	// GetBalance returns the cached balance for an account in a given currency.
	GetBalance(ctx context.Context, accountID string, currency money.Currency) (money.Money, error)
}

// LedgerService is the core accounting engine of Seraph.
type LedgerService struct {
	repo LedgerRepository
}

func New(repo LedgerRepository) *LedgerService { return &LedgerService{repo: repo} }

// ─── Validation ───────────────────────────────────────────────────────────────

// validateDoubleEntry checks the fundamental accounting invariant per currency:
// for every currency in the entry list, total DEBITs must equal total CREDITs.
func validateDoubleEntry(entries []Entry) error {
	if len(entries) == 0 {
		return fmt.Errorf("ledger: transaction must have at least one entry")
	}

	// Use int64 maps keyed by currency to accumulate per-currency sums.
	debits := make(map[money.Currency]int64)
	credits := make(map[money.Currency]int64)

	for _, e := range entries {
		if err := e.Amount.Currency.Validate(); err != nil {
			return fmt.Errorf("ledger: entry has invalid currency: %w", err)
		}
		if e.Amount.IsNegative() {
			return fmt.Errorf("ledger: entry amounts must be non-negative (account=%s)", e.AccountID)
		}
		switch e.Type {
		case EntryDebit:
			debits[e.Amount.Currency] += e.Amount.Amount
		case EntryCredit:
			credits[e.Amount.Currency] += e.Amount.Amount
		default:
			return fmt.Errorf("ledger: unknown entry type %q", e.Type)
		}
	}

	// Collect all currencies referenced by any entry.
	seen := make(map[money.Currency]struct{})
	for c := range debits {
		seen[c] = struct{}{}
	}
	for c := range credits {
		seen[c] = struct{}{}
	}

	// For each currency, debits must equal credits.
	for c := range seen {
		d, cr := debits[c], credits[c]
		if d != cr {
			return fmt.Errorf(
				"ledger: double-entry invariant violated for %s: debits=%d != credits=%d",
				c, d, cr,
			)
		}
	}
	return nil
}

// ─── PostTransaction ──────────────────────────────────────────────────────────

// PostTransaction records a balanced set of ledger entries atomically.
// It is idempotent: repeat calls with the same IdempotencyKey are no-ops.
func (s *LedgerService) PostTransaction(ctx context.Context, req PostRequest) (string, error) {
	if err := validateDoubleEntry(req.Entries); err != nil {
		return "", err
	}

	// Idempotency guard.
	used, err := s.repo.IsIdempotencyKeyUsed(ctx, req.IdempotencyKey)
	if err != nil {
		return "", fmt.Errorf("ledger: idempotency check: %w", err)
	}
	if used {
		return "", nil // already processed — safe to ack
	}

	txnID, err := s.repo.PostTransactionTx(ctx, req)
	if err != nil {
		return "", fmt.Errorf("ledger: post transaction: %w", err)
	}
	return txnID, nil
}

// GetBalance returns the current balance for an account in the given currency.
// The balance is read from account_balances_cache (fast path) maintained by
// the ledger repository. It is always denominated in the currency's minor unit.
func (s *LedgerService) GetBalance(ctx context.Context, accountID string, currency money.Currency) (money.Money, error) {
	if err := currency.Validate(); err != nil {
		return money.Money{}, fmt.Errorf("ledger: %w", err)
	}
	return s.repo.GetBalance(ctx, accountID, currency)
}
