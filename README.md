# Third-Party API Integration Service

Production-style Go integration microservice that normalizes inconsistent provider payloads into a common metric schema.

## Problem

Wearables, laboratories, fintech data vendors, and logistics partners rarely agree on field names or nesting. One provider may send `value` and `recorded_at`, another may wrap the same information under `measurement.amount` and `measured_at`. Downstream product code should not need a custom parser for every integration.

This service creates a provider-specific mapping configuration once, accepts raw payloads, and asynchronously converts them into a stable internal model.

## Architecture

```text
Provider A/B/C
      |
      v
POST /ingest/{provider}
      |
      +--> raw_ingest_logs (PostgreSQL)
      |
      +--> Redis list: raw ingest IDs
                    |
                    v
              Worker pool
                    |
       mapping + validation + retry
                    |
                    v
       normalized_metrics / error_logs
```

PostgreSQL is the source of truth. Redis carries only raw ingest IDs, so payloads and processing history remain inspectable even when a worker crashes. Failed payloads retry three times and then become `dead` with an error log.

## Features

- Provider registry with category and JSON mapping rules
- `POST /ingest/{provider}` raw payload ingestion
- Redis-backed asynchronous worker queue
- Common normalized metric schema
- Provider A, B, and C seed configurations
- Nested field mapping using dot paths
- Payload validation for user, metric type, value, unit, and RFC3339 timestamp
- Idempotent provider external IDs
- Retry and dead-letter behavior through `error_logs`
- Metric queries by user, type, and RFC3339 date range
- Raw ingestion and error debugging endpoints
- PostgreSQL health check and Prometheus-style metrics
- Docker Compose API, worker, PostgreSQL, and Redis setup

## Run Locally

```bash
docker compose up -d --build
```

Services:

- API: `http://localhost:8084`
- PostgreSQL: `localhost:15436`
- Redis: `localhost:16381`
- Health: `http://localhost:8084/healthz`
- Prometheus metrics: `http://localhost:8084/prometheus`

Reset the database:

```bash
docker compose down -v
docker compose up -d --build
```

## Provider Mapping

Provider A sends a flat payload:

```json
{
  "user_id": "user-1001",
  "metric_type": "heart_rate",
  "value": 72,
  "unit": "bpm",
  "recorded_at": "2026-08-20T08:30:00Z"
}
```

Provider B sends nested fields:

```json
{
  "member": {"id": "user-1001"},
  "measurement": {"name": "weight", "amount": 72.4, "uom": "kg"},
  "measured_at": "2026-08-20T08:30:00Z"
}
```

Provider C sends laboratory results:

```json
{
  "subject": {"user_id": "user-1001"},
  "result": {
    "code": "vitamin_d",
    "value": 34.2,
    "unit": "ng/mL",
    "observed_at": "2026-08-20T08:30:00Z"
  }
}
```

The resulting internal metric is always shaped as:

```json
{
  "user_id": "user-1001",
  "metric_type": "weight",
  "value": 72.4,
  "unit": "kg",
  "occurred_at": "2026-08-20T08:30:00Z",
  "source_provider": "provider-b"
}
```

## Example Requests

List configured providers:

```bash
curl http://localhost:8084/providers
```

Ingest Provider A data:

```bash
curl -X POST http://localhost:8084/ingest/provider-a \
  -H 'Content-Type: application/json' \
  -H 'X-External-ID: pulse-evt-1001' \
  -d '{
    "user_id":"user-1001",
    "metric_type":"heart_rate",
    "value":72,
    "unit":"bpm",
    "recorded_at":"2026-08-20T08:30:00Z"
  }'
```

Query normalized metrics:

```bash
curl 'http://localhost:8084/metrics?user_id=user-1001&metric_type=heart_rate&from=2026-08-20T00:00:00Z&to=2026-08-21T00:00:00Z'
```

Inspect raw processing:

```bash
curl 'http://localhost:8084/ingestion-logs?limit=50'
curl 'http://localhost:8084/error-logs?limit=50'
```

## API

```text
POST /providers
GET  /providers
POST /ingest/{provider}
GET  /metrics
GET  /ingestion-logs
GET  /error-logs
GET  /healthz
GET  /prometheus
```

## Industry Applications

- Health-tech: normalize heart rate, weight, sleep, glucose, and laboratory results from different vendors.
- Fintech: normalize transaction, balance, and risk events from banking and payment providers.
- Logistics: normalize shipment status, location, delivery estimate, and carrier exception data.

## Testing

```bash
gofmt -w .
go test ./...
go test -race ./...
go vet ./...
go build ./cmd/api
go build ./cmd/worker
docker compose config
```

Run the Docker-backed lifecycle test:

```bash
docker compose up -d --build
E2E_BASE_URL=http://localhost:8084 go test ./tests/e2e -v
docker compose down -v
```

## Production Improvements

- Add provider authentication and tenant-scoped credentials.
- Use an outbox to atomically persist raw data and publish queue work.
- Move mapping rules to versioned configuration with approval history.
- Add Kafka for high-volume integrations and partition by provider or user.
- Add provider-specific signature verification and webhook replay protection.
- Add OpenTelemetry traces and per-provider delivery/normalization SLOs.
