package repository

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

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

// GetBalanceAny returns the balance for the most recently active currency of accountID.
// Used by the HTTP balance endpoint when the caller doesn't specify a currency.
func (r *Repository) GetBalanceAny(ctx context.Context, accountID string) (money.Money, error) {
	const q = `
		SELECT balance, currency
		FROM account_balances_cache
		WHERE account_id = $1
		ORDER BY updated_at DESC
		LIMIT 1`

	var (
		bal      int64
		currency string
	)
	err := r.db.QueryRow(ctx, q, accountID).Scan(&bal, &currency)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return money.New(0, money.NGN)
		}
		return money.Money{}, fmt.Errorf("ledger repo: get balance any: %w", err)
	}
	return money.New(bal, money.Currency(currency))
}

// GetEntries returns a cursor-paginated slice of ledger entries for accountID.
// from/to optionally restrict by creation timestamp. cursor is an opaque token
// encoding the last seen (created_at, id) position. Returns at most limit rows.
func (r *Repository) GetEntries(
	ctx context.Context,
	accountID string,
	from, to *time.Time,
	cursor string,
	limit int,
) ([]service.LedgerEntry, string, error) {
	if limit <= 0 {
		limit = 20
	}
	// Fetch limit+1 so we know whether a next page exists.
	fetch := limit + 1

	var (
		afterTime time.Time
		afterID   string
	)
	if cursor != "" {
		var err error
		afterTime, afterID, err = decodeCursor(cursor)
		if err != nil {
			return nil, "", fmt.Errorf("ledger repo: invalid cursor: %w", err)
		}
	}

	// Build parameterised query with optional clauses.
	args := []any{accountID, fetch}
	paramIdx := 3

	whereExtra := ""
	if cursor != "" {
		whereExtra += fmt.Sprintf(" AND (le.created_at, le.id) > ($%d, $%d)", paramIdx, paramIdx+1)
		args = append(args, afterTime, afterID)
		paramIdx += 2
	}
	if from != nil {
		whereExtra += fmt.Sprintf(" AND le.created_at >= $%d", paramIdx)
		args = append(args, from.UTC())
		paramIdx++
	}
	if to != nil {
		whereExtra += fmt.Sprintf(" AND le.created_at <= $%d", paramIdx)
		args = append(args, to.UTC())
	}

	q := fmt.Sprintf(`
		SELECT le.id, le.transaction_id, le.account_id, le.entry_type, le.amount, le.currency, le.created_at
		FROM ledger_entries le
		WHERE le.account_id = $1
		%s
		ORDER BY le.created_at ASC, le.id ASC
		LIMIT $2`, whereExtra)

	rows, err := r.db.Query(ctx, q, args...)
	if err != nil {
		return nil, "", fmt.Errorf("ledger repo: get entries for account %s: %w", accountID, err)
	}
	defer rows.Close()

	var entries []service.LedgerEntry
	for rows.Next() {
		var e service.LedgerEntry
		if err := rows.Scan(
			&e.ID, &e.TransactionID, &e.AccountID,
			&e.EntryType, &e.Amount, &e.Currency, &e.CreatedAt,
		); err != nil {
			return nil, "", fmt.Errorf("ledger repo: scan entry: %w", err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("ledger repo: rows error: %w", err)
	}

	// Detect next page and build cursor from last item.
	var nextCursor string
	if len(entries) > limit {
		entries = entries[:limit]
		last := entries[len(entries)-1]
		nextCursor = encodeCursor(last.CreatedAt, last.ID)
	}

	return entries, nextCursor, nil
}

// ─── Cursor helpers ───────────────────────────────────────────────────────────

func encodeCursor(t time.Time, id string) string {
	raw := fmt.Sprintf("%d:%s", t.UnixNano(), id)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeCursor(cursor string) (time.Time, string, error) {
	b, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("base64 decode: %w", err)
	}
	parts := strings.SplitN(string(b), ":", 2)
	if len(parts) != 2 {
		return time.Time{}, "", fmt.Errorf("malformed cursor")
	}
	ns, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("parse timestamp: %w", err)
	}
	return time.Unix(0, ns).UTC(), parts[1], nil
}

// ─── PostTransactionTx ────────────────────────────────────────────────────────

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
