package main

import (
	"context"
	"errors"
	"github.com/dinailman/third-party-api-integration-service/internal/config"
	"github.com/dinailman/third-party-api-integration-service/internal/database"
	"github.com/dinailman/third-party-api-integration-service/internal/handlers"
	"github.com/dinailman/third-party-api-integration-service/internal/queue"
	"github.com/dinailman/third-party-api-integration-service/internal/repositories"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
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
	if err = q.Ping(ctx); err != nil {
		logger.Error("redis connection failed", "error", err)
		os.Exit(1)
	}
	s := &handlers.Server{Repo: &repositories.Repository{DB: db}, Queue: q}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /providers", s.CreateProvider)
	mux.HandleFunc("GET /providers", s.ListProviders)
	mux.HandleFunc("POST /ingest/{provider}", s.Ingest)
	mux.HandleFunc("GET /metrics", s.Metrics)
	mux.HandleFunc("GET /ingestion-logs", s.RawLogs)
	mux.HandleFunc("GET /error-logs", s.Errors)
	mux.HandleFunc("GET /healthz", s.Health)
	mux.HandleFunc("GET /prometheus", s.Prometheus)
	server := &http.Server{Addr: cfg.HTTPAddr, Handler: s.RateLogged(mux), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		logger.Info("api listening", "addr", cfg.HTTPAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("api stopped", "error", err)
			stop()
		}
	}()
	<-ctx.Done()
	shutdown, cancel := context.WithTimeout(context.Background(), cfg.ShutdownPeriod)
	defer cancel()
	_ = server.Shutdown(shutdown)
}
