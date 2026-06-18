package service

import (
	"context"
	"fmt"
	"time"

	"github.com/Dubjay18/seraph/services/accounts/internal/dto"
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
	ListAccountsByOwnerCursor(ctx context.Context, ownerID string, cursor string, limit int) ([]repository.Account, error)
	ChangeAccountStatus(ctx context.Context, accountID string, newStatus repository.AccountStatus) error
}

// UserValidator is satisfied by client.AuthClient or any test double.
type UserValidator interface {
	UserExists(ctx context.Context, userID string) (bool, error)
}

// LedgerQueries is satisfied by *ledger.Queries or any test double.
type LedgerQueries interface {
	GetBalance(ctx context.Context, accountID string) (money.Money, error)
	GetEntries(ctx context.Context, accountID string, from, to *time.Time, cursor string, limit int) ([]dto.LedgerEntryResponse, string, error)
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

// GetAccountWithBalance retrieves a single account and enforces that callerID is
// its owner. It also fetches the live balance from the ledger service.
// Returns CodeNotFound if the account does not exist, CodeUnauthorized if the
// caller does not own the account.
func (s *AccountsService) GetAccountWithBalance(
	ctx context.Context,
	accountID string,
	callerID string,
) (*repository.Account, money.Money, error) {
	acc, err := s.repo.GetAccount(ctx, accountID)
	if err != nil {
		return nil, money.Money{}, fmt.Errorf("accounts: get account: %w", err)
	}

	if acc.OwnerID != callerID {
		return nil, money.Money{}, apperrors.New(apperrors.CodeUnauthorized, "you do not own this account")
	}

	balance, err := s.ledger.GetBalance(ctx, accountID)
	if err != nil {
		return nil, money.Money{}, fmt.Errorf("accounts: get balance: %w", err)
	}

	return acc, balance, nil
}

// ListAccountsByOwner returns all accounts belonging to a user.
func (s *AccountsService) ListAccountsByOwner(ctx context.Context, ownerID string) ([]repository.Account, error) {
	return s.repo.ListAccountsByOwner(ctx, ownerID)
}

// ListAccountsCursor returns a cursor-paginated list of accounts for the owner.
// limit+1 rows are requested from the repo to detect whether a next page exists.
func (s *AccountsService) ListAccountsCursor(
	ctx context.Context,
	ownerID string,
	cursor string,
	limit int,
) ([]repository.Account, bool, error) {
	if limit <= 0 {
		limit = 20
	}
	// Fetch one extra to determine has_more without a COUNT query.
	rows, err := s.repo.ListAccountsByOwnerCursor(ctx, ownerID, cursor, limit+1)
	if err != nil {
		return nil, false, err
	}

	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	return rows, hasMore, nil
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
// Enforces ownership and zero-balance before closing.
// Once closed, it publishes an account.closed event to RabbitMQ.
func (s *AccountsService) CloseAccount(ctx context.Context, accountID string, callerID string) error {
	account, err := s.repo.GetAccount(ctx, accountID)
	if err != nil {
		return fmt.Errorf("accounts: get account: %w", err)
	}

	if account.OwnerID != callerID {
		return apperrors.New(apperrors.CodeUnauthorized, "you do not own this account")
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

// GetStatement returns paginated ledger entries for an account by proxying to
// the ledger service. Enforces ownership before calling the ledger.
func (s *AccountsService) GetStatement(
	ctx context.Context,
	accountID string,
	callerID string,
	from, to *time.Time,
	cursor string,
	limit int,
) ([]dto.LedgerEntryResponse, string, error) {
	acc, err := s.repo.GetAccount(ctx, accountID)
	if err != nil {
		return nil, "", fmt.Errorf("accounts: get account: %w", err)
	}

	if acc.OwnerID != callerID {
		return nil, "", apperrors.New(apperrors.CodeUnauthorized, "you do not own this account")
	}

	if limit <= 0 {
		limit = 20
	}

	entries, nextCursor, err := s.ledger.GetEntries(ctx, accountID, from, to, cursor, limit)
	if err != nil {
		return nil, "", fmt.Errorf("accounts: get statement: %w", err)
	}

	return entries, nextCursor, nil
}

func (s *AccountsService) SuspendAccount(ctx context.Context, accountID string) error {
	account, err := s.repo.GetAccount(ctx, accountID)
	if err != nil {
		return fmt.Errorf("accounts: get account: %w", err)
	}

	if account.Status == repository.AccountStatusSuspended {
		return nil // already suspended, idempotent success
	}

	err = s.repo.ChangeAccountStatus(ctx, accountID, repository.AccountStatusSuspended)
	if err != nil {
		return err
	}

	return nil
}