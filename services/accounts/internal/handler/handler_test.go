package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/Dubjay18/seraph/services/accounts/internal/dto"
	"github.com/Dubjay18/seraph/services/accounts/internal/handler"
	"github.com/Dubjay18/seraph/services/accounts/internal/repository"
	"github.com/Dubjay18/seraph/services/accounts/internal/service"
	sharedmiddleware "github.com/Dubjay18/seraph/shared/middleware"
	"github.com/Dubjay18/seraph/shared/money"
)

// ─── Mocks ───────────────────────────────────────────────────────────────────

type mockRepo struct {
	createErr   error
	created     *repository.Account
	getAccount  *repository.Account
	getErr      error
	statusErr   error
	statusCalls []repository.AccountStatus
}

func (m *mockRepo) CreateAccount(
	ctx context.Context,
	ownerID string,
	accountType repository.AccountType,
	currency money.Currency,
) (*repository.Account, error) {
	if m.createErr != nil {
		return nil, m.createErr
	}
	if m.created != nil {
		return m.created, nil
	}
	return &repository.Account{
		ID:            "11111111-2222-3333-4444-555555555555",
		OwnerID:       ownerID,
		AccountNumber: "1000000001",
		AccountType:   accountType,
		Currency:      currency,
		Status:        repository.AccountStatusActive,
		CreatedAt:     time.Now(),
	}, nil
}

func (m *mockRepo) GetAccount(ctx context.Context, id string) (*repository.Account, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	if m.getAccount != nil {
		return m.getAccount, nil
	}
	return &repository.Account{
		ID:            id,
		OwnerID:       "user-123",
		AccountNumber: "1000000001",
		AccountType:   repository.AccountTypeChecking,
		Currency:      money.USD,
		Status:        repository.AccountStatusActive,
		CreatedAt:     time.Now(),
	}, nil
}

func (m *mockRepo) ListAccountsByOwner(ctx context.Context, ownerID string) ([]repository.Account, error) {
	return nil, nil
}

func (m *mockRepo) ListAccountsByOwnerCursor(ctx context.Context, ownerID string, cursor string, limit int) ([]repository.Account, error) {
	return []repository.Account{}, nil
}

func (m *mockRepo) ChangeAccountStatus(ctx context.Context, accountID string, newStatus repository.AccountStatus) error {
	if m.statusErr != nil {
		return m.statusErr
	}
	m.statusCalls = append(m.statusCalls, newStatus)
	return nil
}

type mockValidator struct {
	exists bool
	err    error
}

func (m *mockValidator) UserExists(ctx context.Context, userID string) (bool, error) {
	return m.exists, m.err
}

type mockLedger struct {
	balance money.Money
	err     error
}

func (m *mockLedger) GetBalance(ctx context.Context, accountID string) (money.Money, error) {
	if m.err != nil {
		return money.Money{}, m.err
	}
	return m.balance, nil
}

func (m *mockLedger) GetEntries(ctx context.Context, accountID string, from, to *time.Time, cursor string, limit int) ([]dto.LedgerEntryResponse, string, error) {
	return []dto.LedgerEntryResponse{}, "", nil
}

type mockPublisher struct {
	publishedKey string
	payload      any
	err          error
}

func (m *mockPublisher) Publish(ctx context.Context, routingKey string, payload any) error {
	if m.err != nil {
		return m.err
	}
	m.publishedKey = routingKey
	m.payload = payload
	return nil
}

// ─── Tests ───────────────────────────────────────────────────────────────────

func TestCreateAccount_Validation(t *testing.T) {
	repo := &mockRepo{}
	val := &mockValidator{exists: true}
	ledger := &mockLedger{}
	pub := &mockPublisher{}
	svc := service.New(repo, val, ledger, pub)
	h := handler.New(svc, zap.NewNop())

	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r = r.WithContext(sharedmiddleware.WithUserID(r.Context(), "user-123"))
			next.ServeHTTP(w, r)
		})
	})
	r.Post("/accounts", h.CreateAccount)

	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantError  string
	}{
		{
			name:       "empty type",
			body:       `{"currency":"USD"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "account_type is required",
		},
		{
			name:       "empty currency",
			body:       `{"account_type":"SAVINGS"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "currency is required",
		},
		{
			name:       "invalid type",
			body:       `{"account_type":"INVALID","currency":"USD"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid account_type",
		},
		{
			name:       "invalid currency",
			body:       `{"account_type":"SAVINGS","currency":"XYZ"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid currency",
		},
		{
			name:       "valid CHECKING USD",
			body:       `{"account_type":"CHECKING","currency":"USD"}`,
			wantStatus: http.StatusCreated,
			wantError:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/accounts", bytes.NewBufferString(tt.body))
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("expected status %d, got %d", tt.wantStatus, rec.Code)
			}

			if tt.wantError != "" {
				var errResp map[string]string
				if err := json.NewDecoder(rec.Body).Decode(&errResp); err != nil {
					t.Fatalf("failed to decode error response: %v", err)
				}
				if !strings.Contains(errResp["error"], tt.wantError) {
					t.Errorf("expected error containing %q, got %q", tt.wantError, errResp["error"])
				}
			}
		})
	}
}

func TestGetAccount_Validation(t *testing.T) {
	repo := &mockRepo{}
	val := &mockValidator{exists: true}
	ledger := &mockLedger{}
	pub := &mockPublisher{}
	svc := service.New(repo, val, ledger, pub)
	h := handler.New(svc, zap.NewNop())

	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r = r.WithContext(sharedmiddleware.WithUserID(r.Context(), "user-123"))
			next.ServeHTTP(w, r)
		})
	})
	r.Get("/accounts/{id}", h.GetAccount)

	tests := []struct {
		name       string
		id         string
		wantStatus int
		wantError  string
	}{
		{
			name:       "invalid UUID",
			id:         "123",
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid account id format",
		},
		{
			name:       "valid UUID format",
			id:         "11111111-2222-3333-4444-555555555555",
			wantStatus: http.StatusOK,
			wantError:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/accounts/"+tt.id, nil)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("expected status %d, got %d", tt.wantStatus, rec.Code)
			}

			if tt.wantError != "" {
				var errResp map[string]string
				if err := json.NewDecoder(rec.Body).Decode(&errResp); err != nil {
					t.Fatalf("failed to decode error response: %v", err)
				}
				if !strings.Contains(errResp["error"], tt.wantError) {
					t.Errorf("expected error containing %q, got %q", tt.wantError, errResp["error"])
				}
			}
		})
	}
}

func TestListAccounts_Validation(t *testing.T) {
	repo := &mockRepo{}
	val := &mockValidator{exists: true}
	ledger := &mockLedger{}
	pub := &mockPublisher{}
	svc := service.New(repo, val, ledger, pub)
	h := handler.New(svc, zap.NewNop())

	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r = r.WithContext(sharedmiddleware.WithUserID(r.Context(), "user-123"))
			next.ServeHTTP(w, r)
		})
	})
	r.Get("/accounts", h.ListAccounts)

	tests := []struct {
		name       string
		query      string
		wantStatus int
		wantError  string
	}{
		{
			name:       "invalid limit",
			query:      "?limit=-5",
			wantStatus: http.StatusBadRequest,
			wantError:  "limit must be a positive integer",
		},
		{
			name:       "invalid cursor (not base64)",
			query:      "?cursor=@@@",
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid pagination cursor",
		},
		{
			name:       "valid pagination params",
			query:      "?limit=50&cursor=ZXlKaGJHY2lPaUpTVXpJMU5pSjlmUT09", // valid base64
			wantStatus: http.StatusOK,
			wantError:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/accounts"+tt.query, nil)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("expected status %d, got %d", tt.wantStatus, rec.Code)
			}

			if tt.wantError != "" {
				var errResp map[string]string
				if err := json.NewDecoder(rec.Body).Decode(&errResp); err != nil {
					t.Fatalf("failed to decode error response: %v", err)
				}
				if !strings.Contains(errResp["error"], tt.wantError) {
					t.Errorf("expected error containing %q, got %q", tt.wantError, errResp["error"])
				}
			}
		})
	}
}

func TestGetStatement_Validation(t *testing.T) {
	repo := &mockRepo{}
	val := &mockValidator{exists: true}
	ledger := &mockLedger{}
	pub := &mockPublisher{}
	svc := service.New(repo, val, ledger, pub)
	h := handler.New(svc, zap.NewNop())

	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r = r.WithContext(sharedmiddleware.WithUserID(r.Context(), "user-123"))
			next.ServeHTTP(w, r)
		})
	})
	r.Get("/accounts/{id}/statement", h.GetStatement)

	tests := []struct {
		name       string
		id         string
		query      string
		wantStatus int
		wantError  string
	}{
		{
			name:       "invalid UUID",
			id:         "123",
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid account id format",
		},
		{
			name:       "invalid limit",
			id:         "11111111-2222-3333-4444-555555555555",
			query:      "?limit=abc",
			wantStatus: http.StatusBadRequest,
			wantError:  "limit must be a positive integer",
		},
		{
			name:       "invalid from date format",
			id:         "11111111-2222-3333-4444-555555555555",
			query:      "?from=2026-06-18",
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid 'from' date",
		},
		{
			name:       "invalid to date format",
			id:         "11111111-2222-3333-4444-555555555555",
			query:      "?to=2026-06-18",
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid 'to' date",
		},
		{
			name:       "from date after to date",
			id:         "11111111-2222-3333-4444-555555555555",
			query:      "?from=2026-06-19T00:00:00Z&to=2026-06-18T00:00:00Z",
			wantStatus: http.StatusBadRequest,
			wantError:  "from' date cannot be after 'to' date",
		},
		{
			name:       "invalid cursor",
			id:         "11111111-2222-3333-4444-555555555555",
			query:      "?cursor=@@@",
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid pagination cursor",
		},
		{
			name:       "valid query params",
			id:         "11111111-2222-3333-4444-555555555555",
			query:      "?from=2026-06-18T00:00:00Z&to=2026-06-19T00:00:00Z&limit=10&cursor=ZXlKaGJHY2lPaUpTVXpJMU5pSjlmUT09",
			wantStatus: http.StatusOK,
			wantError:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/accounts/"+tt.id+"/statement"+tt.query, nil)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("expected status %d, got %d", tt.wantStatus, rec.Code)
			}

			if tt.wantError != "" {
				var errResp map[string]string
				if err := json.NewDecoder(rec.Body).Decode(&errResp); err != nil {
					t.Fatalf("failed to decode error response: %v", err)
				}
				if !strings.Contains(errResp["error"], tt.wantError) {
					t.Errorf("expected error containing %q, got %q", tt.wantError, errResp["error"])
				}
			}
		})
	}
}
