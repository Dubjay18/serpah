package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Dubjay18/seraph/services/accounts/internal/repository"
	"github.com/Dubjay18/seraph/services/accounts/internal/service"
	apperrors "github.com/Dubjay18/seraph/shared/errors"
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
		ID:            "acc-123",
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
		OwnerID:       "owner-123",
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

func TestCreateAccount_Success(t *testing.T) {
	repo := &mockRepo{}
	val := &mockValidator{exists: true}
	ledger := &mockLedger{}
	pub := &mockPublisher{}
	svc := service.New(repo, val, ledger, pub)

	acc, err := svc.CreateAccount(context.Background(), "user-123", repository.AccountTypeChecking, money.NGN)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if acc.OwnerID != "user-123" || acc.Currency != money.NGN || acc.AccountNumber != "1000000001" {
		t.Errorf("unexpected account fields: %+v", acc)
	}
}

func TestCreateAccount_InvalidCurrencyRejected(t *testing.T) {
	repo := &mockRepo{}
	val := &mockValidator{exists: true}
	ledger := &mockLedger{}
	pub := &mockPublisher{}
	svc := service.New(repo, val, ledger, pub)

	_, err := svc.CreateAccount(context.Background(), "user-123", repository.AccountTypeChecking, "XYZ")
	if err == nil {
		t.Fatal("expected error for invalid currency")
	}

	if !apperrors.IsCode(err, apperrors.CodeInvalidInput) {
		t.Errorf("expected INVALID_INPUT error, got: %v", err)
	}
}

func TestCreateAccount_NonExistentOwnerRejected(t *testing.T) {
	repo := &mockRepo{}
	val := &mockValidator{exists: false}
	ledger := &mockLedger{}
	pub := &mockPublisher{}
	svc := service.New(repo, val, ledger, pub)

	_, err := svc.CreateAccount(context.Background(), "user-nonexistent", repository.AccountTypeChecking, money.USD)
	if err == nil {
		t.Fatal("expected error for non-existent user ID")
	}

	if !apperrors.IsCode(err, apperrors.CodeNotFound) {
		t.Errorf("expected NOT_FOUND error, got: %v", err)
	}
}

func TestCreateAccount_ValidatorErrorPropagated(t *testing.T) {
	repo := &mockRepo{}
	expectedErr := errors.New("network timeout")
	val := &mockValidator{err: expectedErr}
	ledger := &mockLedger{}
	pub := &mockPublisher{}
	svc := service.New(repo, val, ledger, pub)

	_, err := svc.CreateAccount(context.Background(), "user-123", repository.AccountTypeChecking, money.USD)
	if err == nil {
		t.Fatal("expected validator error to be propagated")
	}

	if !errors.Is(err, expectedErr) {
		t.Errorf("expected wrapped validator error, got: %v", err)
	}
}

func TestCloseAccount_Success(t *testing.T) {
	repo := &mockRepo{
		getAccount: &repository.Account{
			ID:       "acc-abc",
			OwnerID:  "owner-123",
			Status:   repository.AccountStatusActive,
			Currency: money.USD,
		},
	}
	val := &mockValidator{}
	ledger := &mockLedger{
		balance: money.MustNew(0, money.USD),
	}
	pub := &mockPublisher{}
	svc := service.New(repo, val, ledger, pub)

	err := svc.CloseAccount(context.Background(), "acc-abc")
	if err != nil {
		t.Fatalf("expected no error closing account, got: %v", err)
	}

	if len(repo.statusCalls) != 1 || repo.statusCalls[0] != repository.AccountStatusClosed {
		t.Errorf("expected ChangeAccountStatus to be called with CLOSED status, got calls: %v", repo.statusCalls)
	}

	if pub.publishedKey != "accounts.account.closed" {
		t.Errorf("expected event accounts.account.closed to be published, got key: %q", pub.publishedKey)
	}
}

func TestCloseAccount_AlreadyClosedIdempotent(t *testing.T) {
	repo := &mockRepo{
		getAccount: &repository.Account{
			ID:     "acc-abc",
			Status: repository.AccountStatusClosed,
		},
	}
	val := &mockValidator{}
	ledger := &mockLedger{}
	pub := &mockPublisher{}
	svc := service.New(repo, val, ledger, pub)

	err := svc.CloseAccount(context.Background(), "acc-abc")
	if err != nil {
		t.Fatalf("expected no error closing already closed account, got: %v", err)
	}

	if len(repo.statusCalls) != 0 {
		t.Errorf("expected no ChangeAccountStatus calls for already closed account")
	}

	if pub.publishedKey != "" {
		t.Errorf("expected no events to be published")
	}
}

func TestCloseAccount_NonZeroBalanceRejected(t *testing.T) {
	repo := &mockRepo{
		getAccount: &repository.Account{
			ID:       "acc-abc",
			Status:   repository.AccountStatusActive,
			Currency: money.USD,
		},
	}
	val := &mockValidator{}
	ledger := &mockLedger{
		balance: money.MustNew(100, money.USD), // non-zero balance
	}
	pub := &mockPublisher{}
	svc := service.New(repo, val, ledger, pub)

	err := svc.CloseAccount(context.Background(), "acc-abc")
	if err == nil {
		t.Fatal("expected error closing account with non-zero balance")
	}

	if !apperrors.IsCode(err, apperrors.CodeInvalidInput) {
		t.Errorf("expected CodeInvalidInput, got: %v", err)
	}

	if len(repo.statusCalls) != 0 {
		t.Errorf("expected no ChangeAccountStatus calls")
	}
}
