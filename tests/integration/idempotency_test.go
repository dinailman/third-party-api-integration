package integration

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/dinailman/third-party-api-integration-service/internal/models"
	"github.com/dinailman/third-party-api-integration-service/internal/normalizer"
	"github.com/jackc/pgx/v5/pgconn"
)

// concurrentIngests is how many callers race to ingest the same external ID below.
const concurrentIngests = 50

// uniqueViolation is the SQLSTATE PostgreSQL raises for a unique constraint breach.
const uniqueViolation = "23505"

// payload is one provider-a delivery, shaped by the mapping rules in 002_seed.sql.
func payload() map[string]any {
	return map[string]any{
		"user_id":     "concurrency-user",
		"metric_type": "heart_rate",
		"value":       72,
		"unit":        "bpm",
		"recorded_at": "2026-08-20T08:30:00Z",
	}
}

// isUniqueViolation reports whether err is the unique constraint breach the ingest path
// relies on, rather than some other database failure that happens to be non-nil.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolation
}

// TestConcurrentIngestCreatesOneMetric is the measured form of the idempotency claim:
// fifty callers racing to ingest one external ID must leave exactly one raw ingest row
// and exactly one normalized metric behind.
//
// The guard is UNIQUE(provider_id, external_id) on raw_ingest_logs. CreateRaw carries no
// ON CONFLICT clause, so a loser's insert blocks until the winner commits and then fails
// with SQLSTATE 23505 -- the duplicate is refused by the database, never stored. The
// handler turns that into HTTP 409.
func TestConcurrentIngestCreatesOneMetric(t *testing.T) {
	r := repo(t)
	ctx := context.Background()

	external := "pulse-evt-" + time.Now().Format("20060102150405.000000000")

	raws := make([]models.RawIngest, concurrentIngests)
	errs := make([]error, concurrentIngests)

	// Every goroutine blocks on the same channel so the ingests genuinely overlap rather
	// than queueing behind each other's start-up.
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < concurrentIngests; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			raw, err := r.CreateRaw(ctx, "provider-a", payload(), external)
			if err != nil {
				errs[i] = err
				return
			}
			// Only the winner gets this far, and it carries the delivery through the rest
			// of the path so the assertion below is about a real normalized metric.
			provider, err := r.GetProvider(ctx, raw.ProviderSlug)
			if err != nil {
				errs[i] = err
				return
			}
			metric, err := normalizer.Normalize(provider, raw)
			if err != nil {
				errs[i] = err
				return
			}
			raws[i], errs[i] = raw, r.SaveMetric(ctx, metric)
		}(i)
	}
	close(start)
	wg.Wait()

	winner := ""
	succeeded := 0
	for i, err := range errs {
		if err == nil {
			succeeded++
			winner = raws[i].ID
			continue
		}
		if !isUniqueViolation(err) {
			t.Fatalf("ingest %d failed with %v, want SQLSTATE %s", i, err, uniqueViolation)
		}
	}
	if succeeded != 1 {
		t.Fatalf("%d ingests of external_id %q produced %d successes, want 1", concurrentIngests, external, succeeded)
	}

	var storedRaw int
	if err := r.DB.QueryRow(ctx, `SELECT count(*) FROM raw_ingest_logs WHERE external_id=$1`, external).Scan(&storedRaw); err != nil {
		t.Fatalf("count raw ingest logs: %v", err)
	}
	if storedRaw != 1 {
		t.Fatalf("%d raw ingest rows stored for external_id %q, want 1", storedRaw, external)
	}

	var storedMetrics int
	if err := r.DB.QueryRow(ctx, `SELECT count(*) FROM normalized_metrics WHERE raw_ingest_id=$1`, winner).Scan(&storedMetrics); err != nil {
		t.Fatalf("count normalized metrics: %v", err)
	}
	if storedMetrics != 1 {
		t.Fatalf("%d normalized metrics stored for raw ingest %s, want 1", storedMetrics, winner)
	}
}

// TestConcurrentSaveMetricStoresOneMetric covers the second constraint. A raw payload can
// be normalized more than once -- the recovery sweep requeues rows whose lock has expired,
// so two workers can hold the same ID -- and normalized_metrics.raw_ingest_id UNIQUE plus
// ON CONFLICT DO NOTHING in SaveMetric is what keeps that from double-storing. In the test
// above only the winner reaches SaveMetric, so nothing races there; here it does.
func TestConcurrentSaveMetricStoresOneMetric(t *testing.T) {
	r := repo(t)
	ctx := context.Background()

	external := "pulse-evt-resave-" + time.Now().Format("20060102150405.000000000")
	raw, err := r.CreateRaw(ctx, "provider-a", payload(), external)
	if err != nil {
		t.Fatalf("create raw ingest: %v", err)
	}
	provider, err := r.GetProvider(ctx, raw.ProviderSlug)
	if err != nil {
		t.Fatalf("load provider: %v", err)
	}
	metric, err := normalizer.Normalize(provider, raw)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}

	errs := make([]error, concurrentIngests)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < concurrentIngests; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			errs[i] = r.SaveMetric(ctx, metric)
		}(i)
	}
	close(start)
	wg.Wait()

	// DO NOTHING is not an error: every caller is expected to return cleanly, and the
	// constraint -- not the caller -- is what makes the write happen only once.
	for i, err := range errs {
		if err != nil {
			t.Fatalf("save %d failed: %v", i, err)
		}
	}

	var stored int
	if err := r.DB.QueryRow(ctx, `SELECT count(*) FROM normalized_metrics WHERE raw_ingest_id=$1`, raw.ID).Scan(&stored); err != nil {
		t.Fatalf("count normalized metrics: %v", err)
	}
	if stored != 1 {
		t.Fatalf("%d normalized metrics stored for raw ingest %s, want 1", stored, raw.ID)
	}
}
