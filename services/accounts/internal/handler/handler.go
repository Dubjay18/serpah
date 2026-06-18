package handler

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/Dubjay18/seraph/services/accounts/internal/dto"
	"github.com/Dubjay18/seraph/services/accounts/internal/repository"
	"github.com/Dubjay18/seraph/services/accounts/internal/service"
	apperrors "github.com/Dubjay18/seraph/shared/errors"
	"github.com/Dubjay18/seraph/shared/middleware"
	"github.com/Dubjay18/seraph/shared/money"
)

var rxUUID = regexp.MustCompile("^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}$")

func isValidUUID(uuid string) bool {
	return rxUUID.MatchString(uuid)
}

// Handler holds HTTP dependencies for the accounts domain.
type Handler struct {
	svc *service.AccountsService
	log *zap.Logger
}

func New(svc *service.AccountsService, log *zap.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

// ─── Health ───────────────────────────────────────────────────────────────────

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

// ─── POST /accounts ───────────────────────────────────────────────────────────

// CreateAccount godoc
//
//	@Summary		Create account
//	@Description	Creates a new financial account for the authenticated user. Owner is taken from the JWT subject — do not pass owner_id in the body.
//	@Tags			accounts
//	@Accept			json
//	@Produce		json
//	@Param			body	body		dto.CreateAccountRequest	true	"Account creation payload"
//	@Success		201		{object}	dto.AccountResponse
//	@Failure		400		{object}	map[string]string
//	@Failure		401		{object}	map[string]string
//	@Failure		404		{object}	map[string]string
//	@Router			/accounts [post]
func (h *Handler) CreateAccount(w http.ResponseWriter, r *http.Request) {
	callerID, err := middleware.UserIDFromContext(r.Context())
	if err != nil {
		h.writeError(w, apperrors.New(apperrors.CodeUnauthorized, "unauthenticated"))
		return
	}

	var req dto.CreateAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.log.Warn("failed to decode create account request", zap.Error(err))
		h.writeError(w, apperrors.Wrap(apperrors.CodeInvalidInput, "invalid json body", err))
		return
	}

	if err := req.Validate(); err != nil {
		h.log.Warn("invalid create account request", zap.Error(err))
		h.writeError(w, apperrors.New(apperrors.CodeInvalidInput, err.Error()))
		return
	}

	acc, err := h.svc.CreateAccount(r.Context(), callerID, repository.AccountType(req.AccountType), money.Currency(req.Currency))
	if err != nil {
		h.log.Warn("account creation failed", zap.String("owner_id", callerID), zap.Error(err))
		h.writeError(w, err)
		return
	}

	h.log.Info("account created", zap.String("account_id", acc.ID), zap.String("account_number", acc.AccountNumber))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(accountToDTO(acc, nil))
}

// ─── GET /accounts/:id ───────────────────────────────────────────────────────

// GetAccount godoc
//
//	@Summary		Get account
//	@Description	Returns account details and live balance from the ledger service. The caller must own the account.
//	@Tags			accounts
//	@Produce		json
//	@Param			id	path		string	true	"Account UUID"
//	@Success		200	{object}	dto.AccountResponse
//	@Failure		401	{object}	map[string]string
//	@Failure		404	{object}	map[string]string
//	@Router			/accounts/{id} [get]
func (h *Handler) GetAccount(w http.ResponseWriter, r *http.Request) {
	callerID, err := middleware.UserIDFromContext(r.Context())
	if err != nil {
		h.writeError(w, apperrors.New(apperrors.CodeUnauthorized, "unauthenticated"))
		return
	}

	accountID := chi.URLParam(r, "id")
	if accountID == "" {
		h.writeError(w, apperrors.New(apperrors.CodeInvalidInput, "account id is required"))
		return
	}
	if !isValidUUID(accountID) {
		h.writeError(w, apperrors.New(apperrors.CodeInvalidInput, "invalid account id format: must be a UUID"))
		return
	}

	acc, balance, err := h.svc.GetAccountWithBalance(r.Context(), accountID, callerID)
	if err != nil {
		h.log.Warn("get account failed", zap.String("account_id", accountID), zap.Error(err))
		h.writeError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(accountToDTO(acc, &balance))
}

// ─── GET /accounts ────────────────────────────────────────────────────────────

// ListAccounts godoc
//
//	@Summary		List accounts
//	@Description	Returns a cursor-paginated list of all accounts belonging to the authenticated user.
//	@Tags			accounts
//	@Produce		json
//	@Param			cursor	query		string	false	"Opaque pagination cursor from previous response"
//	@Param			limit	query		int		false	"Page size (default 20, max 100)"
//	@Success		200		{object}	dto.ListAccountsResponse
//	@Failure		401		{object}	map[string]string
//	@Router			/accounts [get]
func (h *Handler) ListAccounts(w http.ResponseWriter, r *http.Request) {
	callerID, err := middleware.UserIDFromContext(r.Context())
	if err != nil {
		h.writeError(w, apperrors.New(apperrors.CodeUnauthorized, "unauthenticated"))
		return
	}

	cursor := r.URL.Query().Get("cursor")
	if cursor != "" {
		if _, err := base64.RawURLEncoding.DecodeString(cursor); err != nil {
			h.writeError(w, apperrors.New(apperrors.CodeInvalidInput, "invalid pagination cursor"))
			return
		}
	}

	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		parsed, parseErr := strconv.Atoi(l)
		if parseErr != nil || parsed <= 0 {
			h.writeError(w, apperrors.New(apperrors.CodeInvalidInput, "limit must be a positive integer"))
			return
		}
		if parsed > 100 {
			parsed = 100
		}
		limit = parsed
	}

	accounts, hasMore, err := h.svc.ListAccountsCursor(r.Context(), callerID, cursor, limit)
	if err != nil {
		h.log.Warn("list accounts failed", zap.String("owner_id", callerID), zap.Error(err))
		h.writeError(w, err)
		return
	}

	resp := dto.ListAccountsResponse{
		Data:    make([]dto.AccountResponse, 0, len(accounts)),
		HasMore: hasMore,
	}

	for _, a := range accounts {
		resp.Data = append(resp.Data, accountToDTO(&a, nil))
	}

	// Build next cursor from the last item.
	if hasMore && len(accounts) > 0 {
		last := accounts[len(accounts)-1]
		resp.NextCursor = repository.EncodeCursor(last.CreatedAt, last.ID)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// ─── DELETE /accounts/:id ────────────────────────────────────────────────────

// CloseAccount godoc
//
//	@Summary		Close account
//	@Description	Closes an account. The caller must own the account and the balance must be zero.
//	@Tags			accounts
//	@Produce		json
//	@Param			id	path	string	true	"Account UUID"
//	@Success		204
//	@Failure		400	{object}	map[string]string
//	@Failure		401	{object}	map[string]string
//	@Failure		404	{object}	map[string]string
//	@Router			/accounts/{id} [delete]
func (h *Handler) CloseAccount(w http.ResponseWriter, r *http.Request) {
	callerID, err := middleware.UserIDFromContext(r.Context())
	if err != nil {
		h.writeError(w, apperrors.New(apperrors.CodeUnauthorized, "unauthenticated"))
		return
	}

	accountID := chi.URLParam(r, "id")
	if accountID == "" {
		h.writeError(w, apperrors.New(apperrors.CodeInvalidInput, "account id is required"))
		return
	}
	if !isValidUUID(accountID) {
		h.writeError(w, apperrors.New(apperrors.CodeInvalidInput, "invalid account id format: must be a UUID"))
		return
	}

	if err := h.svc.CloseAccount(r.Context(), accountID, callerID); err != nil {
		h.log.Warn("close account failed", zap.String("account_id", accountID), zap.Error(err))
		h.writeError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ─── GET /accounts/:id/statement ─────────────────────────────────────────────

// GetStatement godoc
//
//	@Summary		Account statement
//	@Description	Returns paginated ledger entries for an account. Proxies to the ledger service. The caller must own the account.
//	@Tags			accounts
//	@Produce		json
//	@Param			id		path		string	true	"Account UUID"
//	@Param			from	query		string	false	"Start of date range (RFC3339)"
//	@Param			to		query		string	false	"End of date range (RFC3339)"
//	@Param			cursor	query		string	false	"Opaque pagination cursor"
//	@Param			limit	query		int		false	"Page size (default 20, max 100)"
//	@Success		200		{object}	dto.StatementResponse
//	@Failure		400		{object}	map[string]string
//	@Failure		401		{object}	map[string]string
//	@Failure		404		{object}	map[string]string
//	@Router			/accounts/{id}/statement [get]
func (h *Handler) GetStatement(w http.ResponseWriter, r *http.Request) {
	callerID, err := middleware.UserIDFromContext(r.Context())
	if err != nil {
		h.writeError(w, apperrors.New(apperrors.CodeUnauthorized, "unauthenticated"))
		return
	}

	accountID := chi.URLParam(r, "id")
	if accountID == "" {
		h.writeError(w, apperrors.New(apperrors.CodeInvalidInput, "account id is required"))
		return
	}
	if !isValidUUID(accountID) {
		h.writeError(w, apperrors.New(apperrors.CodeInvalidInput, "invalid account id format: must be a UUID"))
		return
	}

	// Parse optional date range.
	var from, to *time.Time
	if s := r.URL.Query().Get("from"); s != "" {
		t, parseErr := time.Parse(time.RFC3339, s)
		if parseErr != nil {
			h.writeError(w, apperrors.New(apperrors.CodeInvalidInput, "invalid 'from' date: use RFC3339 format"))
			return
		}
		from = &t
	}
	if s := r.URL.Query().Get("to"); s != "" {
		t, parseErr := time.Parse(time.RFC3339, s)
		if parseErr != nil {
			h.writeError(w, apperrors.New(apperrors.CodeInvalidInput, "invalid 'to' date: use RFC3339 format"))
			return
		}
		to = &t
	}

	if from != nil && to != nil && from.After(*to) {
		h.writeError(w, apperrors.New(apperrors.CodeInvalidInput, "'from' date cannot be after 'to' date"))
		return
	}

	cursor := r.URL.Query().Get("cursor")
	if cursor != "" {
		if _, err := base64.RawURLEncoding.DecodeString(cursor); err != nil {
			h.writeError(w, apperrors.New(apperrors.CodeInvalidInput, "invalid pagination cursor"))
			return
		}
	}

	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		parsed, parseErr := strconv.Atoi(l)
		if parseErr != nil || parsed <= 0 {
			h.writeError(w, apperrors.New(apperrors.CodeInvalidInput, "limit must be a positive integer"))
			return
		}
		if parsed > 100 {
			parsed = 100
		}
		limit = parsed
	}

	entries, nextCursor, err := h.svc.GetStatement(r.Context(), accountID, callerID, from, to, cursor, limit)
	if err != nil {
		h.log.Warn("get statement failed", zap.String("account_id", accountID), zap.Error(err))
		h.writeError(w, err)
		return
	}

	resp := dto.StatementResponse{
		Data:       entries,
		NextCursor: nextCursor,
		HasMore:    nextCursor != "",
	}
	if resp.Data == nil {
		resp.Data = []dto.LedgerEntryResponse{}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// accountToDTO converts a repository.Account to dto.AccountResponse.
// balance is optional — pass nil on list endpoints to omit the field.
func accountToDTO(a *repository.Account, balance *money.Money) dto.AccountResponse {
	resp := dto.AccountResponse{
		ID:            a.ID,
		OwnerID:       a.OwnerID,
		AccountNumber: a.AccountNumber,
		AccountType:   string(a.AccountType),
		Currency:      string(a.Currency),
		Status:        string(a.Status),
		Balance:       balance,
		CreatedAt:     a.CreatedAt,
		ClosedAt:      a.ClosedAt,
	}
	return resp
}

// writeError maps AppError codes to HTTP status codes and writes a JSON body.
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
		case apperrors.CodeInsufficientFunds:
			status = http.StatusUnprocessableEntity
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
	json.NewEncoder(w).Encode(map[string]string{"error": "internal server error"})
}
