package service

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"

	"github.com/Dubjay18/seraph/services/auth/internal/repository"
	apperrors "github.com/Dubjay18/seraph/shared/errors"
)

// mockEventPublisher is a no-op EventPublisher used in tests so the service
// can be constructed without a real RabbitMQ connection.
type mockEventPublisher struct{}

func (m *mockEventPublisher) Publish(_ context.Context, _ string, _ any) error { return nil }

type mockRepository struct {
	users                  map[uuid.UUID]*repository.User
	tokens                 map[string]*repository.RefreshToken
	createUserFunc         func(ctx context.Context, email, passwordHash, firstName, lastName string) (*repository.User, error)
	getUserByEmailFunc     func(ctx context.Context, email string) (*repository.User, error)
	getUserByIDFunc        func(ctx context.Context, id uuid.UUID) (*repository.User, error)
	createRefreshTokenFunc func(ctx context.Context, userID uuid.UUID, tokenHash string, expiresAt time.Time) error
	getRefreshTokenFunc    func(ctx context.Context, tokenHash string) (*repository.RefreshToken, error)
	revokeRefreshTokenFunc func(ctx context.Context, tokenHash string) error
	revokeUserTokensFunc   func(ctx context.Context, userID uuid.UUID) error
	createUserFromOAuthFunc func(ctx context.Context, email, firstName, lastName string) (*repository.User, error)
	getOAuthIdentityFunc    func(ctx context.Context, provider, providerID string) (*repository.OAuthIdentity, error)
	createOAuthIdentityFunc func(ctx context.Context, userID uuid.UUID, provider, providerID string) error
}

func (m *mockRepository) CreateUser(ctx context.Context, email, passwordHash, firstName, lastName string) (*repository.User, error) {
	if m.createUserFunc != nil {
		return m.createUserFunc(ctx, email, passwordHash, firstName, lastName)
	}
	ph := passwordHash
	user := &repository.User{
		ID:           uuid.New(),
		Email:        email,
		PasswordHash: &ph,
		FirstName:    firstName,
		LastName:     lastName,
		IsActive:     true,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	m.users[user.ID] = user
	return user, nil
}

func (m *mockRepository) GetUserByEmail(ctx context.Context, email string) (*repository.User, error) {
	if m.getUserByEmailFunc != nil {
		return m.getUserByEmailFunc(ctx, email)
	}
	for _, u := range m.users {
		if u.Email == email {
			return u, nil
		}
	}
	return nil, apperrors.New(apperrors.CodeNotFound, "user not found")
}

func (m *mockRepository) GetUserByID(ctx context.Context, id uuid.UUID) (*repository.User, error) {
	if m.getUserByIDFunc != nil {
		return m.getUserByIDFunc(ctx, id)
	}
	u, ok := m.users[id]
	if !ok {
		return nil, apperrors.New(apperrors.CodeNotFound, "user not found")
	}
	return u, nil
}

func (m *mockRepository) CreateRefreshToken(ctx context.Context, userID uuid.UUID, tokenHash string, expiresAt time.Time) error {
	if m.createRefreshTokenFunc != nil {
		return m.createRefreshTokenFunc(ctx, userID, tokenHash, expiresAt)
	}
	m.tokens[tokenHash] = &repository.RefreshToken{
		ID:        uuid.New(),
		UserID:    userID,
		TokenHash: tokenHash,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now(),
	}
	return nil
}

func (m *mockRepository) GetRefreshToken(ctx context.Context, tokenHash string) (*repository.RefreshToken, error) {
	if m.getRefreshTokenFunc != nil {
		return m.getRefreshTokenFunc(ctx, tokenHash)
	}
	t, ok := m.tokens[tokenHash]
	if !ok {
		return nil, apperrors.New(apperrors.CodeNotFound, "token not found")
	}
	return t, nil
}

func (m *mockRepository) RevokeRefreshToken(ctx context.Context, tokenHash string) error {
	if m.revokeRefreshTokenFunc != nil {
		return m.revokeRefreshTokenFunc(ctx, tokenHash)
	}
	t, ok := m.tokens[tokenHash]
	if ok {
		now := time.Now()
		t.RevokedAt = &now
	}
	return nil
}

func (m *mockRepository) RevokeUserRefreshTokens(ctx context.Context, userID uuid.UUID) error {
	if m.revokeUserTokensFunc != nil {
		return m.revokeUserTokensFunc(ctx, userID)
	}
	now := time.Now()
	for _, t := range m.tokens {
		if t.UserID == userID {
			t.RevokedAt = &now
		}
	}
	return nil
}

func (m *mockRepository) CreateUserFromOAuth(ctx context.Context, email, firstName, lastName string) (*repository.User, error) {
	if m.createUserFromOAuthFunc != nil {
		return m.createUserFromOAuthFunc(ctx, email, firstName, lastName)
	}
	user := &repository.User{
		ID:        uuid.New(),
		Email:     email,
		FirstName: firstName,
		LastName:  lastName,
		IsActive:  true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	m.users[user.ID] = user
	return user, nil
}

func (m *mockRepository) GetOAuthIdentity(ctx context.Context, provider, providerID string) (*repository.OAuthIdentity, error) {
	if m.getOAuthIdentityFunc != nil {
		return m.getOAuthIdentityFunc(ctx, provider, providerID)
	}
	return nil, apperrors.New(apperrors.CodeNotFound, "oauth identity not found")
}

func (m *mockRepository) CreateOAuthIdentity(ctx context.Context, userID uuid.UUID, provider, providerID string) error {
	if m.createOAuthIdentityFunc != nil {
		return m.createOAuthIdentityFunc(ctx, userID, provider, providerID)
	}
	return nil
}

func generateTestKeys(t *testing.T) (*rsa.PrivateKey, *rsa.PublicKey) {
	priv, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("failed to generate test RSA key: %v", err)
	}
	return priv, &priv.PublicKey
}

func TestRegister(t *testing.T) {
	priv, pub := generateTestKeys(t)
	repo := &mockRepository{users: make(map[uuid.UUID]*repository.User)}
	svc := New(repo, priv, pub, 15*time.Minute, 24*time.Hour, zap.NewNop(), &mockEventPublisher{}, "", "", "", make([]byte, 16))

	// Happy path
	user, err := svc.Register(context.Background(), "test@example.com", "securepassword", "John", "Doe")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if user.Email != "test@example.com" {
		t.Errorf("expected email test@example.com, got %s", user.Email)
	}
	if user.FirstName != "John" {
		t.Errorf("expected first name John, got %s", user.FirstName)
	}
	if user.LastName != "Doe" {
		t.Errorf("expected last name Doe, got %s", user.LastName)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(*user.PasswordHash), []byte("securepassword")); err != nil {
		t.Errorf("password hash does not match original password: %v", err)
	}

	// Empty inputs
	_, err = svc.Register(context.Background(), "", "pass", "", "")
	if !apperrors.IsCode(err, apperrors.CodeInvalidInput) {
		t.Errorf("expected CodeInvalidInput for empty email, got %v", err)
	}

	// Unique violation mapping (mock repo returns conflict)
	repo.createUserFunc = func(ctx context.Context,  email, passwordHash, firstName, lastName string) (*repository.User, error) {
		return nil, apperrors.New(apperrors.CodeConflict, "duplicate key")
	}
	_, err = svc.Register(context.Background(), "test@example.com", "securepassword", "John", "Doe")
	if !apperrors.IsCode(err, apperrors.CodeConflict) {
		t.Errorf("expected CodeConflict for duplicate email, got %v", err)
	}
}

func TestLogin(t *testing.T) {
	priv, pub := generateTestKeys(t)
	repo := &mockRepository{users: make(map[uuid.UUID]*repository.User), tokens: make(map[string]*repository.RefreshToken)}
	svc := New(repo, priv, pub, 15*time.Minute, 24*time.Hour, zap.NewNop(), &mockEventPublisher{}, "", "", "", make([]byte, 16))

	passHash, _ := bcrypt.GenerateFromPassword([]byte("mypassword"), bcrypt.DefaultCost)
	passHashStr := string(passHash)
	user := &repository.User{
		ID:           uuid.New(),
		Email:        "user@example.com",
		PasswordHash: &passHashStr,
		IsActive:     true,
	}
	repo.users[user.ID] = user

	// Happy path
	access, refresh, err := svc.Login(context.Background(), "user@example.com", "mypassword")
	if err != nil {
		t.Fatalf("expected login to succeed, got %v", err)
	}
	if access == "" || refresh == "" {
		t.Error("expected tokens to be non-empty")
	}

	// Verify JWT access token claims
	parsedToken, err := jwt.ParseWithClaims(access, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return pub, nil
	})
	if err != nil {
		t.Fatalf("failed to parse access token: %v", err)
	}
	claims, ok := parsedToken.Claims.(*Claims)
	if !ok || !parsedToken.Valid {
		t.Fatal("invalid access token or claims structure")
	}
	if claims.Email != "user@example.com" || claims.Subject != user.ID.String() {
		t.Errorf("token claims mismatch: email=%s, sub=%s", claims.Email, claims.Subject)
	}

	// Verify refresh token hash stored in DB
	h := sha256.New()
	h.Write([]byte(refresh))
	tokenHash := hex.EncodeToString(h.Sum(nil))
	storedToken, ok := repo.tokens[tokenHash]
	if !ok {
		t.Fatal("refresh token hash not stored in database")
	}
	if storedToken.UserID != user.ID {
		t.Errorf("refresh token user ID mismatch: expected %s, got %s", user.ID, storedToken.UserID)
	}

	// Invalid password
	_, _, err = svc.Login(context.Background(), "user@example.com", "wrongpass")
	if !apperrors.IsCode(err, apperrors.CodeUnauthorized) {
		t.Errorf("expected CodeUnauthorized for invalid password, got %v", err)
	}

	// Deactivated user
	user.IsActive = false
	_, _, err = svc.Login(context.Background(), "user@example.com", "mypassword")
	if !apperrors.IsCode(err, apperrors.CodeUnauthorized) {
		t.Errorf("expected CodeUnauthorized for deactivated user, got %v", err)
	}
}

func TestRefresh(t *testing.T) {
	priv, pub := generateTestKeys(t)
	repo := &mockRepository{users: make(map[uuid.UUID]*repository.User), tokens: make(map[string]*repository.RefreshToken)}
	svc := New(repo, priv, pub, 15*time.Minute, 24*time.Hour, zap.NewNop(), &mockEventPublisher{}, "", "", "", make([]byte, 16))
	passHash := "something"
	user := &repository.User{
		ID:           uuid.New(),
		Email:        "user@example.com",
		PasswordHash: &passHash,
		IsActive:     true,
	}
	repo.users[user.ID] = user

	// Generate initial token pair
	_, refresh, err := svc.generateTokenPair(context.Background(), user.ID, user.Email)
	if err != nil {
		t.Fatalf("failed to generate token pair: %v", err)
	}

	// Happy path refresh rotation
	time.Sleep(10 * time.Millisecond) // Ensure time moves forward
	newAccess, newRefresh, err := svc.Refresh(context.Background(), refresh)
	if err != nil {
		t.Fatalf("expected refresh to succeed, got %v", err)
	}
	if newAccess == "" || newRefresh == "" || newRefresh == refresh {
		t.Error("expected new tokens and refresh token rotation")
	}

	// Old token should be revoked
	h := sha256.New()
	h.Write([]byte(refresh))
	oldHash := hex.EncodeToString(h.Sum(nil))
	oldToken, _ := repo.GetRefreshToken(context.Background(), oldHash)
	if oldToken.RevokedAt == nil {
		t.Error("old refresh token was not marked as revoked")
	}

	// Try to refresh again using the revoked old token (Replay Attack)
	var revokedAllCount int
	repo.revokeUserTokensFunc = func(ctx context.Context, userID uuid.UUID) error {
		revokedAllCount++
		return nil
	}
	_, _, err = svc.Refresh(context.Background(), refresh)
	if err == nil {
		t.Error("expected error when reusing a revoked refresh token")
	}
	if revokedAllCount != 1 {
		t.Error("expected replay attack protection to revoke all user refresh tokens")
	}
}
