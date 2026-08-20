package normalizer

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/dinailman/third-party-api-integration-service/internal/models"
	"strconv"
	"strings"
	"time"
)

var ErrInvalidPayload = errors.New("invalid provider payload")

func Normalize(provider models.Provider, raw models.RawIngest) (models.Metric, error) {
	value := func(path string) any { return lookup(raw.Payload, path) }
	userID, ok := value(provider.MappingRules["user_id"]).(string)
	if !ok || strings.TrimSpace(userID) == "" {
		return models.Metric{}, fmt.Errorf("%w: user_id is required", ErrInvalidPayload)
	}
	metricType, ok := value(provider.MappingRules["metric_type"]).(string)
	if !ok || strings.TrimSpace(metricType) == "" {
		return models.Metric{}, fmt.Errorf("%w: metric_type is required", ErrInvalidPayload)
	}
	unit, ok := value(provider.MappingRules["unit"]).(string)
	if !ok || strings.TrimSpace(unit) == "" {
		return models.Metric{}, fmt.Errorf("%w: unit is required", ErrInvalidPayload)
	}
	number, err := numberValue(value(provider.MappingRules["value"]))
	if err != nil {
		return models.Metric{}, fmt.Errorf("%w: value must be numeric", ErrInvalidPayload)
	}
	occurred, err := timeValue(value(provider.MappingRules["timestamp"]))
	if err != nil {
		return models.Metric{}, fmt.Errorf("%w: timestamp is invalid", ErrInvalidPayload)
	}
	return models.Metric{UserID: userID, MetricType: metricType, Value: number, Unit: unit, OccurredAt: occurred, SourceProvider: provider.Slug, RawIngestID: raw.ID}, nil
}

func lookup(payload map[string]any, path string) any {
	var current any = payload
	for _, part := range strings.Split(path, ".") {
		object, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = object[part]
	}
	return current
}
func numberValue(value any) (float64, error) {
	switch x := value.(type) {
	case float64:
		return x, nil
	case float32:
		return float64(x), nil
	case int:
		return float64(x), nil
	case json.Number:
		return x.Float64()
	case string:
		return strconv.ParseFloat(x, 64)
	}
	return 0, errors.New("not numeric")
}
func timeValue(value any) (time.Time, error) {
	text, ok := value.(string)
	if !ok {
		return time.Time{}, errors.New("not a timestamp")
	}
	return time.Parse(time.RFC3339, text)
}
