package main

import (
	"context"
	"github.com/dinailman/third-party-api-integration-service/internal/config"
	"github.com/dinailman/third-party-api-integration-service/internal/database"
	"github.com/dinailman/third-party-api-integration-service/internal/queue"
	"github.com/dinailman/third-party-api-integration-service/internal/repositories"
	"github.com/dinailman/third-party-api-integration-service/internal/worker"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	cfg := config.Load()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	db, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	q := queue.New(cfg.RedisAddr, cfg.QueueName)
	defer q.Close()
	(&worker.Worker{Repo: &repositories.Repository{DB: db}, Queue: q, Logger: logger, Count: cfg.WorkerCount}).Run(ctx)
}
