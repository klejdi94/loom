// Package analytics HTTP API for recording and querying prompt runs.
package analytics

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

// Server exposes Store over HTTP: POST /record, GET /aggregates.
type Server struct {
	Store Store
	Addr  string
}

// NewServer creates a server that uses the given Store.
func NewServer(store Store, addr string) *Server {
	if addr == "" {
		addr = ":8080"
	}
	return &Server{Store: store, Addr: addr}
}

// recordRequest is the JSON body for POST /record.
type recordRequest struct {
	PromptID       string `json:"prompt_id"`
	Version        string `json:"version"`
	LatencyMs      int64  `json:"latency_ms"`
	InputTokens    int    `json:"input_tokens"`
	OutputTokens   int    `json:"output_tokens"`
	Success        bool   `json:"success"`
	At             string `json:"at,omitempty"` // RFC3339
}

// aggregateResponse is the JSON response for GET /aggregates.
type aggregateResponse struct {
	Aggregates []Aggregate `json:"aggregates"`
}

// ListenAndServe starts the HTTP server. Use go s.ListenAndServe() to run in background.
func (s *Server) ListenAndServe() error {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /record", s.handleRecord)
	mux.HandleFunc("PUT /record", s.handleRecord)
	mux.HandleFunc("GET /aggregates", s.handleAggregates)
	mux.HandleFunc("GET /health", s.handleHealth)
	return http.ListenAndServe(s.Addr, mux)
}

func (s *Server) handleRecord(w http.ResponseWriter, r *http.Request) {
	var req recordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.PromptID == "" || req.Version == "" {
		http.Error(w, "prompt_id and version required", http.StatusBadRequest)
		return
	}
	rec := RunRecord{
		PromptID:      req.PromptID,
		Version:       req.Version,
		LatencyMs:     req.LatencyMs,
		InputTokens:   req.InputTokens,
		OutputTokens:  req.OutputTokens,
		Success:       req.Success,
	}
	if req.At != "" {
		if t, err := time.Parse(time.RFC3339, req.At); err == nil {
			rec.At = t
		}
	}
	if err := s.Store.Record(r.Context(), rec); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAggregates(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	q := Query{
		PromptID: r.URL.Query().Get("prompt_id"),
		Version:  r.URL.Query().Get("version"),
		GroupBy:  r.URL.Query().Get("group_by"),
		Limit:    100,
	}
	if from := r.URL.Query().Get("from"); from != "" {
		if t, err := time.Parse(time.RFC3339, from); err == nil {
			q.From = t
		}
	}
	if to := r.URL.Query().Get("to"); to != "" {
		if t, err := time.Parse(time.RFC3339, to); err == nil {
			q.To = t
		}
	}
	if limit := r.URL.Query().Get("limit"); limit != "" {
		if n, err := strconv.Atoi(limit); err == nil && n > 0 {
			q.Limit = n
		}
	}
	agg, err := s.Store.Query(r.Context(), q)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(aggregateResponse{Aggregates: agg})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}
