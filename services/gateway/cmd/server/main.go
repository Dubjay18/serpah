package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	httpSwagger "github.com/swaggo/http-swagger/v2"
	"go.uber.org/zap"

	"github.com/Dubjay18/seraph/services/gateway/config"
	"github.com/Dubjay18/seraph/services/gateway/internal/handler"
	"github.com/Dubjay18/seraph/shared/logger"
	"github.com/Dubjay18/seraph/shared/middleware"
)

// @title			Seraph API Gateway
// @version		1.0
// @description	Aggregated API documentation for all Seraph microservices
// @host			localhost:8080
// @BasePath		/
func main() {
	log := logger.New("gateway")
	defer func() {
		_ = log.Sync()
	}()
	log.Info("initializing gateway service")

	cfg := config.Load()

	// Parse Auth Service URL
	authURL, err := url.Parse(cfg.AuthServiceURL)
	if err != nil {
		log.Error("failed to parse auth service URL", zap.String("url", cfg.AuthServiceURL), zap.Error(err))
		os.Exit(1)
	}
	log.Info("configured auth service target", zap.String("url", authURL.String()))

	// Build upstream spec list for aggregation
	upstreams := []handler.ServiceSpec{
		{Name: "auth", URL: cfg.AuthServiceURL},
		{Name: "accounts", URL: cfg.AccountsServiceURL},
		{Name: "ledger", URL: cfg.LedgerServiceURL},
		{Name: "payments", URL: cfg.PaymentsServiceURL},
	}

	// Wire up handler
	h := handler.New(authURL, log, upstreams)

	// Set up Chi router
	r := chi.NewRouter()

	// Apply standard middlewares
	r.Use(middleware.RequestID)
	r.Use(middleware.Logger(log))

	// --- Service routes ---
	r.Get("/gateway/health", h.Health)
	r.HandleFunc("/auth/*", h.ProxyAuth)

	// --- Swagger / Docs routes ---
	// Aggregated OpenAPI JSON (fetched fresh from each upstream on each request)
	r.Get("/docs/openapi.json", h.AggregatedSpec)

	// Swagger UI — point it at the aggregated spec
	r.Get("/docs", http.RedirectHandler("/docs/", http.StatusMovedPermanently).ServeHTTP)
	r.Get("/docs/*", httpSwagger.Handler(
		httpSwagger.URL("/docs/openapi.json"),
		httpSwagger.DeepLinking(true),
		httpSwagger.DocExpansion("list"),
	))

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Info("starting gateway server", zap.Int("port", cfg.Port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("server error", zap.Error(err))
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("shutting down gateway server gracefully")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("shutdown error", zap.Error(err))
	}

	log.Info("shutdown complete")
}
