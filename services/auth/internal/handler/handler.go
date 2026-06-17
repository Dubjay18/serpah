package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/Dubjay18/seraph/services/auth/internal/service"
	apperrors "github.com/Dubjay18/seraph/shared/errors"
	"go.uber.org/zap"
)

const (
	oauthStateCookieName = "oauth_state"
	oauthRawCookieName   = "oauth_raw_state"
)

type Handler struct {
	svc *service.AuthService
	log *zap.Logger
}

func New(svc *service.AuthService, log *zap.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

// Health godoc
//
//	@Summary		Health check
//	@Description	Returns the health status of the auth service
//	@Tags			auth
//	@Produce		json
//	@Success		200	{object}	map[string]string
//	@Router			/auth/health [get]
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok","service":"auth"}`))
}

// Register godoc
//
//	@Summary		Register a new user
//	@Description	Creates a new user account with email and password
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Param			body	body		RegisterRequest	true	"Registration payload"
//	@Success		201		{object}	map[string]interface{}
//	@Failure		400		{object}	map[string]string
//	@Failure		409		{object}	map[string]string
//	@Router			/auth/register [post]
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.writeError(w, apperrors.New(apperrors.CodeInvalidInput, "method not allowed"))
		return
	}

	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.log.Warn("failed to decode register request body", zap.Error(err))
		h.writeError(w, apperrors.Wrap(apperrors.CodeInvalidInput, "invalid json body", err))
		return
	}

	user, err := h.svc.Register(r.Context(), req.Email, req.Password, req.FirstName, req.LastName)
	if err != nil {
		h.log.Warn("registration failed", zap.String("email", req.Email), zap.Error(err))
		h.writeError(w, err)
		return
	}

	h.log.Info("user registered successfully", zap.String("email", user.Email), zap.String("user_id", user.ID.String()))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(user)
}

// Login godoc
//
//	@Summary		Login
//	@Description	Authenticates a user and returns access + refresh tokens
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Param			body	body		LoginRequest	true	"Login payload"
//	@Success		200		{object}	map[string]string
//	@Failure		400		{object}	map[string]string
//	@Failure		401		{object}	map[string]string
//	@Router			/auth/login [post]
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.writeError(w, apperrors.New(apperrors.CodeInvalidInput, "method not allowed"))
		return
	}

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.log.Warn("failed to decode login request body", zap.Error(err))
		h.writeError(w, apperrors.Wrap(apperrors.CodeInvalidInput, "invalid json body", err))
		return
	}

	accessToken, refreshToken, err := h.svc.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		h.log.Warn("login failed", zap.String("email", req.Email), zap.Error(err))
		h.writeError(w, err)
		return
	}

	h.log.Info("user logged in successfully", zap.String("email", req.Email))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
	})
}

// Refresh godoc
//
//	@Summary		Refresh tokens
//	@Description	Issues a new access + refresh token pair given a valid refresh token
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Param			body	body		RefreshRequest	true	"Refresh payload"
//	@Success		200		{object}	map[string]string
//	@Failure		400		{object}	map[string]string
//	@Failure		401		{object}	map[string]string
//	@Router			/auth/refresh [post]
func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.writeError(w, apperrors.New(apperrors.CodeInvalidInput, "method not allowed"))
		return
	}

	var req RefreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.log.Warn("failed to decode refresh request body", zap.Error(err))
		h.writeError(w, apperrors.Wrap(apperrors.CodeInvalidInput, "invalid json body", err))
		return
	}

	accessToken, refreshToken, err := h.svc.Refresh(r.Context(), req.RefreshToken)
	if err != nil {
		h.log.Warn("refresh failed", zap.Error(err))
		h.writeError(w, err)
		return
	}

	h.log.Info("token refreshed successfully")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
	})
}

// Logout godoc
//
//	@Summary		Logout
//	@Description	Revokes the provided refresh token
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Param			body	body		LogoutRequest	true	"Logout payload"
//	@Success		200		{object}	map[string]string
//	@Failure		400		{object}	map[string]string
//	@Router			/auth/logout [post]
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.writeError(w, apperrors.New(apperrors.CodeInvalidInput, "method not allowed"))
		return
	}

	var req LogoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.log.Warn("failed to decode logout request body", zap.Error(err))
		h.writeError(w, apperrors.Wrap(apperrors.CodeInvalidInput, "invalid json body", err))
		return
	}

	err := h.svc.Logout(r.Context(), req.RefreshToken)
	if err != nil {
		h.log.Warn("logout failed", zap.Error(err))
		h.writeError(w, err)
		return
	}

	h.log.Info("user logged out successfully")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "logged out successfully",
	})
}

// GoogleOAuthInitiate godoc
//
//	@Summary		Google OAuth sign-in
//	@Description	Redirects the browser to Google's OAuth consent page. Sets a short-lived signed CSRF state cookie.
//	@Tags			auth
//	@Produce		json
//	@Success		302
//	@Router			/auth/google [get]
func (h *Handler) GoogleOAuthInitiate(w http.ResponseWriter, r *http.Request) {
	rawState, signedState, err := h.svc.GenerateOAuthState()
	if err != nil {
		h.log.Error("failed to generate oauth state", zap.Error(err))
		h.writeError(w, err)
		return
	}

	// Store signed state in an HttpOnly cookie (CSRF guard).
	http.SetCookie(w, &http.Cookie{
		Name:     oauthStateCookieName,
		Value:    signedState,
		MaxAge:   int((10 * time.Minute).Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Path:     "/",
	})

	// Store the raw state so we can verify it matches what Google echoes back.
	http.SetCookie(w, &http.Cookie{
		Name:     oauthRawCookieName,
		Value:    rawState,
		MaxAge:   int((10 * time.Minute).Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Path:     "/",
	})

	url := h.svc.GoogleOAuthURL(rawState)
	http.Redirect(w, r, url, http.StatusFound)
}

// GoogleOAuthCallback godoc
//
//	@Summary		Google OAuth callback
//	@Description	Handles the Google OAuth redirect, exchanges the code, and returns Seraph access + refresh tokens.
//	@Tags			auth
//	@Produce		json
//	@Param			code	query		string	true	"Authorization code from Google"
//	@Param			state	query		string	true	"CSRF state parameter"
//	@Success		200		{object}	map[string]string
//	@Failure		400		{object}	map[string]string
//	@Failure		401		{object}	map[string]string
//	@Router			/auth/google/callback [get]
func (h *Handler) GoogleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	queryState := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")

	if queryState == "" || code == "" {
		h.writeError(w, apperrors.New(apperrors.CodeInvalidInput, "missing state or code parameter"))
		return
	}

	// Read the signed cookie and raw state cookie.
	signedCookie, err := r.Cookie(oauthStateCookieName)
	if err != nil {
		h.writeError(w, apperrors.New(apperrors.CodeInvalidInput, "missing oauth state cookie"))
		return
	}
	rawCookie, err := r.Cookie(oauthRawCookieName)
	if err != nil {
		h.writeError(w, apperrors.New(apperrors.CodeInvalidInput, "missing oauth raw state cookie"))
		return
	}

	// The query param state must match the raw state cookie, and the signed cookie
	// must be a valid HMAC of that raw state.
	if queryState != rawCookie.Value || !h.svc.ValidateOAuthState(rawCookie.Value, signedCookie.Value) {
		h.log.Warn("oauth state mismatch - possible CSRF attempt")
		h.writeError(w, apperrors.New(apperrors.CodeUnauthorized, "invalid oauth state"))
		return
	}

	// Clear the short-lived state cookies.
	http.SetCookie(w, &http.Cookie{Name: oauthStateCookieName, MaxAge: -1, Path: "/"})
	http.SetCookie(w, &http.Cookie{Name: oauthRawCookieName, MaxAge: -1, Path: "/"})

	accessToken, refreshToken, err := h.svc.GoogleOAuthCallback(r.Context(), code)
	if err != nil {
		h.log.Warn("google oauth callback failed", zap.Error(err))
		h.writeError(w, err)
		return
	}

	h.log.Info("google oauth login successful")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
	})
}

func (h *Handler) writeError(w http.ResponseWriter, err error) {
	var appErr *apperrors.AppError
	if errors.As(err, &appErr) {
		var status int
		switch appErr.Code {
		case apperrors.CodeNotFound:
			status = http.StatusNotFound
		case apperrors.CodeConflict:
			status = http.StatusConflict
		case apperrors.CodeUnauthorized:
			status = http.StatusUnauthorized
		case apperrors.CodeInvalidInput:
			status = http.StatusBadRequest
		default:
			status = http.StatusInternalServerError
			h.log.Error("internal application error", zap.Error(err))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(map[string]string{"error": appErr.Message})
		return
	}

	h.log.Error("unexpected internal server error", zap.Error(err))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	json.NewEncoder(w).Encode(map[string]string{"error": "Internal server error"})
}

// GetUserByID godoc
//
//	@Summary		Get User by ID
//	@Description	Retrieves user details by user ID (internal endpoint for other services)
//	@Tags			auth
//	@Produce		json
//	@Param			id	path		string	true	"User ID"
//	@Success		200	{object}	repository.User
//	@Failure		400	{object}	map[string]string
//	@Failure		444	{object}	map[string]string
//	@Router			/auth/users/{id} [get]
func (h *Handler) GetUserByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.writeError(w, apperrors.New(apperrors.CodeInvalidInput, "method not allowed"))
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		h.writeError(w, apperrors.New(apperrors.CodeInvalidInput, "missing user ID"))
		return
	}

	user, err := h.svc.GetUserByID(r.Context(), id)
	if err != nil {
		h.writeError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(user)
}

// RegisterRequest defines the payload for user registration.
type RegisterRequest struct {
	Email     string `json:"email"      example:"jane@example.com"`
	Password  string `json:"password"   example:"s3cr3t"`
	FirstName string `json:"first_name" example:"Jane"`
	LastName  string `json:"last_name"  example:"Doe"`
}

// LoginRequest defines the payload for user login.
type LoginRequest struct {
	Email    string `json:"email"    example:"jane@example.com"`
	Password string `json:"password" example:"s3cr3t"`
}

// RefreshRequest defines the payload for token refresh.
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" example:"eyJhbGc..."`
}

// LogoutRequest defines the payload for logout.
type LogoutRequest struct {
	RefreshToken string `json:"refresh_token" example:"eyJhbGc..."`
}
