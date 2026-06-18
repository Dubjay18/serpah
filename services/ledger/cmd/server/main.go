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

	"github.com/Dubjay18/seraph/services/ledger/config"
	_ "github.com/Dubjay18/seraph/services/ledger/docs"
	"github.com/Dubjay18/seraph/services/ledger/internal/consumer"
	"github.com/Dubjay18/seraph/services/ledger/internal/handler"
	"github.com/Dubjay18/seraph/services/ledger/internal/repository"
	"github.com/Dubjay18/seraph/services/ledger/internal/service"
	"github.com/Dubjay18/seraph/shared/rabbitmq"
)

// @title			Seraph Ledger Service
// @version		1.0
// @description	Financial ledger service
// @host			localhost:8083
// @BasePath		/
//
//go:generate swag init -g main.go
func main() {
	// ── Logger ───────────────────────────────────────────────────────────────
	logCfg := zap.NewProductionConfig()
	logCfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	log, _ := logCfg.Build()
	log = log.With(zap.String("service", "ledger"))
	defer log.Sync()
	log.Info("starting ledger service")

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

	// ── Repository & Service ─────────────────────────────────────────────────
	repo := repository.New(dbPool)
	ledgerSvc := service.New(repo)

	rabbitURL := getEnv("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/")

	// ── RabbitMQ ─────────────────────────────────────────────────────────────
	amqpConn, err := rabbitmq.Connect(rabbitmq.Config{URL: rabbitURL})
	if err != nil {
		log.Error("failed to connect to rabbitmq", zap.Error(err))
		os.Exit(1)
	}
	defer amqpConn.Close()

	conCh, err := amqpConn.Channel()
	if err != nil {
		log.Error("failed to open consumer channel", zap.Error(err))
		os.Exit(1)
	}
	if err := rabbitmq.DeclareTopology(conCh); err != nil {
		log.Error("failed to declare rabbitmq topology", zap.Error(err))
		os.Exit(1)
	}

	consumerCtx, cancelConsumers := context.WithCancel(context.Background())
	defer cancelConsumers()

	lc := consumer.New(rabbitmq.NewConsumer(conCh), ledgerSvc, log)
	lc.Start(consumerCtx)
	log.Info("ledger event consumers started")

	h := handler.New(ledgerSvc, log)

	r := chi.NewRouter()
	r.Get("/ledger/health", h.Health)
	r.Get("/ledger/accounts/{id}/balance", h.GetBalance)
	r.Get("/ledger/accounts/{id}/entries", h.GetEntries)

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
		Addr:         fmt.Sprintf(":%d", 8083),
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

	cancelConsumers()

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
