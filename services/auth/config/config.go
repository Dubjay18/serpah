package config

import (
	"os"
	"strconv"
	"time"

	"github.com/Dubjay18/seraph/shared/postgres"
)

type Config struct {
	Port              int
	Postgres          postgres.Config
	RabbitMQURL       string
	JWTPrivateKeyPath string
	JWTPublicKeyPath  string
	JWTAccessExpiry   time.Duration
	JWTRefreshExpiry  time.Duration

	// Google OAuth 2.0
	GoogleClientID         string
	GoogleClientSecret     string
	GoogleCallbackURL      string
	OAuthStateCookieSecret string
}

func Load() Config {
	port, _ := strconv.Atoi(getEnv("AUTH_PORT", "8081"))
	dbPort, _ := strconv.Atoi(getEnv("POSTGRES_PORT", "5432"))

	accessExpiry, err := time.ParseDuration(getEnv("JWT_ACCESS_EXPIRY", "15m"))
	if err != nil {
		accessExpiry = 15 * time.Minute
	}

	refreshExpiry, err := time.ParseDuration(getEnv("JWT_REFRESH_EXPIRY", "168h"))
	if err != nil {
		refreshExpiry = 168 * time.Hour
	}

	return Config{
		Port: port,
		Postgres: postgres.Config{
			Host:     getEnv("POSTGRES_HOST", "localhost"),
			Port:     dbPort,
			User:     getEnv("POSTGRES_USER", "seraph"),
			Password: getEnv("POSTGRES_PASSWORD", "seraph_dev"),
			DBName:   getEnv("POSTGRES_DB", "seraph"),
		},
		JWTPrivateKeyPath: getEnv("JWT_PRIVATE_KEY_PATH", "./keys/private.pem"),
		JWTPublicKeyPath:  getEnv("JWT_PUBLIC_KEY_PATH", "./keys/public.pem"),
		JWTAccessExpiry:   accessExpiry,
		JWTRefreshExpiry:  refreshExpiry,
		RabbitMQURL:       getEnv("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/"),

		GoogleClientID:         getEnv("GOOGLE_CLIENT_ID", ""),
		GoogleClientSecret:     getEnv("GOOGLE_CLIENT_SECRET", ""),
		GoogleCallbackURL:      getEnv("GOOGLE_CALLBACK_URL", "http://localhost:8081/auth/google/callback"),
		OAuthStateCookieSecret: getEnv("OAUTH_STATE_SECRET", ""),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
