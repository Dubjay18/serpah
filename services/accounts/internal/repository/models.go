package repository

import (
	"time"

	"github.com/Dubjay18/seraph/shared/money"
)

type AccountStatus string
type AccountType string

const (
	AccountStatusActive    AccountStatus = "ACTIVE"
	AccountStatusSuspended AccountStatus = "SUSPENDED"
	AccountStatusClosed    AccountStatus = "CLOSED"
)

const (
	AccountTypeChecking AccountType = "CHECKING"
	AccountTypeSavings  AccountType = "SAVINGS"
	AccountTypeFloat    AccountType = "FLOAT"
)

type Account struct {
	ID            string         `json:"id" db:"id"`
	OwnerID       string         `json:"owner_id" db:"owner_id"`
	AccountNumber string         `json:"account_number" db:"account_number"`
	AccountType   AccountType    `json:"account_type" db:"account_type"`
	Currency      money.Currency `json:"currency" db:"currency"`
	Status        AccountStatus  `json:"status" db:"status"`
	CreatedAt     time.Time      `json:"created_at" db:"created_at"`
	ClosedAt      *time.Time     `json:"closed_at,omitempty" db:"closed_at"`
}