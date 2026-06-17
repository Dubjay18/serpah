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
	httpSwagger "github.com/swaggo/http-swagger/v2"
	swaggoSwagger "github.com/swaggo/swag"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	_ "github.com/Dubjay18/seraph/services/payments/docs"
	"github.com/Dubjay18/seraph/services/payments/internal/handler"
	"github.com/Dubjay18/seraph/services/payments/internal/service"
	"github.com/Dubjay18/seraph/shared/rabbitmq"
)

// @title			Seraph Payments Service
// @version		1.0
// @description	Payments processing service
// @host			localhost:8084
// @BasePath		/
func main() {
	// ── Logger ───────────────────────────────────────────────────────────────
	cfg := zap.NewProductionConfig()
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	log, _ := cfg.Build()
	log = log.With(zap.String("service", "payments"))
	defer log.Sync()
	log.Info("starting payments service", zap.Int("port", 8084))

	rabbitURL := getEnv("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/")

	// ── RabbitMQ ─────────────────────────────────────────────────────────────
	amqpConn, err := rabbitmq.Connect(rabbitmq.Config{URL: rabbitURL})
	if err != nil {
		log.Error("failed to connect to rabbitmq", zap.Error(err))
		os.Exit(1)
	}
	defer amqpConn.Close()

	pubCh, err := amqpConn.Channel()
	if err != nil {
		log.Error("failed to open publisher channel", zap.Error(err))
		os.Exit(1)
	}
	if err := rabbitmq.DeclareTopology(pubCh); err != nil {
		log.Error("failed to declare rabbitmq topology", zap.Error(err))
		os.Exit(1)
	}
	publisher := rabbitmq.NewPublisher(pubCh)

	svc := service.New(publisher, log)
	_ = svc // handler will receive svc once handler is wired

	h := handler.New()

	r := chi.NewRouter()
	r.Get("/payments/health", h.Health)

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
		Addr:         fmt.Sprintf(":%d", 8084),
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

