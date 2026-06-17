package handler

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"

	"github.com/Dubjay18/seraph/services/auth/internal/repository"
	"github.com/Dubjay18/seraph/services/auth/internal/service"
	apperrors "github.com/Dubjay18/seraph/shared/errors"
)
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
	revokeUserTokensFunc        func(ctx context.Context, userID uuid.UUID) error
	createUserFromOAuthFunc     func(ctx context.Context, email, firstName, lastName string) (*repository.User, error)
	getOAuthIdentityFunc        func(ctx context.Context, provider, providerID string) (*repository.OAuthIdentity, error)
	createOAuthIdentityFunc     func(ctx context.Context, userID uuid.UUID, provider, providerID string) error
}

func (m *mockRepository) CreateUser(ctx context.Context, email, passwordHash, firstName, lastName string) (*repository.User, error) {
	if m.createUserFunc != nil {
		return m.createUserFunc(ctx, email, passwordHash, firstName, lastName)
	}
	ph := passwordHash
	user := &repository.User{
		ID:           uuid.New(),
		Email:        email,
		FirstName:    firstName,
		LastName:     lastName,
		PasswordHash: &ph,
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

func TestHandlerRegister(t *testing.T) {
	priv, pub := generateTestKeys(t)
	repo := &mockRepository{users: make(map[uuid.UUID]*repository.User)}
	googleClientID := "test-google-client-id"
	googleSecret := "test-google-secret"
	googleCallbackURL := "http://localhost:8080/auth/google/callback"
	stateCookieBytes := make([]byte, 16)

	svc := service.New(repo, priv, pub, 15*time.Minute, 24*time.Hour, zap.NewNop(),&mockEventPublisher{},googleClientID, googleSecret,googleCallbackURL,stateCookieBytes)
	h := New(svc, zap.NewNop())

	// Valid Request
	body, _ := json.Marshal(RegisterRequest{
		Email:    "newuser@example.com",
		Password: "password123",
		FirstName: "John",
		LastName: "Doe",
	})
	req := httptest.NewRequest("POST", "/auth/register", bytes.NewBuffer(body))
	w := httptest.NewRecorder()

	h.Register(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d", w.Code)
	}

	var user repository.User
	if err := json.NewDecoder(w.Body).Decode(&user); err != nil {
		t.Fatalf("failed to parse response body: %v", err)
	}
	if user.Email != "newuser@example.com" {
		t.Errorf("expected email newuser@example.com, got %s", user.Email)
	}

	// Conflict/Duplicate
	repo.createUserFunc = func(ctx context.Context, email, passwordHash, firstName, lastName string) (*repository.User, error) {
		return nil, apperrors.New(apperrors.CodeConflict, "email already registered")
	}
	req = httptest.NewRequest("POST", "/auth/register", bytes.NewBuffer(body))
	w = httptest.NewRecorder()

	h.Register(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected status 409, got %d", w.Code)
	}
}

func TestHandlerLogin(t *testing.T) {
	priv, pub := generateTestKeys(t)
	repo := &mockRepository{users: make(map[uuid.UUID]*repository.User), tokens: make(map[string]*repository.RefreshToken)}
	svc := service.New(repo, priv, pub, 15*time.Minute, 24*time.Hour, zap.NewNop(),&mockEventPublisher{}, "", "", "", make([]byte, 16))
	h := New(svc, zap.NewNop())

	passHash, _ := bcrypt.GenerateFromPassword([]byte("loginpass"), bcrypt.DefaultCost)
	passHashStr := string(passHash)
	user := &repository.User{
		ID:           uuid.New(),
		Email:        "login@example.com",
		PasswordHash: &passHashStr,
		IsActive:     true,
	}
	repo.users[user.ID] = user

	// Valid Login
	body, _ := json.Marshal(LoginRequest{
		Email:    "login@example.com",
		Password: "loginpass",
	})
	req := httptest.NewRequest("POST", "/auth/login", bytes.NewBuffer(body))
	w := httptest.NewRecorder()

	h.Login(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["access_token"] == "" || resp["refresh_token"] == "" {
		t.Error("expected access and refresh tokens in login response")
	}

	// Invalid password
	body, _ = json.Marshal(LoginRequest{
		Email:    "login@example.com",
		Password: "wrongpassword",
	})
	req = httptest.NewRequest("POST", "/auth/login", bytes.NewBuffer(body))
	w = httptest.NewRecorder()

	h.Login(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", w.Code)
	}
}
