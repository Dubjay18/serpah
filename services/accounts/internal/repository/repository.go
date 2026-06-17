package repository

import (
	"context"
	"fmt"

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

func (r *Repository) ChangeAccountStatus(ctx context.Context, accountID string, newStatus AccountStatus) error {
	// if newstatus is closed, we should also set the closed_at timestamp
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