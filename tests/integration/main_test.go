// Package integration exercises the repository against a real PostgreSQL instance.
//
// The tests are skipped unless TEST_DATABASE_URL is set. That database is reset before
// the suite runs -- the public schema is dropped and the migrations are re-applied -- so
// it must be a throwaway database, never a development one.
//
//	docker compose up -d postgres
//	docker compose exec -T postgres psql -U postgres -c 'CREATE DATABASE integrations_test'
//	TEST_DATABASE_URL='postgres://postgres:postgres@localhost:15436/integrations_test?sslmode=disable' \
//	  go test -race ./tests/integration -v
package integration

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/dinailman/third-party-api-integration-service/internal/repositories"
	"github.com/jackc/pgx/v5/pgxpool"
)

var pool *pgxpool.Pool

func TestMain(m *testing.M) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		// Skipping is right on a laptop with no database, but under CI it is the failure
		// this suite is meant to catch: a green run that tested nothing. CI must configure
		// the service or hear about it.
		if os.Getenv("CI") != "" {
			panic("TEST_DATABASE_URL is unset under CI, so the integration suite would skip and report success")
		}
		os.Exit(m.Run())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	p, err := pgxpool.New(ctx, url)
	if err != nil {
		panic("connect to TEST_DATABASE_URL: " + err.Error())
	}
	if err := reset(ctx, p); err != nil {
		panic("reset test schema: " + err.Error())
	}
	pool = p
	code := m.Run()
	p.Close()
	os.Exit(code)
}

// reset drops the public schema and replays every migration, giving each run a database
// in a known state without depending on psql being installed. Replaying 002_seed.sql is
// what supplies the provider-a row the tests ingest against.
func reset(ctx context.Context, p *pgxpool.Pool) error {
	if _, err := p.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		return err
	}
	files, err := filepath.Glob(filepath.Join("..", "..", "migrations", "*.sql"))
	if err != nil {
		return err
	}
	sort.Strings(files)
	for _, file := range files {
		statements, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		if _, err := p.Exec(ctx, string(statements)); err != nil {
			return err
		}
	}
	return nil
}

func repo(t *testing.T) *repositories.Repository {
	t.Helper()
	if pool == nil {
		t.Skip("set TEST_DATABASE_URL to run integration tests")
	}
	return &repositories.Repository{DB: pool}
}
