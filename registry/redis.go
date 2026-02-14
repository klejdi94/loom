// Package registry Redis storage implementation. Use: go get github.com/redis/go-redis/v9
package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/klejdi94/loom/core"
	"github.com/redis/go-redis/v9"
)

const (
	redisKeyPrompt     = "prompt:%s:%s"
	redisKeyMeta       = "meta:%s:%s"
	redisKeyProduction = "production:%s"
	redisKeyIDs        = "index:ids"
	redisKeyVersions   = "index:versions:%s"
)

// RedisRegistry stores prompts in Redis. Keys: prompt:id:version (JSON), meta:id:version (JSON), production:id (version), index:ids (SET), index:versions:id (SET).
type RedisRegistry struct {
	client redis.UniversalClient
	prefix string
}

// RedisClient is the minimal Redis interface needed (satisfied by *redis.Client, *redis.ClusterClient).
type RedisClient interface {
	Get(ctx context.Context, key string) *redis.StringCmd
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd
	Del(ctx context.Context, keys ...string) *redis.IntCmd
	Keys(ctx context.Context, pattern string) *redis.StringSliceCmd
	SAdd(ctx context.Context, key string, members ...interface{}) *redis.IntCmd
	SRem(ctx context.Context, key string, members ...interface{}) *redis.IntCmd
	SMembers(ctx context.Context, key string) *redis.StringSliceCmd
}

// NewRedisRegistry creates a registry using the given Redis client. Optional key prefix (e.g. "loom:").
func NewRedisRegistry(client redis.UniversalClient, prefix string) *RedisRegistry {
	if prefix != "" && !strings.HasSuffix(prefix, ":") {
		prefix += ":"
	}
	return &RedisRegistry{client: client, prefix: prefix}
}

func (r *RedisRegistry) key(format string, a ...interface{}) string {
	return r.prefix + fmt.Sprintf(format, a...)
}

// Store saves a prompt in Redis.
func (r *RedisRegistry) Store(ctx context.Context, prompt *core.Prompt) error {
	if prompt == nil || prompt.ID == "" || prompt.Version == "" {
		return fmt.Errorf("redis registry: prompt id and version required")
	}
	data, err := json.Marshal(prompt)
	if err != nil {
		return fmt.Errorf("redis registry encode: %w", err)
	}
	k := r.key(redisKeyPrompt, prompt.ID, prompt.Version)
	if err := r.client.Set(ctx, k, data, 0).Err(); err != nil {
		return err
	}
	meta := struct {
		Stage     string    `json:"stage"`
		Tags      []string  `json:"tags"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
	}{
		Stage:     "dev",
		Tags:      nil,
		CreatedAt: prompt.CreatedAt,
		UpdatedAt: prompt.UpdatedAt,
	}
	metaData, _ := json.Marshal(meta)
	if err := r.client.Set(ctx, r.key(redisKeyMeta, prompt.ID, prompt.Version), metaData, 0).Err(); err != nil {
		return err
	}
	r.client.SAdd(ctx, r.key(redisKeyIDs), prompt.ID)
	r.client.SAdd(ctx, r.key(redisKeyVersions, prompt.ID), prompt.Version)
	return nil
}

// Get retrieves a prompt by id and version.
func (r *RedisRegistry) Get(ctx context.Context, id, version string) (*core.Prompt, error) {
	data, err := r.client.Get(ctx, r.key(redisKeyPrompt, id, version)).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, core.ErrPromptNotFound
		}
		return nil, err
	}
	var p core.Prompt
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("redis registry decode: %w", err)
	}
	return p.Copy(), nil
}

// GetProduction returns the production version for the id.
func (r *RedisRegistry) GetProduction(ctx context.Context, id string) (*core.Prompt, error) {
	version, err := r.client.Get(ctx, r.key(redisKeyProduction, id)).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, core.ErrPromptNotFound
		}
		return nil, err
	}
	return r.Get(ctx, id, version)
}

// List returns prompts matching the filter (scans index).
func (r *RedisRegistry) List(ctx context.Context, filter Filter) ([]*core.Prompt, error) {
	ids, err := r.client.SMembers(ctx, r.key(redisKeyIDs)).Result()
	if err != nil {
		return nil, err
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 1000
	}
	var out []*core.Prompt
	offset := filter.Offset
	for _, id := range ids {
		if len(filter.IDs) > 0 && !contains(filter.IDs, id) {
			continue
		}
		vers, _ := r.client.SMembers(ctx, r.key(redisKeyVersions, id)).Result()
		for _, version := range vers {
			metaData, err := r.client.Get(ctx, r.key(redisKeyMeta, id, version)).Bytes()
			if err == redis.Nil {
				continue
			}
			if err != nil {
				continue
			}
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
			if offset > 0 {
				offset--
				continue
			}
			p, err := r.Get(ctx, id, version)
			if err != nil {
				continue
			}
			out = append(out, p)
			if len(out) >= limit {
				return out, nil
			}
		}
	}
	return out, nil
}

// ListVersions returns version info for an id.
func (r *RedisRegistry) ListVersions(ctx context.Context, id string) ([]VersionInfo, error) {
	vers, err := r.client.SMembers(ctx, r.key(redisKeyVersions, id)).Result()
	if err != nil {
		return nil, err
	}
	var infos []VersionInfo
	for _, version := range vers {
		p, err := r.Get(ctx, id, version)
		if err != nil {
			continue
		}
		metaData, _ := r.client.Get(ctx, r.key(redisKeyMeta, id, version)).Bytes()
		vi := VersionInfo{ID: id, Version: version, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt}
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

// Promote sets the stage for id+version and updates production pointer.
func (r *RedisRegistry) Promote(ctx context.Context, id, version string, stage Stage) error {
	_, err := r.client.Get(ctx, r.key(redisKeyPrompt, id, version)).Result()
	if err == redis.Nil {
		return core.ErrPromptNotFound
	}
	if err != nil {
		return err
	}
	metaData, _ := r.client.Get(ctx, r.key(redisKeyMeta, id, version)).Bytes()
	var meta struct {
		Stage     string   `json:"stage"`
		Tags      []string `json:"tags"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
	}
	if len(metaData) > 0 {
		_ = json.Unmarshal(metaData, &meta)
	}
	meta.Stage = string(stage)
	newMeta, _ := json.Marshal(meta)
	if err := r.client.Set(ctx, r.key(redisKeyMeta, id, version), newMeta, 0).Err(); err != nil {
		return err
	}
	if stage == StageProduction {
		if err := r.client.Set(ctx, r.key(redisKeyProduction, id), version, 0).Err(); err != nil {
			return err
		}
	}
	return nil
}

// Delete removes a prompt version from Redis.
func (r *RedisRegistry) Delete(ctx context.Context, id, version string) error {
	k := r.key(redisKeyPrompt, id, version)
	_, err := r.client.Get(ctx, k).Result()
	if err == redis.Nil {
		return core.ErrPromptNotFound
	}
	if err != nil {
		return err
	}
	r.client.Del(ctx, k, r.key(redisKeyMeta, id, version))
	r.client.SRem(ctx, r.key(redisKeyVersions, id), version)
	prod, _ := r.client.Get(ctx, r.key(redisKeyProduction, id)).Result()
	if prod == version {
		r.client.Del(ctx, r.key(redisKeyProduction, id))
	}
	vers, _ := r.client.SMembers(ctx, r.key(redisKeyVersions, id)).Result()
	if len(vers) == 0 {
		r.client.SRem(ctx, r.key(redisKeyIDs), id)
	}
	return nil
}

// Tag sets tags for a prompt version.
func (r *RedisRegistry) Tag(ctx context.Context, id, version string, tags []string) error {
	_, err := r.client.Get(ctx, r.key(redisKeyPrompt, id, version)).Result()
	if err == redis.Nil {
		return core.ErrPromptNotFound
	}
	if err != nil {
		return err
	}
	metaData, _ := r.client.Get(ctx, r.key(redisKeyMeta, id, version)).Bytes()
	var meta struct {
		Stage     string   `json:"stage"`
		Tags      []string `json:"tags"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
	}
	if len(metaData) > 0 {
		_ = json.Unmarshal(metaData, &meta)
	}
	meta.Tags = append([]string(nil), tags...)
	newMeta, _ := json.Marshal(meta)
	return r.client.Set(ctx, r.key(redisKeyMeta, id, version), newMeta, 0).Err()
}
