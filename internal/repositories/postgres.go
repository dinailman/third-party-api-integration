package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/dinailman/third-party-api-integration-service/internal/models"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"strings"
	"time"
)

var ErrNotFound = errors.New("resource not found")
var ErrProviderNotFound = errors.New("provider not found")

type Repository struct{ DB *pgxpool.Pool }

func (r *Repository) Health(ctx context.Context) error { return r.DB.Ping(ctx) }

func (r *Repository) CreateProvider(ctx context.Context, p models.Provider) (models.Provider, error) {
	raw, err := json.Marshal(p.MappingRules)
	if err != nil {
		return p, err
	}
	if p.Slug == "" || p.Name == "" || len(p.MappingRules) < 5 {
		return p, fmt.Errorf("slug, name, and five mapping rules are required")
	}
	err = r.DB.QueryRow(ctx, `INSERT INTO providers(slug,name,category,mapping_rules,enabled) VALUES($1,$2,$3,$4,$5) RETURNING id,created_at`, strings.ToLower(p.Slug), p.Name, p.Category, raw, p.Enabled).Scan(&p.ID, &p.CreatedAt)
	return p, err
}
func (r *Repository) GetProvider(ctx context.Context, slug string) (models.Provider, error) {
	var p models.Provider
	var raw []byte
	err := r.DB.QueryRow(ctx, `SELECT id,slug,name,category,mapping_rules,enabled,created_at FROM providers WHERE slug=$1`, slug).Scan(&p.ID, &p.Slug, &p.Name, &p.Category, &raw, &p.Enabled, &p.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, ErrProviderNotFound
	}
	if err != nil {
		return p, err
	}
	if err = json.Unmarshal(raw, &p.MappingRules); err != nil {
		return p, err
	}
	return p, nil
}
func (r *Repository) ListProviders(ctx context.Context) ([]models.Provider, error) {
	rows, err := r.DB.Query(ctx, `SELECT id,slug,name,category,mapping_rules,enabled,created_at FROM providers ORDER BY slug`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.Provider{}
	for rows.Next() {
		var p models.Provider
		var raw []byte
		if err := rows.Scan(&p.ID, &p.Slug, &p.Name, &p.Category, &raw, &p.Enabled, &p.CreatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &p.MappingRules); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *Repository) CreateRaw(ctx context.Context, provider string, payload map[string]any, external string) (models.RawIngest, error) {
	rawPayload, err := json.Marshal(payload)
	if err != nil {
		return models.RawIngest{}, err
	}
	var x models.RawIngest
	err = r.DB.QueryRow(ctx, `INSERT INTO raw_ingest_logs(provider_id,external_id,payload,status) SELECT id,NULLIF($2,''),$3,'pending' FROM providers WHERE slug=$1 RETURNING id,provider_id,COALESCE(external_id,''),payload,status,attempt_count,received_at`, provider, external, rawPayload).Scan(&x.ID, &x.ProviderID, &x.ExternalID, &rawPayload, &x.Status, &x.AttemptCount, &x.ReceivedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return x, ErrProviderNotFound
	}
	if err != nil {
		return x, err
	}
	x.ProviderSlug = provider
	_ = json.Unmarshal(rawPayload, &x.Payload)
	return x, nil
}

func (r *Repository) ClaimRaw(ctx context.Context, id string) (models.RawIngest, bool, error) {
	var x models.RawIngest
	var raw []byte
	err := r.DB.QueryRow(ctx, `UPDATE raw_ingest_logs SET status='processing',attempt_count=attempt_count+1,locked_until=now()+interval '60 seconds' WHERE id=$1 AND status IN ('pending','failed') AND (locked_until IS NULL OR locked_until < now()) RETURNING id,provider_id,COALESCE(external_id,''),payload,status,attempt_count,COALESCE(error_message,''),received_at,processed_at`, id).Scan(&x.ID, &x.ProviderID, &x.ExternalID, &raw, &x.Status, &x.AttemptCount, &x.ErrorMessage, &x.ReceivedAt, &x.ProcessedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return x, false, nil
	}
	if err != nil {
		return x, false, err
	}
	if err = json.Unmarshal(raw, &x.Payload); err != nil {
		return x, false, err
	}
	err = r.DB.QueryRow(ctx, `SELECT slug FROM providers WHERE id=$1`, x.ProviderID).Scan(&x.ProviderSlug)
	return x, err == nil, err
}
func (r *Repository) RecoverRaw(ctx context.Context, now time.Time) ([]string, error) {
	rows, err := r.DB.Query(ctx, `SELECT id FROM raw_ingest_logs WHERE status IN ('pending','failed') AND (locked_until IS NULL OR locked_until < $1) ORDER BY received_at LIMIT 100`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *Repository) SaveMetric(ctx context.Context, m models.Metric) error {
	_, err := r.DB.Exec(ctx, `INSERT INTO normalized_metrics(user_id,metric_type,value,unit,occurred_at,source_provider,raw_ingest_id) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(raw_ingest_id) DO NOTHING`, m.UserID, m.MetricType, m.Value, m.Unit, m.OccurredAt, m.SourceProvider, m.RawIngestID)
	return err
}
func (r *Repository) MarkRawSucceeded(ctx context.Context, id string) error {
	_, err := r.DB.Exec(ctx, `UPDATE raw_ingest_logs SET status='succeeded',processed_at=now(),locked_until=NULL,error_message=NULL WHERE id=$1`, id)
	return err
}
func (r *Repository) MarkRawError(ctx context.Context, x models.RawIngest, errType, message string, dead bool) error {
	status := models.RawFailed
	if dead {
		status = models.RawDead
	}
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `UPDATE raw_ingest_logs SET status=$2,error_message=$3,locked_until=NULL WHERE id=$1`, x.ID, status, message); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO error_logs(raw_ingest_id,provider_slug,attempt,error_type,message) VALUES($1,$2,$3,$4,$5)`, x.ID, x.ProviderSlug, x.AttemptCount, errType, message); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) Metrics(ctx context.Context, userID, metricType, from, to string) ([]models.Metric, error) {
	q := `SELECT id,user_id,metric_type,value,unit,occurred_at,source_provider,raw_ingest_id,created_at FROM normalized_metrics WHERE 1=1`
	args := []any{}
	add := func(expr string, v any) { args = append(args, v); q += fmt.Sprintf(" AND %s=$%d", expr, len(args)) }
	if userID != "" {
		add("user_id", userID)
	}
	if metricType != "" {
		add("metric_type", metricType)
	}
	if from != "" {
		t, err := time.Parse(time.RFC3339, from)
		if err != nil {
			return nil, err
		}
		args = append(args, t)
		q += fmt.Sprintf(" AND occurred_at >= $%d", len(args))
	}
	if to != "" {
		t, err := time.Parse(time.RFC3339, to)
		if err != nil {
			return nil, err
		}
		args = append(args, t)
		q += fmt.Sprintf(" AND occurred_at < $%d", len(args))
	}
	q += " ORDER BY occurred_at DESC LIMIT 500"
	rows, err := r.DB.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.Metric{}
	for rows.Next() {
		var m models.Metric
		if err := rows.Scan(&m.ID, &m.UserID, &m.MetricType, &m.Value, &m.Unit, &m.OccurredAt, &m.SourceProvider, &m.RawIngestID, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
func (r *Repository) RawLogs(ctx context.Context, limit int) ([]models.RawIngest, error) {
	rows, err := r.DB.Query(ctx, `SELECT l.id,l.provider_id,p.slug,COALESCE(l.external_id,''),l.payload,l.status,l.attempt_count,COALESCE(l.error_message,''),l.received_at,l.processed_at FROM raw_ingest_logs l JOIN providers p ON p.id=l.provider_id ORDER BY l.received_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.RawIngest{}
	for rows.Next() {
		var x models.RawIngest
		var raw []byte
		if err := rows.Scan(&x.ID, &x.ProviderID, &x.ProviderSlug, &x.ExternalID, &raw, &x.Status, &x.AttemptCount, &x.ErrorMessage, &x.ReceivedAt, &x.ProcessedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(raw, &x.Payload)
		out = append(out, x)
	}
	return out, rows.Err()
}
func (r *Repository) Errors(ctx context.Context, limit int) ([]models.ErrorLog, error) {
	rows, err := r.DB.Query(ctx, `SELECT id,raw_ingest_id,provider_slug,attempt,error_type,message,created_at FROM error_logs ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.ErrorLog{}
	for rows.Next() {
		var x models.ErrorLog
		if err := rows.Scan(&x.ID, &x.RawIngestID, &x.ProviderSlug, &x.Attempt, &x.ErrorType, &x.Message, &x.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
