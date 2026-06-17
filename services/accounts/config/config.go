package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Port           int
	DatabaseURL    string
	AuthServiceURL string
	LedgerServiceURL string
}

func Load() Config {
	port, _ := strconv.Atoi(getEnv("ACCOUNTS_PORT", "8082"))

	dbHost := getEnv("POSTGRES_HOST", "localhost")
	dbPort := getEnv("POSTGRES_PORT", "5432")
	dbUser := getEnv("POSTGRES_USER", "seraph")
	dbPass := getEnv("POSTGRES_PASSWORD", "seraph_dev")
	dbName := getEnv("POSTGRES_DB", "seraph")

	defaultDBURL := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", dbUser, dbPass, dbHost, dbPort, dbName)

	return Config{
		Port:           port,
		DatabaseURL:    getEnv("DATABASE_URL", defaultDBURL),
		AuthServiceURL: getEnv("AUTH_SERVICE_URL", "http://localhost:8081"),
		LedgerServiceURL: getEnv("LEDGER_SERVICE_URL", "http://localhost:8083"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
