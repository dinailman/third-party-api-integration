package e2e

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func TestProviderIngestLifecycle(t *testing.T) {
	base := strings.TrimRight(os.Getenv("E2E_BASE_URL"), "/")
	if base == "" {
		t.Skip("set E2E_BASE_URL to run against Docker Compose")
	}
	client := &http.Client{Timeout: 5 * time.Second}
	payload := map[string]any{"user_id": "e2e-user", "metric_type": "heart_rate", "value": 72, "unit": "bpm", "recorded_at": "2026-08-20T08:30:00Z"}
	response := post(t, client, base+"/ingest/provider-a", payload, http.StatusAccepted)
	if response["raw_ingest_id"] == nil {
		t.Fatal("raw ingest id missing")
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		metrics := get(t, client, base+"/metrics?user_id=e2e-user", http.StatusOK).([]any)
		if len(metrics) == 1 {
			metric := metrics[0].(map[string]any)
			if metric["source_provider"] != "provider-a" || metric["metric_type"] != "heart_rate" {
				t.Fatalf("metric = %v", metric)
			}
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatal("normalized metric was not written before timeout")
}
func post(t *testing.T, c *http.Client, url string, payload map[string]any, status int) map[string]any {
	t.Helper()
	body, _ := json.Marshal(payload)
	request, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-External-ID", "e2e-provider-a-1")
	response, err := c.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	raw, _ := io.ReadAll(response.Body)
	if response.StatusCode != status {
		t.Fatalf("POST %s status=%d want=%d body=%s", url, response.StatusCode, status, raw)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}
func get(t *testing.T, c *http.Client, url string, status int) any {
	t.Helper()
	response, err := c.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	raw, _ := io.ReadAll(response.Body)
	if response.StatusCode != status {
		t.Fatalf("GET %s status=%d want=%d body=%s", url, response.StatusCode, status, raw)
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}
