package normalizer

import (
	"github.com/dinailman/third-party-api-integration-service/internal/models"
	"testing"
	"time"
)

func TestNormalizeNestedProviderPayload(t *testing.T) {
	provider := models.Provider{Slug: "provider-b", MappingRules: map[string]string{"user_id": "member.id", "metric_type": "measurement.name", "value": "measurement.amount", "unit": "measurement.uom", "timestamp": "measured_at"}}
	raw := models.RawIngest{ID: "raw-1", Payload: map[string]any{"member": map[string]any{"id": "user-1"}, "measurement": map[string]any{"name": "weight", "amount": 72.4, "uom": "kg"}, "measured_at": "2026-08-20T08:30:00Z"}}
	metric, err := Normalize(provider, raw)
	if err != nil {
		t.Fatal(err)
	}
	if metric.UserID != "user-1" || metric.MetricType != "weight" || metric.Value != 72.4 || metric.SourceProvider != "provider-b" {
		t.Fatalf("metric = %+v", metric)
	}
}
func TestNormalizeRejectsInvalidPayload(t *testing.T) {
	provider := models.Provider{Slug: "provider-a", MappingRules: map[string]string{"user_id": "user_id", "metric_type": "metric_type", "value": "value", "unit": "unit", "timestamp": "recorded_at"}}
	_, err := Normalize(provider, models.RawIngest{Payload: map[string]any{"user_id": "user-1", "metric_type": "heart_rate", "value": "bad", "unit": "bpm", "recorded_at": "bad"}})
	if err == nil {
		t.Fatal("invalid payload accepted")
	}
}
func TestTimestampIsRFC3339(t *testing.T) {
	if _, err := time.Parse(time.RFC3339, "2026-08-20T08:30:00Z"); err != nil {
		t.Fatal(err)
	}
}
