// Package analytics provides prompt run recording and aggregate queries for observability.
package analytics

import (
	"context"
	"sync"
	"time"
)

// RunRecord is a single recorded execution (prompt id/version, latency, tokens, success).
type RunRecord struct {
	PromptID   string
	Version    string
	LatencyMs  int64
	InputTokens  int
	OutputTokens int
	Success    bool
	At         time.Time
}

// Store is the interface for recording and querying prompt runs.
type Store interface {
	Record(ctx context.Context, r RunRecord) error
	Query(ctx context.Context, q Query) ([]Aggregate, error)
}

// Query filters and groups runs for aggregation.
type Query struct {
	PromptID   string
	Version    string
	From       time.Time
	To         time.Time
	GroupBy    string // "prompt", "version", "day", "hour"
	Limit      int
}

// Aggregate is a bucketed aggregate (e.g. per prompt or per day).
type Aggregate struct {
	Key          string  // e.g. prompt id or "2024-01-15"
	Runs         int64
	SuccessCount int64
	AvgLatencyMs float64
	TotalInputTokens  int64
	TotalOutputTokens int64
}

// MemoryStore is an in-memory implementation (bounded slice, no persistence).
type MemoryStore struct {
	mu     sync.RWMutex
	max    int
	records []RunRecord
}

// NewMemoryStore creates an in-memory store that keeps at most max records (0 = unbounded).
func NewMemoryStore(max int) *MemoryStore {
	return &MemoryStore{max: max, records: make([]RunRecord, 0, 256)}
}

// Record implements Store.
func (m *MemoryStore) Record(ctx context.Context, r RunRecord) error {
	if r.At.IsZero() {
		r.At = time.Now()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.records = append(m.records, r)
	if m.max > 0 && len(m.records) > m.max {
		m.records = m.records[len(m.records)-m.max:]
	}
	return nil
}

// Query implements Store. GroupBy "prompt" groups by PromptID, "version" by PromptID+Version, "day" by date.
func (m *MemoryStore) Query(ctx context.Context, q Query) ([]Aggregate, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	type key struct {
		s string
	}
	agg := make(map[string]*Aggregate)
	for _, r := range m.records {
		if q.PromptID != "" && r.PromptID != q.PromptID {
			continue
		}
		if q.Version != "" && r.Version != q.Version {
			continue
		}
		if !q.From.IsZero() && r.At.Before(q.From) {
			continue
		}
		if !q.To.IsZero() && r.At.After(q.To) {
			continue
		}
		var k string
		switch q.GroupBy {
		case "prompt":
			k = r.PromptID
		case "version":
			k = r.PromptID + "@" + r.Version
		case "day":
			k = r.At.Format("2006-01-02")
		case "hour":
			k = r.At.Format("2006-01-02-15")
		default:
			k = "all"
		}
		if agg[k] == nil {
			agg[k] = &Aggregate{Key: k}
		}
		a := agg[k]
		a.Runs++
		if r.Success {
			a.SuccessCount++
		}
		a.AvgLatencyMs = (a.AvgLatencyMs*float64(a.Runs-1) + float64(r.LatencyMs)) / float64(a.Runs)
		a.TotalInputTokens += int64(r.InputTokens)
		a.TotalOutputTokens += int64(r.OutputTokens)
	}
	out := make([]Aggregate, 0, len(agg))
	for _, a := range agg {
		out = append(out, *a)
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 100
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
