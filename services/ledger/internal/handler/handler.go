package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/Dubjay18/seraph/services/ledger/internal/service"
	apperrors "github.com/Dubjay18/seraph/shared/errors"
)

// Handler holds HTTP dependencies for the ledger domain.
type Handler struct {
	svc *service.LedgerService
	log *zap.Logger
}

func New(svc *service.LedgerService, log *zap.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

// Health godoc
//
//	@Summary		Health check
//	@Description	Returns the health status of the ledger service
//	@Tags			ledger
//	@Produce		json
//	@Success		200	{object}	map[string]string
//	@Router			/ledger/health [get]
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok","service":"ledger"}`))
}

// GetBalance godoc
//
//	@Summary		Get account balance
//	@Description	Returns the live balance for an account (most recently active currency).
//	@Tags			ledger
//	@Produce		json
//	@Param			id	path		string	true	"Account UUID"
//	@Success		200	{object}	map[string]interface{}
//	@Failure		404	{object}	map[string]string
//	@Router			/ledger/accounts/{id}/balance [get]
func (h *Handler) GetBalance(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "id")
	if accountID == "" {
		h.writeError(w, apperrors.New(apperrors.CodeInvalidInput, "account id is required"), http.StatusBadRequest)
		return
	}

	balance, err := h.svc.GetBalanceAny(r.Context(), accountID)
	if err != nil {
		h.log.Warn("get balance failed", zap.String("account_id", accountID), zap.Error(err))
		h.writeError(w, err, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"amount":   balance.Amount,
		"currency": string(balance.Currency),
	})
}

// GetEntries godoc
//
//	@Summary		Get ledger entries
//	@Description	Returns cursor-paginated ledger entries for an account. Supports optional date range filter.
//	@Tags			ledger
//	@Produce		json
//	@Param			id		path		string	true	"Account UUID"
//	@Param			from	query		string	false	"Start date (RFC3339)"
//	@Param			to		query		string	false	"End date (RFC3339)"
//	@Param			cursor	query		string	false	"Pagination cursor"
//	@Param			limit	query		int		false	"Page size (default 20, max 100)"
//	@Success		200		{object}	map[string]interface{}
//	@Failure		400		{object}	map[string]string
//	@Router			/ledger/accounts/{id}/entries [get]
func (h *Handler) GetEntries(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "id")
	if accountID == "" {
		h.writeError(w, apperrors.New(apperrors.CodeInvalidInput, "account id is required"), http.StatusBadRequest)
		return
	}

	// Parse optional date range.
	var from, to *time.Time
	if s := r.URL.Query().Get("from"); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			h.writeError(w, apperrors.New(apperrors.CodeInvalidInput, "invalid 'from' date: use RFC3339 format"), http.StatusBadRequest)
			return
		}
		from = &t
	}
	if s := r.URL.Query().Get("to"); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			h.writeError(w, apperrors.New(apperrors.CodeInvalidInput, "invalid 'to' date: use RFC3339 format"), http.StatusBadRequest)
			return
		}
		to = &t
	}

	cursor := r.URL.Query().Get("cursor")

	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		parsed, err := strconv.Atoi(l)
		if err != nil || parsed <= 0 {
			h.writeError(w, apperrors.New(apperrors.CodeInvalidInput, "limit must be a positive integer"), http.StatusBadRequest)
			return
		}
		if parsed > 100 {
			parsed = 100
		}
		limit = parsed
	}

	entries, nextCursor, err := h.svc.GetEntries(r.Context(), accountID, from, to, cursor, limit)
	if err != nil {
		h.log.Warn("get entries failed", zap.String("account_id", accountID), zap.Error(err))
		h.writeError(w, err, http.StatusInternalServerError)
		return
	}

	// Map service.LedgerEntry to a JSON-friendly shape.
	type entryJSON struct {
		ID            string    `json:"id"`
		TransactionID string    `json:"transaction_id"`
		EntryType     string    `json:"entry_type"`
		Amount        int64     `json:"amount"`
		Currency      string    `json:"currency"`
		CreatedAt     time.Time `json:"created_at"`
	}

	data := make([]entryJSON, 0, len(entries))
	for _, e := range entries {
		data = append(data, entryJSON{
			ID:            e.ID,
			TransactionID: e.TransactionID,
			EntryType:     e.EntryType,
			Amount:        e.Amount,
			Currency:      e.Currency,
			CreatedAt:     e.CreatedAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"data":        data,
		"next_cursor": nextCursor,
		"has_more":    nextCursor != "",
	})
}

// writeError writes a JSON error response with the given status code.
func (h *Handler) writeError(w http.ResponseWriter, err error, status int) {
	msg := "internal server error"
	if err != nil {
		msg = err.Error()
		// If it's an AppError, surface just the message.
		var appErr *apperrors.AppError
		if asErr, ok := err.(*apperrors.AppError); ok {
			msg = asErr.Message
		}
		_ = appErr
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
