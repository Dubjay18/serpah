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
	"github.com/jackc/pgx/v5/pgxpool"
	httpSwagger "github.com/swaggo/http-swagger/v2"
	swaggoSwagger "github.com/swaggo/swag"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/Dubjay18/seraph/services/accounts/config"
	_ "github.com/Dubjay18/seraph/services/accounts/docs"
	"github.com/Dubjay18/seraph/services/accounts/internal/client"
	"github.com/Dubjay18/seraph/services/accounts/internal/consumer"
	"github.com/Dubjay18/seraph/services/accounts/internal/handler"
	"github.com/Dubjay18/seraph/services/accounts/internal/repository"
	"github.com/Dubjay18/seraph/services/accounts/internal/service"
	"github.com/Dubjay18/seraph/shared/rabbitmq"
)

// @title			Seraph Accounts Service
// @version		1.0
// @description	Accounts management service
// @host			localhost:8082
// @BasePath		/
//
//go:generate swag init -g main.go
func main() {
	// ── Logger ───────────────────────────────────────────────────────────────
	logCfg := zap.NewProductionConfig()
	logCfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	log, _ := logCfg.Build()
	log = log.With(zap.String("service", "accounts"))
	defer log.Sync()
	log.Info("starting accounts service")

	cfg := config.Load()

	// ── Database ─────────────────────────────────────────────────────────────
	dbCtx, dbCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer dbCancel()

	dbPool, err := pgxpool.New(dbCtx, cfg.DatabaseURL)
	if err != nil {
		log.Error("failed to create database pool", zap.Error(err))
		os.Exit(1)
	}
	defer dbPool.Close()

	if err := dbPool.Ping(dbCtx); err != nil {
		log.Error("failed to ping database", zap.Error(err))
		os.Exit(1)
	}
	log.Info("connected to postgres database pool")
	// ── Repository, Client & service setup ───────────────────────────────────
	repo := repository.New(dbPool)
	authCl := client.NewAuthClient(cfg.AuthServiceURL)
	ledgerCl := client.NewLedgerClient(cfg.LedgerServiceURL)

	rabbitURL := getEnv("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/")

	// ── RabbitMQ ─────────────────────────────────────────────────────────────
	amqpConn, err := rabbitmq.Connect(rabbitmq.Config{URL: rabbitURL})
	if err != nil {
		log.Error("failed to connect to rabbitmq", zap.Error(err))
		os.Exit(1)
	}
	defer amqpConn.Close()

	// Consumer channel
	conCh, err := amqpConn.Channel()
	if err != nil {
		log.Error("failed to open consumer channel", zap.Error(err))
		os.Exit(1)
	}

	// Publisher channel
	pubCh, err := amqpConn.Channel()
	if err != nil {
		log.Error("failed to open publisher channel", zap.Error(err))
		os.Exit(1)
	}
	publisher := rabbitmq.NewPublisher(pubCh)

	// Topology may already be declared by auth/payments; idempotent call is safe.
	if err := rabbitmq.DeclareTopology(conCh); err != nil {
		log.Error("failed to declare rabbitmq topology", zap.Error(err))
		os.Exit(1)
	}

	accountsSvc := service.New(repo, authCl, ledgerCl, publisher)

	consumerCtx, cancelConsumers := context.WithCancel(context.Background())
	defer cancelConsumers()

	ac := consumer.New(rabbitmq.NewConsumer(conCh), log)
	ac.Start(consumerCtx)
	log.Info("accounts event consumers started")

	h := handler.New(accountsSvc, log)

	r := chi.NewRouter()
	r.Get("/accounts/health", h.Health)
	r.Post("/accounts", h.CreateAccount)

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

	r.Get("/swagger/*", httpSwagger.Handler(
		httpSwagger.URL("/openapi.json"),
	))

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", 8082),
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("server error", zap.Error(err))
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	cancelConsumers() // stop RabbitMQ consumers gracefully before HTTP shutdown

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Error("shutdown error", zap.Error(err))
	}
	log.Info("shutdown complete")
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

