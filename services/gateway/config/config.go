package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port               int
	DatabaseURL        string
	AuthServiceURL     string
	AccountsServiceURL string
	LedgerServiceURL   string
	PaymentsServiceURL string
}

func Load() Config {
	port, _ := strconv.Atoi(getEnv("GATEWAY_PORT", "8080"))
	return Config{
		Port:               port,
		DatabaseURL:        getEnv("DATABASE_URL", ""),
		AuthServiceURL:     getEnv("AUTH_SERVICE_URL", "http://localhost:8081"),
		AccountsServiceURL: getEnv("ACCOUNTS_SERVICE_URL", "http://localhost:8082"),
		LedgerServiceURL:   getEnv("LEDGER_SERVICE_URL", "http://localhost:8083"),
		PaymentsServiceURL: getEnv("PAYMENTS_SERVICE_URL", "http://localhost:8084"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
