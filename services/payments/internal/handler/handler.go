package handler

import "net/http"

type Handler struct{}

func New() *Handler { return &Handler{} }

// Health godoc
//
//	@Summary		Health check
//	@Description	Returns the health status of the payments service
//	@Tags			payments
//	@Produce		json
//	@Success		200	{object}	map[string]string
//	@Router			/payments/health [get]
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok","service":"payments"}`))
}
