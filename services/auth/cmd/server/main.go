package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	httpSwagger "github.com/swaggo/http-swagger/v2"
	swaggoSwagger "github.com/swaggo/swag"
	"go.uber.org/zap"

	"github.com/Dubjay18/seraph/services/auth/config"
	_ "github.com/Dubjay18/seraph/services/auth/docs"
	"github.com/Dubjay18/seraph/services/auth/internal/handler"
	"github.com/Dubjay18/seraph/services/auth/internal/repository"
	"github.com/Dubjay18/seraph/services/auth/internal/service"
	"github.com/Dubjay18/seraph/shared/logger"
	"github.com/Dubjay18/seraph/shared/middleware"
	"github.com/Dubjay18/seraph/shared/postgres"
	"github.com/Dubjay18/seraph/shared/rabbitmq"
)

// @title			Seraph Auth Service
// @version		1.0
// @description	Authentication and authorization service
// @host			localhost:8081
// @BasePath		/
func main() {
	log := logger.New("auth")
	defer log.Sync()
	log.Info("initializing auth service")

	cfg := config.Load()

	privateKeyPEM, err := os.ReadFile(cfg.JWTPrivateKeyPath)
	if err != nil {
		log.Error("failed to read private key file", zap.String("path", cfg.JWTPrivateKeyPath), zap.Error(err))
		os.Exit(1)
	}
	privateKey, err := jwt.ParseRSAPrivateKeyFromPEM(privateKeyPEM)
	if err != nil {
		log.Error("failed to parse private key", zap.Error(err))
		os.Exit(1)
	}

	publicKeyPEM, err := os.ReadFile(cfg.JWTPublicKeyPath)
	if err != nil {
		log.Error("failed to read public key file", zap.String("path", cfg.JWTPublicKeyPath), zap.Error(err))
		os.Exit(1)
	}
	publicKey, err := jwt.ParseRSAPublicKeyFromPEM(publicKeyPEM)
	if err != nil {
		log.Error("failed to parse public key", zap.Error(err))
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	dbPool, err := postgres.Connect(ctx, cfg.Postgres)
	if err != nil {
		log.Error("failed to connect to database", zap.Error(err))
		os.Exit(1)
	}
	defer dbPool.Close()
	log.Info("connected to postgres database pool")

	// ── RabbitMQ ─────────────────────────────────────────────────────────────
	ramqpConn, err := rabbitmq.Connect(rabbitmq.Config{URL: cfg.RabbitMQURL})
	if err != nil {
		log.Error("failed to connect to rabbitmq", zap.Error(err))
		os.Exit(1)
	}
	defer ramqpConn.Close()
	log.Info("connected to rabbitmq")

	pubCh, err := ramqpConn.Channel()
	if err != nil {
		log.Error("failed to open rabbitmq publisher channel", zap.Error(err))
		os.Exit(1)
	}
	if err := rabbitmq.DeclareTopology(pubCh); err != nil {
		log.Error("failed to declare rabbitmq topology", zap.Error(err))
		os.Exit(1)
	}
	publisher := rabbitmq.NewPublisher(pubCh)

	repo := repository.New(dbPool, log)
	svc := service.New(
		repo,
		privateKey, publicKey,
		cfg.JWTAccessExpiry, cfg.JWTRefreshExpiry,
		log,
		publisher,
		cfg.GoogleClientID, cfg.GoogleClientSecret, cfg.GoogleCallbackURL,
		[]byte(cfg.OAuthStateCookieSecret),
	)
	h := handler.New(svc, log)

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Logger(log))

	// Service routes
	r.Post("/auth/register", h.Register)
	r.Post("/auth/login", h.Login)
	r.Post("/auth/refresh", h.Refresh)
	r.Post("/auth/logout", h.Logout)
	r.Get("/auth/health", h.Health)
	r.Get("/auth/users/{id}", h.GetUserByID)

	// Google OAuth routes
	r.Get("/auth/google", h.GoogleOAuthInitiate)
	r.Get("/auth/google/callback", h.GoogleOAuthCallback)

	// OpenAPI spec endpoint — consumed by the gateway aggregator
	r.Get("/openapi.json", func(w http.ResponseWriter, r *http.Request) {
		doc, err := swaggoSwagger.ReadDoc()
		if err != nil {
			http.Error(w, "spec not available", http.StatusInternalServerError)
			return
		}
		var spec map[string]interface{}
		if err := json.Unmarshal([]byte(doc), &spec); err != nil {
			http.Error(w, "failed to parse spec", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(spec)
	})

	// Local Swagger UI (useful in dev)
	r.Get("/swagger/*", httpSwagger.Handler(
		httpSwagger.URL("/openapi.json"),
	))

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Info("starting server", zap.Int("port", cfg.Port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("server error", zap.Error(err))
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("shutting down server gracefully")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("shutdown error", zap.Error(err))
	}

	log.Info("shutdown complete")
}
