// Package dto holds Data Transfer Objects for the accounts HTTP layer.
// These types are the sole boundary between JSON on the wire and the domain
// model; no repository or service type should appear directly in a handler
// response body.
package dto

import (
	"time"

	"github.com/Dubjay18/seraph/shared/money"
)

// ─── Request types ────────────────────────────────────────────────────────────

// CreateAccountRequest is the body accepted by POST /accounts.
// The owner_id is NOT included here — it is sourced from the JWT subject
// stored in the request context.
type CreateAccountRequest struct {
	AccountType string `json:"account_type"`
	Currency    string `json:"currency"`
}

// ─── Response types ───────────────────────────────────────────────────────────

// AccountResponse is the standard account representation returned by the API.
// The Balance field is populated only on the detail endpoint (GET /accounts/:id).
type AccountResponse struct {
	ID            string         `json:"id"`
	OwnerID       string         `json:"owner_id"`
	AccountNumber string         `json:"account_number"`
	AccountType   string         `json:"account_type"`
	Currency      string         `json:"currency"`
	Status        string         `json:"status"`
	Balance       *money.Money   `json:"balance,omitempty"` // nil on list endpoint
	CreatedAt     time.Time      `json:"created_at"`
	ClosedAt      *time.Time     `json:"closed_at,omitempty"`
}

// ListAccountsResponse wraps a page of accounts with cursor-based pagination
// metadata for GET /accounts.
type ListAccountsResponse struct {
	Data       []AccountResponse `json:"data"`
	NextCursor string            `json:"next_cursor,omitempty"` // opaque; empty when no more pages
	HasMore    bool              `json:"has_more"`
}

// ─── Statement types ─────────────────────────────────────────────────────────

// LedgerEntryResponse represents one line in an account statement.
type LedgerEntryResponse struct {
	ID            string     `json:"id"`
	TransactionID string     `json:"transaction_id"`
	EntryType     string     `json:"entry_type"` // "DEBIT" | "CREDIT"
	Amount        int64      `json:"amount"`     // minor units (e.g. kobo)
	Currency      string     `json:"currency"`
	CreatedAt     time.Time  `json:"created_at"`
}

// StatementResponse wraps a paginated slice of ledger entries for
// GET /accounts/:id/statement.
type StatementResponse struct {
	Data       []LedgerEntryResponse `json:"data"`
	NextCursor string                `json:"next_cursor,omitempty"`
	HasMore    bool                  `json:"has_more"`
}
