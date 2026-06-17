package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	apperrors "github.com/Dubjay18/seraph/shared/errors"
	"go.uber.org/zap"
)

type Repository struct {
	db  *pgxpool.Pool
	log *zap.Logger
}

func New(db *pgxpool.Pool, log *zap.Logger) *Repository {
	return &Repository{db: db, log: log}
}

func (r *Repository) CreateUser(ctx context.Context, email, passwordHash, firstName, lastName string) (*User, error) {
	var user User
	err := r.db.QueryRow(ctx,
		"INSERT INTO users (email, password_hash, first_name, last_name) VALUES ($1, $2, $3, $4) RETURNING id, email, password_hash, first_name, last_name, is_active, created_at, updated_at",
		email, passwordHash, firstName, lastName,
	).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.FirstName, &user.LastName, &user.IsActive, &user.CreatedAt, &user.UpdatedAt)

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, apperrors.Wrap(apperrors.CodeConflict, "email already registered", err)
		}
		r.log.Error("database error: failed to create user", zap.String("email", email), zap.Error(err))
		return nil, apperrors.Wrap(apperrors.CodeInternal, "failed to create user", err)
	}
	return &user, nil
}

// CreateUserFromOAuth inserts a user without a password hash (OAuth-only account).
func (r *Repository) CreateUserFromOAuth(ctx context.Context, email, firstName, lastName string) (*User, error) {
	var user User
	err := r.db.QueryRow(ctx,
		"INSERT INTO users (email, first_name, last_name) VALUES ($1, $2, $3) RETURNING id, email, password_hash, first_name, last_name, is_active, created_at, updated_at",
		email, firstName, lastName,
	).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.FirstName, &user.LastName, &user.IsActive, &user.CreatedAt, &user.UpdatedAt)

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, apperrors.Wrap(apperrors.CodeConflict, "email already registered", err)
		}
		r.log.Error("database error: failed to create oauth user", zap.String("email", email), zap.Error(err))
		return nil, apperrors.Wrap(apperrors.CodeInternal, "failed to create user", err)
	}
	return &user, nil
}

// GetOAuthIdentity looks up an oauth_identities row by provider + provider_id.
func (r *Repository) GetOAuthIdentity(ctx context.Context, provider, providerID string) (*OAuthIdentity, error) {
	var identity OAuthIdentity
	err := r.db.QueryRow(ctx,
		"SELECT id, user_id, provider, provider_id, created_at FROM oauth_identities WHERE provider = $1 AND provider_id = $2",
		provider, providerID,
	).Scan(&identity.ID, &identity.UserID, &identity.Provider, &identity.ProviderID, &identity.CreatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperrors.Wrap(apperrors.CodeNotFound, "oauth identity not found", err)
		}
		r.log.Error("database error: failed to query oauth identity",
			zap.String("provider", provider), zap.String("provider_id", providerID), zap.Error(err))
		return nil, apperrors.Wrap(apperrors.CodeInternal, "failed to query oauth identity", err)
	}
	return &identity, nil
}

// CreateOAuthIdentity inserts a new row into oauth_identities.
func (r *Repository) CreateOAuthIdentity(ctx context.Context, userID uuid.UUID, provider, providerID string) error {
	_, err := r.db.Exec(ctx,
		"INSERT INTO oauth_identities (user_id, provider, provider_id) VALUES ($1, $2, $3)",
		userID, provider, providerID,
	)
	if err != nil {
		r.log.Error("database error: failed to create oauth identity",
			zap.String("user_id", userID.String()), zap.String("provider", provider), zap.Error(err))
		return apperrors.Wrap(apperrors.CodeInternal, "failed to create oauth identity", err)
	}
	return nil
}

func (r *Repository) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	var user User
	err := r.db.QueryRow(ctx,
		"SELECT id, email, password_hash, first_name, last_name, is_active, created_at, updated_at FROM users WHERE email = $1",
		email,
	).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.FirstName, &user.LastName, &user.IsActive, &user.CreatedAt, &user.UpdatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperrors.Wrap(apperrors.CodeNotFound, "user not found", err)
		}
		r.log.Error("database error: failed to query user by email", zap.String("email", email), zap.Error(err))
		return nil, apperrors.Wrap(apperrors.CodeInternal, "failed to query user by email", err)
	}
	return &user, nil
}

func (r *Repository) GetUserByID(ctx context.Context, id uuid.UUID) (*User, error) {
	var user User
	err := r.db.QueryRow(ctx,
		"SELECT id, email, password_hash, first_name, last_name, is_active, created_at, updated_at FROM users WHERE id = $1",
		id,
	).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.FirstName, &user.LastName, &user.IsActive, &user.CreatedAt, &user.UpdatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperrors.Wrap(apperrors.CodeNotFound, "user not found", err)
		}
		r.log.Error("database error: failed to query user by id", zap.String("user_id", id.String()), zap.Error(err))
		return nil, apperrors.Wrap(apperrors.CodeInternal, "failed to query user by id", err)
	}
	return &user, nil
}

func (r *Repository) CreateRefreshToken(ctx context.Context, userID uuid.UUID, tokenHash string, expiresAt time.Time) error {
	_, err := r.db.Exec(ctx,
		"INSERT INTO refresh_tokens (user_id, token_hash, expires_at) VALUES ($1, $2, $3)",
		userID, tokenHash, expiresAt,
	)
	if err != nil {
		r.log.Error("database error: failed to store refresh token", zap.String("user_id", userID.String()), zap.Error(err))
		return apperrors.Wrap(apperrors.CodeInternal, "failed to store refresh token", err)
	}
	return nil
}

func (r *Repository) GetRefreshToken(ctx context.Context, tokenHash string) (*RefreshToken, error) {
	var token RefreshToken
	err := r.db.QueryRow(ctx,
		"SELECT id, user_id, token_hash, expires_at, revoked_at, created_at FROM refresh_tokens WHERE token_hash = $1",
		tokenHash,
	).Scan(&token.ID, &token.UserID, &token.TokenHash, &token.ExpiresAt, &token.RevokedAt, &token.CreatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperrors.Wrap(apperrors.CodeNotFound, "refresh token not found", err)
		}
		r.log.Error("database error: failed to query refresh token", zap.String("token_hash", tokenHash), zap.Error(err))
		return nil, apperrors.Wrap(apperrors.CodeInternal, "failed to query refresh token", err)
	}
	return &token, nil
}

func (r *Repository) RevokeRefreshToken(ctx context.Context, tokenHash string) error {
	_, err := r.db.Exec(ctx,
		"UPDATE refresh_tokens SET revoked_at = NOW() WHERE token_hash = $1 AND revoked_at IS NULL",
		tokenHash,
	)
	if err != nil {
		r.log.Error("database error: failed to revoke refresh token", zap.String("token_hash", tokenHash), zap.Error(err))
		return apperrors.Wrap(apperrors.CodeInternal, "failed to revoke refresh token", err)
	}
	return nil
}

func (r *Repository) RevokeUserRefreshTokens(ctx context.Context, userID uuid.UUID) error {
	_, err := r.db.Exec(ctx,
		"UPDATE refresh_tokens SET revoked_at = NOW() WHERE user_id = $1 AND revoked_at IS NULL",
		userID,
	)
	if err != nil {
		r.log.Error("database error: failed to revoke user refresh tokens", zap.String("user_id", userID.String()), zap.Error(err))
		return apperrors.Wrap(apperrors.CodeInternal, "failed to revoke user refresh tokens", err)
	}
	return nil
}