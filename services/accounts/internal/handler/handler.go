package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"go.uber.org/zap"

	"github.com/Dubjay18/seraph/services/accounts/internal/repository"
	"github.com/Dubjay18/seraph/services/accounts/internal/service"
	apperrors "github.com/Dubjay18/seraph/shared/errors"
	"github.com/Dubjay18/seraph/shared/money"
)

type Handler struct {
	svc *service.AccountsService
	log *zap.Logger
}

func New(svc *service.AccountsService, log *zap.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

// Health godoc
//
//	@Summary		Health check
//	@Description	Returns the health status of the accounts service
//	@Tags			accounts
//	@Produce		json
//	@Success		200	{object}	map[string]string
//	@Router			/accounts/health [get]
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok","service":"accounts"}`))
}

type CreateAccountRequest struct {
	OwnerID     string                 `json:"owner_id"`
	AccountType repository.AccountType `json:"account_type"`
	Currency    string                 `json:"currency"`
}

// CreateAccount godoc
//
//	@Summary		Create Account
//	@Description	Creates a new financial account for a user (validates owner existence with auth service)
//	@Tags			accounts
//	@Accept			json
//	@Produce		json
//	@Param			body	body		CreateAccountRequest	true	"Account creation payload"
//	@Success		201		{object}	repository.Account
//	@Failure		400		{object}	map[string]string
//	@Failure		404		{object}	map[string]string
//	@Router			/accounts [post]
func (h *Handler) CreateAccount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.writeError(w, apperrors.New(apperrors.CodeInvalidInput, "method not allowed"))
		return
	}

	var req CreateAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.log.Warn("failed to decode create account request", zap.Error(err))
		h.writeError(w, apperrors.Wrap(apperrors.CodeInvalidInput, "invalid json body", err))
		return
	}

	acc, err := h.svc.CreateAccount(r.Context(), req.OwnerID, req.AccountType, money.Currency(req.Currency))
	if err != nil {
		h.log.Warn("account creation failed", zap.String("owner_id", req.OwnerID), zap.Error(err))
		h.writeError(w, err)
		return
	}

	h.log.Info("account created successfully", zap.String("account_id", acc.ID), zap.String("account_number", acc.AccountNumber))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(acc)
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
