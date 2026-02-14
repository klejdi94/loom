// Package registry S3-compatible storage via BlobStore interface.
package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/klejdi94/loom/core"
)

// BlobStore is a minimal key-value store for S3-compatible backends (e.g. AWS S3, MinIO).
type BlobStore interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Put(ctx context.Context, key string, body []byte) error
	List(ctx context.Context, prefix string) ([]string, error)
	Delete(ctx context.Context, key string) error
}

// S3Registry stores prompts using a BlobStore. Keys: prefix/prompt/id/version.json, prefix/meta/id/version.json, prefix/production/id.txt.
type S3Registry struct {
	store  BlobStore
	prefix string
}

// NewS3Registry creates a registry using the given BlobStore (e.g. from registry/s3blob) and key prefix.
func NewS3Registry(store BlobStore, prefix string) *S3Registry {
	prefix = strings.Trim(prefix, "/")
	if prefix != "" {
		prefix += "/"
	}
	return &S3Registry{store: store, prefix: prefix}
}

func (s *S3Registry) promptKey(id, version string) string {
	return s.prefix + "prompt/" + id + "/" + version + ".json"
}
func (s *S3Registry) metaKey(id, version string) string {
	return s.prefix + "meta/" + id + "/" + version + ".json"
}
func (s *S3Registry) productionKey(id string) string {
	return s.prefix + "production/" + id + ".txt"
}

// Store saves a prompt to the blob store.
func (s *S3Registry) Store(ctx context.Context, prompt *core.Prompt) error {
	if prompt == nil || prompt.ID == "" || prompt.Version == "" {
		return fmt.Errorf("s3 registry: prompt id and version required")
	}
	data, err := json.Marshal(prompt)
	if err != nil {
		return err
	}
	if err := s.store.Put(ctx, s.promptKey(prompt.ID, prompt.Version), data); err != nil {
		return err
	}
	meta := struct {
		Stage     string   `json:"stage"`
		Tags      []string `json:"tags"`
		CreatedAt string   `json:"created_at"`
		UpdatedAt string   `json:"updated_at"`
	}{
		Stage:     "dev",
		CreatedAt: prompt.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt: prompt.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
	metaData, _ := json.Marshal(meta)
	return s.store.Put(ctx, s.metaKey(prompt.ID, prompt.Version), metaData)
}

// Get retrieves a prompt by id and version.
func (s *S3Registry) Get(ctx context.Context, id, version string) (*core.Prompt, error) {
	data, err := s.store.Get(ctx, s.promptKey(id, version))
	if err != nil {
		return nil, core.ErrPromptNotFound
	}
	var p core.Prompt
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, err
	}
	return p.Copy(), nil
}

// GetProduction returns the production version for the id.
func (s *S3Registry) GetProduction(ctx context.Context, id string) (*core.Prompt, error) {
	data, err := s.store.Get(ctx, s.productionKey(id))
	if err != nil {
		return nil, core.ErrPromptNotFound
	}
	version := strings.TrimSpace(string(data))
	if version == "" {
		return nil, core.ErrPromptNotFound
	}
	return s.Get(ctx, id, version)
}

// List returns prompts matching the filter by listing the prompt prefix.
func (s *S3Registry) List(ctx context.Context, filter Filter) ([]*core.Prompt, error) {
	keys, err := s.store.List(ctx, s.prefix+"prompt/")
	if err != nil {
		return nil, err
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 1000
	}
	var out []*core.Prompt
	offset := filter.Offset
	seen := make(map[string]bool)
	for _, key := range keys {
		if !strings.HasSuffix(key, ".json") {
			continue
		}
		trim := strings.TrimPrefix(key, s.prefix+"prompt/")
		parts := strings.SplitN(trim, "/", 2)
		if len(parts) != 2 {
			continue
		}
		id, ver := parts[0], strings.TrimSuffix(parts[1], ".json")
		if seen[id+"/"+ver] {
			continue
		}
		seen[id+"/"+ver] = true
		if len(filter.IDs) > 0 && !contains(filter.IDs, id) {
			continue
		}
		metaData, err := s.store.Get(ctx, s.metaKey(id, ver))
		if err == nil {
			var meta struct {
				Stage string   `json:"stage"`
				Tags  []string `json:"tags"`
			}
			_ = json.Unmarshal(metaData, &meta)
			if filter.Stage != "" && Stage(meta.Stage) != filter.Stage {
				continue
			}
			if len(filter.Tags) > 0 && !hasAll(meta.Tags, filter.Tags) {
				continue
			}
		}
		if offset > 0 {
			offset--
			continue
		}
		p, err := s.Get(ctx, id, ver)
		if err != nil {
			continue
		}
		out = append(out, p)
		if len(out) >= limit {
			return out, nil
		}
	}
	return out, nil
}

// ListVersions returns version info for an id.
func (s *S3Registry) ListVersions(ctx context.Context, id string) ([]VersionInfo, error) {
	keys, err := s.store.List(ctx, s.prefix+"prompt/"+id+"/")
	if err != nil {
		return nil, err
	}
	var infos []VersionInfo
	for _, key := range keys {
		if !strings.HasSuffix(key, ".json") {
			continue
		}
		suffix := strings.TrimPrefix(key, s.prefix+"prompt/"+id+"/")
		ver := strings.TrimSuffix(suffix, ".json")
		p, err := s.Get(ctx, id, ver)
		if err != nil {
			continue
		}
		vi := VersionInfo{ID: id, Version: ver, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt}
		metaData, _ := s.store.Get(ctx, s.metaKey(id, ver))
		if len(metaData) > 0 {
			var meta struct {
				Stage string   `json:"stage"`
				Tags  []string `json:"tags"`
			}
			_ = json.Unmarshal(metaData, &meta)
			vi.Stage = Stage(meta.Stage)
			vi.Tags = meta.Tags
		}
		infos = append(infos, vi)
	}
	return infos, nil
}

// Promote sets the stage and production pointer.
func (s *S3Registry) Promote(ctx context.Context, id, version string, stage Stage) error {
	_, err := s.store.Get(ctx, s.promptKey(id, version))
	if err != nil {
		return core.ErrPromptNotFound
	}
	metaData, _ := s.store.Get(ctx, s.metaKey(id, version))
	var meta struct {
		Stage     string   `json:"stage"`
		Tags      []string `json:"tags"`
		CreatedAt string   `json:"created_at"`
		UpdatedAt string   `json:"updated_at"`
	}
	if len(metaData) > 0 {
		_ = json.Unmarshal(metaData, &meta)
	}
	meta.Stage = string(stage)
	newMeta, _ := json.Marshal(meta)
	if err := s.store.Put(ctx, s.metaKey(id, version), newMeta); err != nil {
		return err
	}
	if stage == StageProduction {
		return s.store.Put(ctx, s.productionKey(id), []byte(version))
	}
	return nil
}

// Delete removes a prompt version.
func (s *S3Registry) Delete(ctx context.Context, id, version string) error {
	_, err := s.store.Get(ctx, s.promptKey(id, version))
	if err != nil {
		return core.ErrPromptNotFound
	}
	_ = s.store.Delete(ctx, s.promptKey(id, version))
	_ = s.store.Delete(ctx, s.metaKey(id, version))
	prod, _ := s.store.Get(ctx, s.productionKey(id))
	if string(prod) == version {
		_ = s.store.Delete(ctx, s.productionKey(id))
	}
	return nil
}

// Tag updates meta with new tags.
func (s *S3Registry) Tag(ctx context.Context, id, version string, tags []string) error {
	_, err := s.store.Get(ctx, s.promptKey(id, version))
	if err != nil {
		return core.ErrPromptNotFound
	}
	metaData, _ := s.store.Get(ctx, s.metaKey(id, version))
	var meta struct {
		Stage     string   `json:"stage"`
		Tags      []string `json:"tags"`
		CreatedAt string   `json:"created_at"`
		UpdatedAt string   `json:"updated_at"`
	}
	if len(metaData) > 0 {
		_ = json.Unmarshal(metaData, &meta)
	}
	meta.Tags = append([]string(nil), tags...)
	newMeta, _ := json.Marshal(meta)
	return s.store.Put(ctx, s.metaKey(id, version), newMeta)
}
