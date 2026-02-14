// Package analytics: Redis Store for persistent run history.
package analytics

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

const defaultRedisKey = "loom:analytics:runs"

// RedisStore implements Store using Redis (sorted set by timestamp, value = JSON RunRecord).
type RedisStore struct {
	client redis.UniversalClient
	key    string
}

// NewRedisStore creates a store that uses the given Redis client.
func NewRedisStore(client redis.UniversalClient, key string) *RedisStore {
	if key == "" {
		key = defaultRedisKey
	}
	return &RedisStore{client: client, key: key}
}

type redisRecord struct {
	PromptID      string `json:"prompt_id"`
	Version       string `json:"version"`
	LatencyMs     int64  `json:"latency_ms"`
	InputTokens   int    `json:"input_tokens"`
	OutputTokens  int    `json:"output_tokens"`
	Success       bool   `json:"success"`
	At            string `json:"at"` // RFC3339
}

// Record implements Store.
func (r *RedisStore) Record(ctx context.Context, rec RunRecord) error {
	if rec.At.IsZero() {
		rec.At = time.Now()
	}
	score := float64(rec.At.UnixNano()) / 1e9
	payload := redisRecord{
		PromptID:     rec.PromptID,
		Version:      rec.Version,
		LatencyMs:    rec.LatencyMs,
		InputTokens:  rec.InputTokens,
		OutputTokens: rec.OutputTokens,
		Success:      rec.Success,
		At:           rec.At.Format(time.RFC3339),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return r.client.ZAdd(ctx, r.key, redis.Z{Score: score, Member: string(raw)}).Err()
}

// Query implements Store by reading from the sorted set and aggregating in memory.
func (r *RedisStore) Query(ctx context.Context, q Query) ([]Aggregate, error) {
	min, max := "-inf", "+inf"
	if !q.From.IsZero() {
		min = strconv.FormatFloat(float64(q.From.UnixNano())/1e9, 'f', -1, 64)
	}
	if !q.To.IsZero() {
		max = strconv.FormatFloat(float64(q.To.UnixNano())/1e9, 'f', -1, 64)
	}
	const batch = 10000
	var records []RunRecord
	for offset := int64(0); ; offset += batch {
		vals, err := r.client.ZRangeByScoreWithScores(ctx, r.key, &redis.ZRangeBy{
			Min: min, Max: max, Offset: offset, Count: batch,
		}).Result()
		if err != nil {
			return nil, err
		}
		for _, z := range vals {
			mem, ok := z.Member.(string)
			if !ok {
				continue
			}
			var rr redisRecord
			if err := json.Unmarshal([]byte(mem), &rr); err != nil {
				continue
			}
			at, _ := time.Parse(time.RFC3339, rr.At)
			records = append(records, RunRecord{
				PromptID:     rr.PromptID,
				Version:      rr.Version,
				LatencyMs:    rr.LatencyMs,
				InputTokens:  rr.InputTokens,
				OutputTokens: rr.OutputTokens,
				Success:      rr.Success,
				At:           at,
			})
		}
		if len(vals) < batch {
			break
		}
	}
	// Filter and aggregate (same logic as MemoryStore)
	agg := make(map[string]*Aggregate)
	for _, rec := range records {
		if q.PromptID != "" && rec.PromptID != q.PromptID {
			continue
		}
		if q.Version != "" && rec.Version != q.Version {
			continue
		}
		var k string
		switch q.GroupBy {
		case "prompt":
			k = rec.PromptID
		case "version":
			k = rec.PromptID + "@" + rec.Version
		case "day":
			k = rec.At.Format("2006-01-02")
		case "hour":
			k = rec.At.Format("2006-01-02-15")
		default:
			k = "all"
		}
		if agg[k] == nil {
			agg[k] = &Aggregate{Key: k}
		}
		a := agg[k]
		a.Runs++
		if rec.Success {
			a.SuccessCount++
		}
		a.AvgLatencyMs = (a.AvgLatencyMs*float64(a.Runs-1) + float64(rec.LatencyMs)) / float64(a.Runs)
		a.TotalInputTokens += int64(rec.InputTokens)
		a.TotalOutputTokens += int64(rec.OutputTokens)
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
