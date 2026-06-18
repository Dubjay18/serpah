package repository

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Dubjay18/seraph/shared/money"
)

type Repository struct {
	db *pgxpool.Pool
}

func New(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

// CreateAccount inserts a new account row and returns the created record.
func (r *Repository) CreateAccount(
	ctx context.Context,
	ownerID string,
	accountType AccountType,
	currency money.Currency,
) (*Account, error) {
	const q = `
		INSERT INTO accounts (owner_id, account_type, currency)
		VALUES ($1, $2, $3)
		RETURNING id, owner_id, account_number, account_type, currency, status, created_at, closed_at`

	row := r.db.QueryRow(ctx, q, ownerID, accountType, string(currency))
	var a Account
	if err := row.Scan(
		&a.ID, &a.OwnerID, &a.AccountNumber, &a.AccountType,
		&a.Currency, &a.Status, &a.CreatedAt, &a.ClosedAt,
	); err != nil {
		return nil, fmt.Errorf("accounts: create account: %w", err)
	}
	return &a, nil
}

// GetAccount fetches a single account by UUID.
func (r *Repository) GetAccount(ctx context.Context, id string) (*Account, error) {
	const q = `
		SELECT id, owner_id, account_number, account_type, currency, status, created_at, closed_at
		FROM accounts WHERE id = $1`

	row := r.db.QueryRow(ctx, q, id)
	var a Account
	if err := row.Scan(
		&a.ID, &a.OwnerID, &a.AccountNumber, &a.AccountType,
		&a.Currency, &a.Status, &a.CreatedAt, &a.ClosedAt,
	); err != nil {
		return nil, fmt.Errorf("accounts: get account %s: %w", id, err)
	}
	return &a, nil
}

// ListAccountsByOwner returns all accounts for the given owner UUID.
func (r *Repository) ListAccountsByOwner(ctx context.Context, ownerID string) ([]Account, error) {
	const q = `
		SELECT id, owner_id, account_number, account_type, currency, status, created_at, closed_at
		FROM accounts WHERE owner_id = $1 ORDER BY created_at`

	rows, err := r.db.Query(ctx, q, ownerID)
	if err != nil {
		return nil, fmt.Errorf("accounts: list accounts for owner %s: %w", ownerID, err)
	}
	defer rows.Close()

	var accounts []Account
	for rows.Next() {
		var a Account
		if err := rows.Scan(
			&a.ID, &a.OwnerID, &a.AccountNumber, &a.AccountType,
			&a.Currency, &a.Status, &a.CreatedAt, &a.ClosedAt,
		); err != nil {
			return nil, fmt.Errorf("accounts: scan account: %w", err)
		}
		accounts = append(accounts, a)
	}
	return accounts, rows.Err()
}

// ListAccountsByOwnerCursor returns up to limit accounts for ownerID, starting
// after the position encoded in cursor. Pass an empty cursor for the first page.
func (r *Repository) ListAccountsByOwnerCursor(
	ctx context.Context,
	ownerID string,
	cursor string,
	limit int,
) ([]Account, error) {
	if limit <= 0 {
		limit = 20
	}

	// Decode cursor into (created_at, id) if provided.
	var (
		afterTime time.Time
		afterID   string
	)
	if cursor != "" {
		var err error
		afterTime, afterID, err = decodeCursor(cursor)
		if err != nil {
			return nil, fmt.Errorf("accounts: invalid cursor: %w", err)
		}
	}

	var (
		accounts []Account
		err      error
	)

	if cursor == "" {
		const q = `
			SELECT id, owner_id, account_number, account_type, currency, status, created_at, closed_at
			FROM accounts
			WHERE owner_id = $1
			ORDER BY created_at ASC, id ASC
			LIMIT $2`
		rows, qErr := r.db.Query(ctx, q, ownerID, limit)
		if qErr != nil {
			return nil, fmt.Errorf("accounts: list accounts cursor for owner %s: %w", ownerID, qErr)
		}
		defer rows.Close()
		for rows.Next() {
			var a Account
			if err = rows.Scan(
				&a.ID, &a.OwnerID, &a.AccountNumber, &a.AccountType,
				&a.Currency, &a.Status, &a.CreatedAt, &a.ClosedAt,
			); err != nil {
				return nil, fmt.Errorf("accounts: scan account: %w", err)
			}
			accounts = append(accounts, a)
		}
		err = rows.Err()
	} else {
		const q = `
			SELECT id, owner_id, account_number, account_type, currency, status, created_at, closed_at
			FROM accounts
			WHERE owner_id = $1
			  AND (created_at, id) > ($2, $3)
			ORDER BY created_at ASC, id ASC
			LIMIT $4`
		rows, qErr := r.db.Query(ctx, q, ownerID, afterTime, afterID, limit)
		if qErr != nil {
			return nil, fmt.Errorf("accounts: list accounts cursor for owner %s: %w", ownerID, qErr)
		}
		defer rows.Close()
		for rows.Next() {
			var a Account
			if err = rows.Scan(
				&a.ID, &a.OwnerID, &a.AccountNumber, &a.AccountType,
				&a.Currency, &a.Status, &a.CreatedAt, &a.ClosedAt,
			); err != nil {
				return nil, fmt.Errorf("accounts: scan account: %w", err)
			}
			accounts = append(accounts, a)
		}
		err = rows.Err()
	}

	return accounts, err
}

// ChangeAccountStatus updates the status of an account.
// If the new status is CLOSED, it also records the closed_at timestamp.
func (r *Repository) ChangeAccountStatus(ctx context.Context, accountID string, newStatus AccountStatus) error {
	const q = `
		UPDATE accounts SET status = $1, closed_at = CASE WHEN $1 = 'CLOSED' THEN NOW() ELSE NULL END WHERE id = $2`

	cmdTag, err := r.db.Exec(ctx, q, newStatus, accountID)
	if err != nil {
		return fmt.Errorf("accounts: change account status: %w", err)
	}
	if cmdTag.RowsAffected() == 0 {
		return fmt.Errorf("accounts: change account status: no rows affected")
	}
	return nil
}

// ─── Cursor helpers ───────────────────────────────────────────────────────────

// EncodeCursor encodes a (createdAt, id) pair into an opaque base64 cursor string.
func EncodeCursor(createdAt time.Time, id string) string {
	raw := fmt.Sprintf("%d:%s", createdAt.UnixNano(), id)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// decodeCursor decodes a cursor produced by EncodeCursor.
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