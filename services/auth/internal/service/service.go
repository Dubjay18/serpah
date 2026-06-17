package service

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/idtoken"

	"github.com/Dubjay18/seraph/services/auth/internal/repository"
	apperrors "github.com/Dubjay18/seraph/shared/errors"
	"github.com/Dubjay18/seraph/shared/rabbitmq"
)

type Repository interface {
	CreateUser(ctx context.Context, email, passwordHash, firstName, lastName string) (*repository.User, error)
	GetUserByEmail(ctx context.Context, email string) (*repository.User, error)
	GetUserByID(ctx context.Context, id uuid.UUID) (*repository.User, error)
	CreateRefreshToken(ctx context.Context, userID uuid.UUID, tokenHash string, expiresAt time.Time) error
	GetRefreshToken(ctx context.Context, tokenHash string) (*repository.RefreshToken, error)
	RevokeRefreshToken(ctx context.Context, tokenHash string) error
	RevokeUserRefreshTokens(ctx context.Context, userID uuid.UUID) error

	// OAuth
	CreateUserFromOAuth(ctx context.Context, email, firstName, lastName string) (*repository.User, error)
	GetOAuthIdentity(ctx context.Context, provider, providerID string) (*repository.OAuthIdentity, error)
	CreateOAuthIdentity(ctx context.Context, userID uuid.UUID, provider, providerID string) error
}

// EventPublisher is a minimal interface over rabbitmq.Publisher, kept here so
// the service layer can be tested without a real broker.
type EventPublisher interface {
	Publish(ctx context.Context, routingKey string, payload any) error
}

type AuthService struct {
	repo          Repository
	privateKey    *rsa.PrivateKey
	publicKey     *rsa.PublicKey
	accessExpiry  time.Duration
	refreshExpiry time.Duration
	log           *zap.Logger
	events        EventPublisher

	// Google OAuth
	googleOAuthConfig  *oauth2.Config
	stateCookieSecret  []byte
}

type Claims struct {
	jwt.RegisteredClaims
	Email string `json:"email"`
}

func New(
	repo Repository,
	privateKey *rsa.PrivateKey,
	publicKey *rsa.PublicKey,
	accessExpiry, refreshExpiry time.Duration,
	log *zap.Logger,
	events EventPublisher,
	googleClientID, googleClientSecret, googleCallbackURL string,
	stateCookieSecret []byte,
) *AuthService {
	return &AuthService{
		repo:          repo,
		privateKey:    privateKey,
		publicKey:     publicKey,
		accessExpiry:  accessExpiry,
		refreshExpiry: refreshExpiry,
		log:           log,
		events:        events,
		googleOAuthConfig: &oauth2.Config{
			ClientID:     googleClientID,
			ClientSecret: googleClientSecret,
			RedirectURL:  googleCallbackURL,
			Scopes:       []string{"openid", "email", "profile"},
			Endpoint:     google.Endpoint,
		},
		stateCookieSecret: stateCookieSecret,
	}
}

func (s *AuthService) Register(ctx context.Context, email, password, firstName, lastName string) (*repository.User, error) {
	if email == "" || password == "" {
		return nil, apperrors.New(apperrors.CodeInvalidInput, "email and password cannot be empty")
	}

	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		s.log.Error("failed to generate bcrypt hash for password", zap.Error(err))
		return nil, apperrors.Wrap(apperrors.CodeInternal, "failed to hash password", err)
	}

	user, err := s.repo.CreateUser(ctx, email, string(hashedBytes), firstName, lastName)
	if err != nil {
		return nil, err
	}

	// Publish event — non-fatal if the broker is temporarily unavailable.
	if pubErr := s.events.Publish(ctx, rabbitmq.EventUserRegistered, rabbitmq.UserRegisteredPayload{
		UserID:    user.ID.String(),
		Email:     user.Email,
		FirstName: user.FirstName,
		LastName:  user.LastName,
	}); pubErr != nil {
		s.log.Warn("failed to publish user.registered event", zap.Error(pubErr))
	}

	return user, nil
}

func (s *AuthService) Login(ctx context.Context, email, password string) (string, string, error) {
	if email == "" || password == "" {
		return "", "", apperrors.New(apperrors.CodeInvalidInput, "email and password cannot be empty")
	}

	user, err := s.repo.GetUserByEmail(ctx, email)
	if err != nil {
		if apperrors.IsCode(err, apperrors.CodeNotFound) {
			s.log.Warn("login attempt for unregistered email", zap.String("email", email))
			return "", "", apperrors.Wrap(apperrors.CodeUnauthorized, "invalid credentials", err)
		}
		return "", "", err
	}

	if !user.IsActive {
		s.log.Warn("login attempt for deactivated user", zap.String("email", email), zap.String("user_id", user.ID.String()))
		return "", "", apperrors.New(apperrors.CodeUnauthorized, "user account is deactivated")
	}

	if user.PasswordHash == nil {
		s.log.Warn("login attempt with password for oauth-only account", zap.String("email", email))
		return "", "", apperrors.New(apperrors.CodeUnauthorized, "this account uses Google sign-in; please use Google to log in")
	}

	err = bcrypt.CompareHashAndPassword([]byte(*user.PasswordHash), []byte(password))
	if err != nil {
		s.log.Warn("login attempt with invalid password", zap.String("email", email))
		return "", "", apperrors.Wrap(apperrors.CodeUnauthorized, "invalid credentials", err)
	}

	return s.generateTokenPair(ctx, user.ID, user.Email)
}

func (s *AuthService) Refresh(ctx context.Context, refreshTokenStr string) (string, string, error) {
	if refreshTokenStr == "" {
		return "", "", apperrors.New(apperrors.CodeInvalidInput, "refresh token cannot be empty")
	}

	tokenHash := s.hashToken(refreshTokenStr)
	token, err := s.repo.GetRefreshToken(ctx, tokenHash)
	if err != nil {
		if apperrors.IsCode(err, apperrors.CodeNotFound) {
			s.log.Warn("refresh attempt with non-existent token", zap.String("token_hash", tokenHash))
			return "", "", apperrors.Wrap(apperrors.CodeUnauthorized, "invalid refresh token", err)
		}
		return "", "", err
	}

	if token.RevokedAt != nil {
		// Potential token reuse detection: revoke all tokens for this user for security.
		s.log.Warn("revoked refresh token presented - possible token reuse attack detected",
			zap.String("user_id", token.UserID.String()),
			zap.String("token_hash", tokenHash),
		)
		_ = s.repo.RevokeUserRefreshTokens(ctx, token.UserID)
		return "", "", apperrors.New(apperrors.CodeUnauthorized, "refresh token has been revoked")
	}

	if time.Now().After(token.ExpiresAt) {
		s.log.Info("expired refresh token presented",
			zap.String("user_id", token.UserID.String()),
			zap.Time("expired_at", token.ExpiresAt),
		) 
		return "", "", apperrors.New(apperrors.CodeUnauthorized, "refresh token is expired")
	}

	// Revoke current token (Refresh Token Rotation)
	err = s.repo.RevokeRefreshToken(ctx, tokenHash)
	if err != nil {
		s.log.Error("failed to revoke refresh token during rotation", zap.String("token_hash", tokenHash), zap.Error(err))
		return "", "", err
	}

	user, err := s.repo.GetUserByID(ctx, token.UserID)
	if err != nil {
		if apperrors.IsCode(err, apperrors.CodeNotFound) {
			s.log.Warn("refresh attempted for non-existent user", zap.String("user_id", token.UserID.String()))
			return "", "", apperrors.Wrap(apperrors.CodeUnauthorized, "user not found", err)
		}
		return "", "", err
	}

	if !user.IsActive {
		s.log.Warn("refresh attempted for deactivated user", zap.String("user_id", token.UserID.String()))
		return "", "", apperrors.New(apperrors.CodeUnauthorized, "user account is deactivated")
	}

	return s.generateTokenPair(ctx, user.ID, user.Email)
}

func (s *AuthService) Logout(ctx context.Context, refreshTokenStr string) error {
	if refreshTokenStr == "" {
		return apperrors.New(apperrors.CodeInvalidInput, "refresh token cannot be empty")
	}

	tokenHash := s.hashToken(refreshTokenStr)
	return s.repo.RevokeRefreshToken(ctx, tokenHash)
}

// ─── Google OAuth ─────────────────────────────────────────────────────────────

// GoogleOAuthURL generates the Google consent-page redirect URL.
// state must be a value you have already persisted (e.g. in a signed cookie).
func (s *AuthService) GoogleOAuthURL(state string) string {
	return s.googleOAuthConfig.AuthCodeURL(state, oauth2.AccessTypeOnline)
}

// GenerateOAuthState creates a random state token and an HMAC-signed version for
// double-submit cookie verification.  Returns (rawState, signedState).
func (s *AuthService) GenerateOAuthState() (string, string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", "", apperrors.Wrap(apperrors.CodeInternal, "failed to generate oauth state", err)
	}
	raw := base64.URLEncoding.EncodeToString(b)
	sig := s.signState(raw)
	return raw, sig, nil
}

// ValidateOAuthState verifies that the signed state cookie matches the raw state
// from the query parameter, guarding against CSRF.
func (s *AuthService) ValidateOAuthState(rawState, signedCookie string) bool {
	expected := s.signState(rawState)
	return hmac.Equal([]byte(expected), []byte(signedCookie))
}

func (s *AuthService) signState(raw string) string {
	mac := hmac.New(sha256.New, s.stateCookieSecret)
	mac.Write([]byte(raw))
	return base64.URLEncoding.EncodeToString(mac.Sum(nil))
}

// GoogleOAuthCallback exchanges the authorization code for tokens, verifies the
// Google ID token, and upserts the user + oauth identity, returning a Seraph
// access + refresh token pair.
func (s *AuthService) GoogleOAuthCallback(ctx context.Context, code string) (string, string, error) {
	// Exchange code for Google token set.
	googleToken, err := s.googleOAuthConfig.Exchange(ctx, code)
	if err != nil {
		s.log.Warn("google oauth code exchange failed", zap.Error(err))
		return "", "", apperrors.Wrap(apperrors.CodeUnauthorized, "failed to exchange google auth code", err)
	}

	rawIDToken, ok := googleToken.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		return "", "", apperrors.New(apperrors.CodeUnauthorized, "google did not return an id_token")
	}

	// Verify and decode the ID token.
	payload, err := idtoken.Validate(ctx, rawIDToken, s.googleOAuthConfig.ClientID)
	if err != nil {
		s.log.Warn("google id_token validation failed", zap.Error(err))
		return "", "", apperrors.Wrap(apperrors.CodeUnauthorized, "invalid google id_token", err)
	}

	sub, _ := payload.Claims["sub"].(string)
	email, _ := payload.Claims["email"].(string)
	firstName, _ := payload.Claims["given_name"].(string)
	lastName, _ := payload.Claims["family_name"].(string)

	if sub == "" || email == "" {
		return "", "", apperrors.New(apperrors.CodeUnauthorized, "google id_token missing required claims")
	}

	// Look up existing OAuth identity.
	identity, err := s.repo.GetOAuthIdentity(ctx, "google", sub)
	if err == nil {
		// Identity exists – just load the user and issue tokens.
		user, err := s.repo.GetUserByID(ctx, identity.UserID)
		if err != nil {
			return "", "", err
		}
		if !user.IsActive {
			return "", "", apperrors.New(apperrors.CodeUnauthorized, "user account is deactivated")
		}
		s.log.Info("google oauth login: existing identity", zap.String("user_id", user.ID.String()))
		return s.generateTokenPair(ctx, user.ID, user.Email)
	}

	if !apperrors.IsCode(err, apperrors.CodeNotFound) {
		return "", "", err
	}

	// Identity not found – check if a user with this email already exists (auto-link).
	user, err := s.repo.GetUserByEmail(ctx, email)
	if err != nil && !apperrors.IsCode(err, apperrors.CodeNotFound) {
		return "", "", err
	}

	if apperrors.IsCode(err, apperrors.CodeNotFound) {
		// Brand-new user – create account without a password.
		user, err = s.repo.CreateUserFromOAuth(ctx, email, firstName, lastName)
		if err != nil {
			return "", "", err
		}
		s.log.Info("google oauth login: new user created", zap.String("user_id", user.ID.String()), zap.String("email", email))

		// Publish event for the newly created OAuth user.
		if pubErr := s.events.Publish(ctx, rabbitmq.EventUserRegistered, rabbitmq.UserRegisteredPayload{
			UserID:    user.ID.String(),
			Email:     user.Email,
			FirstName: user.FirstName,
			LastName:  user.LastName,
		}); pubErr != nil {
			s.log.Warn("failed to publish user.registered event (oauth)", zap.Error(pubErr))
		}
	} else {
		s.log.Info("google oauth login: auto-linking existing account", zap.String("user_id", user.ID.String()), zap.String("email", email))
	}

	// Create the OAuth identity link.
	if err := s.repo.CreateOAuthIdentity(ctx, user.ID, "google", sub); err != nil {
		return "", "", err
	}

	return s.generateTokenPair(ctx, user.ID, user.Email)
}

// ─── Internal helpers ─────────────────────────────────────────────────────────

func (s *AuthService) hashToken(token string) string {
	h := sha256.New()
	h.Write([]byte(token))
	return hex.EncodeToString(h.Sum(nil))
}

func (s *AuthService) generateAccessToken(userID uuid.UUID, email string) (string, error) {
	now := time.Now()
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.accessExpiry)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
		},
		Email: email,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	return token.SignedString(s.privateKey)
}

func (s *AuthService) generateRefreshToken() (string, error) {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	if err != nil {
		s.log.Error("failed to generate random bytes for refresh token", zap.Error(err))
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (s *AuthService) generateTokenPair(ctx context.Context, userID uuid.UUID, email string) (string, string, error) {
	accessToken, err := s.generateAccessToken(userID, email)
	if err != nil {
		s.log.Error("failed to generate access token", zap.String("user_id", userID.String()), zap.Error(err))
		return "", "", apperrors.Wrap(apperrors.CodeInternal, "failed to generate access token", err)
	}

	rawRefreshToken, err := s.generateRefreshToken()
	if err != nil {
		s.log.Error("failed to generate raw refresh token", zap.String("user_id", userID.String()), zap.Error(err))
		return "", "", apperrors.Wrap(apperrors.CodeInternal, "failed to generate refresh token", err)
	}

	tokenHash := s.hashToken(rawRefreshToken)
	expiresAt := time.Now().Add(s.refreshExpiry)

	err = s.repo.CreateRefreshToken(ctx, userID, tokenHash, expiresAt)
	if err != nil {
		s.log.Error("failed to save refresh token to database", zap.String("user_id", userID.String()), zap.Error(err))
		return "", "", err
	}

	return accessToken, rawRefreshToken, nil
}

// GetUserByID parses the user ID and retrieves the user from the repository.
func (s *AuthService) GetUserByID(ctx context.Context, idStr string) (*repository.User, error) {
	id, err := uuid.Parse(idStr)
	if err != nil {
		return nil, apperrors.New(apperrors.CodeInvalidInput, "invalid user ID format")
	}
	return s.repo.GetUserByID(ctx, id)
}
