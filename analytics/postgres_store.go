// Package analytics: PostgreSQL Store for persistent run history.
package analytics

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

const defaultTableName = "prompt_runs"

// PostgresStore implements Store using a PostgreSQL table.
type PostgresStore struct {
	db        *sql.DB
	tableName string
}

// NewPostgresStore creates a store that uses the given *sql.DB (e.g. driver "postgres").
// Table is created if it doesn't exist (id, prompt_id, version, latency_ms, input_tokens, output_tokens, success, at).
func NewPostgresStore(db *sql.DB, tableName string) (*PostgresStore, error) {
	if tableName == "" {
		tableName = defaultTableName
	}
	s := &PostgresStore{db: db, tableName: tableName}
	if err := s.migrate(context.Background()); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *PostgresStore) migrate(ctx context.Context) error {
	q := `CREATE TABLE IF NOT EXISTS ` + s.tableName + ` (
		id BIGSERIAL PRIMARY KEY,
		prompt_id TEXT NOT NULL,
		version TEXT NOT NULL,
		latency_ms BIGINT NOT NULL DEFAULT 0,
		input_tokens INT NOT NULL DEFAULT 0,
		output_tokens INT NOT NULL DEFAULT 0,
		success BOOLEAN NOT NULL DEFAULT false,
		at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);
	CREATE INDEX IF NOT EXISTS idx_prompt_runs_prompt_version ON ` + s.tableName + ` (prompt_id, version);
	CREATE INDEX IF NOT EXISTS idx_prompt_runs_at ON ` + s.tableName + ` (at);`
	_, err := s.db.ExecContext(ctx, q)
	return err
}

// Record implements Store.
func (s *PostgresStore) Record(ctx context.Context, r RunRecord) error {
	if r.At.IsZero() {
		r.At = time.Now()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO `+s.tableName+` (prompt_id, version, latency_ms, input_tokens, output_tokens, success, at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		r.PromptID, r.Version, r.LatencyMs, r.InputTokens, r.OutputTokens, r.Success, r.At)
	return err
}

// Query implements Store.
func (s *PostgresStore) Query(ctx context.Context, q Query) ([]Aggregate, error) {
	args := []interface{}{}
	where := "1=1"
	n := 1
	if q.PromptID != "" {
		args = append(args, q.PromptID)
		where += fmt.Sprintf(" AND prompt_id = $%d", n)
		n++
	}
	if q.Version != "" {
		args = append(args, q.Version)
		where += fmt.Sprintf(" AND version = $%d", n)
		n++
	}
	if !q.From.IsZero() {
		args = append(args, q.From)
		where += fmt.Sprintf(" AND at >= $%d", n)
		n++
	}
	if !q.To.IsZero() {
		args = append(args, q.To)
		where += fmt.Sprintf(" AND at <= $%d", n)
		n++
	}

	groupCol := "NULL"
	switch q.GroupBy {
	case "prompt":
		groupCol = "prompt_id"
	case "version":
		groupCol = "prompt_id || '@' || version"
	case "day":
		groupCol = "date_trunc('day', at)::date::text"
	case "hour":
		groupCol = "to_char(date_trunc('hour', at), 'YYYY-MM-DD-HH24')"
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 100
	}
	args = append(args, limit)
	limitPlaceholder := fmt.Sprintf("$%d", n)

	query := `SELECT ` + groupCol + ` AS key,
		COUNT(*)::bigint AS runs,
		COUNT(*) FILTER (WHERE success)::bigint AS success_count,
		COALESCE(AVG(latency_ms) FILTER (WHERE success), 0) AS avg_latency_ms,
		COALESCE(SUM(input_tokens), 0)::bigint AS total_input_tokens,
		COALESCE(SUM(output_tokens), 0)::bigint AS total_output_tokens
		FROM ` + s.tableName + `
		WHERE ` + where + `
		GROUP BY ` + groupCol + `
		ORDER BY runs DESC
		LIMIT ` + limitPlaceholder

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Aggregate
	for rows.Next() {
		var a Aggregate
		var k sql.NullString
		if err := rows.Scan(&k, &a.Runs, &a.SuccessCount, &a.AvgLatencyMs, &a.TotalInputTokens, &a.TotalOutputTokens); err != nil {
			return nil, err
		}
		if k.Valid {
			a.Key = k.String
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
