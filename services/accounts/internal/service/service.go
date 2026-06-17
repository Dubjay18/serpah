package service

import (
	"context"
	"fmt"

	"github.com/Dubjay18/seraph/services/accounts/internal/repository"
	apperrors "github.com/Dubjay18/seraph/shared/errors"
	"github.com/Dubjay18/seraph/shared/money"
	"github.com/Dubjay18/seraph/shared/rabbitmq"
)

// AccountRepository is the data-access interface satisfied by *repository.Repository.
type AccountRepository interface {
	CreateAccount(ctx context.Context, ownerID string, accountType repository.AccountType, currency money.Currency) (*repository.Account, error)
	GetAccount(ctx context.Context, id string) (*repository.Account, error)
	ListAccountsByOwner(ctx context.Context, ownerID string) ([]repository.Account, error)
	ChangeAccountStatus(ctx context.Context, accountID string, newStatus repository.AccountStatus) error
}

// UserValidator is satisfied by client.AuthClient or any test double.
type UserValidator interface {
	UserExists(ctx context.Context, userID string) (bool, error)
}

// LedgerQueries is satisfied by *ledger.Queries or any test double.
type LedgerQueries interface {
	GetBalance(ctx context.Context, accountID string) (money.Money, error)
}

// EventPublisher is satisfied by *rabbitmq.Publisher and any test double.
type EventPublisher interface {
	Publish(ctx context.Context, routingKey string, payload any) error
}

// AccountsService holds business logic for the accounts domain.
type AccountsService struct {
	repo      AccountRepository
	validator UserValidator
	ledger    LedgerQueries
	events    EventPublisher
}

func New(
	repo AccountRepository,
	validator UserValidator,
	ledger LedgerQueries,
	events EventPublisher,
) *AccountsService {
	return &AccountsService{
		repo:      repo,
		validator: validator,
		ledger:    ledger,
		events:    events,
	}
}

// CreateAccount opens a new account for ownerID in the given currency.
// It verifies that ownerID represents a valid user in the auth service.
// Returns an error if the currency is unsupported or the owner does not exist.
func (s *AccountsService) CreateAccount(
	ctx context.Context,
	ownerID string,
	accountType repository.AccountType,
	currency money.Currency,
) (*repository.Account, error) {
	if err := currency.Validate(); err != nil {
		return nil, apperrors.New(apperrors.CodeInvalidInput, fmt.Sprintf("invalid currency: %v", err))
	}

	exists, err := s.validator.UserExists(ctx, ownerID)
	if err != nil {
		return nil, fmt.Errorf("accounts: validate user existence: %w", err)
	}
	if !exists {
		return nil, apperrors.New(apperrors.CodeNotFound, fmt.Sprintf("user %s not found", ownerID))
	}

	return s.repo.CreateAccount(ctx, ownerID, accountType, currency)
}

// GetAccount retrieves a single account by its UUID.
func (s *AccountsService) GetAccount(ctx context.Context, id string) (*repository.Account, error) {
	return s.repo.GetAccount(ctx, id)
}

// ListAccountsByOwner returns all accounts belonging to a user.
func (s *AccountsService) ListAccountsByOwner(ctx context.Context, ownerID string) ([]repository.Account, error) {
	return s.repo.ListAccountsByOwner(ctx, ownerID)
}

// BalanceQuery returns the current balance of the account.
func (s *AccountsService) BalanceQuery(ctx context.Context, accountID string) (money.Money, error) {
	account, err := s.repo.GetAccount(ctx, accountID)
	if err != nil {
		return money.Money{}, fmt.Errorf("accounts: get account: %w", err)
	}
	return s.ledger.GetBalance(ctx, account.ID)
}

// CloseAccount marks an account as closed. It does not delete the record from the database.
// Once closed, it publishes an account.closed event to RabbitMQ.
func (s *AccountsService) CloseAccount(ctx context.Context, accountID string) error {
	account, err := s.repo.GetAccount(ctx, accountID)
	if err != nil {
		return fmt.Errorf("accounts: get account: %w", err)
	}

	if account.Status == repository.AccountStatusClosed {
		return nil // already closed, idempotent success
	}

	balance, err := s.ledger.GetBalance(ctx, accountID)
	if err != nil {
		return fmt.Errorf("accounts: get balance: %w", err)
	}
	if balance.Amount != 0 {
		return apperrors.New(apperrors.CodeInvalidInput, "cannot close account with non-zero balance")
	}

	err = s.repo.ChangeAccountStatus(ctx, accountID, repository.AccountStatusClosed)
	if err != nil {
		return err
	}

	// Emit account.closed event.
	if err := s.events.Publish(ctx, rabbitmq.EventAccountClosed, rabbitmq.AccountClosedPayload{
		AccountID: account.ID,
		OwnerID:   account.OwnerID,
	}); err != nil {
		// Event publication failure is logged but doesn't roll back the DB status change.
		// A background outbox/retry daemon would typically handle reliability.
		return fmt.Errorf("accounts: publish account closed event: %w", err)
	}

	return nil
}