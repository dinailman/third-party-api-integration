package models

import "time"

const (
	RawPending    = "pending"
	RawProcessing = "processing"
	RawSucceeded  = "succeeded"
	RawFailed     = "failed"
	RawDead       = "dead"
)

type Provider struct {
	ID           string            `json:"id"`
	Slug         string            `json:"slug"`
	Name         string            `json:"name"`
	Category     string            `json:"category"`
	MappingRules map[string]string `json:"mapping_rules"`
	Enabled      bool              `json:"enabled"`
	CreatedAt    time.Time         `json:"created_at"`
}

type RawIngest struct {
	ID           string         `json:"id"`
	ProviderID   string         `json:"provider_id"`
	ProviderSlug string         `json:"provider_slug"`
	ExternalID   string         `json:"external_id,omitempty"`
	Payload      map[string]any `json:"payload"`
	Status       string         `json:"status"`
	AttemptCount int            `json:"attempt_count"`
	ErrorMessage string         `json:"error_message,omitempty"`
	ReceivedAt   time.Time      `json:"received_at"`
	ProcessedAt  *time.Time     `json:"processed_at,omitempty"`
}

type Metric struct {
	ID             string    `json:"id"`
	UserID         string    `json:"user_id"`
	MetricType     string    `json:"metric_type"`
	Value          float64   `json:"value"`
	Unit           string    `json:"unit"`
	OccurredAt     time.Time `json:"occurred_at"`
	SourceProvider string    `json:"source_provider"`
	RawIngestID    string    `json:"raw_ingest_id"`
	CreatedAt      time.Time `json:"created_at"`
}

type ErrorLog struct {
	ID           string    `json:"id"`
	RawIngestID  string    `json:"raw_ingest_id"`
	ProviderSlug string    `json:"provider_slug"`
	Attempt      int       `json:"attempt"`
	ErrorType    string    `json:"error_type"`
	Message      string    `json:"message"`
	CreatedAt    time.Time `json:"created_at"`
}
