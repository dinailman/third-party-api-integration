package worker

import (
	"context"
	"fmt"
	"github.com/dinailman/third-party-api-integration/internal/normalizer"
	"github.com/dinailman/third-party-api-integration/internal/queue"
	"github.com/dinailman/third-party-api-integration/internal/repositories"
	"log/slog"
	"sync"
	"time"
)

type Worker struct {
	Repo   *repositories.Repository
	Queue  *queue.Queue
	Logger *slog.Logger
	Count  int
}

func (w *Worker) Run(ctx context.Context) {
	if w.Count < 1 {
		w.Count = 1
	}
	var wg sync.WaitGroup
	go w.recover(ctx)
	for i := 0; i < w.Count; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); w.loop(ctx) }()
	}
	<-ctx.Done()
	wg.Wait()
}
func (w *Worker) loop(ctx context.Context) {
	for ctx.Err() == nil {
		id, err := w.Queue.Dequeue(ctx)
		if err != nil {
			w.Logger.Error("dequeue failed", "error", err)
			continue
		}
		if id != "" {
			w.process(ctx, id)
		}
	}
}
func (w *Worker) process(ctx context.Context, id string) {
	raw, claimed, err := w.Repo.ClaimRaw(ctx, id)
	if err != nil || !claimed {
		return
	}
	provider, err := w.Repo.GetProvider(ctx, raw.ProviderSlug)
	if err == nil && !provider.Enabled {
		err = fmt.Errorf("provider %s is disabled", provider.Slug)
	}
	if err == nil {
		metric, normalizeErr := normalizer.Normalize(provider, raw)
		if normalizeErr == nil {
			err = w.Repo.SaveMetric(ctx, metric)
		} else {
			err = normalizeErr
		}
	}
	if err == nil {
		if markErr := w.Repo.MarkRawSucceeded(ctx, raw.ID); markErr != nil {
			w.Logger.Error("mark raw success failed", "error", markErr)
		}
		return
	}
	dead := raw.AttemptCount >= 3
	if markErr := w.Repo.MarkRawError(ctx, raw, "normalization_error", err.Error(), dead); markErr != nil {
		w.Logger.Error("record integration error failed", "error", markErr)
	}
	if !dead {
		time.Sleep(time.Duration(1<<uint(raw.AttemptCount-1)) * time.Second)
		if enqueueErr := w.Queue.Enqueue(ctx, raw.ID); enqueueErr != nil {
			w.Logger.Error("retry enqueue failed", "error", enqueueErr)
		}
	}
}
func (w *Worker) recover(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	w.requeue(ctx)
	for {
		select {
		case <-ticker.C:
			w.requeue(ctx)
		case <-ctx.Done():
			return
		}
	}
}
func (w *Worker) requeue(ctx context.Context) {
	ids, err := w.Repo.RecoverRaw(ctx, time.Now().UTC())
	if err != nil {
		w.Logger.Error("recovery failed", "error", err)
		return
	}
	for _, id := range ids {
		if err := w.Queue.Enqueue(ctx, id); err != nil {
			w.Logger.Error("recovery enqueue failed", "error", err, "raw_ingest_id", id)
		}
	}
}
