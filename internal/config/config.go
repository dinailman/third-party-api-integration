package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	HTTPAddr       string
	DatabaseURL    string
	RedisAddr      string
	QueueName      string
	WorkerCount    int
	ShutdownPeriod time.Duration
}

func Load() Config {
	return Config{
		HTTPAddr:       env("HTTP_ADDR", ":8084"),
		DatabaseURL:    env("DATABASE_URL", "postgres://postgres:postgres@localhost:5436/integrations?sslmode=disable"),
		RedisAddr:      env("REDIS_ADDR", "localhost:16381"),
		QueueName:      env("QUEUE_NAME", "integration:raw-ingest"),
		WorkerCount:    envInt("WORKER_COUNT", 4),
		ShutdownPeriod: time.Duration(envInt("SHUTDOWN_SECONDS", 10)) * time.Second,
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
func envInt(key string, fallback int) int {
	value, err := strconv.Atoi(os.Getenv(key))
	if err != nil || value < 1 {
		return fallback
	}
	return value
}
