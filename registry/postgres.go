// Package registry PostgreSQL storage. Use: go get github.com/lib/pq and import _ "github.com/lib/pq".
package registry

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/klejdi94/loom/core"
	"github.com/lib/pq"
)

// PostgresRegistry stores prompts in PostgreSQL.
type PostgresRegistry struct {
	db    *sql.DB
	table string
}

// NewPostgresRegistry creates a registry. table defaults to "prompts". If createTable is true, the table is created.
func NewPostgresRegistry(db *sql.DB, table string, createTable bool) (*PostgresRegistry, error) {
	if table == "" {
		table = "prompts"
	}
	r := &PostgresRegistry{db: db, table: table}
	if createTable {
		if err := r.createTable(context.Background()); err != nil {
			return nil, err
		}
	}
	return r, nil
}

func (r *PostgresRegistry) createTable(ctx context.Context) error {
	// Use placeholder that works with lib/pq ($1, $2) and pgx
	q := `CREATE TABLE IF NOT EXISTS ` + r.table + ` (
		id VARCHAR(255) NOT NULL,
		version VARCHAR(64) NOT NULL,
		name VARCHAR(255),
		description TEXT,
		system TEXT,
		template TEXT NOT NULL,
		variables JSONB,
		examples JSONB,
		metadata JSONB,
		stage VARCHAR(32) DEFAULT 'dev',
		tags JSONB,
		created_at TIMESTAMPTZ,
		updated_at TIMESTAMPTZ,
		PRIMARY KEY (id, version)
	)`
	if _, err := r.db.ExecContext(ctx, q); err != nil {
		return err
	}
	_, err := r.db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_`+r.table+`_id_stage ON `+r.table+`(id, stage)`)
	return err
}

func (r *PostgresRegistry) Store(ctx context.Context, prompt *core.Prompt) error {
	if prompt == nil || prompt.ID == "" || prompt.Version == "" {
		return fmt.Errorf("postgres registry: prompt id and version required")
	}
	variables, _ := json.Marshal(prompt.Variables)
	examples, _ := json.Marshal(prompt.Examples)
	metadata, _ := json.Marshal(prompt.Metadata)
	now := time.Now()
	if prompt.CreatedAt.IsZero() {
		prompt.CreatedAt = now
	}
	prompt.UpdatedAt = now
	q := `INSERT INTO ` + r.table + ` (id, version, name, description, system, template, variables, examples, metadata, stage, tags, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'dev', '[]', $10, $11)
		ON CONFLICT (id, version) DO UPDATE SET
			name = EXCLUDED.name, description = EXCLUDED.description, system = EXCLUDED.system, template = EXCLUDED.template,
			variables = EXCLUDED.variables, examples = EXCLUDED.examples, metadata = EXCLUDED.metadata,
			updated_at = EXCLUDED.updated_at`
	_, err := r.db.ExecContext(ctx, q,
		prompt.ID, prompt.Version, prompt.Name, prompt.Description, prompt.System, prompt.Template,
		variables, examples, metadata, prompt.CreatedAt, prompt.UpdatedAt)
	return err
}

func (r *PostgresRegistry) Get(ctx context.Context, id, version string) (*core.Prompt, error) {
	q := `SELECT id, version, name, description, system, template, variables, examples, metadata, created_at, updated_at FROM ` + r.table + ` WHERE id = $1 AND version = $2`
	var p core.Prompt
	var variables, examples, metadata []byte
	err := r.db.QueryRowContext(ctx, q, id, version).Scan(
		&p.ID, &p.Version, &p.Name, &p.Description, &p.System, &p.Template,
		&variables, &examples, &metadata, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, core.ErrPromptNotFound
	}
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(variables, &p.Variables)
	_ = json.Unmarshal(examples, &p.Examples)
	_ = json.Unmarshal(metadata, &p.Metadata)
	return p.Copy(), nil
}

func (r *PostgresRegistry) GetProduction(ctx context.Context, id string) (*core.Prompt, error) {
	q := `SELECT id, version, name, description, system, template, variables, examples, metadata, created_at, updated_at FROM ` + r.table + ` WHERE id = $1 AND stage = 'production' LIMIT 1`
	var p core.Prompt
	var variables, examples, metadata []byte
	err := r.db.QueryRowContext(ctx, q, id).Scan(
		&p.ID, &p.Version, &p.Name, &p.Description, &p.System, &p.Template,
		&variables, &examples, &metadata, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, core.ErrPromptNotFound
	}
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(variables, &p.Variables)
	_ = json.Unmarshal(examples, &p.Examples)
	_ = json.Unmarshal(metadata, &p.Metadata)
	return p.Copy(), nil
}

func (r *PostgresRegistry) List(ctx context.Context, filter Filter) ([]*core.Prompt, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 1000
	}
	q := `SELECT id, version, name, description, system, template, variables, examples, metadata, tags, created_at, updated_at FROM ` + r.table + ` WHERE 1=1`
	args := []interface{}{}
	argNum := 1
	if len(filter.IDs) > 0 {
		q += ` AND id = ANY($` + fmt.Sprint(argNum) + `)`
		args = append(args, pq.Array(filter.IDs))
		argNum++
	}
	if filter.Stage != "" {
		q += ` AND stage = $` + fmt.Sprint(argNum)
		args = append(args, string(filter.Stage))
		argNum++
	}
	q += ` ORDER BY id, version OFFSET $` + fmt.Sprint(argNum) + ` LIMIT $` + fmt.Sprint(argNum+1)
	args = append(args, filter.Offset, limit)
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*core.Prompt
	for rows.Next() {
		var p core.Prompt
		var variables, examples, metadata, tagsRaw []byte
		if err := rows.Scan(&p.ID, &p.Version, &p.Name, &p.Description, &p.System, &p.Template, &variables, &examples, &metadata, &tagsRaw, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(variables, &p.Variables)
		_ = json.Unmarshal(examples, &p.Examples)
		_ = json.Unmarshal(metadata, &p.Metadata)
		if len(filter.Tags) > 0 {
			var tags []string
			_ = json.Unmarshal(tagsRaw, &tags)
			if !hasAll(tags, filter.Tags) {
				continue
			}
		}
		out = append(out, p.Copy())
	}
	return out, nil
}

func (r *PostgresRegistry) ListVersions(ctx context.Context, id string) ([]VersionInfo, error) {
	q := `SELECT id, version, stage, tags, created_at, updated_at FROM ` + r.table + ` WHERE id = $1 ORDER BY version`
	rows, err := r.db.QueryContext(ctx, q, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var infos []VersionInfo
	for rows.Next() {
		var vi VersionInfo
		var stage string
		var tags []byte
		if err := rows.Scan(&vi.ID, &vi.Version, &stage, &tags, &vi.CreatedAt, &vi.UpdatedAt); err != nil {
			return nil, err
		}
		vi.Stage = Stage(stage)
		_ = json.Unmarshal(tags, &vi.Tags)
		infos = append(infos, vi)
	}
	return infos, nil
}

func (r *PostgresRegistry) Promote(ctx context.Context, id, version string, stage Stage) error {
	// Demote others of same id from production if promoting to production
	if stage == StageProduction {
		_, _ = r.db.ExecContext(ctx, `UPDATE `+r.table+` SET stage = 'dev' WHERE id = $1 AND stage = 'production'`, id)
	}
	_, err := r.db.ExecContext(ctx, `UPDATE `+r.table+` SET stage = $1 WHERE id = $2 AND version = $3`, string(stage), id, version)
	return err
}

func (r *PostgresRegistry) Delete(ctx context.Context, id, version string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM `+r.table+` WHERE id = $1 AND version = $2`, id, version)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return core.ErrPromptNotFound
	}
	return nil
}

func (r *PostgresRegistry) Tag(ctx context.Context, id, version string, tags []string) error {
	data, _ := json.Marshal(tags)
	_, err := r.db.ExecContext(ctx, `UPDATE `+r.table+` SET tags = $1 WHERE id = $2 AND version = $3`, data, id, version)
	return err
}
