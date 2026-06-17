package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Port        int
	DatabaseURL string
}

func Load() Config {
	port, _ := strconv.Atoi(getEnv("LEDGER_PORT", "8083"))

	dbHost := getEnv("POSTGRES_HOST", "localhost")
	dbPort := getEnv("POSTGRES_PORT", "5432")
	dbUser := getEnv("POSTGRES_USER", "seraph")
	dbPass := getEnv("POSTGRES_PASSWORD", "seraph_dev")
	dbName := getEnv("POSTGRES_DB", "seraph")

	defaultURL := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", dbUser, dbPass, dbHost, dbPort, dbName)

	return Config{
		Port:        port,
		DatabaseURL: getEnv("DATABASE_URL", defaultURL),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
