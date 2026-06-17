package client

import (
	"context"
	"net/http"
	"time"

	"github.com/Dubjay18/seraph/shared/money"
)


type LedgerClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewLedgerClient(baseURL string) *LedgerClient {
	return &LedgerClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}


// GetBalance queries the ledger service to get the balance of the given account ID.
func (c *LedgerClient) GetBalance(ctx context.Context, accountID string) (money.Money, error) {
	// Implement the logic to query the ledger service for the account balance.
	// This is a placeholder implementation and should be replaced with actual HTTP request logic.
	return money.Money{Amount: 0.0, Currency: money.USD}, nil
}