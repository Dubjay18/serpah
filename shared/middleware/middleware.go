package middleware

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// contextKey is an unexported type for context keys defined in this package.
// Using a distinct type prevents collisions with keys from other packages.
type contextKey string

// ContextKeyUserID is the context key under which the authenticated user's ID
// (JWT subject) is stored by any JWT-validating middleware or gateway.
const ContextKeyUserID contextKey = "user_id"

// WithUserID returns a new context that carries the given userID.
func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, ContextKeyUserID, userID)
}

// UserIDFromContext extracts the authenticated user ID from ctx.
// Returns an error if the key is absent or holds an unexpected type.
func UserIDFromContext(ctx context.Context) (string, error) {
	v := ctx.Value(ContextKeyUserID)
	if v == nil {
		return "", fmt.Errorf("middleware: user_id not found in context")
	}
	id, ok := v.(string)
	if !ok || id == "" {
		return "", fmt.Errorf("middleware: user_id in context is not a valid string")
	}
	return id, nil
}

// RequestID injects a unique X-Request-ID header into every request.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = uuid.New().String()
		}
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r)
	})
}

// Logger logs every request with method, path, status, and duration.
func Logger(log *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rw, r)
			log.Info("request",
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.Int("status", rw.status),
				zap.Int64("duration_ms", time.Since(start).Milliseconds()),
				zap.String("request_id", w.Header().Get("X-Request-ID")),
			)
		})
	}
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}
