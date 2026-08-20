CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE providers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    category TEXT NOT NULL DEFAULT 'health-tech',
    mapping_rules JSONB NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE raw_ingest_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id UUID NOT NULL REFERENCES providers(id),
    external_id TEXT,
    payload JSONB NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('pending','processing','succeeded','failed','dead')),
    attempt_count INTEGER NOT NULL DEFAULT 0,
    error_message TEXT,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    processed_at TIMESTAMPTZ,
    locked_until TIMESTAMPTZ,
    UNIQUE(provider_id, external_id)
);

CREATE TABLE normalized_metrics (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id TEXT NOT NULL,
    metric_type TEXT NOT NULL,
    value DOUBLE PRECISION NOT NULL,
    unit TEXT NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL,
    source_provider TEXT NOT NULL,
    raw_ingest_id UUID NOT NULL UNIQUE REFERENCES raw_ingest_logs(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE error_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    raw_ingest_id UUID NOT NULL REFERENCES raw_ingest_logs(id),
    provider_slug TEXT NOT NULL,
    attempt INTEGER NOT NULL,
    error_type TEXT NOT NULL,
    message TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX raw_ingest_status_idx ON raw_ingest_logs(status, received_at);
CREATE INDEX metric_user_time_idx ON normalized_metrics(user_id, occurred_at DESC);
CREATE INDEX metric_type_time_idx ON normalized_metrics(metric_type, occurred_at DESC);
CREATE INDEX error_provider_time_idx ON error_logs(provider_slug, created_at DESC);
