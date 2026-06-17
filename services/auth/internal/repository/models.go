package repository

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID           uuid.UUID  `json:"id" db:"id"`
	Email        string     `json:"email" db:"email"`
	FirstName    string     `json:"first_name" db:"first_name"`
	LastName     string     `json:"last_name" db:"last_name"`
	PasswordHash *string    `json:"-" db:"password_hash"` // nullable: OAuth-only accounts have no password
	IsActive     bool       `json:"is_active" db:"is_active"`
	CreatedAt    time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at" db:"updated_at"`
}

type RefreshToken struct {
	ID        uuid.UUID  `json:"id" db:"id"`
	UserID    uuid.UUID  `json:"user_id" db:"user_id"`
	TokenHash string     `json:"token_hash" db:"token_hash"`
	ExpiresAt time.Time  `json:"expires_at" db:"expires_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty" db:"revoked_at"`
	CreatedAt time.Time  `json:"created_at" db:"created_at"`
}

// OAuthIdentity links an external provider identity to an internal user.
type OAuthIdentity struct {
	ID         uuid.UUID `json:"id" db:"id"`
	UserID     uuid.UUID `json:"user_id" db:"user_id"`
	Provider   string    `json:"provider" db:"provider"`
	ProviderID string    `json:"provider_id" db:"provider_id"`
	CreatedAt  time.Time `json:"created_at" db:"created_at"`
}