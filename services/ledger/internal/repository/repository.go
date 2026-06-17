package repository

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Dubjay18/seraph/services/ledger/internal/service"
	apperrors "github.com/Dubjay18/seraph/shared/errors"
	"github.com/Dubjay18/seraph/shared/money"
)

type Repository struct {
	db *pgxpool.Pool
}

func New(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

// IsIdempotencyKeyUsed checks if the idempotency key has already been recorded.
func (r *Repository) IsIdempotencyKeyUsed(ctx context.Context, key string) (bool, error) {
	const q = `SELECT EXISTS(SELECT 1 FROM transactions WHERE idempotency_key = $1)`
	var exists bool
	err := r.db.QueryRow(ctx, q, key).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("ledger repo: check idempotency key: %w", err)
	}
	return exists, nil
}

// GetBalance retrieves the current cached balance for an account/currency pair.
// Returns a 0 balance if no entries have been recorded yet for the account.
func (r *Repository) GetBalance(ctx context.Context, accountID string, currency money.Currency) (money.Money, error) {
	const q = `SELECT balance FROM account_balances_cache WHERE account_id = $1 AND currency = $2`
	var bal int64
	err := r.db.QueryRow(ctx, q, accountID, string(currency)).Scan(&bal)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// New account or account with no ledger transactions yet.
			return money.New(0, currency)
		}
		return money.Money{}, fmt.Errorf("ledger repo: get balance: %w", err)
	}
	return money.New(bal, currency)
}

// accountCurrencyKey is a map key for grouping changes by account and currency.
type accountCurrencyKey struct {
	accountID string
	currency  money.Currency
}

// PostTransactionTx writes the transaction, its entries, and updates/checks the balance cache.
func (r *Repository) PostTransactionTx(ctx context.Context, req service.PostRequest) (string, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("ledger repo: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// 1. Insert Transaction
	const insertTx = `
		INSERT INTO transactions (idempotency_key, status, description, posted_at)
		VALUES ($1, 'POSTED', $2, NOW())
		RETURNING id`
	var txnID string
	err = tx.QueryRow(ctx, insertTx, req.IdempotencyKey, req.Description).Scan(&txnID)
	if err != nil {
		return "", fmt.Errorf("ledger repo: insert transaction: %w", err)
	}

	// 2. Insert Idempotency Key record
	const insertIdemp = `
		INSERT INTO idempotency_keys (key, transaction_id)
		VALUES ($1, $2)`
	_, err = tx.Exec(ctx, insertIdemp, req.IdempotencyKey, txnID)
	if err != nil {
		return "", fmt.Errorf("ledger repo: insert idempotency key: %w", err)
	}

	// Calculate net balance changes per (account, currency)
	netChanges := make(map[accountCurrencyKey]int64)

	// 3. Insert Entries
	const insertEntry = `
		INSERT INTO ledger_entries (transaction_id, account_id, entry_type, amount, currency)
		VALUES ($1, $2, $3, $4, $5)`

	for _, e := range req.Entries {
		_, err = tx.Exec(ctx, insertEntry, txnID, e.AccountID, string(e.Type), e.Amount.Amount, string(e.Amount.Currency))
		if err != nil {
			return "", fmt.Errorf("ledger repo: insert entry: %w", err)
		}

		key := accountCurrencyKey{accountID: e.AccountID, currency: e.Amount.Currency}
		if e.Type == service.EntryCredit {
			netChanges[key] += e.Amount.Amount
		} else {
			netChanges[key] -= e.Amount.Amount
		}
	}

	// 4. Update and check balance cache in a deadlock-free order.
	// We extract keys and sort them lexicographically by (accountID, currency).
	type sortedChange struct {
		key    accountCurrencyKey
		change int64
	}
	var changes []sortedChange
	for k, change := range netChanges {
		changes = append(changes, sortedChange{key: k, change: change})
	}
	sort.Slice(changes, func(i, j int) bool {
		if changes[i].key.accountID != changes[j].key.accountID {
			return changes[i].key.accountID < changes[j].key.accountID
		}
		return changes[i].key.currency < changes[j].key.currency
	})

	const upsertBalance = `
		INSERT INTO account_balances_cache (account_id, currency, balance, updated_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (account_id, currency)
		DO UPDATE SET balance = account_balances_cache.balance + EXCLUDED.balance, updated_at = NOW()
		RETURNING balance`

	for _, c := range changes {
		var newBal int64
		err = tx.QueryRow(ctx, upsertBalance, c.key.accountID, string(c.key.currency), c.change).Scan(&newBal)
		if err != nil {
			return "", fmt.Errorf("ledger repo: upsert balance: %w", err)
		}

		// Enforce sufficient funds (balance must not be negative).
		if newBal < 0 {
			return "", apperrors.New(
				apperrors.CodeInsufficientFunds,
				fmt.Sprintf("insufficient funds in account %s for currency %s (resulting balance: %d)", c.key.accountID, c.key.currency, newBal),
			)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("ledger repo: commit tx: %w", err)
	}

	return txnID, nil
}
