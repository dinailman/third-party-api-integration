package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/dinailman/third-party-api-integration/internal/models"
	"github.com/dinailman/third-party-api-integration/internal/queue"
	"github.com/dinailman/third-party-api-integration/internal/repositories"
	"io"
	"net/http"
	"sync/atomic"
)

type Server struct {
	Repo     *repositories.Repository
	Queue    *queue.Queue
	Requests atomic.Uint64
	Ingested atomic.Uint64
}
type providerRequest struct {
	Slug         string            `json:"slug"`
	Name         string            `json:"name"`
	Category     string            `json:"category"`
	MappingRules map[string]string `json:"mapping_rules"`
	Enabled      *bool             `json:"enabled"`
}

func (s *Server) CreateProvider(w http.ResponseWriter, r *http.Request) {
	var req providerRequest
	if !decode(w, r, &req) {
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	p, err := s.Repo.CreateProvider(r.Context(), models.Provider{Slug: req.Slug, Name: req.Name, Category: req.Category, MappingRules: req.MappingRules, Enabled: enabled})
	if err != nil {
		errorJSON(w, 409, "provider could not be created")
		return
	}
	jsonResponse(w, 201, p)
}
func (s *Server) ListProviders(w http.ResponseWriter, r *http.Request) {
	p, err := s.Repo.ListProviders(r.Context())
	if err != nil {
		errorJSON(w, 500, "could not list providers")
		return
	}
	jsonResponse(w, 200, p)
}
func (s *Server) Ingest(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	var payload map[string]any
	if !decode(w, r, &payload) {
		return
	}
	external := r.Header.Get("X-External-ID")
	raw, err := s.Repo.CreateRaw(r.Context(), provider, payload, external)
	if errors.Is(err, repositories.ErrProviderNotFound) {
		errorJSON(w, 404, "provider not found")
		return
	}
	if err != nil {
		errorJSON(w, 409, "payload could not be recorded")
		return
	}
	if err = s.Queue.Enqueue(r.Context(), raw.ID); err != nil {
		errorJSON(w, 503, "payload recorded but queue unavailable")
		return
	}
	s.Ingested.Add(1)
	jsonResponse(w, 202, map[string]any{"raw_ingest_id": raw.ID, "status": raw.Status, "provider": provider})
}
func (s *Server) Metrics(w http.ResponseWriter, r *http.Request) {
	values, err := s.Repo.Metrics(r.Context(), r.URL.Query().Get("user_id"), r.URL.Query().Get("metric_type"), r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	if err != nil {
		errorJSON(w, 400, "invalid metric query")
		return
	}
	jsonResponse(w, 200, values)
}
func (s *Server) RawLogs(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if value := r.URL.Query().Get("limit"); value != "" {
		fmt.Sscan(value, &limit)
	}
	if limit < 1 || limit > 500 {
		limit = 100
	}
	out, err := s.Repo.RawLogs(r.Context(), limit)
	if err != nil {
		errorJSON(w, 500, "could not list ingestion logs")
		return
	}
	jsonResponse(w, 200, out)
}
func (s *Server) Errors(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if value := r.URL.Query().Get("limit"); value != "" {
		fmt.Sscan(value, &limit)
	}
	if limit < 1 || limit > 500 {
		limit = 100
	}
	out, err := s.Repo.Errors(r.Context(), limit)
	if err != nil {
		errorJSON(w, 500, "could not list error logs")
		return
	}
	jsonResponse(w, 200, out)
}
func (s *Server) Health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2e9)
	defer cancel()
	if err := s.Repo.Health(ctx); err != nil {
		errorJSON(w, 503, "database unavailable")
		return
	}
	if err := s.Queue.Ping(ctx); err != nil {
		errorJSON(w, 503, "redis unavailable")
		return
	}
	jsonResponse(w, 200, map[string]string{"status": "ok"})
}
func (s *Server) Prometheus(w http.ResponseWriter, r *http.Request) {
	depth, _ := s.Queue.Depth(r.Context())
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	fmt.Fprintf(w, "# TYPE http_requests_total counter\nhttp_requests_total %d\n# TYPE ingest_requests_total counter\ningest_requests_total %d\n# TYPE integration_queue_depth gauge\nintegration_queue_depth %d\n", s.Requests.Load(), s.Ingested.Load(), depth)
}
func (s *Server) RateLogged(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { s.Requests.Add(1); next.ServeHTTP(w, r) })
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	defer r.Body.Close()
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		errorJSON(w, 400, "invalid JSON body")
		return false
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		errorJSON(w, 400, "request body must contain one JSON object")
		return false
	}
	return true
}
func jsonResponse(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func errorJSON(w http.ResponseWriter, status int, msg string) {
	jsonResponse(w, status, map[string]string{"error": msg})
}
