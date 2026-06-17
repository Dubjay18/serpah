package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port        int
	DatabaseURL string
}

func Load() Config {
	port, _ := strconv.Atoi(getEnv("PAYMENTS_PORT", "8084"))
	return Config{
		Port:        port,
		DatabaseURL: getEnv("DATABASE_URL", ""),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
