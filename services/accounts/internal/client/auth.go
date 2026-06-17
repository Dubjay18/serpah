package client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

type AuthClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewAuthClient(baseURL string) *AuthClient {
	return &AuthClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// UserExists queries the auth service to check if the given user ID exists.
func (c *AuthClient) UserExists(ctx context.Context, userID string) (bool, error) {
	if userID == "" {
		return false, fmt.Errorf("auth client: user ID cannot be empty")
	}

	u, err := url.Parse(fmt.Sprintf("%s/auth/users/%s", c.baseURL, url.PathEscape(userID)))
	if err != nil {
		return false, fmt.Errorf("auth client: invalid URL: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return false, fmt.Errorf("auth client: create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("auth client: perform request: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, fmt.Errorf("auth client: unexpected response status: %d", resp.StatusCode)
	}
}
