package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/Dubjay18/seraph/services/accounts/internal/dto"
	"github.com/Dubjay18/seraph/shared/money"
)

// LedgerClient is an HTTP client for the ledger service.
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

// balanceResponse matches the JSON returned by GET /ledger/accounts/:id/balance.
type balanceResponse struct {
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

// GetBalance queries the ledger service for the live balance of accountID.
func (c *LedgerClient) GetBalance(ctx context.Context, accountID string) (money.Money, error) {
	u := fmt.Sprintf("%s/ledger/accounts/%s/balance", c.baseURL, url.PathEscape(accountID))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return money.Money{}, fmt.Errorf("ledger client: build request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return money.Money{}, fmt.Errorf("ledger client: get balance: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		// Account has no ledger entries yet — zero balance is fine.
		return money.Money{Amount: 0, Currency: money.NGN}, nil
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return money.Money{}, fmt.Errorf("ledger client: get balance: unexpected status %d: %s", resp.StatusCode, body)
	}

	var payload balanceResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return money.Money{}, fmt.Errorf("ledger client: decode balance response: %w", err)
	}

	return money.Money{
		Amount:   payload.Amount,
		Currency: money.Currency(payload.Currency),
	}, nil
}

// entriesResponse matches the JSON returned by GET /ledger/accounts/:id/entries.
type entriesResponse struct {
	Data       []dto.LedgerEntryResponse `json:"data"`
	NextCursor string                    `json:"next_cursor"`
	HasMore    bool                      `json:"has_more"`
}

// GetEntries fetches paginated ledger entries from the ledger service for accountID.
// from/to are optional date range filters; pass nil to omit them.
// cursor is the opaque pagination token; pass "" for the first page.
func (c *LedgerClient) GetEntries(
	ctx context.Context,
	accountID string,
	from, to *time.Time,
	cursor string,
	limit int,
) ([]dto.LedgerEntryResponse, string, error) {
	base := fmt.Sprintf("%s/ledger/accounts/%s/entries", c.baseURL, url.PathEscape(accountID))

	params := url.Values{}
	if from != nil {
		params.Set("from", from.UTC().Format(time.RFC3339))
	}
	if to != nil {
		params.Set("to", to.UTC().Format(time.RFC3339))
	}
	if cursor != "" {
		params.Set("cursor", cursor)
	}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}

	fullURL := base
	if len(params) > 0 {
		fullURL = base + "?" + params.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("ledger client: build request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("ledger client: get entries: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, "", fmt.Errorf("ledger client: get entries: unexpected status %d: %s", resp.StatusCode, body)
	}

	var payload entriesResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, "", fmt.Errorf("ledger client: decode entries response: %w", err)
	}

	return payload.Data, payload.NextCursor, nil
}